//go:build darwin

package terminal

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
)

// darwinFileClipboardScript lists the files on the general pasteboard as a JSON
// array of absolute paths. Finder offers a copied selection as file URLs next
// to the plain names, so the text on the pasteboard alone cannot tell "these
// files" from "this string"; AppKit reads the URLs themselves. An empty
// pasteboard, text or an image give "[]".
const darwinFileClipboardScript = `ObjC.import('AppKit');
var out = [];
try {
	var pb = $.NSPasteboard.generalPasteboard;
	var opts = $.NSDictionary.dictionaryWithObjectForKey($(true), $.NSPasteboardURLReadingFileURLsOnlyKey);
	var urls = pb.readObjectsForClassesOptions($.NSArray.arrayWithObject($.NSURL), opts);
	var n = urls.count;
	for (var i = 0; i < n; i++) {
		out.push(ObjC.unwrap(urls.objectAtIndex(i).path));
	}
} catch (e) {}
JSON.stringify(out);`

// darwinFileClipboardPasteboard names the pasteboard readDarwinFileClipboard
// reads; empty is the general one, the clipboard of the desktop. A test names
// one of its own: the general pasteboard is shared by every process of the
// session, and another test binary writing it between the test's write and
// read made the reader find one file of two, or none (main c64da8294, train
// unxed/f4#2065, on the macOS runners).
var darwinFileClipboardPasteboard = ""

var pasteboardName = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// darwinFileClipboardScriptFor is darwinFileClipboardScript reading the named
// pasteboard (a name that is not a plain identifier reads the general one).
func darwinFileClipboardScriptFor(name string) string {
	if name == "" || !pasteboardName.MatchString(name) {
		return darwinFileClipboardScript
	}
	return strings.Replace(darwinFileClipboardScript, "$.NSPasteboard.generalPasteboard",
		"$.NSPasteboard.pasteboardWithName($('"+name+"'))", 1)
}

// readDarwinFileClipboard reads the files Finder (or any other application)
// copied. Finder has no cut: its move is a paste with Option, so the second
// result is always false.
func readDarwinFileClipboard(ctx context.Context) ([]string, bool, error) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return nil, false, errFileClipboardUnavailable
	}
	script := darwinFileClipboardScriptFor(darwinFileClipboardPasteboard)
	out, err := exec.CommandContext(ctx, "osascript", "-l", "JavaScript", "-e", script).Output() // #nosec G204 -- fixed script; the only part put in is a pasteboard name checked to be a plain identifier.
	if err != nil {
		return nil, false, errFileClipboardUnavailable
	}
	paths := parseFileClipboardJSON(out)
	if len(paths) == 0 {
		return nil, false, errFileClipboardUnavailable
	}
	return paths, false, nil
}
