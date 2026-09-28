package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/diffview"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/internal/vtvibe/ap"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Patch review after a dry run (f4#1606, docs/VTVIBE.md §7.3).
//
// This is the first, deliberately small cut of the review screen: a dry run
// no longer ends in a wall of patcher text but in a table with one row per
// modification (ap.Result.ModificationResults) - which file, which action,
// what it searched for, and whether it would apply, is already applied, or
// fails and why. From there the human either applies the patch for real or
// walks away having written nothing.
//
// Every row can be switched off and on again (Space, or Ins, which also
// moves down as it does in the panels). "Apply" then applies only the
// checked rows (ap.Options.Only), and "Dry run" runs the check again with
// the current choice - a row left out comes back as "excluded", and
// anything that only worked together with it shows up as failing before a
// byte is written.
//
// Enter or F3 on a row opens that one edit side by side (diffview, the same
// view "Compare by content" uses): the fragment of the file around the
// edit before and after it, as the dry run computed it
// (ap.ModificationResult.Preview). Not here yet: a diff pane next to the
// table, F8, Ctrl+Z.

// aiReview is the review screen's state: the dry run's rows and which of
// them are checked.
type aiReview struct {
	mods []ap.ModificationResult
	on   []bool
}

// newAIReview checks every row except the ones the dry run itself was told
// to leave out, so a re-run dry run keeps the choice it was made with.
func newAIReview(mods []ap.ModificationResult) *aiReview {
	r := &aiReview{mods: mods, on: make([]bool, len(mods))}
	for i, m := range mods {
		r.on[i] = m.Status != ap.ModExcluded
	}
	return r
}

func (r *aiReview) toggle(i int) {
	if i >= 0 && i < len(r.on) {
		r.on[i] = !r.on[i]
	}
}

func (r *aiReview) checked() int {
	n := 0
	for _, on := range r.on {
		if on {
			n++
		}
	}
	return n
}

// only is the ap.Options.Only for the current choice: nil when every row is
// checked (the whole patch, exactly as before there was a choice), else the
// checked rows' keys.
func (r *aiReview) only() map[ap.ModKey]bool {
	if r.checked() == len(r.on) {
		return nil
	}
	only := make(map[ap.ModKey]bool, len(r.on))
	for i, on := range r.on {
		if on {
			only[r.mods[i].Key()] = true
		}
	}
	return only
}

// canApply: some checked row would write something. An excluded row that
// has been checked again counts too - the dry run did not look at it, so
// nothing says it would not apply.
func (r *aiReview) canApply() bool {
	for i, on := range r.on {
		if on && aiReviewMayApply(r.mods[i].Status) {
			return true
		}
	}
	return false
}

func aiReviewMayApply(s ap.ModStatus) bool { return s == ap.ModOK || s == ap.ModExcluded }

// aiReviewRow is one ModificationResult as a table row.
type aiReviewRow struct {
	r *aiReview
	i int
}

const (
	aiReviewColCheck = iota
	aiReviewColFile
	aiReviewColAction
	aiReviewColStatus
	aiReviewColLocator
)

func (row aiReviewRow) GetCellText(col int) string {
	r := row.r.mods[row.i]
	switch col {
	case aiReviewColCheck:
		if row.r.on[row.i] {
			return "[x]"
		}
		return "[ ]"
	case aiReviewColFile:
		return r.FilePath
	case aiReviewColAction:
		if r.Action == "" {
			return "-"
		}
		return r.Action
	case aiReviewColStatus:
		return aiReviewStatusText(r.Status)
	case aiReviewColLocator:
		return aiReviewOneLine(r.Locator)
	}
	return ""
}

func aiReviewStatusText(s ap.ModStatus) string {
	switch s {
	case ap.ModOK:
		return i18n.Msg("AI.ReviewOK")
	case ap.ModSkipped:
		return i18n.Msg("AI.ReviewSkipped")
	case ap.ModExcluded:
		return i18n.Msg("AI.ReviewExcluded")
	default:
		return i18n.Msg("AI.ReviewFailed")
	}
}

// aiReviewOneLine squeezes a multi-line locator into one table cell: the
// first non-blank line, trimmed, with an ellipsis when there was more.
func aiReviewOneLine(s string) string {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	switch len(lines) {
	case 0:
		return ""
	case 1:
		return lines[0]
	default:
		return lines[0] + " …"
	}
}

// aiReviewDetail is the line under the table for the selected row: why it
// fails, or, for a row that is fine, the locator it matched.
func aiReviewDetail(m ap.ModificationResult) string {
	if m.Status == ap.ModFailed && m.Err != nil {
		msg := aiReviewOneLine(m.Err.Message)
		if msg == "" {
			return string(m.Err.Code)
		}
		return string(m.Err.Code) + ": " + msg
	}
	return aiReviewOneLine(m.Locator)
}

// aiReviewCounts splits the modifications by outcome. Excluded rows are
// their own count: leaving an edit out is a choice, not an error.
func aiReviewCounts(mods []ap.ModificationResult) (ok, skipped, failed, excluded int) {
	for _, m := range mods {
		switch m.Status {
		case ap.ModOK:
			ok++
		case ap.ModSkipped:
			skipped++
		case ap.ModExcluded:
			excluded++
		default:
			failed++
		}
	}
	return
}

// aiReviewTotals is the dry run's summary line.
func aiReviewTotals(mods []ap.ModificationResult) string {
	ok, skipped, failed, excluded := aiReviewCounts(mods)
	if excluded > 0 {
		return fmt.Sprintf(i18n.Msg("AI.ReviewTotalsExcluded"), ok, skipped, failed, excluded)
	}
	return fmt.Sprintf(i18n.Msg("AI.ReviewTotals"), ok, skipped, failed)
}

// aiReviewTable is the review table with the row toggles on top of the
// ordinary vtui.Table keys.
type aiReviewTable struct {
	*vtui.Table
	onToggle func(idx int)
	onDiff   func(idx int)
}

func (t *aiReviewTable) ProcessKey(e *vtinput.InputEvent) bool {
	if e != nil && e.KeyDown && e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|
		vtinput.LeftAltPressed|vtinput.RightAltPressed|vtinput.ShiftPressed) == 0 {
		switch e.VirtualKeyCode {
		case vtinput.VK_SPACE, vtinput.VK_INSERT:
			t.onToggle(t.RowAt(t.SelectPos))
			if e.VirtualKeyCode == vtinput.VK_INSERT {
				t.MoveSelection(1)
			}
			return true
		case vtinput.VK_RETURN, vtinput.VK_F3:
			// Enter is taken from the dialog's default button on purpose:
			// on a list of edits it means "show me this one", and applying
			// the patch stays one deliberate press of Apply away.
			if t.onDiff != nil {
				t.onDiff(t.RowAt(t.SelectPos))
			}
			return true
		}
	}
	return t.Table.ProcessKey(e)
}

