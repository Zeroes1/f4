package dialog

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// ntfsFlagsDialog opens the Windows attributes dialog for targets over a VFS
// that records the attributes written, and returns the dialog, the check boxes
// by their label and the Set button.
func ntfsFlagsDialog(t *testing.T, targets []AttributesTarget, written map[string]uint32) (vtui.Frame, map[string]*vtui.Checkbox, *vtui.Button) {
	t.Helper()
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())
	mock := &mockMetadataVFS{
		VFS: vfs.NewOSVFS(t.TempDir()),
		onSetAttrPath: func(path string, item vfs.VFSItem) {
			written[path] = item.WinAttrs
		},
	}
	ShowAttributesWindowsForTargets(nil, mock, targets)
	dlg := fm.GetTopFrame().(vtui.Container)
	boxes := map[string]*vtui.Checkbox{}
	var set *vtui.Button
	walkUI(dlg.(vtui.UIElement), func(el vtui.UIElement) bool {
		switch c := el.(type) {
		case *vtui.Checkbox:
			boxes[c.GetText()] = c
		case *vtui.Button:
			if strings.Contains(c.GetText(), i18n.Msg("Attributes.BtnSet")) {
				set = c
			}
		}
		return true
	})
	if set == nil {
		t.Fatal("Set button not found")
	}
	return dlg.(vtui.Frame), boxes, set
}

func ntfsBox(t *testing.T, boxes map[string]*vtui.Checkbox, key string) *vtui.Checkbox {
	t.Helper()
	box := boxes[i18n.Msg(key)]
	if box == nil {
		t.Fatalf("check box %q not found", key)
	}
	return box
}

// Several objects with different flags: setting Temporary on all of them keeps
// every other attribute, including the Offline and Read only flags that stay
// mixed.
func TestAttributesDialog_TemporaryOnMultipleSelectionKeepsOtherAttributes(t *testing.T) {
	written := map[string]uint32{}
	targets := []AttributesTarget{
		{Path: "a.txt", Item: vfs.VFSItem{Name: "a.txt", WinAttrs: 0x1 | 0x20 | 0x100, MTime: time.Now()}},
		{Path: "b.txt", Item: vfs.VFSItem{Name: "b.txt", WinAttrs: 0x2 | 0x1000, MTime: time.Now()}},
	}
	dlg, boxes, set := ntfsFlagsDialog(t, targets, written)

	temporary := ntfsBox(t, boxes, "Attributes.Temporary")
	offline := ntfsBox(t, boxes, "Attributes.Offline")
	if temporary.State != 2 || offline.State != 2 {
		t.Fatalf("Temporary = %d, Offline = %d, both want the mixed state 2", temporary.State, offline.State)
	}
	temporary.State = 1
	set.OnClick()
	runUITasksUntil(t, vtui.FrameManager.TaskChan, dlg.IsDone)

	if got, want := written["a.txt"], uint32(0x1|0x20|0x100); got != want {
		t.Errorf("a.txt attributes = %#x, want %#x", got, want)
	}
	if got, want := written["b.txt"], uint32(0x2|0x100|0x1000); got != want {
		t.Errorf("b.txt attributes = %#x, want %#x", got, want)
	}
}

func TestAttributesDialog_OfflineCanBeCleared(t *testing.T) {
	written := map[string]uint32{}
	targets := []AttributesTarget{
		{Path: "a.txt", Item: vfs.VFSItem{Name: "a.txt", WinAttrs: 0x1000 | 0x20, MTime: time.Now()}},
		{Path: "b.txt", Item: vfs.VFSItem{Name: "b.txt", WinAttrs: 0x1000 | 0x20, MTime: time.Now()}},
	}
	dlg, boxes, set := ntfsFlagsDialog(t, targets, written)

	offline := ntfsBox(t, boxes, "Attributes.Offline")
	if offline.State != 1 {
		t.Fatalf("Offline = %d, want 1", offline.State)
	}
	offline.State = 0
	set.OnClick()
	runUITasksUntil(t, vtui.FrameManager.TaskChan, dlg.IsDone)

	for path, got := range written {
		if got != 0x20 {
			t.Errorf("%s attributes = %#x, want 0x20", path, got)
		}
	}
	if len(written) != 2 {
		t.Errorf("attributes written for %d objects, want 2", len(written))
	}
}

