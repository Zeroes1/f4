package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// The keys of the player, pressed through the key router the way a user
// presses them (#2185). The player plays into a paneltest.FakeAudio, which
// says what it was told to play.

// playerFixture is a panels frame whose active panel, the right one, lists
// files of a temporary folder, with the cursor on the first of them, and the
// player opened from it by Ctrl+Shift+M: the player is on the left. The
// playlist lives in a profile of the test's own.
type playerFixture struct {
	t      *testing.T
	pf     *panel.PanelsFrame
	player *panel.PlayerPanel
	audio  *paneltest.FakeAudio
	dir    string
	scr    *vtui.ScreenBuf
}

func newPlayerFixture(t *testing.T, files ...string) *playerFixture {
	t.Helper()
	profile := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return profile, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	config.ResetConfigDirForTest()
	t.Cleanup(config.ResetConfigDirForTest)
	engines := paneltest.UseFakeAudio(t)

	f := &playerFixture{t: t, scr: initFrameworkActionTestScreen(t), dir: t.TempDir()}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(f.dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.pf = paneltest.SetupMockPanelsFrame(t)
	f.pf.ResizeConsole(100, 30)
	t.Cleanup(f.pf.Close)
	f.pf.ShowPanels = true
	vtui.FrameManager.Push(f.pf)
	fsp := f.pf.GetActivePanel()
	f.pf.SwitchToVFS(fsp, vfs.NewOSVFS(f.dir))
	paneltest.WaitForLoad(t, fsp)
	fsp.SetCursorIndex(1)

	f.press(playerKey(vtinput.VK_M, vtinput.LeftCtrlPressed|vtinput.ShiftPressed))
	player, ok := f.pf.AltPanels[1-f.pf.ActiveIdx].(*panel.PlayerPanel)
	if !ok {
		t.Fatalf("Ctrl+Shift+M opened %T in the passive slot, want the player", f.pf.AltPanels[1-f.pf.ActiveIdx])
	}
	t.Cleanup(player.Close)
	made := engines()
	if len(made) != 1 {
		t.Fatalf("the player made %d audio engines, want 1", len(made))
	}
	f.player, f.audio = player, made[0]
	return f
}

// press is a key pressed in the panels, and the screen drawn after it as f4
// draws it before the next key: drawing is what gives an alternative panel
// the focus after Tab.
func (f *playerFixture) press(e *vtinput.InputEvent) {
	f.t.Helper()
	pressKey(f.pf, e)
	f.pf.Show(f.scr)
}

// newFolder is F7 in the player and name typed into the dialog it opens.
func (f *playerFixture) newFolder(name string) {
	f.t.Helper()
	f.press(playerKey(vtinput.VK_F7, 0))
	dlg := vtui.FrameManager.GetTopFrame()
	if dlg == f.pf {
		f.t.Fatal("F7 in the player opened nothing")
	}
	for _, ch := range name {
		pressKey(dlg, playerChar(0, ch))
	}
	pressKey(dlg, playerKey(vtinput.VK_RETURN, 0))
	f.pf.Show(f.scr)
}

func (f *playerFixture) path(name string) string { return filepath.Join(f.dir, name) }

func playerKey(vk uint16, mods vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: mods}
}

func playerChar(vk uint16, ch rune) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, Char: ch}
}

