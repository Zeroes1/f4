package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/diffview"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/internal/vtvibe/ap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func aiReviewTestMods() []ap.ModificationResult {
	return []ap.ModificationResult{
		{FilePath: "vfs/ai_vfs.go", ModIdx: 0, Action: "REPLACE", Locator: "return 0, ErrNotSupported", Status: ap.ModOK},
		{FilePath: "vfs/ai_vfs.go", ModIdx: 1, Action: "INSERT_AFTER", Locator: "func (d *aiDrive) Open(\n\tctx context.Context, p string", Status: ap.ModSkipped},
		{FilePath: "go.mod", ModIdx: 0, Action: "REPLACE", Locator: "require x v1", Status: ap.ModFailed,
			Err: &ap.AppError{Code: ap.ErrCode("SNIPPET_NOT_FOUND"), Message: "Snippet not found in go.mod & nowhere near"}},
		{FilePath: "old.txt", ModIdx: -1, Action: "RENAME", Locator: "new.txt", Status: ap.ModOK},
	}
}

// aiScreenText renders frame into a fresh buffer and returns the text part
// of vtui's screen dump (the same format Ctrl+Shift+P writes).
func aiScreenText(t *testing.T, scr *vtui.ScreenBuf, frame vtui.Frame) string {
	t.Helper()
	frame.Show(scr)
	var buf bytes.Buffer
	scr.Dump(&buf)
	text := buf.String()
	if i := strings.Index(text, "--- CELL METADATA"); i >= 0 {
		text = text[:i]
	}
	return text
}

func aiReviewButtons(w *vtui.Window) map[string]*vtui.Button {
	res := map[string]*vtui.Button{}
	for _, it := range w.GetChildren() {
		if b, ok := it.(*vtui.Button); ok {
			res[b.GetCaption()] = b
		}
	}
	return res
}

// aiCaption is a button label as Button.GetCaption reports it.
func aiCaption(key string) string { return strings.ReplaceAll(i18n.Msg(key), "&", "") }

func TestAIShowPatchReviewListsEveryModification(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "afailed.md"), []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	patch := &vtvibe.Patch{ID: "aa000001", Text: "aa000001 AP 3.2\n"}
	dlg := aiShowPatchReview(nil, patch, root, aiReviewTestMods(), 2, "patcher output")
	if top := vtui.FrameManager.GetTopFrame(); top != vtui.Frame(dlg) {
		t.Fatalf("top frame = %T, want the review dialog", top)
	}

	text := aiScreenText(t, scr, dlg)
	t.Logf("screen dump:\n%s", text)
	for _, want := range []string{
		i18n.Msg("AI.ReviewColFile"), i18n.Msg("AI.ReviewColLocator"),
		"vfs/ai_vfs.go", "go.mod", "old.txt", "INSERT_AFTER", "RENAME",
		i18n.Msg("AI.ReviewOK"), i18n.Msg("AI.ReviewSkipped"), i18n.Msg("AI.ReviewFailed"),
		"return 0, ErrNotSupported",
		"func (d *aiDrive) Open( …",
		"new.txt",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("review screen lacks %q", want)
		}
	}
	// 2 would apply (REPLACE + RENAME), 1 already applied, 1 fails.
	if want := fmt.Sprintf(i18n.Msg("AI.ReviewTotals"), 2, 1, 1); !strings.Contains(text, want) {
		t.Errorf("review screen lacks totals %q", want)
	}
	// The first row is selected, so its locator is the detail line.
	if !strings.Contains(text, " return 0, ErrNotSupported") {
		t.Error("detail line missing")
	}

	btns := aiReviewButtons(dlg)
	for _, key := range []string{"AI.BtnApplyPatch", "AI.BtnViewLog", "AI.BtnAttachReport", "AI.ReviewBtnClose"} {
		if btns[aiCaption(key)] == nil {
			t.Errorf("review dialog lacks the %s button", key)
		}
	}
	if b := btns[aiCaption("AI.BtnApplyPatch")]; b != nil && !b.IsDefault {
		t.Error("Apply should be the default button when something would apply")
	}

	btns[aiCaption("AI.ReviewBtnClose")].OnClick()
	if !dlg.IsDone() {
		t.Fatal("Close did not close the review dialog")
	}
}