// Outside a cloud files provider Pinned and Unpinned are grey, and Set leaves
// them as each file has them even if a stray state reaches the box.
func TestAttributesDialog_PinnedAndUnpinnedAreGreyOutsideCloudFiles(t *testing.T) {
	written := map[string]uint32{}
	targets := []AttributesTarget{{Path: "a.txt", Item: vfs.VFSItem{Name: "a.txt", WinAttrs: 0x20, MTime: time.Now()}}}
	dlg, boxes, set := ntfsFlagsDialog(t, targets, written)

	pinned := ntfsBox(t, boxes, "Attributes.Pinned")
	unpinned := ntfsBox(t, boxes, "Attributes.Unpinned")
	if !pinned.IsDisabled() || !unpinned.IsDisabled() {
		t.Fatalf("Pinned disabled = %v, Unpinned disabled = %v, want both disabled", pinned.IsDisabled(), unpinned.IsDisabled())
	}
	for _, key := range []string{"Attributes.Temporary", "Attributes.Offline"} {
		if ntfsBox(t, boxes, key).IsDisabled() {
			t.Errorf("%s is disabled, want it usable on any file", key)
		}
	}
	pinned.State = 1
	set.OnClick()
	runUITasksUntil(t, vtui.FrameManager.TaskChan, dlg.IsDone)
	if got := written["a.txt"]; got != 0x20 {
		t.Errorf("a.txt attributes = %#x, want 0x20", got)
	}
}

// In a cloud files folder the pair is usable, and checking one clears the other.
func TestAttributesDialog_PinnedAndUnpinnedExcludeEachOther(t *testing.T) {
	written := map[string]uint32{}
	targets := []AttributesTarget{
		{Path: "a.txt", Item: vfs.VFSItem{Name: "a.txt", WinAttrs: 0x100000 | 0x20, MTime: time.Now()}},
		{Path: "b.txt", Item: vfs.VFSItem{Name: "b.txt", WinAttrs: 0x400000 | 0x20, MTime: time.Now()}},
	}
	dlg, boxes, set := ntfsFlagsDialog(t, targets, written)

	pinned := ntfsBox(t, boxes, "Attributes.Pinned")
	unpinned := ntfsBox(t, boxes, "Attributes.Unpinned")
	if pinned.IsDisabled() || unpinned.IsDisabled() {
		t.Fatal("Pinned and Unpinned are disabled for cloud files")
	}
	if unpinned.State != 2 {
		t.Fatalf("Unpinned = %d, want the mixed state 2", unpinned.State)
	}
	pinned.State = 0
	pinned.Toggle() // 0 -> 1
	if pinned.State != 1 || unpinned.State != 0 {
		t.Fatalf("after checking Pinned: Pinned = %d, Unpinned = %d, want 1 and 0", pinned.State, unpinned.State)
	}
	unpinned.Toggle() // 0 -> 1
	if unpinned.State != 1 || pinned.State != 0 {
		t.Fatalf("after checking Unpinned: Pinned = %d, Unpinned = %d, want 0 and 1", pinned.State, unpinned.State)
	}
	unpinned.State = 0
	pinned.State = 1
	set.OnClick()
	runUITasksUntil(t, vtui.FrameManager.TaskChan, dlg.IsDone)
	for path, want := range map[string]uint32{"a.txt": 0x80000 | 0x20, "b.txt": 0x80000 | 0x400000 | 0x20} {
		if got := written[path]; got != want {
			t.Errorf("%s attributes = %#x, want %#x", path, got, want)
		}
	}
}

func TestSetWindowsAttributesRefusesPinnedWithUnpinned(t *testing.T) {
	mock := &mockMetadataVFS{VFS: vfs.NewOSVFS(t.TempDir())}
	targets := []AttributesTarget{{Path: "a.txt", Item: vfs.VFSItem{Name: "a.txt", WinAttrs: 0x400000}}}
	err := setWindowsAttributesForTargets(t.Context(), mock, targets,
		windowsAttributesEdit{winAttrs: winAttrPinned | winAttrUnpinned})
	if err == nil || !strings.Contains(err.Error(), i18n.Msg("Attributes.PinnedExclusive")) {
		t.Fatalf("error = %v, want the Pinned/Unpinned refusal", err)
	}
}