// aiReviewShowDiff opens one row's own edit in a diffview: left the
// fragment before it, right after, titled with the file and the line the
// fragment starts at. A row with nothing to compare (already applied,
// failing, excluded, a RENAME or a whole-file DELETE) gets a short message
// instead.
func aiReviewShowDiff(m ap.ModificationResult) {
	p := m.Preview
	if p == nil {
		vtui.ShowMessage(i18n.Msg("AI.ReviewTitle"), i18n.Msg("AI.ReviewNoDiff"), []string{i18n.Msg("vtui.Ok")})
		return
	}
	where := fmt.Sprintf("%s:%d", m.FilePath, p.StartLine)
	dv, err := diffview.NewDiffView(where+" ("+i18n.Msg("AI.ReviewDiffBefore")+")",
		where+" ("+i18n.Msg("AI.ReviewDiffAfter")+")", p.Before, p.After)
	if err != nil {
		aiShowError(err)
		return
	}
	dv.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
	vtui.FrameManager.AddScreen(dv)
}

// aiReviewLabel fits s into exactly w columns for a vtui.Text: '&' would
// otherwise be read as a hotkey marker, and a long line is cut rather than
// drawn over the frame.
func aiReviewLabel(s string, w int) string {
	s = runewidth.Truncate(s, w, "…")
	return strings.ReplaceAll(padLabelTo(s, w), "&", "&&")
}

