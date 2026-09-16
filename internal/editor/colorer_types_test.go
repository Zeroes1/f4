package editor

import (
	"context"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func TestColorerTypeRowAndHotkey(t *testing.T) {
	if got := colorerTypeRow(colorerTypeEntry{description: "C & C++", hotkey: "C"}); got != "&C C && C++" {
		t.Errorf("row %q", got)
	}
	if got := colorerTypeRow(colorerTypeEntry{description: "JSON"}); got != "  JSON" {
		t.Errorf("row %q", got)
	}
	for in, want := range map[string]string{"j": "J", " 7x": "7", "-": "", "": "", "ж": "Ж"} {
		if got := colorerHotkey(in); got != want {
			t.Errorf("colorerHotkey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestColorerRegionAt(t *testing.T) {
	spans := []colorerRegionSpan{{0, -1}, {4, 9}, {5, 8}}
	if got, ok := colorerRegionAt(spans, 6, 12); !ok || got != (colorerRegionSpan{5, 8}) {
		t.Errorf("inner = %+v, %v", got, ok)
	}
	if got, ok := colorerRegionAt(spans, 9, 12); !ok || got != (colorerRegionSpan{4, 9}) {
		t.Errorf("at the end of a region = %+v, %v", got, ok)
	}
	if got, ok := colorerRegionAt(spans, 11, 12); !ok || got != (colorerRegionSpan{0, 12}) {
		t.Errorf("line-wide = %+v, %v", got, ok)
	}
}

func TestColorerProfileRoundTrip(t *testing.T) {
	config.GetF4ConfigDir() // resolve once, so the cache is what is swapped
	old := config.CachedF4ConfigDir
	dir := t.TempDir()
	config.CachedF4ConfigDir = dir
	t.Cleanup(func() { config.CachedF4ConfigDir = old })

	profile := map[string]map[string]string{"json": {"favorite": "true", "hotkey": "J"}, "c": {"favorite": "false"}}
	if err := saveColorerProfile(profile); err != nil {
		t.Fatal(err)
	}
	got := loadColorerProfile()
	if got["json"]["favorite"] != "true" || got["json"]["hotkey"] != "J" || got["c"]["favorite"] != "false" {
		t.Errorf("profile read back as %v", got)
	}
}

// Picking a type from the list rehighlights the file as that type: the
// worker resets its session with SetFileType.
func TestColorerSetFileType_RehighlightsAsTheType(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)
	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRC: user}

	session, err := acquireCancelableColorerSession(context.Background(), src)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if err := session.SetHRD("rgb", "default"); err != nil {
		t.Fatalf("SetHRD: %v", err)
	}
	// "a.txt" matches no type of this catalog; the parser gets none of the
	// pair test's regions.
	session.SelectType("a.txt", "")

	ev := NewEditorView(piecetable.New([]byte("{\n}\n")), nil, "a.txt")
	defer ev.Close()
	ch := &ColorerHighlighter{owner: ev, postTask: vtui.FrameManager.PostTask, redraw: func() {}, colorerSrc: src, filename: "a.txt"}
	ch.SetLineSource(ev.lineTextForHighlight)
	ev.Highlighter = ch
	ch.session = session
	ch.startWorker(session)

	ch.HighlightLine(0, "{", 0)
	pumpUntil(t, "line 0 parsed", func() bool { _, ok := ch.cachedPairs(0); return ok && !ch.pending })
	if pairs, _ := ch.cachedPairs(0); len(pairs) != 0 {
		t.Fatalf("pairs before choosing the type: %+v", pairs)
	}

	ch.setFileType("pairtest")
	ch.HighlightLine(0, "{", 0)
	pumpUntil(t, "line 0 parsed as pairtest", func() bool { _, ok := ch.cachedPairs(0); return ok && !ch.pending })
	if pairs, _ := ch.cachedPairs(0); len(pairs) != 1 || !pairs[0].Opens {
		t.Fatalf("pairs as pairtest: %+v", pairs)
	}

}
