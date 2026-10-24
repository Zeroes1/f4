package app

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtinput"
)

// screenRows is what the player shows, row by row, on the fixture's screen.
func (f *playerFixture) screenRows() []string {
	f.pf.Show(f.scr)
	x1, y1, x2, y2 := f.player.GetPosition()
	var rows []string
	for y := y1; y <= y2; y++ {
		var sb strings.Builder
		for x := x1; x <= x2; x++ {
			ch := rune(f.scr.GetCell(x, y).Char) // #nosec G115 -- a cell holds one character
			if ch == 0 {
				ch = ' '
			}
			sb.WriteRune(ch)
		}
		rows = append(rows, sb.String())
	}
	return rows
}

// The player drew a spectrum, which says little, and nothing that showed how
// far the track had got (#2185). The bar between the two clocks does, and
// the spectrum is a setting, off by default, a row under the bar.
func TestPlayerDrawsThePlaybackBar(t *testing.T) {
	old := config.App.PlayerSpectrum
	t.Cleanup(func() { config.App.PlayerSpectrum = old })
	config.App.PlayerSpectrum = false

	f := newPlayerFixture(t, "a.mp3")
	f.press(playerKey(vtinput.VK_RETURN, 0))
	f.audio.Seek(90 * time.Second) // half of the fake's three minutes

	rows := f.screenRows()
	bar := ""
	for _, row := range rows {
		if strings.Contains(row, "01:30") {
			bar = row
		}
	}
	if !strings.Contains(bar, "03:00") || !strings.Contains(bar, "●") {
		t.Fatalf("no playback bar with 01:30 and 03:00 in the player:\n%s", strings.Join(rows, "\n"))
	}
	filled, rest := strings.Count(bar, "━"), strings.Count(bar, "─")
	if filled == 0 || rest == 0 || filled-rest > 1 || rest-filled > 1 {
		t.Errorf("at half the track the bar is %q: %d cells played, %d to go", bar, filled, rest)
	}

	separator := func(rows []string) int {
		for i, row := range rows {
			if strings.HasPrefix(row, "├") {
				return i
			}
		}
		t.Fatalf("no separator above the playlist:\n%s", strings.Join(rows, "\n"))
		return -1
	}
	without := separator(rows)
	config.App.PlayerSpectrum = true
	if with := separator(f.screenRows()); with != without+1 {
		t.Errorf("with the spectrum on the playlist starts at row %d, want %d: a row under the bar", with, without+1)
	}
}
