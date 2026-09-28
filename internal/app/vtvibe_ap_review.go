package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/internal/vtvibe/ap"
	"github.com/unxed/f4/vfs"
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
// Not here yet, on purpose (each needs work in the engine, not the screen):
// toggling individual modifications on and off, a diff pane, Ctrl+Z. The
// table is read-only; "Apply" applies the whole patch exactly as the plain
// Apply button in the confirmation dialog does.

// aiReviewRow is one ModificationResult as a table row.
type aiReviewRow struct {
	m ap.ModificationResult
}

const (
	aiReviewColFile = iota
	aiReviewColAction
	aiReviewColStatus
	aiReviewColLocator
)

func (r aiReviewRow) GetCellText(col int) string {
	switch col {
	case aiReviewColFile:
		return r.m.FilePath
	case aiReviewColAction:
		if r.m.Action == "" {
			return "-"
		}
		return r.m.Action
	case aiReviewColStatus:
		return aiReviewStatusText(r.m.Status)
	case aiReviewColLocator:
		return aiReviewOneLine(r.m.Locator)
	}
	return ""
}

func aiReviewStatusText(s ap.ModStatus) string {
	switch s {
	case ap.ModOK:
		return i18n.Msg("AI.ReviewOK")
	case ap.ModSkipped:
		return i18n.Msg("AI.ReviewSkipped")
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

// aiReviewCounts splits the modifications by outcome.
func aiReviewCounts(mods []ap.ModificationResult) (ok, skipped, failed int) {
	for _, m := range mods {
		switch m.Status {
		case ap.ModOK:
			ok++
		case ap.ModSkipped:
			skipped++
		default:
			failed++
		}
	}
	return
}

// aiReviewLabel fits s into exactly w columns for a vtui.Text: '&' would
// otherwise be read as a hotkey marker, and a long line is cut rather than
// drawn over the frame.
func aiReviewLabel(s string, w int) string {
	s = runewidth.Truncate(s, w, "…")
	return strings.ReplaceAll(padLabelTo(s, w), "&", "&&")
}

// aiShowPatchReview is what a dry run ends in when the patcher got as far
// as reasoning about individual modifications. exitCode/output are the same
// as for aiShowPatchResult, which remains the screen for a real run and for
// a dry run that failed before any modification (a patch that does not
// parse, say).
func aiShowPatchReview(pf *panel.PanelsFrame, patch *vtvibe.Patch, root string, mods []ap.ModificationResult, exitCode int, output string) *vtui.Window {
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	dlgW := min(max(scrW-4, 60), 100)
	dlgH := min(max(len(mods)+10, 14), max(scrH-2, 14))
	inner := dlgW - 4

	dlg := vtui.NewCenteredDialog(dlgW, dlgH, i18n.Msg("AI.ReviewTitle"))
	dlg.ShowClose = true

	statusW := runewidth.StringWidth(i18n.Msg("AI.ReviewColStatus"))
	for _, s := range []ap.ModStatus{ap.ModOK, ap.ModSkipped, ap.ModFailed} {
		statusW = max(statusW, runewidth.StringWidth(aiReviewStatusText(s)))
	}
	cols := []vtui.TableColumn{
		{Title: i18n.Msg("AI.ReviewColFile"), MinWidth: 12},
		{Title: i18n.Msg("AI.ReviewColAction"), Width: 13},
		{Title: i18n.Msg("AI.ReviewColStatus"), Width: statusW},
		{Title: i18n.Msg("AI.ReviewColLocator"), MinWidth: 12},
	}
	// Below the table: the selected row's detail, the totals, a blank line
	// and the buttons.
	table := vtui.NewTable(0, 0, inner, dlgH-7, cols)
	table.SetOwner(dlg)
	table.ShowHeader = true
	table.ShowScrollBar = true
	rows := make([]vtui.TableRow, len(mods))
	for i := range mods {
		rows[i] = aiReviewRow{m: mods[i]}
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

	okN, skippedN, failedN := aiReviewCounts(mods)
	totals := vtui.NewText(0, 0, aiReviewLabel(fmt.Sprintf(i18n.Msg("AI.ReviewTotals"), okN, skippedN, failedN), inner), 0)

	var buttons []*vtui.Button
	addButton := func(label string, onClick func()) *vtui.Button {
		b := vtui.NewButton(0, 0, label)
		b.SetOwner(dlg)
		b.OnClick = onClick
		buttons = append(buttons, b)
		return b
	}
	// Apply is offered only when there is something left to apply: a patch
	// whose every row is "already applied" or "fails" would write nothing.
	if okN > 0 && patch != nil {
		addButton(i18n.Msg("AI.BtnApplyPatch"), func() {
			dlg.Close()
			aiRunPatcher(pf, patch, root, false)
		}).IsDefault = true
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
	if okN == 0 {
		closeBtn.IsDefault = true
	}

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+1, inner, dlgH-2)
	vbox.Add(table, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(detail, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(totals, vtui.Margins{Bottom: 1}, vtui.AlignFill)
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
