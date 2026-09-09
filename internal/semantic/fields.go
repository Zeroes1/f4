// Package semantic holds the readers every frame needs to speak the GUI
// semantic protocol: the four that unpack a value out of an incoming action
// map, and the two that turn a row of screen cells into the run models an
// external UI draws.
//
// They live here because four layer-3 packages need them — the panel, the
// editor, the viewer and the command line — and a function three of them had
// copied is a function without a home. The package imports nothing above
// layer 0, which is what lets all four have it.
package semantic

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/unxed/f4/internal/numeric"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func String(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func Int(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		value, _ := numeric.BoundedInt64ToInt(n)
		return value
	case uint:
		value, _ := numeric.BoundedUint64ToInt(uint64(n))
		return value
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		value, _ := numeric.BoundedUint64ToInt(uint64(n))
		return value
	case uint64:
		value, _ := numeric.BoundedUint64ToInt(n)
		return value
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func BaseName(v interface{ Base(string) string }, path string) string {
	if path == "" {
		return ""
	}
	if v != nil {
		return v.Base(path)
	}
	return filepath.Base(path)
}
func Bool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if n, ok := v.(int); ok {
		return n != 0
	}
	if f, ok := v.(float64); ok {
		return f != 0
	}
	return false
}
func RunsFromCells(cells []vtui.CharInfo) []extui.RunModel {
	if len(cells) == 0 {
		return nil
	}
	var runs []extui.RunModel
	var b strings.Builder
	var attr uint64
	haveRun := false
	flush := func() {
		if !haveRun {
			return
		}
		runs = append(runs, extui.RunModel{
			Text: b.String(),
			Attr: attr,
		})
		b.Reset()
	}
	for _, cell := range cells {
		if cell.Char == vtui.WideCharFiller {
			continue
		}
		ch := CellRune(cell.Char)
		if !haveRun {
			attr = cell.Attributes
			haveRun = true
		} else if cell.Attributes != attr {
			flush()
			attr = cell.Attributes
			haveRun = true
		}
		b.WriteRune(ch)
	}
	flush()
	return runs
}

func CellRune(ch uint64) rune {
	if ch == 0 || ch > utf8.MaxRune || (ch >= 0xD800 && ch <= 0xDFFF) {
		return ' '
	}
	return rune(ch)
}
