//go:build darwin

package main

import (
	"testing"

	"github.com/unxed/goclip"
)

// The text clipboard on macOS is goclip's own darwin driver, which talks to
// NSPasteboard and needs no locale. A driver that f4 registers under the same
// name wins over it, since f4's init runs after goclip's, and the one f4 once
// had there ran pbpaste: a GUI f4 started without a UTF-8 locale (from far2l's
// terminal) pasted "Лунобот-2" as "???????-2" (f4#2057). The test lives here,
// where every package of f4 has run its init, so an override anywhere shows.
func TestDarwinClipboardIsGoclipsNativeDriver_Issue2057(t *testing.T) {
	d, ok := goclip.GetDriver("darwin")
	if !ok {
		t.Fatal(`goclip has no "darwin" driver registered`)
	}
	if _, native := d.(*goclip.DarwinDriver); !native {
		t.Errorf(`the "darwin" clipboard slot holds %T, not *goclip.DarwinDriver: something in f4 overrides goclip's native pasteboard driver`, d)
	}
}
