package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/internal/vtvibe/ap"
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

	if ok, skipped, failed := aiReviewCounts(mods); ok != 2 || skipped != 1 || failed != 1 {
		t.Errorf("counts = %d/%d/%d, want 2/1/1", ok, skipped, failed)
	}

	// '&' must survive as a literal, and the label is exactly w wide.
	if got := aiReviewLabel("a&b", 5); got != "a&&b  " {
		t.Errorf("aiReviewLabel = %q", got)
	}
	if got := aiReviewLabel("abcdefgh", 5); got != "abcd…" {
		t.Errorf("aiReviewLabel truncated = %q", got)
	}

	row := aiReviewRow{m: ap.ModificationResult{FilePath: "f", Status: ap.ModOK}}
	if row.GetCellText(aiReviewColAction) != "-" || row.GetCellText(aiReviewColFile) != "f" ||
		row.GetCellText(aiReviewColStatus) != i18n.Msg("AI.ReviewOK") || row.GetCellText(99) != "" {
		t.Error("aiReviewRow cells")
	}
}
