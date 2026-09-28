package intchecker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Plugin is the integrity checker's host registration.
type Plugin struct {
	api          vfs.HostAPI
	registration vfs.Registration
}

// NewPlugin returns the integrity checker plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) GetName() string { return "Integrity Checker" }

func (p *Plugin) Init(api vfs.HostAPI) error {
	p.api = api
	if contributions, ok := api.(vfs.ContributionHost); ok {
		registration, err := contributions.RegisterPluginCommand(vfs.PluginCommand{
			ID:             "intchecker.menu",
			Location:       vfs.PluginCommandPanel,
			Label:          "Integrity &checker",
			LabelKey:       "IntChecker.Menu",
			Description:    "Generate and verify checksum files (SFV, MD5, SHA)",
			DescriptionKey: "IntChecker.Command.Desc",
			SearchKeys:     []string{"IntChecker.Generate", "IntChecker.Validate"},
			SearchTerms:    []string{"checksum", "hash", "crc32", "md5", "sha1", "sha256", "sfv", "verify"},
			Enabled:        canRun,
			Run:            p.showMenu,
		})
		if err != nil {
			p.api = nil
			return fmt.Errorf("integrity checker: register command: %w", err)
		}
		p.registration = registration
		return nil
	}
	api.RegisterPluginMenuItem(vtui.Msg("IntChecker.Menu"), p.showMenu)
	return nil
}

func (p *Plugin) Close() error {
	if p.registration != nil {
		p.registration.Unregister()
		p.registration = nil
	}
	p.api = nil
	return nil
}

// canRun dims the command when there is no panel filesystem to work on.
func canRun(app vfs.App) bool {
	return app.GetActivePanelVFS() != nil
}

// Menu items, in the order showMenu lists them.
const (
	menuGenerate = iota
	menuValidate
)

func (p *Plugin) showMenu(app vfs.App) {
	app.Menu(vtui.Msg("IntChecker.Title"), []string{
		vtui.Msg("IntChecker.Generate"),
		vtui.Msg("IntChecker.Validate"),
	}, func(idx int) {
		switch idx {
		case menuGenerate:
			showGenerateDialog(app)
		case menuValidate:
			showValidate(app)
		}
	})
}

// selectedFileNames is what "Generate hashes" works on: the marked panel
// items, or the one under the cursor.
func selectedFileNames(app vfs.App) []string {
	var names []string
	for _, name := range app.GetSelectedNames() {
		if name != "" && name != ".." {
			names = append(names, name)
		}
	}
	return names
}

func showGenerateDialog(app vfs.App) {
	fs := app.GetActivePanelVFS()
	if fs == nil {
		return
	}
	names := selectedFileNames(app)
	if len(names) == 0 {
		vtui.ShowMessage(vtui.Msg("IntChecker.Title"), vtui.Msg("IntChecker.NothingSelected"), []string{vtui.Msg("vtui.Ok")})
		return
	}
	dir := fs.GetPath()

	width, height := 60, 16
	dlg := vtui.NewCenteredDialog(width, height, vtui.Msg("IntChecker.GenerateTitle"))
	dlg.ShowClose = true

	algorithmGroup := vtui.NewRadioGroup(0, 0, 2, algorithmNames())
	algorithmGroup.Selected = int(DefaultAlgorithm)
	lblAlgorithm := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.Algorithm"), algorithmGroup)

	lblOutput := vtui.NewText(0, 0, vtui.Msg("IntChecker.OutputSingleFile"), vtui.Palette[vtui.ColDialogText])
	editOutput := vtui.NewEdit(0, 0, width-6, defaultOutputName(fs.Base(dir), DefaultAlgorithm))
	lblOutputEdit := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.OutputFileName"), editOutput)

	current := DefaultAlgorithm
	algorithmGroup.OnChange = func(idx int) {
		next := Algorithm(idx)
		if !next.valid() {
			return
		}
		editOutput.SetText(switchExtension(editOutput.GetText(), current, next))
		current = next
	}

	btnOK := vtui.NewButton(0, 0, vtui.Msg("vtui.Ok"))
	btnOK.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, vtui.Msg("vtui.Cancel"))

	for _, item := range []vtui.UIElement{lblAlgorithm, algorithmGroup, lblOutput, lblOutputEdit, editOutput, btnOK, btnCancel} {
		dlg.AddItem(item)
	}

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(lblAlgorithm, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(algorithmGroup, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(lblOutput, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(lblOutputEdit, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(editOutput, vtui.Margins{}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnOK, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnOK.OnClick = func() {
		output := strings.TrimSpace(editOutput.GetText())
		if !validOutputName(output) {
			vtui.ShowMessage(vtui.Msg("IntChecker.Title"), vtui.Msg("IntChecker.BadOutputName"), []string{vtui.Msg("vtui.Ok")})
			return
		}
		algorithm := Algorithm(algorithmGroup.Selected)
		if !algorithm.valid() {
			return
		}
		dlg.Close()
		job := generateJob{fs: fs, dir: dir, names: names, algorithm: algorithm, output: output}
		// app.Message waits for the answer, so everything from the
		// overwrite question on runs off the UI goroutine.
		go startGenerate(app, job)
	}
	btnCancel.OnClick = func() { dlg.Close() }

	vtui.FrameManager.Push(dlg)
}

// validOutputName accepts a plain file name in the current directory. Other
// directories come with the later output modes.
func validOutputName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`+"\x00")
}

func startGenerate(app vfs.App, job generateJob) {
	target := job.fs.Join(job.dir, job.output)
	if _, err := job.fs.Stat(context.Background(), target); err == nil {
		answer := app.Message(vtui.Msg("IntChecker.Title"),
			fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), job.output),
			[]string{vtui.Msg("IntChecker.Overwrite"), vtui.Msg("vtui.Cancel")})
		if answer != 0 {
			return
		}
		job.overwrite = true
	}
	var res generateResult
	app.RunAdvancedProgressTask(vtui.Msg("IntChecker.GenerateTitle"), false, func(ctx context.Context, reporter vfs.TaskReporter) error {
		var err error
		res, err = runGenerate(ctx, job, reporter)
		return err
	}, func(err error) {
		finishGenerate(app, job, res, err)
	})
}

// finishGenerate tells the user how the run ended and puts the panel cursor
// on the new checksum file.
func finishGenerate(app vfs.App, job generateJob, res generateResult, err error) {
	title := vtui.Msg("IntChecker.Title")
	ok := []string{vtui.Msg("vtui.Ok")}
	switch {
	case errors.Is(err, context.Canceled):
		go app.Message(title, vtui.Msg("IntChecker.Cancelled"), ok)
		return
	case errors.Is(err, errNothingToHash):
		go app.Message(title, vtui.Msg("IntChecker.NoFiles"), ok)
		return
	case err != nil:
		go app.Message(title, fmt.Sprintf(vtui.Msg("IntChecker.WriteError"), job.output, err), ok)
		return
	}
	if res.Written > 0 {
		app.SetPendingSelection(job.output)
		app.RefreshAll()
	}
	if len(res.Failures) == 0 {
		return
	}
	lines := make([]string, 0, len(res.Failures)+1)
	lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.ReadErrors"), len(res.Failures)))
	const shown = 10
	for i, failure := range res.Failures {
		if i == shown {
			lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.MoreErrors"), len(res.Failures)-shown))
			break
		}
		lines = append(lines, fmt.Sprintf("%s: %v", failure.Name, failure.Err))
	}
	go app.Message(title, strings.Join(lines, "\n"), ok)
}
