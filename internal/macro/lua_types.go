package macro

import (
	"context"

	"github.com/unxed/vtinput"
)

// MacroPanelInfo is the panel state a macro can see, gathered in one shot.
// Reading it costs a round trip to the UI goroutine, so it is fetched whole
// rather than field by field.
type MacroPanelInfo struct {
	Path      string
	Current   string
	ItemCount int
	SelCount  int
	CurPos    int
	TopPos    int
	IsFolder  bool
	Empty     bool
	Left      bool
	Visible   bool
	Root      bool
	Bof       bool
	Eof       bool
	Type      int
}

// MacroHost is everything the macro engine needs from f4. Keeping it an
// interface is what makes the engine testable without a terminal, and it is
// also the seam where the "must run on the UI goroutine" rule is enforced
// exactly once instead of in every API function.
type MacroHost interface {
	CurrentArea() string
	Panel(active bool) MacroPanelInfo
	CommandLine() string
	ScreenSize() (width, height int)
	Version() string
	WindowTitle() string
	Message(title, text string)
	InjectKeys(keys []*vtinput.InputEvent)
	Log(format string, args ...any)
	RunAction(name string) bool
	CallPlugin(context.Context, string, []any) ([]any, error)
}

// LuaMacroBinding is the discoverable, immutable part of a Lua macro. It is
// used by command surfaces without exposing interpreter-owned functions.
type LuaMacroBinding struct {
	Area        string
	Key         string
	Description string
	Source      string
}

// macroAreaAliases maps f4's own area names onto Far's. f4 reports Terminal
// when the panels are hidden; Far has no such area, and its Shell macros are
// what a user expects to fire there.
var macroAreaAliases = map[string]string{
	"terminal": "shell",
}
