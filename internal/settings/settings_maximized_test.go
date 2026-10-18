package settings

import (
	"context"
	"testing"

	"github.com/unxed/f4/internal/config"
)

// f4#1239: the user can have Settings open maximized every time, also on a
// screen big enough for the ordinary window; the zoom button keeps the
// ordinary bounds to go back to.
func TestSettingsCenterOpensMaximizedWhenAsked(t *testing.T) {
	saved := config.App.SettingsMaximized
	t.Cleanup(func() { config.App.SettingsMaximized = saved })

	open := func() *settingsCenter {
		d, _ := (coreSettingsProvider{}).Begin(context.Background())
		t.Cleanup(d.Close)
		c := newSettingsCenter([]*settingsSession{{catalog: (coreSettingsProvider{}).Catalog(), draft: d}})
		c.ResizeConsole(180, 60)
		return c
	}

	config.App.SettingsMaximized = false
	if c := open(); c.SavedBounds != nil || c.X1 == 0 {
		t.Fatalf("without the setting a big screen keeps the ordinary window, got %d,%d-%d,%d", c.X1, c.Y1, c.X2, c.Y2)
	}

	config.App.SettingsMaximized = true
	c := open()
	if c.SavedBounds == nil || c.X1 != 0 || c.X2 != 179 {
		t.Fatalf("with the setting the window opens over the whole width, got %d,%d-%d,%d", c.X1, c.Y1, c.X2, c.Y2)
	}
	if b := c.SavedBounds; b.X1 == 0 || b.X2 == 179 {
		t.Fatalf("the ordinary bounds for the zoom button are kept, got %+v", *b)
	}
}