func TestAIShowPatchReviewNothingToApply(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	mods := []ap.ModificationResult{
		{FilePath: "a.txt", Action: "REPLACE", Locator: "x", Status: ap.ModSkipped},
		{FilePath: "b.txt", Action: "DELETE", Status: ap.ModFailed, Err: &ap.AppError{Code: ap.ErrCode("FILE_NOT_FOUND")}},
	}
	dlg := aiShowPatchReview(nil, &vtvibe.Patch{}, t.TempDir(), mods, 2, "")
	btns := aiReviewButtons(dlg)
	if btns[aiCaption("AI.BtnApplyPatch")] != nil {
		t.Error("Apply offered although nothing would be written")
	}
	if btns[aiCaption("AI.BtnViewLog")] != nil {
		t.Error("View log offered for empty output")
	}
	if btns[aiCaption("AI.BtnAttachReport")] != nil {
		t.Error("Attach report offered without afailed.md")
	}
	if b := btns[aiCaption("AI.ReviewBtnClose")]; b == nil || !b.IsDefault {
		t.Error("Close should be the default button when nothing would apply")
	}
	dlg.Close()
}

func TestAIReviewHelpers(t *testing.T) {
	if got := aiReviewOneLine("\n  first  \n\nsecond\n"); got != "first …" {
		t.Errorf("aiReviewOneLine multi = %q", got)
	}
	if got := aiReviewOneLine("  only \r\n"); got != "only" {
		t.Errorf("aiReviewOneLine single = %q", got)
	}
	if got := aiReviewOneLine(" \n "); got != "" {
		t.Errorf("aiReviewOneLine blank = %q", got)
	}

	mods := aiReviewTestMods()
	if got := aiReviewDetail(mods[2]); got != "SNIPPET_NOT_FOUND: Snippet not found in go.mod & nowhere near" {
		t.Errorf("failed detail = %q", got)
	}
	if got := aiReviewDetail(ap.ModificationResult{Status: ap.ModFailed, Err: &ap.AppError{Code: "X"}}); got != "X" {
		t.Errorf("failed detail without message = %q", got)
	}
	if got := aiReviewDetail(mods[1]); got != "func (d *aiDrive) Open( …" {
		t.Errorf("ok detail = %q", got)
	}

	if ok, skipped, failed, excluded := aiReviewCounts(mods); ok != 2 || skipped != 1 || failed != 1 || excluded != 0 {
		t.Errorf("counts = %d/%d/%d/%d, want 2/1/1/0", ok, skipped, failed, excluded)
	}
	// An excluded row is neither an error nor "already applied".
	withExcluded := append(append([]ap.ModificationResult(nil), mods...), ap.ModificationResult{FilePath: "x", Status: ap.ModExcluded})
	if ok, skipped, failed, excluded := aiReviewCounts(withExcluded); ok != 2 || skipped != 1 || failed != 1 || excluded != 1 {
		t.Errorf("counts with an excluded row = %d/%d/%d/%d, want 2/1/1/1", ok, skipped, failed, excluded)
	}
	if got := aiReviewStatusText(ap.ModExcluded); got != i18n.Msg("AI.ReviewExcluded") || got == i18n.Msg("AI.ReviewFailed") {
		t.Errorf("excluded status text = %q", got)
	}
	if got, want := aiReviewTotals(withExcluded), fmt.Sprintf(i18n.Msg("AI.ReviewTotalsExcluded"), 2, 1, 1, 1); got != want {
		t.Errorf("totals with an excluded row = %q, want %q", got, want)
	}
	if got, want := aiReviewTotals(mods), fmt.Sprintf(i18n.Msg("AI.ReviewTotals"), 2, 1, 1); got != want {
		t.Errorf("totals = %q, want %q", got, want)
	}

	// '&' must survive as a literal, and the label is exactly w wide.
	if got := aiReviewLabel("a&b", 5); got != "a&&b  " {
		t.Errorf("aiReviewLabel = %q", got)
	}
	if got := aiReviewLabel("abcdefgh", 5); got != "abcd…" {
		t.Errorf("aiReviewLabel truncated = %q", got)
	}

	rev := newAIReview([]ap.ModificationResult{{FilePath: "f", Status: ap.ModOK}})
	row := aiReviewRow{r: rev, i: 0}
	if row.GetCellText(aiReviewColAction) != "-" || row.GetCellText(aiReviewColFile) != "f" ||
		row.GetCellText(aiReviewColStatus) != i18n.Msg("AI.ReviewOK") || row.GetCellText(99) != "" ||
		row.GetCellText(aiReviewColCheck) != "[x]" {
		t.Error("aiReviewRow cells")
	}
	rev.toggle(0)
	if row.GetCellText(aiReviewColCheck) != "[ ]" {
		t.Error("unchecked row still shows a check mark")
	}
	rev.toggle(5) // out of range: ignored
}

