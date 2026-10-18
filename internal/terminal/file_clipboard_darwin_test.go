//go:build darwin

package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// The real pasteboard through the real reader. Finder offers copied files as
// file URLs and f4 used to read only the plain text beside them, so Ctrl+V of a
// Finder copy put the file name on the command line (f4#2007). The test
// overwrites the user's clipboard, so it runs only on CI runners.
func TestReadDarwinFileClipboardPasteboard(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Skip("overwrites the system clipboard; runs on CI runners only")
	}
	if _, err := exec.LookPath("osascript"); err != nil {
		t.Skipf("osascript is not available: %v", err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	one, two := filepath.Join(dir, "one.txt"), filepath.Join(dir, "two words.txt")
	for _, p := range []string{one, two} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The test reads and writes a pasteboard of its own, not the general one:
	// that is the clipboard of the whole desktop session, and another test
	// binary writing it between this test's write and read made the reader
	// find one file of two, or none (main c64da8294, train unxed/f4#2065).
	name := fmt.Sprintf("org.f4.test.files.%d", os.Getpid())
	old := darwinFileClipboardPasteboard
	darwinFileClipboardPasteboard = name
	t.Cleanup(func() { darwinFileClipboardPasteboard = old })
	board := "var pb = $.NSPasteboard.pasteboardWithName($('" + name + "'));\n"
	run := func(t *testing.T, body string) string {
		t.Helper()
		script := "ObjC.import('AppKit');\n" + board + body
		out, err := exec.Command("osascript", "-l", "JavaScript", "-e", script).CombinedOutput() // #nosec G204 -- fixed test script; the paths are JSON-encoded.
		if err != nil {
			t.Fatalf("%s: %v: %s", script, err, out)
		}
		return string(out)
	}
	t.Cleanup(func() { run(t, "pb.releaseGlobally; ''") })
	// writeFiles puts each path on the pasteboard as an item of its own, as
	// Finder does with a copied selection, and reads them back before the
	// writer exits.
	writeFiles := func(t *testing.T, paths ...string) {
		t.Helper()
		names, err := json.Marshal(paths)
		if err != nil {
			t.Fatal(err)
		}
		out := run(t, `pb.clearContents;
var items = [];
`+"JSON.parse('"+string(names)+"')"+`.forEach(function (p) { items.push($.NSURL.fileURLWithPath(p)); });
var wrote = pb.writeObjects($(items));
var back = pb.readObjectsForClassesOptions($.NSArray.arrayWithObject($.NSURL), $.NSDictionary.dictionaryWithObjectForKey($(true), $.NSPasteboardURLReadingFileURLsOnlyKey));
'items=' + items.length + ' wrote=' + wrote + ' pasteboard items=' + pb.pasteboardItems.count + ' back=' + back.count;`)
		t.Logf("pasteboard: %s", out)
	}

	t.Run("files", func(t *testing.T) {
		writeFiles(t, one, two)
		got, cut, err := readFileClipboard(context.Background())
		if err != nil {
			t.Fatalf("reading copied files failed: %v", err)
		}
		sort.Strings(got)
		if want := []string{one, two}; !reflect.DeepEqual(got, want) || cut {
			t.Fatalf("files = %q cut = %v, want %q", got, cut, want)
		}
	})
	t.Run("one file", func(t *testing.T) {
		writeFiles(t, one)
		got, _, err := readFileClipboard(context.Background())
		if err != nil || !reflect.DeepEqual(got, []string{one}) {
			t.Fatalf("files = %q, err = %v, want %q", got, err, []string{one})
		}
	})
	t.Run("text is not a file", func(t *testing.T) {
		path, err := json.Marshal(one)
		if err != nil {
			t.Fatal(err)
		}
		run(t, "pb.clearContents; pb.setStringForType($("+string(path)+"), $.NSPasteboardTypeString); ''")
		if got, _, err := readFileClipboard(context.Background()); err == nil {
			t.Fatalf("plain text with a path read as files %q", got)
		}
	})
	t.Run("empty", func(t *testing.T) {
		run(t, "pb.clearContents; ''")
		if got, _, err := readFileClipboard(context.Background()); err == nil {
			t.Fatalf("an empty clipboard read as files %q", got)
		}
	})
}
