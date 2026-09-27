package multiarc

import (
	"path/filepath"
	"testing"
)

// "tar -cf x.tar -C dir ." stores every member under "./", and GNU tar will
// not extract "./dir/file.txt" when asked for "dir/file.txt". Viewing such a
// member used to fail with "Not found in archive"; Open now asks for the
// name the listing gave.
func TestTarRealOpensDotSlashMember(t *testing.T) {
	requireRealTool(t, "tar")
	src := t.TempDir()
	writeTree(t, src, map[string]string{"dir/file.txt": "hello"})
	arc := filepath.Join(t.TempDir(), "dot.tar")
	runReal(t, src, "tar", "-cf", arc, ".")
	t.Cleanup(closeSharedMultiArcTempDirs)

	v := openReal(t, arc)
	if got := readMember(t, v, "/dir/file.txt"); got != "hello" {
		t.Fatalf("member content = %q, want hello", got)
	}
}
