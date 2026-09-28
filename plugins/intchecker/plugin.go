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

// generateDialog asks how to generate the hashes.
type generateDialog struct {
	win        *vtui.Window
	algorithm  *vtui.RadioGroup
	output     *vtui.RadioGroup
	editOutput *vtui.Edit
	recursive  *vtui.Checkbox
	absolute   *vtui.Checkbox
	editMask   *vtui.Edit
	btnOK      *vtui.Button
}

// defaultMask is the file mask the dialog offers: every file.
const defaultMask = "*"

// outputModeNames lists the "Output to" choices in radio button order.
func outputModeNames() []string {
	return []string{
		vtui.Msg("IntChecker.OutputSingle"),
		vtui.Msg("IntChecker.OutputSeparate"),
		vtui.Msg("IntChecker.OutputDirectory"),
		vtui.Msg("IntChecker.OutputDisplay"),
	}
}

// newGenerateDialog builds the dialog for a panel directory named dirBase.
// The file name field belongs to the "Single file" output and is disabled
// for the others. Recursion is on by default, so selected directories are
// hashed with everything in them, as IntChecker does.
func newGenerateDialog(dirBase string) *generateDialog {
	width, height := 60, 23
	d := &generateDialog{win: vtui.NewCenteredDialog(width, height, vtui.Msg("IntChecker.GenerateTitle"))}
	d.win.ShowClose = true

	d.algorithm = vtui.NewRadioGroup(0, 0, 2, algorithmNames())
	d.algorithm.Selected = int(DefaultAlgorithm)
	lblAlgorithm := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.Algorithm"), d.algorithm)

	d.output = vtui.NewRadioGroup(0, 0, 1, outputModeNames())
	d.output.Selected = int(outputSingle)
	lblOutput := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.OutputTo"), d.output)

	d.editOutput = vtui.NewEdit(0, 0, width-6, defaultOutputName(dirBase, DefaultAlgorithm))
	lblOutputEdit := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.OutputFileName"), d.editOutput)

	current := DefaultAlgorithm
	d.algorithm.OnChange = func(idx int) {
		next := Algorithm(idx)
		if !next.valid() {
			return
		}
		d.editOutput.SetText(switchExtension(d.editOutput.GetText(), current, next))
		current = next
	}
	d.output.OnChange = func(idx int) {
		d.editOutput.SetDisabled(outputMode(idx) != outputSingle)
	}

	d.recursive = vtui.NewCheckbox(0, 0, vtui.Msg("IntChecker.Recursive"), false)
	d.recursive.State = 1
	d.absolute = vtui.NewCheckbox(0, 0, vtui.Msg("IntChecker.AbsolutePaths"), false)
	d.editMask = vtui.NewEdit(0, 0, 10, defaultMask)
	lblMask := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.FileMask"), d.editMask)
	// The mask field takes the rest of its row.
	lx1, _, lx2, _ := lblMask.GetPosition()
	maskWidth := width - 4 - (lx2 - lx1 + 1) - 1
	d.editMask.SetPosition(0, 0, maskWidth-1, 0)

	d.btnOK = vtui.NewButton(0, 0, vtui.Msg("vtui.Ok"))
	d.btnOK.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, vtui.Msg("vtui.Cancel"))
	btnCancel.OnClick = func() { d.win.Close() }

	for _, item := range []vtui.UIElement{lblAlgorithm, d.algorithm, lblOutput, d.output, lblOutputEdit, d.editOutput, d.recursive, d.absolute, lblMask, d.editMask, d.btnOK, btnCancel} {
		d.win.AddItem(item)
	}

	vbox := vtui.NewVBoxLayout(d.win.X1+2, d.win.Y1+2, width-4, height-4)
	vbox.Add(lblAlgorithm, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(d.algorithm, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(lblOutput, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(d.output, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(lblOutputEdit, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(d.editOutput, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(d.recursive, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(d.absolute, vtui.Margins{}, vtui.AlignLeft)
	maskRow := vtui.NewHBoxLayout(0, 0, width-4, 1)
	maskRow.Add(lblMask, vtui.Margins{}, vtui.AlignLeft)
	maskRow.Add(d.editMask, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(maskRow, vtui.Margins{}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(d.btnOK, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()
	return d
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
	d := newGenerateDialog(fs.Base(dir))
	d.btnOK.OnClick = func() {
		algorithm := Algorithm(d.algorithm.Selected)
		mode := outputMode(d.output.Selected)
		if !algorithm.valid() || !mode.valid() {
			return
		}
		output := strings.TrimSpace(d.editOutput.GetText())
		if mode == outputSingle && !validOutputName(output) {
			vtui.ShowMessage(vtui.Msg("IntChecker.Title"), vtui.Msg("IntChecker.BadOutputName"), []string{vtui.Msg("vtui.Ok")})
			return
		}
		d.win.Close()
		job := generateJob{
			fs: fs, dir: dir, names: names, algorithm: algorithm, mode: mode, output: output,
			recursive: d.recursive.State == 1,
			absolute:  d.absolute.State == 1,
			mask:      strings.TrimSpace(d.editMask.GetText()),
		}
		startGenerate(app, job)
	}
	vtui.FrameManager.Push(d.win)
}

// validOutputName accepts a plain file name in the current directory. Other
// directories come with the later output modes.
func validOutputName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`+"\x00")
}

// confirmOverwrite asks what to do with checksum files that already exist and
// updates the job. It returns false when the user cancelled. It waits for the
// answer, so it must not run on the UI goroutine.
func confirmOverwrite(app vfs.App, job *generateJob, existing []string) bool {
	if len(existing) == 0 {
		return true
	}
	title := vtui.Msg("IntChecker.Title")
	if !job.mode.writesManyFiles() {
		answer := app.Message(title,
			fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), existing[0]),
			[]string{vtui.Msg("IntChecker.Overwrite"), vtui.Msg("vtui.Cancel")})
		if answer != 0 {
			return false
		}
		job.overwrite = true
		return true
	}
	text := fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), existing[0])
	if len(existing) > 1 {
		text = fmt.Sprintf(vtui.Msg("IntChecker.OverwriteManyQuestion"), len(existing), existing[0])
	}
	switch app.Message(title, text, []string{vtui.Msg("IntChecker.Overwrite"), vtui.Msg("IntChecker.Skip"), vtui.Msg("vtui.Cancel")}) {
	case 0:
		job.overwrite = true
	case 1:
		job.skipExisting = true
	default:
		return false
	}
	return true
}

// startGenerate runs a job in two steps, each with its own progress and
// cancellation: first the selected files are collected (walking the selected
// directories when the job is recursive) and the existing checksum files are
// found, then, once the user has answered the overwrite question, everything
// is hashed and written.
func startGenerate(app vfs.App, job generateJob) {
	var (
		res      generateResult
		inputs   []hashInput
		existing []string
	)
	title := vtui.Msg("IntChecker.GenerateTitle")
	hash := func(job generateJob) {
		app.RunAdvancedProgressTask(title, false, func(ctx context.Context, reporter vfs.TaskReporter) error {
			var err error
			res, err = hashInputs(ctx, job, inputs, res, reporter)
			return err
		}, func(err error) {
			finishGenerate(app, job, res, err)
		})
	}
	app.RunAdvancedProgressTask(title, false, func(ctx context.Context, reporter vfs.TaskReporter) error {
		var err error
		inputs, err = collectInputs(ctx, job, &res, func(dir string, files, dirs int64) {
			reporter.UpdateScan(dir, files, dirs)
		})
		if err != nil {
			return err
		}
		existing, err = existingOutputs(ctx, job, inputs)
		return err
	}, func(err error) {
		if err != nil {
			finishGenerate(app, job, res, err)
			return
		}
		if len(existing) == 0 || len(inputs) == 0 {
			hash(job)
			return
		}
		// app.Message waits for the answer, so the overwrite question
		// runs off the UI goroutine.
		go func() {
			if confirmOverwrite(app, &job, existing) {
				hash(job)
			}
		}()
	})
}

// finishGenerate tells the user how the run ended, puts the panel cursor on
// the new checksum file and, for the display mode, opens the list window. It
// runs on the UI goroutine.
func finishGenerate(app vfs.App, job generateJob, res generateResult, err error) {
	title := vtui.Msg("IntChecker.Title")
	ok := []string{vtui.Msg("vtui.Ok")}
	if len(res.Outputs) > 0 {
		if !strings.Contains(res.Outputs[0], "/") {
			app.SetPendingSelection(res.Outputs[0])
		}
		app.RefreshAll()
	}
	switch {
	case errors.Is(err, context.Canceled):
		go app.Message(title, vtui.Msg("IntChecker.Cancelled"), ok)
		return
	case errors.Is(err, errNothingToHash):
		go app.Message(title, vtui.Msg("IntChecker.NoFiles"), ok)
		return
	case err != nil && job.mode == outputSingle:
		go app.Message(title, fmt.Sprintf(vtui.Msg("IntChecker.WriteError"), job.output, err), ok)
		return
	case err != nil:
		go app.Message(title, err.Error(), ok)
		return
	}
	report := generateReport(job, res)
	if job.mode == outputDisplay && res.Text != "" {
		showHashList(app, job, res.Text, report)
		return
	}
	if report != "" {
		go app.Message(title, report, ok)
	}
}

// generateReport describes what went wrong in a finished run: checksum files
// kept or not written and files that could not be read. It is empty when
// everything went fine.
func generateReport(job generateJob, res generateResult) string {
	var lines []string
	const shown = 10
	appendFailures := func(header string, failures []fileFailure) {
		if len(failures) == 0 {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, fmt.Sprintf(header, len(failures)))
		for i, failure := range failures {
			if i == shown {
				lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.MoreErrors"), len(failures)-shown))
				break
			}
			lines = append(lines, fmt.Sprintf("%s: %v", failure.Name, failure.Err))
		}
	}
	if job.mode.writesManyFiles() && (len(res.SkippedExisting) > 0 || len(res.WriteFailures) > 0) {
		lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.FilesWritten"), len(res.Outputs)))
		if len(res.SkippedExisting) > 0 {
			lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.SkippedExisting"), len(res.SkippedExisting)))
		}
	}
	appendFailures(vtui.Msg("IntChecker.WriteErrors"), res.WriteFailures)
	appendFailures(vtui.Msg("IntChecker.ReadErrors"), res.Failures)
	return strings.Join(lines, "\n")
}
