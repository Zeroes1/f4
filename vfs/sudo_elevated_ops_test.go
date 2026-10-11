package vfs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// f4#1768 step 3: the elevated dispatcher answers the path operations itself.
func TestElevatedFileOpsPathOperations(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if r := ElevatedFileOps(SudoRequest{Cmd: CmdMkDir, Path: sub, Mode: 0o755}); r.Error != "" {
		t.Fatalf("MkDir: %s", r.Error)
	}
	file := filepath.Join(sub, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := ElevatedFileOps(SudoRequest{Cmd: CmdStat, Path: file})
	if r.Error != "" || r.Item.Name != "a.txt" || r.Item.Size != 5 || r.Item.IsDir {
		t.Fatalf("Stat = %+v", r)
	}
	r = ElevatedFileOps(SudoRequest{Cmd: CmdReadDir, Path: sub})
	if r.Error != "" || len(r.Items) != 1 || r.Items[0].Name != "a.txt" {
		t.Fatalf("ReadDir = %+v", r)
	}

	when := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	item := r.Items[0]
	item.MTime, item.ATime = when, when
	if r := ElevatedFileOps(SudoRequest{Cmd: CmdSetAttributes, Path: file, Item: item}); r.Error != "" {
		t.Fatalf("SetAttributes: %s", r.Error)
	}
	if st, err := os.Stat(file); err != nil || !st.ModTime().Equal(when) {
		t.Errorf("modification time after SetAttributes = %v, %v", st.ModTime(), err)
	}

	other := filepath.Join(sub, "b.txt")
	if err := os.WriteFile(other, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := ElevatedFileOps(SudoRequest{Cmd: CmdRename, Path: file, Path2: other, Flags: SudoRenameNoReplace}); r.Error == "" {
		t.Error("a no-replace rename over an existing file succeeded")
	}
	moved := filepath.Join(sub, "c.txt")
	if r := ElevatedFileOps(SudoRequest{Cmd: CmdRename, Path: file, Path2: moved}); r.Error != "" {
		t.Fatalf("Rename: %s", r.Error)
	}
	if r := ElevatedFileOps(SudoRequest{Cmd: CmdRemove, Path: sub}); r.Error != "" {
		t.Fatalf("Remove: %s", r.Error)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Errorf("the folder is still there after Remove: %v", err)
	}

	if r := ElevatedFileOps(SudoRequest{Cmd: CmdOpen, Path: moved}); r.Error == "" {
		t.Error("Open was accepted although contents cannot travel by descriptor yet")
	}
	if r := ElevatedFileOps(SudoRequest{Cmd: CmdStat, Path: filepath.Join(dir, "missing")}); r.Error == "" {
		t.Error("Stat of a missing file reported no error")
	}
}