func TestAIReviewSelection(t *testing.T) {
	mods := append(aiReviewTestMods(), ap.ModificationResult{FilePath: "late.txt", ModIdx: 0, Action: "CREATE", Status: ap.ModExcluded})
	rev := newAIReview(mods)
	// The dry run's own exclusions come back unchecked, the rest checked.
	if got := rev.checked(); got != 4 {
		t.Fatalf("checked = %d, want 4", got)
	}
	if only := rev.only(); len(only) != 4 || only[ap.ModKey{FilePath: "late.txt", ModIdx: 0}] {
		t.Fatalf("only = %v, want the four checked rows", only)
	}
	rev.toggle(4)
	if rev.only() != nil {
		t.Fatal("everything checked should run the whole patch (nil Only)")
	}
	// Only the skipped and the failing row left: nothing to write.
	rev.toggle(0)
	rev.toggle(3)
	rev.toggle(4)
	if rev.canApply() {
		t.Error("canApply with only skipped/failing rows checked")
	}
	want := map[ap.ModKey]bool{{FilePath: "vfs/ai_vfs.go", ModIdx: 1}: true, {FilePath: "go.mod", ModIdx: 0}: true}
	if got := rev.only(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("only = %v, want %v", got, want)
	}
	// A re-checked excluded row may apply: the dry run never looked at it.
	rev.toggle(4)
	if !rev.canApply() {
		t.Error("canApply false with an excluded row checked again")
	}
}

// aiReviewTableOf finds the review table in the dialog.
func aiReviewTableOf(t *testing.T, w *vtui.Window) *aiReviewTable {
	t.Helper()
	for _, it := range w.GetChildren() {
		if tb, ok := it.(*aiReviewTable); ok {
			return tb
		}
	}
	t.Fatal("review dialog has no table")
	return nil
}

func aiKey(vk uint16, ch rune) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, Char: ch}
}

