//go:build !extralite

package macro

// far.Timer (f4#1686, step 7): a function the interpreter calls now and then.
//
//	t = far.Timer(500, function(t) ... end)
//	t.Enabled = false   -- pause
//	t.Interval = 1000   -- change the period, in milliseconds
//	t:Close()           -- forget it
//
// A tick runs on the interpreter like any macro, and is skipped, not queued,
// while a macro or another tick is running: a timer must not pile work up
// behind a slow script. Ticks do not inject keys.

import (
	"sync"
	"sync/atomic"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// minTimerInterval keeps a script from spinning the interpreter.
const minTimerInterval = 10 * time.Millisecond

type luaTimer struct {
	engine   *LuaMacroEngine
	fn       *lua.LFunction
	table    *lua.LTable
	interval atomic.Int64 // nanoseconds
	enabled  atomic.Bool
	stop     chan struct{}
	once     sync.Once
}

func (t *luaTimer) close() { t.once.Do(func() { close(t.stop) }) }

func (t *luaTimer) setInterval(ms float64) {
	d := time.Duration(ms * float64(time.Millisecond))
	if d < minTimerInterval {
		d = minTimerInterval
	}
	t.interval.Store(int64(d))
}

func (t *luaTimer) loop() {
	for {
		select {
		case <-t.stop:
			return
		case <-time.After(time.Duration(t.interval.Load())):
		}
		if t.enabled.Load() {
			t.tick()
		}
	}
}

// tick calls the timer's function on the interpreter, unless it is busy.
func (t *luaTimer) tick() {
	e := t.engine
	if !e.running.CompareAndSwap(false, true) {
		return
	}
	defer e.running.Store(false)
	err := e.rt.Do(func(L *lua.LState) error {
		e.pendingKeys = nil
		L.Push(t.fn)
		L.Push(t.table)
		return L.PCall(1, 0, nil)
	})
	e.pendingKeys = nil
	if err != nil {
		e.host.Log("MACRO: far.Timer callback: %v", err)
	}
}

// luaFarTimer is far.Timer(interval, callback).
func (e *LuaMacroEngine) luaFarTimer(L *lua.LState) int {
	interval := float64(L.CheckNumber(1))
	fn := L.CheckFunction(2)
	t := &luaTimer{engine: e, fn: fn, stop: make(chan struct{})}
	t.setInterval(interval)
	t.enabled.Store(true)

	t.table = L.NewTable()
	meta := L.NewTable()
	meta.RawSetString("__index", L.NewFunction(func(L *lua.LState) int {
		switch L.CheckString(2) {
		case "Enabled":
			L.Push(lua.LBool(t.enabled.Load()))
		case "Interval":
			L.Push(lua.LNumber(float64(t.interval.Load()) / float64(time.Millisecond)))
		case "Close":
			L.Push(L.NewFunction(func(L *lua.LState) int {
				t.close()
				return 0
			}))
		default:
			L.Push(lua.LNil)
		}
		return 1
	}))
	meta.RawSetString("__newindex", L.NewFunction(func(L *lua.LState) int {
		switch L.CheckString(2) {
		case "Enabled":
			t.enabled.Store(lua.LVAsBool(L.Get(3)))
		case "Interval":
			t.setInterval(float64(L.CheckNumber(3)))
		}
		return 0
	}))
	L.SetMetatable(t.table, meta)

	e.mu.Lock()
	e.timers = append(e.timers, t)
	e.mu.Unlock()
	go t.loop()
	L.Push(t.table)
	return 1
}

// stopTimers ends every timer; the engine is going away.
func (e *LuaMacroEngine) stopTimers() {
	e.mu.Lock()
	timers := e.timers
	e.timers = nil
	e.mu.Unlock()
	for _, t := range timers {
		t.close()
	}
}