// aiReviewRunPatcher is what the review screen's Apply and Dry run buttons
// call; a variable only so tests can see what they asked for (assigned in
// init, as a plain initializer would be an initialization cycle through
// aiRunPatcher -> aiShowPatchReview).
var aiReviewRunPatcher func(pf *panel.PanelsFrame, patch *vtvibe.Patch, root string, dry bool, only map[ap.ModKey]bool)

func init() { aiReviewRunPatcher = aiRunPatcher }

// aiShowPatchReview is what a dry run ends in when the patcher got as far
// as reasoning about individual modifications. exitCode/output are the same
// as for aiShowPatchResult, which remains the screen for a real run and for
// a dry run that failed before any modification (a patch that does not
// parse, say).
func aiShowPatchReview(pf *panel.PanelsFrame, patch *vtvibe.Patch, root string, mods []ap.ModificationResult, exitCode int, output string) *vtui.Window {
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	dlgW := min(max(scrW-4, 60), 100)
	dlgH := min(max(len(mods)+12, 16), max(scrH-2, 16))
	inner := dlgW - 4

	dlg := vtui.NewCenteredDialog(dlgW, dlgH, i18n.Msg("AI.ReviewTitle"))
	dlg.ShowClose = true

	statusW := runewidth.StringWidth(i18n.Msg("AI.ReviewColStatus"))
	for _, s := range []ap.ModStatus{ap.ModOK, ap.ModSkipped, ap.ModFailed, ap.ModExcluded} {
		statusW = max(statusW, runewidth.StringWidth(aiReviewStatusText(s)))
	}
	cols := []vtui.TableColumn{
		{Title: "", Width: 3},
		{Title: i18n.Msg("AI.ReviewColFile"), MinWidth: 12},
		{Title: i18n.Msg("AI.ReviewColAction"), Width: 13},
		{Title: i18n.Msg("AI.ReviewColStatus"), Width: statusW},
		{Title: i18n.Msg("AI.ReviewColLocator"), MinWidth: 12},
	}
	// Below the table: the selected row's detail, the totals, how many rows
	// are checked, the diff key hint, a blank line and the buttons.
	rev := newAIReview(mods)
	table := &aiReviewTable{Table: vtui.NewTable(0, 0, inner, dlgH-9, cols)}
	table.SetOwner(dlg)
	table.ShowHeader = true
	table.ShowScrollBar = true
	rows := make([]vtui.TableRow, len(mods))
	for i := range mods {
		rows[i] = aiReviewRow{r: rev, i: i}
	}
	table.SetRows(rows)

	detail := vtui.NewText(0, 0, aiReviewLabel("", inner), 0)
	showDetail := func(idx int) {
		text := ""
		if idx >= 0 && idx < len(mods) {
			text = aiReviewDetail(mods[idx])
		}
		detail.SetText(aiReviewLabel(text, inner))
	}
	showDetail(0)
	table.OnSelect = func(idx int) {
		showDetail(idx)
		vtui.FrameManager.Redraw()
	}

	totals := vtui.NewText(0, 0, aiReviewLabel(aiReviewTotals(mods), inner), 0)
	checkedLabel := func() string {
		return aiReviewLabel(fmt.Sprintf(i18n.Msg("AI.ReviewChecked"), rev.checked(), len(mods)), inner)
	}
	checked := vtui.NewText(0, 0, checkedLabel(), 0)
	diffHint := vtui.NewText(0, 0, aiReviewLabel(i18n.Msg("AI.ReviewDiffHint"), inner), 0)
	table.onDiff = func(idx int) {
		if idx >= 0 && idx < len(mods) {
			aiReviewShowDiff(mods[idx])
		}
	}

	var buttons []*vtui.Button
	addButton := func(label string, onClick func()) *vtui.Button {
		b := vtui.NewButton(0, 0, label)
		b.SetOwner(dlg)
		b.OnClick = onClick
		buttons = append(buttons, b)
		return b
	}
	// Apply is offered only when some row could write something: a patch
	// whose every row is "already applied" or "fails" would write nothing
	// whatever is checked. It is disabled while no such row is checked.
	var applyBtn *vtui.Button
	mayApply := false
	for _, m := range mods {
		mayApply = mayApply || aiReviewMayApply(m.Status)
	}
	if mayApply && patch != nil {
		applyBtn = addButton(i18n.Msg("AI.BtnApplyPatch"), func() {
			if !rev.canApply() {
				return
			}
			dlg.Close()
			aiReviewRunPatcher(pf, patch, root, false, rev.only())
		})
	}
	if patch != nil {
		addButton(i18n.Msg("AI.ReviewBtnDryRun"), func() {
			dlg.Close()
			aiReviewRunPatcher(pf, patch, root, true, rev.only())
		})
	}
	if output = strings.TrimSpace(output); output != "" {
		addButton(i18n.Msg("AI.BtnViewLog"), func() { aiViewPatchLog(pf, output) })
	}
	reportPath := filepath.Join(root, "afailed.md")
	if exitCode != 0 {
		if st, err := os.Stat(reportPath); err == nil && !st.IsDir() {
			addButton(i18n.Msg("AI.BtnAttachReport"), func() { aiAttachFailureReport(reportPath) })
		}
	}
	closeBtn := addButton(i18n.Msg("AI.ReviewBtnClose"), func() { dlg.Close() })
	syncButtons := func() {
		can := applyBtn != nil && rev.canApply()
		if applyBtn != nil {
			applyBtn.SetDisabled(!can)
			applyBtn.IsDefault = can
		}
		closeBtn.IsDefault = !can
	}
	syncButtons()
	table.onToggle = func(idx int) {
		rev.toggle(idx)
		checked.SetText(checkedLabel())
		syncButtons()
		vtui.FrameManager.Redraw()
	}

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+1, inner, dlgH-2)
	vbox.Add(table, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(detail, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(totals, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(checked, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(diffHint, vtui.Margins{Bottom: 1}, vtui.AlignFill)
	btnRow := vtui.NewHBoxLayout(0, 0, inner, 1)
	btnRow.HorizontalAlign = vtui.AlignCenter
	btnRow.Spacing = 2
	for _, b := range buttons {
		btnRow.Add(b, vtui.Margins{}, vtui.AlignTop)
	}
	vbox.Add(btnRow, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()

	dlg.AddItem(table)
	dlg.AddItem(detail)
	dlg.AddItem(totals)
	dlg.AddItem(checked)
	dlg.AddItem(diffHint)
	for _, b := range buttons {
		dlg.AddItem(b)
	}

	vtui.FrameManager.Push(dlg)
	return dlg
}

// aiViewPatchLog opens the patcher's text output in the viewer.
func aiViewPatchLog(pf *panel.PanelsFrame, output string) {
	dir, err := os.MkdirTemp("", "vtvibe-ap-log-")
	if err != nil {
		return
	}
	logPath := filepath.Join(dir, "ap_output.log")
	if os.WriteFile(logPath, []byte(output), 0600) == nil {
		actionOpenViewer(pf, vfs.NewOSVFS(dir), "ap_output.log")
	}
}