// TestAIShowPatchReviewToggles switches rows off with Space/Ins and checks
// that Apply and Dry run hand exactly the checked rows to the patcher.
func TestAIShowPatchReviewToggles(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	type call struct {
		dry  bool
		only map[ap.ModKey]bool
	}
	var calls []call
	saved := aiReviewRunPatcher
	t.Cleanup(func() { aiReviewRunPatcher = saved })
	aiReviewRunPatcher = func(_ *panel.PanelsFrame, _ *vtvibe.Patch, _ string, dry bool, only map[ap.ModKey]bool) {
		calls = append(calls, call{dry, only})
	}

	patch := &vtvibe.Patch{ID: "aa000001", Text: "aa000001 AP 3.2\n"}
	open := func() (*vtui.Window, *aiReviewTable) {
		dlg := aiShowPatchReview(nil, patch, t.TempDir(), aiReviewTestMods(), 2, "")
		return dlg, aiReviewTableOf(t, dlg)
	}

	dlg, table := open()
	// Ins on the first row (REPLACE in vfs/ai_vfs.go): off, and the cursor
	// moves down as it does in the panels.
	if !table.ProcessKey(aiKey(vtinput.VK_INSERT, 0)) || table.SelectPos != 1 {
		t.Fatalf("Ins not handled or did not move down (pos %d)", table.SelectPos)
	}
	// Space on the RENAME row: off, the cursor stays.
	table.MoveSelection(2)
	if !table.ProcessKey(aiKey(vtinput.VK_SPACE, ' ')) || table.SelectPos != 3 {
		t.Fatalf("Space not handled or moved the cursor (pos %d)", table.SelectPos)
	}
	text := aiScreenText(t, scr, dlg)
	t.Logf("screen dump with two of four edits switched off:\n%s", text)
	if want := fmt.Sprintf(i18n.Msg("AI.ReviewChecked"), 2, 4); !strings.Contains(text, want) {
		t.Errorf("screen lacks %q", want)
	}
	if strings.Count(text, "[ ]") != 2 || strings.Count(text, "[x]") != 2 {
		t.Errorf("want two unchecked and two checked rows on screen")
	}
	btns := aiReviewButtons(dlg)
	apply := btns[aiCaption("AI.BtnApplyPatch")]
	if apply == nil || !apply.IsDisabled() || apply.IsDefault {
		t.Fatal("Apply should stay on the dialog but be disabled: only the skipped and the failing row are checked")
	}
	if b := btns[aiCaption("AI.ReviewBtnClose")]; b == nil || !b.IsDefault {
		t.Error("Close should be the default button while Apply is disabled")
	}
	apply.OnClick()
	if len(calls) != 0 || dlg.IsDone() {
		t.Fatal("a disabled Apply still ran the patcher")
	}

	// RENAME back on (the cursor is still on it).
	table.ProcessKey(aiKey(vtinput.VK_SPACE, ' '))
	if apply.IsDisabled() || !apply.IsDefault {
		t.Fatal("Apply should be enabled and default again with RENAME checked")
	}
	apply.OnClick()
	want := map[ap.ModKey]bool{
		{FilePath: "vfs/ai_vfs.go", ModIdx: 1}: true,
		{FilePath: "go.mod", ModIdx: 0}:        true,
		{FilePath: "old.txt", ModIdx: -1}:      true,
	}
	if len(calls) != 1 || calls[0].dry || fmt.Sprint(calls[0].only) != fmt.Sprint(want) || !dlg.IsDone() {
		t.Fatalf("Apply called %+v, want one real run with Only = %v", calls, want)
	}

	// Dry run again with the current choice.
	calls = nil
	dlg, table = open()
	table.ProcessKey(aiKey(vtinput.VK_SPACE, ' '))
	aiReviewButtons(dlg)[aiCaption("AI.ReviewBtnDryRun")].OnClick()
	if len(calls) != 1 || !calls[0].dry || len(calls[0].only) != 3 || calls[0].only[ap.ModKey{FilePath: "vfs/ai_vfs.go", ModIdx: 0}] {
		t.Fatalf("Dry run called %+v, want one dry run without the first row", calls)
	}

	// With nothing switched off, both run the whole patch (nil Only).
	calls = nil
	dlg, _ = open()
	aiReviewButtons(dlg)[aiCaption("AI.BtnApplyPatch")].OnClick()
	if len(calls) != 1 || calls[0].only != nil {
		t.Fatalf("Apply with everything checked called %+v, want nil Only", calls)
	}
}

