package main

import (
	"context"
	"testing"

	"github.com/unxed/vtui"
)

// Include provider snapshots, catalog discovery, layout and the first paint.
// No PanelsFrame is needed: opening settings must also work in standalone editors.
func BenchmarkSettingsOpen(b *testing.B) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(160, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sessions, err := beginSettingsSessions(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		c := newSettingsCenter(sessions)
		c.ResizeConsole(160, 50)
		c.Show(scr)
		for _, s := range sessions {
			s.draft.Close()
		}
	}
}
