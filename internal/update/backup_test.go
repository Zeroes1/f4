package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecutableBackupRestore(t *testing.T) {
	tmpDir := t.TempDir()
	executable := filepath.Join(tmpDir, "f4")
	original := []byte("working f4")
	if err := os.WriteFile(executable, original, 0755); err != nil { // #nosec G306 -- test executable fixture.
		t.Fatal(err)
	}

	oldExecutable := Executable
	Executable = func() (string, error) { return executable, nil }
	defer func() { Executable = oldExecutable }()

	backup, err := BackupExecutable()
	if err != nil {
		t.Fatalf("BackupExecutable() error: %v", err)
	}
	if got, err := os.ReadFile(backup); err != nil || string(got) != string(original) {
		t.Fatalf("backup contents = %q, %v; want %q", got, err, original)
	}
	if err := os.WriteFile(executable, []byte("broken f4"), 0755); err != nil { // #nosec G306 -- test executable fixture.
		t.Fatal(err)
	}

	if err := RestoreExecutable(backup); err != nil {
		t.Fatalf("RestoreExecutable() error: %v", err)
	}
	if got, err := os.ReadFile(executable); err != nil || string(got) != string(original) {
		t.Fatalf("restored contents = %q, %v; want %q", got, err, original)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup still exists: %v", err)
	}
}
