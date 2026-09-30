//go:build !extralite

package macro

import (
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestMacroRaiseEventRunsTheDeclaredActions(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), `
		__n = 0
		Event { group = "FolderChanged"; action = function(group) __n = __n + 1; __group = group end }
		Event { group = "folderchanged"; action = function() __n = __n + 10 end }
		Event { group = "ExitFAR"; action = function() __n = __n + 100 end }
	`)
	if !Engine.RaiseEvent("FolderChanged") {
		t.Fatal("RaiseEvent started nothing for a declared group")
	}
	if !Engine.WaitIdle(5 * time.Second) {
		t.Fatal("the event actions never finished")
	}
	values := macroGlobals(t, Engine, "__n", "__group")
	if lua.LVAsNumber(values["__n"]) != 11 || lua.LVAsString(values["__group"]) != "FolderChanged" {
		t.Fatalf("n=%v group=%v, want 11 and FolderChanged", values["__n"], values["__group"])
	}
	if Engine.RaiseEvent("Nothing") {
		t.Error("RaiseEvent started something for a group nobody declared")
	}
}

func TestMacroRaiseEventSkipsWhileAMacroRuns(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), `
		Event { group = "FolderChanged"; action = function() __ran = true end }
	`)
	Engine.running.Store(true)
	if Engine.RaiseEvent("FolderChanged") {
		t.Error("RaiseEvent ran while a macro was running")
	}
	Engine.running.Store(false)
}

func TestMacroRaiseEventIsSafeWithoutAnEngine(t *testing.T) {
	if (*LuaMacroEngine)(nil).RaiseEvent("FolderChanged") {
		t.Error("a nil engine raised an event")
	}
	if (*MacroManager)(nil).RaiseEvent("FolderChanged") || (&MacroManager{}).RaiseEvent("FolderChanged") {
		t.Error("a manager without Lua macros raised an event")
	}
}
