package editor

import (
	"os"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
)

// The editor draws through f4's palette, which extends vtui's beyond its last
// index — the viewer's top bar and the crosshair both read one of the extra
// colours. Without this a test that happens to run first indexes past the end
// of the default palette, and which test that is depends on the shuffle seed.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, theme.SetDefaultF4Palette, nil))
}
