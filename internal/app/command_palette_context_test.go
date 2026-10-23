package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
)

// openPaletteOver opens the command palette over a panels frame whose active
// side shows the given alternative panel (nil: the file panel) and returns it.
func openPaletteOver(t *testing.T, alt func(fsp *panel.FileSystemPanel) panel.AltPanel) *commandPaletteDialog {
	t.Helper()
	previous := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previous })
	initFrameworkActionTestScreen(t)
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ResizeConsole(100, 30)
	t.Cleanup(pf.Close)
	pf.ShowPanels = true
	vtui.FrameManager.Push(pf)
	if alt != nil {
		pf.AltPanels[pf.ActiveIdx] = alt(pf.GetActivePanel())
	}
	if !ShowCommandPalette() {
		t.Fatal("the command palette did not open")
	}
	d, ok := vtui.FrameManager.GetTopFrame().(*commandPaletteDialog)
	if !ok {
		t.Fatalf("top frame = %T, want the command palette", vtui.FrameManager.GetTopFrame())
	}
	t.Cleanup(d.Close)
	return d
}

func filteredIDs(d *commandPaletteDialog) []string {
	ids := make([]string, 0, len(d.filtered))
	for _, entry := range d.filtered {
		ids = append(ids, entry.ID)
	}
	return ids
}

// checkOwnCommandsFirst says every command in want is offered and every one of
// own comes before anything else the palette's fuzzy match lets through.
func checkOwnCommandsFirst(t *testing.T, ids, want []string, own func(id string) bool) {
	t.Helper()
	for _, name := range want {
		if !slices.Contains(ids, name) {
			t.Errorf("%s is not offered: %v", name, ids)
		}
	}
	other := false
	for _, id := range ids {
		switch {
		case !own(id):
			other = true
		case other:
			t.Errorf("%s comes after a command of something else: %v", id, ids)
		}
	}
}

// The palette offers the focused quick view's and player's own commands only
// while they have the focus, and opens on them then: the query starts with
// the prefix their labels share, and typing goes on from it.
func TestCommandPaletteOpensOnTheFocusedQuickViewsCommands(t *testing.T) {
	d := openPaletteOver(t, func(fsp *panel.FileSystemPanel) panel.AltPanel { return panel.NewQuickViewPanel(fsp) })
	want := i18n.Msg("CommandPalette.QueryQuickView") + " "
	if got := d.query.GetText(); got != want {
		t.Fatalf("the query is %q, want %q", got, want)
	}
	checkOwnCommandsFirst(t, filteredIDs(d),
		[]string{"QuickView.Wrap", "QuickView.OpenViewer", "QuickView.Hex", "QuickView.Search", "QuickView.NextCodepage", "Panel.AltPanelCopy"},
		func(id string) bool {
			return strings.HasPrefix(id, "QuickView.") || id == "Panel.AltPanelCopy" || id == "Panel.QuickView"
		})
}

func TestCommandPaletteOpensOnTheFocusedPlayersCommands(t *testing.T) {
	d := openPaletteOver(t, func(fsp *panel.FileSystemPanel) panel.AltPanel { return panel.NewPlayerPanel(fsp) })
	want := i18n.Msg("CommandPalette.QueryPlayer") + " "
	if got := d.query.GetText(); got != want {
		t.Fatalf("the query is %q, want %q", got, want)
	}
	checkOwnCommandsFirst(t, filteredIDs(d),
		[]string{"Panel.Player.Play", "Panel.Player.Stop", "Panel.Player.MoveItemUp", "Panel.Player.SeekBack", "Panel.Player.SeekForward"},
		func(id string) bool { return strings.HasPrefix(id, "Panel.Player.") || id == "Panel.Player" })
}

func TestCommandPaletteOpensEmptyOverTheFilePanel(t *testing.T) {
	d := openPaletteOver(t, nil)
	if got := d.query.GetText(); got != "" {
		t.Errorf("the query is %q over the file panel, want it empty", got)
	}
	if slices.Contains(filteredIDs(d), "QuickView.Wrap") {
		t.Error("a quick view command is offered with no quick view")
	}
}

// The prefix the palette opens with is the one the labels carry, in every
// language f4 ships, or the action names it in its SearchKeys.
func TestCommandPaletteContextPrefixMatchesTheLabels(t *testing.T) {
	prefixes := map[string]string{
		"QuickViewFocused": "CommandPalette.QueryQuickView",
		"PlayerFocused":    "CommandPalette.QueryPlayer",
	}
	for _, lang := range []string{"en", "ru"} {
		msgs := i18n.LoadEmbeddedLanguageMap(lang)
		for _, spec := range nativeKeyActions() {
			prefixKey, ok := prefixes[spec.condition]
			if !ok || slices.Contains(spec.searchKeys, prefixKey) {
				continue
			}
			prefix, label := msgs[prefixKey], msgs["Action."+spec.name]
			if prefix == "" || !strings.HasPrefix(label, prefix) {
				t.Errorf("%s: %s label %q does not start with %q", lang, spec.name, label, prefix)
			}
		}
	}
}
