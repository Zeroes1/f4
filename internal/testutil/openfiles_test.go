package testutil

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A file or socket opened between two listings is reported with its
// descriptor; closing it makes the listings agree again.
func TestOpenFilesReportsWhatWasOpenedInBetween(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("open files are not listed on Windows")
	}
	before := openFiles()

	f, err := os.Create(filepath.Join(t.TempDir(), "held.txt"))
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	leaked := leakedFiles(before, openFiles())
	var kinds []string
	for _, lf := range leaked {
		kinds = append(kinds, lf.kind)
	}
	if len(leaked) != 2 || !contains(kinds, "file") || !contains(kinds, "socket") {
		t.Fatalf("leaked = %v, want one file and one socket", leaked)
	}
	if runtime.GOOS == "linux" && leaked[0].path == "" && leaked[1].path == "" {
		t.Error("on Linux the file behind a descriptor is named, yet none was")
	}

	_ = f.Close()
	_ = l.Close()
	if leaked := leakedFiles(before, openFiles()); len(leaked) != 0 {
		t.Fatalf("after closing both, leaked = %v", leaked)
	}
}

// A descriptor number reused for something of the same kind is invisible
// one by one; the count per kind still tells that one more is open.
func TestLeakedFilesCountsReusedDescriptors(t *testing.T) {
	before := []openFile{{fd: 3, kind: "file"}}
	after := []openFile{{fd: 3, kind: "file"}, {fd: 4, kind: "file"}}
	if got := leakedFiles(before, after); len(got) != 1 || got[0].fd != 4 {
		t.Fatalf("leaked = %v, want fd 4", got)
	}
	same := []openFile{{fd: 3, kind: "socket"}}
	if got := leakedFiles(before, same); len(got) != 1 || got[0].kind != "socket" {
		t.Fatalf("a descriptor of another kind: leaked = %v, want the socket", got)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
