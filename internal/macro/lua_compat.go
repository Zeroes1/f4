//go:build !extralite

package macro

// Load-time compatibility with the Far 3 / luafar macro vocabulary (f4#1686,
// step 7). A macro file is one chunk, so a single missing global at its top -
// `local F = far.Flags`, `win.Uuid(far.Guids.X)` - aborts the whole file and
// every Macro{} in it is lost. What is here makes such files load and run with
// the answers of a program that has none of Far's own windows: no plugin
// panels, no Far dialogs or menus to be inside of, flags that are all zero. A
// script that tests for them takes its "not in that window" branch, which is
// the truth in f4.
//
// The list of what to cover was taken from a corpus of Far 3's own macros
// (FarManager's extra/Addons/Macros, 49 scripts): the names are the ones the
// most scripts use.

import (
	"crypto/sha256"
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// installCompat adds the tables the Far 3 macros reach for at load time.
func (e *LuaMacroEngine) installCompat(L *lua.LState) {
	// Object is the object the macro runs in: the panel in the panels' areas.
	L.SetGlobal("Object", dynamicTable(L, func(key string) lua.LValue {
		switch strings.ToLower(key) {
		case "bof":
			return lua.LBool(e.host.Panel(true).Bof)
		case "eof":
			return lua.LBool(e.host.Panel(true).Eof)
		case "empty":
			return lua.LBool(e.host.Panel(true).Empty)
		case "selected":
			return lua.LBool(e.host.Panel(true).SelCount > 0)
		}
		return lua.LNil
	}))

	// Menu, Dlg: Far's own menus and dialogs, none of which f4 has to be in.
	L.SetGlobal("Menu", dynamicTable(L, func(key string) lua.LValue {
		switch strings.ToLower(key) {
		case "value", "id":
			return lua.LString("")
		}
		return lua.LNil
	}))
	L.SetGlobal("Dlg", dynamicTable(L, func(key string) lua.LValue {
		if strings.EqualFold(key, "id") {
			return lua.LString("")
		}
		return lua.LNil
	}))

	// Editor, Viewer state words and the mouse: zero, as when nothing is open.
	L.SetGlobal("Editor", dynamicTable(L, func(key string) lua.LValue {
		switch strings.ToLower(key) {
		case "state", "pos", "sel":
			return lua.LNumber(0)
		case "selvalue":
			return lua.LString("")
		}
		return lua.LNil
	}))
	L.SetGlobal("Viewer", dynamicTable(L, func(key string) lua.LValue {
		if strings.EqualFold(key, "state") {
			return lua.LNumber(0)
		}
		return lua.LNil
	}))
	L.SetGlobal("Mouse", dynamicTable(L, func(key string) lua.LValue {
		switch strings.ToLower(key) {
		case "x", "y":
			return lua.LNumber(0)
		}
		return lua.LNil
	}))

	// Far's macro environment has the bit operations as plain globals too.
	if bits, ok := L.GetGlobal("bit").(*lua.LTable); ok {
		for _, name := range []string{"band", "bor", "bxor", "bnot", "lshift", "rshift"} {
			L.SetGlobal(name, bits.RawGetString(name))
		}
	}

	// win: only what a script needs at load time. Uuid turns a GUID string
	// into Far's binary form; here the string is its own form.
	win := L.NewTable()
	L.SetFuncs(win, map[string]lua.LGFunction{
		"Uuid": func(L *lua.LState) int {
			L.Push(lua.LString(L.CheckString(1)))
			return 1
		},
	})
	L.SetGlobal("win", win)
}

// farConstants adds far.Flags and far.Guids to the far namespace: every flag
// is zero (so a test of it is false), every GUID a stable string of its own
// name (so it never equals the id of a window f4 has).
func farConstants(L *lua.LState, namespace *lua.LTable) {
	namespace.RawSetString("Flags", dynamicTable(L, func(string) lua.LValue { return lua.LNumber(0) }))
	namespace.RawSetString("Guids", dynamicTable(L, func(key string) lua.LValue {
		sum := sha256.Sum256([]byte(key))
		return lua.LString(fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16]))
	}))
}
