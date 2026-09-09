package app

import (
	"context"
	"github.com/unxed/f4/internal/panel"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestFileSystemPanelSemanticPanelNode(t *testing.T) {
	tmp := t.TempDir()
	fp := &panel.FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		ViewMode:      panel.ViewModeDetailed,
		SortMode:      panel.SortSize,
		SelectedItems: make(map[string]bool),
		Entries: []*panel.FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true, Mode: "drwxr-xr-x"}},
			{VFSItem: vfs.VFSItem{Name: "alpha.txt", Size: 1234, MTime: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC), Mode: "-rw-r--r--"}, Selected: true},
		},
	}
	fp.SetCanFocus(true)
	fp.SetPosition(0, 0, 39, 9)
	fp.SetCursorIndex(1)

	model := fp.SemanticPanelModel(&vtui.SemanticContext{Width: 80, Height: 25}, 0, true)
	node := model.ToMap()

	if node["kind"] != "filePanel" {
		t.Fatalf("kind = %v, want filePanel", node["kind"])
	}
	if node["active"] != true || node["side"] != 0 {
		t.Fatalf("unexpected panel identity: active=%v side=%v", node["active"], node["side"])
	}
	if node["cursor"] != 1 {
		t.Fatalf("cursor = %v, want 1", node["cursor"])
	}
	if node["path"] != tmp {
		t.Fatalf("path = %v, want %s", node["path"], tmp)
	}
	if node["selectedCount"] != 1 {
		t.Fatalf("selectedCount = %v, want 1", node["selectedCount"])
	}
	entries := node["entries"].([]map[string]any)
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[1]["name"] != "alpha.txt" || entries[1]["selected"] != true {
		t.Fatalf("unexpected entry snapshot: %#v", entries[1])
	}
}

func TestPanelsFrameSemanticActionAcceptsQMLNumbers(t *testing.T) {
	tmp := t.TempDir()
	left := &panel.FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(0, 0, 39, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(1, 1, 38, 6, nil),
		ViewMode:      panel.ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Entries: []*panel.FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "alpha.txt", Size: 12}},
			{VFSItem: vfs.VFSItem{Name: "beta.txt", Size: 34}},
		},
	}
	right := &panel.FileSystemPanel{
		Vfs:           vfs.NewOSVFS(tmp),
		Frame:         vtui.NewBorderedFrame(40, 0, 79, 9, vtui.SingleBox, tmp),
		Table:         vtui.NewTable(41, 1, 78, 6, nil),
		ViewMode:      panel.ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Entries: []*panel.FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "right.txt", Size: 56}},
		},
	}
	pf := &panel.PanelsFrame{
		Panels:    [2]panel.Panel{left, right},
		ActiveIdx: 0,
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.cursor",
		"side":   float64(0),
		"index":  float64(2),
	}) {
		t.Fatal("panel cursor action was not handled")
	}
	if left.GetCursorIndex() != 2 {
		t.Fatalf("left cursor = %d, want 2", left.GetCursorIndex())
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "panel.activate",
		"side":   float64(1),
	}) {
		t.Fatal("activate panel action was not handled")
	}
	if pf.ActiveIdx != 1 {
		t.Fatalf("activeIdx = %d, want 1", pf.ActiveIdx)
	}
}

func TestSemantic_EditorViewActions(t *testing.T) {
	vtui.SetDefaultPalette()
	Pt := piecetable.New([]byte("hello"))
	ev := editor.NewEditorView(Pt, nil, "test.txt")
	defer ev.Close()
	ev.Modified = false
	ev.CursorPos = ev.GetLineLength(0)

	// 1. Test insertText
	actionInsert := map[string]any{
		"target": vtui.SemanticID(ev),
		"action": "editor.insertText",
		"text":   " world",
	}
	if !ev.HandleSemanticAction(actionInsert) {
		t.Fatal("editor insert action was not handled")
	}
	if ev.GetText() != "hello world" {
		t.Errorf("expected 'hello world', got %q", ev.GetText())
	}
	if !ev.Modified {
		t.Error("editor should be marked as modified after insertion")
	}

	// 2. Test Undo
	actionUndo := map[string]any{
		"target": vtui.SemanticID(ev),
		"action": "editor.undo",
	}
	if !ev.HandleSemanticAction(actionUndo) {
		t.Fatal("editor undo action was not handled")
	}
	if ev.GetText() != "hello" {
		t.Errorf("expected 'hello' after undo, got %q", ev.GetText())
	}
}

func TestSemantic_ViewerViewActions(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "view.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmp)
	vv, err := viewer.NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatalf("failed to create vv: %v", err)
	}
	// The vv holds the file open; without this Close Windows cannot
	// delete it during TempDir cleanup.
	defer vv.Close()

	// Test scroll action
	actionScroll := map[string]any{
		"target": vtui.SemanticID(vv),
		"action": "viewer.scroll",
		"offset": float64(6), // Starts 'line2'
	}
	if !vv.HandleSemanticAction(actionScroll) {
		t.Fatal("viewer scroll action was not handled")
	}
	if vv.TopOffset != 6 {
		t.Errorf("expected TopOffset 6, got %d", vv.TopOffset)
	}
}
