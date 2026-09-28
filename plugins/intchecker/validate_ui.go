package intchecker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// showValidate is the "Validate files" menu command. With the cursor on a
// checksum file it checks the files next to it right away; otherwise it asks
// for the checksum file and, optionally, the directory with the files.
func showValidate(app vfs.App) {
	fs := app.GetActivePanelVFS()
	if fs == nil {
		return
	}
	dir := fs.GetPath()
	if name := app.GetSelectedName(); name != "" && name != ".." && isChecksumFileName(name) {
		// Reading and stat'ing may be slow on a remote panel, so they
		// run off the UI goroutine; app.Message waits there, too.
		go startValidate(app, fs, fs.Join(dir, name), dir, autoDetectEncoding)
		return
	}
	openValidateDialog(app, fs, "", "", "", autoDetectEncoding)
}

// validateDialog asks for the checksum file and the directory to check.
type validateDialog struct {
	win      *vtui.Window
	editFile *vtui.Edit
	editDir  *vtui.Edit
	encoding *encodingCombo
	btnOK    *vtui.Button
}

// newValidateDialog builds the dialog. note, when not empty, is shown on top
// and moves the focus to the directory: it explains why the dialog came back.
// enc is the checksum file encoding to preselect.
func newValidateDialog(note, hashText, dirText string, enc fileEncoding) *validateDialog {
	width, height := 70, 13
	if note != "" {
		height += 2
	}
	d := &validateDialog{win: vtui.NewCenteredDialog(width, height, vtui.Msg("IntChecker.ValidateTitle"))}
	d.win.ShowClose = true

	d.editFile = vtui.NewEdit(0, 0, width-6, hashText)
	lblFile := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.ChecksumFile"), d.editFile)
	d.editDir = vtui.NewEdit(0, 0, width-6, dirText)
	lblDir := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.FilesDir"), d.editDir)
	d.encoding = newEncodingCombo(24, readEncodingChoices(), enc)
	lblEncoding := vtui.NewLabel(0, 0, vtui.Msg("IntChecker.FileEncoding"), d.encoding.box)
	d.btnOK = vtui.NewButton(0, 0, vtui.Msg("vtui.Ok"))
	d.btnOK.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, vtui.Msg("vtui.Cancel"))
	btnCancel.OnClick = func() { d.win.Close() }

	vbox := vtui.NewVBoxLayout(d.win.X1+2, d.win.Y1+2, width-4, height-4)
	if note != "" {
		txtNote := vtui.NewText(0, 0, note, vtui.Palette[vtui.ColDialogText])
		d.win.AddItem(txtNote)
		vbox.Add(txtNote, vtui.Margins{}, vtui.AlignLeft)
	}
	for _, item := range []vtui.UIElement{lblFile, d.editFile, lblDir, d.editDir, lblEncoding, d.encoding.box, d.btnOK, btnCancel} {
		d.win.AddItem(item)
	}
	top := 0
	if note != "" {
		top = 1
	}
	vbox.Add(lblFile, vtui.Margins{Top: top}, vtui.AlignLeft)
	vbox.Add(d.editFile, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(lblDir, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(d.editDir, vtui.Margins{}, vtui.AlignFill)
	encodingRow := vtui.NewHBoxLayout(0, 0, width-4, 1)
	encodingRow.Spacing = 1
	encodingRow.Add(lblEncoding, vtui.Margins{}, vtui.AlignLeft)
	encodingRow.Add(d.encoding.box, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(encodingRow, vtui.Margins{Top: 1}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(d.btnOK, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	if note != "" {
		d.win.SetFocusedItem(d.editDir)
	}
	return d
}

func openValidateDialog(app vfs.App, fs vfs.VFS, note, hashText, dirText string, enc fileEncoding) {
	panelDir := fs.GetPath()
	d := newValidateDialog(note, hashText, dirText, enc)
	d.btnOK.OnClick = func() {
		hashPath, dir, ok := resolveValidateInput(fs, panelDir, d.editFile.GetText(), d.editDir.GetText())
		if !ok {
			vtui.ShowMessage(vtui.Msg("IntChecker.Title"), vtui.Msg("IntChecker.EnterChecksumFile"), []string{vtui.Msg("vtui.Ok")})
			return
		}
		d.win.Close()
		go startValidate(app, fs, hashPath, dir, d.encoding.selected())
	}
	vtui.FrameManager.Push(d.win)
}

// resolveValidateInput turns the dialog fields into paths. Relative paths are
// taken from the panel directory, and an empty directory means the panel
// directory itself, as the reporter of f4#1623 asked.
func resolveValidateInput(fs vfs.VFS, panelDir, hashText, dirText string) (hashPath, dir string, ok bool) {
	hashText, dirText = strings.TrimSpace(hashText), strings.TrimSpace(dirText)
	if hashText == "" {
		return "", "", false
	}
	resolve := func(p string) string {
		if fs.IsAbs(p) {
			return p
		}
		return fs.Join(panelDir, p)
	}
	dir = panelDir
	if dirText != "" {
		dir = resolve(dirText)
	}
	return resolve(hashText), dir, true
}

// startValidate loads the checksum file, decodes it as enc says (see
// decodeChecksumFile) and runs the check with a progress dialog. It waits for
// message answers, so it must not run on the UI goroutine.
func startValidate(app vfs.App, fs vfs.VFS, hashPath, dir string, enc fileEncoding) {
	title := vtui.Msg("IntChecker.Title")
	ok := []string{vtui.Msg("vtui.Ok")}
	ctx := context.Background()
	if item, err := fs.Stat(ctx, dir); err != nil || !item.IsDir {
		app.Message(title, fmt.Sprintf(vtui.Msg("IntChecker.DirNotFound"), dir), ok)
		return
	}
	data, err := readChecksumFile(ctx, fs, hashPath)
	if err != nil {
		app.Message(title, fmt.Sprintf(vtui.Msg("IntChecker.ReadChecksumError"), hashPath, err), ok)
		return
	}
	text, _, err := decodeChecksumFile(data, enc.Codepage)
	if err != nil {
		app.Message(title, fmt.Sprintf(vtui.Msg("IntChecker.ReadChecksumError"), hashPath, err), ok)
		return
	}
	file, err := ParseHashFile(fs.Base(hashPath), text)
	if err != nil {
		app.Message(title, parseErrorText(hashPath, err), ok)
		return
	}
	job := validateJob{fs: fs, hashPath: hashPath, dir: dir, file: file, encoding: enc}
	var res validateResult
	app.RunAdvancedProgressTask(vtui.Msg("IntChecker.ValidateTitle"), false, func(ctx context.Context, reporter vfs.TaskReporter) error {
		var err error
		res, err = runValidate(ctx, job, reporter)
		return err
	}, func(err error) {
		finishValidate(app, job, res, err)
	})
}

func parseErrorText(hashPath string, err error) string {
	switch {
	case errors.Is(err, errNoChecksums):
		return fmt.Sprintf(vtui.Msg("IntChecker.NoChecksums"), hashPath)
	case errors.Is(err, errUnknownHashLength):
		return fmt.Sprintf(vtui.Msg("IntChecker.UnknownHashLength"), hashPath)
	}
	return fmt.Sprintf(vtui.Msg("IntChecker.ReadChecksumError"), hashPath, err)
}

// finishValidate runs on the UI goroutine when the check ends: it shows the
// report, or asks for another directory when no listed file was found.
func finishValidate(app vfs.App, job validateJob, res validateResult, err error) {
	title := vtui.Msg("IntChecker.Title")
	ok := []string{vtui.Msg("vtui.Ok")}
	switch {
	case errors.Is(err, context.Canceled):
		go app.Message(title, vtui.Msg("IntChecker.ValidateCancelled"), ok)
	case errors.Is(err, errAllMissing):
		openValidateDialog(app, job.fs, vtui.Msg("IntChecker.FilesNotFound"), job.hashPath, job.dir, job.encoding)
	case err != nil:
		go app.Message(title, err.Error(), ok)
	default:
		go app.Message(title, validateReport(res, job.file.Malformed), ok)
	}
}