// The report: music on the right, the player on the left with those files in
// its playlist, a track of the playlist playing. Tab to the player and back,
// and the file under the cursor of the file panel played instead -- on the
// next arrow there, which the player follows. A track of the playlist now
// stays until the user picks another.
func TestPlayerKeepsThePlaylistTrackWhenTheFocusComesBack(t *testing.T) {
	f := newPlayerFixture(t, "a.mp3", "b.mp3")

	f.press(playerKey(vtinput.VK_F5, 0)) // a.mp3 into the playlist
	f.press(playerKey(vtinput.VK_TAB, 0))
	if !f.player.IsFocused() {
		t.Fatal("Tab did not move the focus to the player")
	}
	f.press(playerKey(vtinput.VK_RETURN, 0)) // play it from the playlist
	if got := f.audio.Path(); got != f.path("a.mp3") {
		t.Fatalf("Enter on the playlist track loaded %q, want a.mp3", got)
	}

	f.press(playerKey(vtinput.VK_TAB, 0))
	if f.player.IsFocused() {
		t.Fatal("the second Tab left the focus in the player")
	}
	f.press(playerKey(vtinput.VK_DOWN, 0))
	if got := f.pf.GetActivePanel().GetSelectedName(); got != "b.mp3" {
		t.Fatalf("Down put the cursor on %q, want b.mp3", got)
	}
	if got := f.audio.Path(); got != f.path("a.mp3") {
		t.Errorf("an arrow in the file panel swapped the playlist track for %q (loads %q)", got, f.audio.Loads)
	}
}

// Without a track of the playlist the player still follows the cursor of the
// file panel, as Far's AudioPlayer does: Enter plays a recording, and Down
// plays the next one (#380).
func TestPlayerFollowsTheFilePanelWhenItPlaysItsFiles(t *testing.T) {
	f := newPlayerFixture(t, "a.mp3", "b.mp3")

	f.press(playerKey(vtinput.VK_RETURN, 0))
	if got := f.audio.Path(); got != f.path("a.mp3") {
		t.Fatalf("Enter on a recording loaded %q, want a.mp3", got)
	}
	f.press(playerKey(vtinput.VK_DOWN, 0))
	if got := f.audio.Path(); got != f.path("b.mp3") {
		t.Errorf("Down in the file panel loaded %q, want b.mp3", got)
	}
}

// Z X C V B are told by the key they are on: with the Russian layout on they
// type я ч с м и, and the player took them for nothing at all. A terminal
// sends only the character; the Windows console and the kitty protocol send
// the key too.
func TestPlayerLetterKeysWorkInTheRussianLayout(t *testing.T) {
	f := newPlayerFixture(t, "a.mp3")
	f.press(playerKey(vtinput.VK_F5, 0))
	f.press(playerKey(vtinput.VK_TAB, 0))

	for _, how := range []struct {
		name    string
		withKey bool
	}{{"the character alone", false}, {"the key and the character", true}} {
		key := func(vk uint16, ch rune) *vtinput.InputEvent {
			if !how.withKey {
				vk = 0
			}
			return playerChar(vk, ch)
		}
		f.press(key(vtinput.VK_X, 'ч'))
		if got := f.audio.Path(); got != f.path("a.mp3") || !f.audio.IsPlaying() {
			t.Errorf("%s: ч (X, Play) left the engine with %q, playing=%v", how.name, got, f.audio.IsPlaying())
		}
		f.press(key(vtinput.VK_C, 'с'))
		if f.audio.IsPlaying() {
			t.Errorf("%s: с (C, Pause) did not pause", how.name)
		}
		f.press(key(vtinput.VK_V, 'м'))
		if f.audio.Path() != "" {
			t.Errorf("%s: м (V, Stop) did not stop", how.name)
		}
	}
	if !f.pf.CmdLine.IsEmpty() {
		t.Errorf("the letters went to the command line too: %q", f.pf.CmdLine.Edit.GetText())
	}
}

// F7 made a playlist folder only with the cursor in the playlist, and with
// the playlist empty the cursor never leaves the buttons, so the first folder
// could not be made at all. It works on the buttons too.
func TestPlayerF7MakesAFolderFromTheButtons(t *testing.T) {
	f := newPlayerFixture(t)
	f.press(playerKey(vtinput.VK_TAB, 0))

	f.newFolder("Album")
	if got := f.player.GetSelectedName(); got != "Album" {
		t.Errorf("after F7 the cursor of the player is on %q, want the new folder Album", got)
	}
}