func TestIsCloudFileItem(t *testing.T) {
	for _, tc := range []struct {
		name string
		item vfs.VFSItem
		want bool
	}{
		{"plain file", vfs.VFSItem{WinAttrs: 0x20}, false},
		{"ordinary reparse point", vfs.VFSItem{WinAttrs: 0x400, ReparseTag: 0xA000000C}, false},
		{"cloud reparse tag", vfs.VFSItem{WinAttrs: 0x400, ReparseTag: 0x9000601A}, true},
		{"recall on data access", vfs.VFSItem{WinAttrs: 0x400000}, true},
		{"already pinned", vfs.VFSItem{WinAttrs: 0x80000}, true},
		{"already unpinned", vfs.VFSItem{WinAttrs: 0x100000}, true},
	} {
		if got := isCloudFileItem(tc.item); got != tc.want {
			t.Errorf("%s: isCloudFileItem = %v, want %v", tc.name, got, tc.want)
		}
	}
	if cloudAttributesAvailable(nil) {
		t.Error("cloudAttributesAvailable(nil) = true")
	}
	mixed := []AttributesTarget{{Item: vfs.VFSItem{WinAttrs: 0x80000}}, {Item: vfs.VFSItem{WinAttrs: 0x20}}}
	if cloudAttributesAvailable(mixed) {
		t.Error("a selection with a non-cloud file offers Pinned and Unpinned")
	}
}

// The advanced flags group holds two columns of check boxes, the new ones on
// the right, then the list of every flag, four rows tall with a scroll bar (a
// list of two rows gets none, and the display-only flags below its first two
// lines could not be seen, f4#1931 part 8). The dialog is 23 rows, and 25 with
// the Created and Accessed rows (TestAttributesDialog_FitsWithCreatedAndAccessed).
// The Compressed, Encrypted and Sparse boxes are offered on Windows only, so
// this dump of the other systems has the left column short.
//
// Screen Dump (80x25, the 23 rows of the dialog, the middle 60 columns; the time
// differs per run):
//
//	╔═══════════════════════ Attributes ═══════════════════[×]═╗
//	║                                                          ║
//	║  File: a.txt                                             ║
//	║  ┌─────────────────────  Flags  ──────────────────────┐  ║
//	║  │ [ ] Read only                    [ ] System        │  ║
//	║  │ [ ] Hidden                       [x] Archive       │  ║
//	║  └────────────────────────────────────────────────────┘  ║
//	║  ┌──────────────  Advanced NTFS Flags  ───────────────┐  ║
//	║  │ [ ] Not Content Indexed          [ ] Temporary     │  ║
//	║  │                                  [ ] Offline       │  ║
//	║  │                                  [ ] Pinned        │  ║
//	║  │                                  [ ] Unpinned      │  ║
//	║  │ Compressed: no                                   ▲ │  ║
//	║  │ Encrypted: no                                    ▓ │  ║
//	║  │ Reparse Point: no                                ░ │  ║
//	║  │ Sparse: no                                       ▼ │  ║
//	║  └────────────────────────────────────────────────────┘  ║
//	║                                                          ║
//	║  Last write:   15.10.2026 16:44:35                       ║
//	║                                                          ║
//	║        [ Set ]  [ System properties ]  [ Cancel ]        ║
//	║                                                          ║
//	╚══════════════════════════════════════════════════════════╝
func TestAttributesDialog_AdvancedFlagsLayout(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()
	ShowAttributesWindowsForTargets(nil, vfs.NewOSVFS(t.TempDir()),
		[]AttributesTarget{{Path: "a.txt", Item: vfs.VFSItem{Name: "a.txt", WinAttrs: 0x20, MTime: time.Now()}}})
	vtui.FrameManager.GetTopFrame().Show(scr)

	var buf bytes.Buffer
	scr.Dump(&buf)
	dump := buf.String()
	t.Logf("Screen Dump (80x25):\n%s", dump)
	lines := strings.Split(dump, "\n")
	rowOf := func(text string) int {
		for i, line := range lines {
			if strings.Contains(line, text) {
				return i
			}
		}
		t.Fatalf("%q is not on the screen:\n%s", text, dump)
		return -1
	}
	if rowOf(i18n.Msg("Attributes.NotContentIndexed")) != rowOf(i18n.Msg("Attributes.Temporary")) {
		t.Errorf("Not Content Indexed and Temporary are not on one row:\n%s", dump)
	}
	rows := []int{rowOf(i18n.Msg("Attributes.Temporary")), rowOf(i18n.Msg("Attributes.Offline")),
		rowOf(i18n.Msg("Attributes.Pinned")), rowOf(i18n.Msg("Attributes.Unpinned"))}
	for i := 1; i < len(rows); i++ {
		if rows[i] != rows[i-1]+1 {
			t.Errorf("right column rows = %v, want consecutive rows", rows)
			break
		}
	}
	if rowOf(i18n.Msg("Attributes.BtnSet")) > 2+24 {
		t.Errorf("the buttons are below row 25 of the screen:\n%s", dump)
	}

	// The list: its first four lines in consecutive rows, a scroll bar on the
	// right of them (an up arrow on the first row, a down arrow on the last).
	first := rowOf("Compressed: ")
	last := rowOf("Sparse: ")
	if last-first != advancedFlagsListRows-1 {
		t.Errorf("the list shows rows %d..%d, want %d consecutive rows:\n%s", first, last, advancedFlagsListRows, dump)
	}
	if got := strings.Count(lines[first], string(testutil.Rune(vtui.ScrollUpArrow))); got != 1 {
		t.Errorf("the first row of the list has %d up arrows, want 1:\n%s", got, dump)
	}
	if got := strings.Count(lines[last], string(testutil.Rune(vtui.ScrollDownArrow))); got != 1 {
		t.Errorf("the last row of the list has %d down arrows, want 1:\n%s", got, dump)
	}
	// The buttons are on the 22nd screen row: a blank row and the frame follow.
	if got := rowOf(i18n.Msg("Attributes.BtnSet")) - 2; got != 21 {
		t.Errorf("the buttons are on screen row %d (from 0), want 21:\n%s", got, dump)
	}
}