// TestAIShowPatchReviewAfterExclusion: the screen a re-run dry run ends on -
// the rows left out come back as "excluded", unchecked, counted apart from
// the errors.
func TestAIShowPatchReviewAfterExclusion(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	mods := aiReviewTestMods()
	mods[0].Status = ap.ModExcluded
	mods[3].Status = ap.ModExcluded
	dlg := aiShowPatchReview(nil, &vtvibe.Patch{}, t.TempDir(), mods, 2, "")
	text := aiScreenText(t, scr, dlg)
	t.Logf("screen dump after a dry run with two edits excluded:\n%s", text)
	for _, want := range []string{
		i18n.Msg("AI.ReviewExcluded"),
		fmt.Sprintf(i18n.Msg("AI.ReviewTotalsExcluded"), 0, 1, 1, 2),
		fmt.Sprintf(i18n.Msg("AI.ReviewChecked"), 2, 4),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
	// Nothing checked would write, but the excluded rows could: Apply is
	// there, disabled until one of them is checked again.
	apply := aiReviewButtons(dlg)[aiCaption("AI.BtnApplyPatch")]
	if apply == nil || !apply.IsDisabled() {
		t.Fatal("Apply should be present and disabled")
	}
	aiReviewTableOf(t, dlg).ProcessKey(aiKey(vtinput.VK_INSERT, 0))
	if apply.IsDisabled() {
		t.Error("checking an excluded row again should enable Apply")
	}
	dlg.Close()
}

// TestAIShowPatchReviewDiff: Enter or F3 on a row opens that edit's own
// before/after fragment in a diffview; a row with nothing to compare says so
// instead, and neither key applies the patch.
func TestAIShowPatchReviewDiff(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	var calls int
	saved := aiReviewRunPatcher
	t.Cleanup(func() { aiReviewRunPatcher = saved })
	aiReviewRunPatcher = func(*panel.PanelsFrame, *vtvibe.Patch, string, bool, map[ap.ModKey]bool) { calls++ }

	mods := aiReviewTestMods()
	mods[0].Preview = &ap.Preview{
		StartLine: 117,
		Before:    []string{"}", "", "func (d *aiDrive) Open(", "\tctx context.Context, p string", "\treturn 0, ErrNotSupported", "}"},
		After: []string{"}", "", "func (d *aiDrive) Open(", "\tctx context.Context, p string",
			"\te, ok := d.lookup(p)", "\tif !ok {", "\t\treturn 0, ErrNotFound", "\t}", "\treturn d.openEntry(ctx, e)", "}"},
	}
	dlg := aiShowPatchReview(nil, &vtvibe.Patch{ID: "aa000001", Text: "aa000001 AP 3.2\n"}, t.TempDir(), mods, 2, "")
	text := aiScreenText(t, scr, dlg)
	t.Logf("review screen:\n%s", text)
	if !strings.Contains(text, i18n.Msg("AI.ReviewDiffHint")) {
		t.Errorf("review screen lacks the diff hint %q", i18n.Msg("AI.ReviewDiffHint"))
	}
	table := aiReviewTableOf(t, dlg)

	// F3 on the skipped row: nothing to compare, a message says so.
	table.MoveSelection(1)
	if !table.ProcessKey(aiKey(vtinput.VK_F3, 0)) {
		t.Fatal("F3 not handled by the review table")
	}
	msg, isWin := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !isWin || msg == dlg {
		t.Fatalf("F3 on a row without a preview: top frame %T, want a message", vtui.FrameManager.GetTopFrame())
	}
	if text := aiScreenText(t, scr, msg); !strings.Contains(text, strings.Fields(i18n.Msg("AI.ReviewNoDiff"))[0]) {
		t.Errorf("message lacks %q:\n%s", i18n.Msg("AI.ReviewNoDiff"), text)
	}
	msg.Close()

	// Enter on the first row: the diff opens on a screen of its own, the
	// review dialog stays where it was.
	table.MoveSelection(-1)
	if !table.ProcessKey(aiKey(vtinput.VK_RETURN, '\r')) {
		t.Fatal("Enter not handled by the review table")
	}
	dv, ok := vtui.FrameManager.GetTopFrame().(*diffview.DiffView)
	if !ok {
		t.Fatalf("top frame after Enter = %T, want *diffview.DiffView", vtui.FrameManager.GetTopFrame())
	}
	diffText := aiScreenText(t, scr, dv)
	t.Logf("diff of the selected edit:\n%s", diffText)
	for _, want := range []string{
		"vfs/ai_vfs.go:117 (" + i18n.Msg("AI.ReviewDiffBefore") + ")",
		"vfs/ai_vfs.go:117 (" + i18n.Msg("AI.ReviewDiffAfter") + ")",
		"return 0, ErrNotSupported", "e, ok := d.lookup(p)", "return d.openEntry(ctx, e)",
	} {
		if !strings.Contains(diffText, want) {
			t.Errorf("diff view lacks %q", want)
		}
	}
	if dlg.IsDone() || calls != 0 {
		t.Fatal("Enter or F3 closed the review or ran the patcher")
	}
	dv.ProcessKey(aiKey(vtinput.VK_ESCAPE, 0))
	if !dv.IsDone() {
		t.Error("Esc does not close the diff view")
	}
}