// Ctrl with the arrows moved playlist items, and macOS keeps those keys for
// Mission Control. Shift with the arrows moves them now, on every system; Ctrl
// still works where it reaches f4.
func TestPlayerShiftArrowsMovePlaylistItems(t *testing.T) {
	for _, mod := range []struct {
		name string
		key  vtinput.ControlKeyState
	}{{"Shift", vtinput.ShiftPressed}, {"Ctrl", vtinput.LeftCtrlPressed}} {
		t.Run(mod.name, func(t *testing.T) {
			f := newPlayerFixture(t, "a.mp3", "b.mp3")
			selected := func() string { return f.player.GetSelectedName() }
			// Both files into the playlist: a, then b.
			f.press(playerKey(vtinput.VK_INSERT, 0))
			f.press(playerKey(vtinput.VK_INSERT, 0))
			f.press(playerKey(vtinput.VK_F5, 0))
			f.press(playerKey(vtinput.VK_TAB, 0))
			if got := selected(); got != "a" {
				t.Fatalf("the playlist starts at %q, want a", got)
			}

			f.press(playerKey(vtinput.VK_DOWN, mod.key))
			if got := selected(); got != "a" {
				t.Fatalf("%s+Down left the cursor on %q, want it to follow a", mod.name, got)
			}
			f.press(playerKey(vtinput.VK_UP, 0))
			if got := selected(); got != "b" {
				t.Fatalf("%s+Down did not move a below b: above a is %q", mod.name, got)
			}

			// A folder at the end, moved to the top: F, b, a.
			f.press(playerKey(vtinput.VK_UP, 0)) // the buttons
			f.newFolder("F")
			f.press(playerKey(vtinput.VK_UP, mod.key))
			f.press(playerKey(vtinput.VK_UP, mod.key))
			f.press(playerKey(vtinput.VK_DOWN, 0))
			if got := selected(); got != "b" {
				t.Fatalf("below the folder is %q, want b", got)
			}
			// b into the folder above it, and out again.
			f.press(playerKey(vtinput.VK_RIGHT, mod.key))
			f.press(playerKey(vtinput.VK_LEFT, 0)) // Left on a track in a folder goes to the folder
			if got := selected(); got != "F" {
				t.Fatalf("%s+Right did not put b into the folder: Left from it went to %q", mod.name, got)
			}
			f.press(playerKey(vtinput.VK_DOWN, 0))
			f.press(playerKey(vtinput.VK_LEFT, mod.key))
			f.press(playerKey(vtinput.VK_LEFT, 0)) // on the top level it stays
			if got := selected(); got != "b" {
				t.Errorf("%s+Left did not take b out of the folder: Left from it went to %q", mod.name, got)
			}
			if f.pf.WidthDecrement != 0 {
				t.Errorf("%s with the arrows moved the split between the panels by %d", mod.name, f.pf.WidthDecrement)
			}
		})
	}
}

// Ctrl+Alt+Left and Right seek from the file panel, without going to the
// player -- but the action looked for the player in the slot that has the
// focus, which is the file panel's, and never found it.
func TestPlayerSeeksFromTheFilePanel(t *testing.T) {
	f := newPlayerFixture(t, "a.mp3")
	f.press(playerKey(vtinput.VK_RETURN, 0))
	if f.player.IsFocused() || f.audio.Path() == "" {
		t.Fatalf("Enter on a recording: player focused=%v, loaded %q", f.player.IsFocused(), f.audio.Path())
	}
	ctrlAlt := vtinput.ControlKeyState(vtinput.LeftCtrlPressed | vtinput.LeftAltPressed)
	f.press(playerKey(vtinput.VK_RIGHT, ctrlAlt))
	f.press(playerKey(vtinput.VK_RIGHT, ctrlAlt))
	f.press(playerKey(vtinput.VK_LEFT, ctrlAlt))
	if got := f.audio.Position(); got != 5*time.Second {
		t.Errorf("two Ctrl+Alt+Right and a Ctrl+Alt+Left left the track at %s, want 5s", got)
	}
}