// With the Created and Accessed rows, which Windows shows for any file, the
// dialog still fits an 80x25 terminal whole: the buttons are on the 23rd row,
// a blank row and the frame follow, and the list keeps its four rows with the
// scroll bar (f4#1931 part 8).
func TestAttributesDialog_FitsWithCreatedAndAccessed(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()
	now := time.Now()
	ShowAttributesWindowsForTargets(nil, vfs.NewOSVFS(t.TempDir()), []AttributesTarget{{Path: "a.txt", Item: vfs.VFSItem{
		Name: "a.txt", WinAttrs: 0x20, MTime: now, BTime: now, ATime: now,
		KnownMetadata: vfs.MetadataExplicit | vfs.MetadataBTime | vfs.MetadataATime,
	}}})
	vtui.FrameManager.GetTopFrame().Show(scr)

	var buf bytes.Buffer
	scr.Dump(&buf)
	dump := buf.String()
	t.Logf("Screen Dump (80x25):\n%s", dump)
	lines := strings.Split(dump, "\n")
	rowOf := func(text string) int {
		for i, line := range lines {
			if strings.Contains(line, text) {
				return i - 2
			}
		}
		t.Fatalf("%q is not on the screen:\n%s", text, dump)
		return -1
	}
	if got := rowOf(i18n.Msg("Attributes.BtnSet")); got != 22 {
		t.Errorf("the buttons are on screen row %d (from 0), want 22:\n%s", got, dump)
	}
	if rowOf(i18n.Msg("Attributes.Created")) >= rowOf(i18n.Msg("Attributes.Accessed")) {
		t.Errorf("Created is not above Accessed:\n%s", dump)
	}
	if got := rowOf("Sparse: ") - rowOf("Compressed: "); got != advancedFlagsListRows-1 {
		t.Errorf("the list rows are %d apart, want %d:\n%s", got, advancedFlagsListRows-1, dump)
	}
}
