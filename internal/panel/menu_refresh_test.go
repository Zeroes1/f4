package panel

import (
	"testing"
	"time"
)

// GetMenuBar is called several times for every key event and for every frame;
// setting the checkmarks and the letters each time took most of the time of a
// long paste into the command line (unxed/f4#2157).
func TestMenuRefreshIsDueOnlyWhenOldOrInUse(t *testing.T) {
	var m menuRefresh
	now := time.Now()
	if !m.due(false, now) {
		t.Fatal("a bar never set is not due")
	}
	m.set(now)
	if m.due(false, now.Add(100*time.Millisecond)) {
		t.Error("a bar set 100 ms ago that nobody uses is due")
	}
	if !m.due(true, now.Add(100*time.Millisecond)) {
		t.Error("a bar that is up is not due")
	}
	if !m.due(false, now.Add(menuRefreshAge+time.Millisecond)) {
		t.Error("an old bar is not due")
	}
}
