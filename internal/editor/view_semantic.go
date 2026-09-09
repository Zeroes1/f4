package editor

// The editor's side of the GUI semantic protocol: what an external UI is told
// the editor contains, and what it is allowed to ask for. Go requires these
// with EditorView; the frame-level half of the protocol is internal/app's.

import (
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func (ev *EditorView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	rows := ev.semanticRows()

	surface := extui.SurfaceModel{
		ID:           vtui.SemanticID(ev),
		Kind:         "editor",
		Title:        ev.GetTitle(),
		Path:         ev.FilePath,
		BaseName:     semantic.BaseName(ev.Vfs, ev.FilePath),
		Busy:         ev.IsBusy(),
		Dirty:        ev.Modified,
		Saving:       ev.Saving,
		WordWrap:     ev.WordWrap,
		Overtype:     ev.Overtype,
		CursorLine:   ev.CursorLine,
		CursorPos:    ev.CursorPos,
		ScrollTop:    ev.ScrollTopRow,
		ScrollLeft:   ev.ScrollLeft,
		Selection:    ev.SelActive,
		Rows:         rows,
		Autocomplete: ev.semanticAutocomplete(),
	}
	return surface.ToMap()
}

// GetText возвращает текущий текст редактора из PieceTable
func (ev *EditorView) GetText() string {
	if ev.Pt == nil {
		return ""
	}
	return ev.Pt.String()
}

// HandleSemanticAction обрабатывает нативные GUI-действия для EditorView
func (ev *EditorView) HandleSemanticAction(action map[string]any) bool {
	target := semantic.String(action["target"])
	if vtui.SemanticID(ev) != target {
		return false
	}

	switch semantic.String(action["action"]) {
	case "editor.setText":
		text := semantic.String(action["text"])
		ev.SetText(text)
		return true
	case "editor.insertText":
		text := semantic.String(action["text"])
		ev.PasteText(text)
		return true
	case "editor.deleteSelection":
		ev.DeleteSelection()
		return true
	case "editor.undo":
		ev.Undo()
		return true
	case "editor.redo":
		ev.Redo()
		return true
	case "editor.save":
		ev.SaveToFile(nil)
		return true
	case "editor.search":
		pattern := semantic.String(action["pattern"])
		caseSensitive := semantic.Bool(action["case"])
		reverse := semantic.Bool(action["reverse"])
		next := semantic.Bool(action["next"])
		ev.Search(pattern, caseSensitive, reverse, false, false, next)
		return true
	case "control.focus":
		ev.SetFocus(true)
		return true
	}
	return false
}

func (ev *EditorView) semanticRows() []extui.TextRowModel {
	if ev.Pt == nil || ev.Li == nil || ev.Engine == nil {
		return nil
	}
	ev.ensureEngineWidth()
	height := ev.Y2 - ev.Y1
	if height <= 0 {
		return nil
	}
	startLogLine, startFragIdx := ev.Engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	var rows []extui.TextRowModel
	for logIdx := startLogLine; logIdx < ev.Li.LineCount() && len(rows) < height; logIdx++ {
		frags := ev.Engine.GetFragments(logIdx)
		baseVRow := ev.Engine.GetRowOffset(logIdx)
		for fIdx, frag := range frags {
			if logIdx == startLogLine && fIdx < startFragIdx {
				continue
			}
			data, err := ev.Pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
			text := string(data)
			if err == piecetable.ErrLoading {
				text = " [ Loading... ] "
			} else if err != nil {
				text = ""
			}
			rows = append(rows, extui.TextRowModel{
				Index:       len(rows),
				VisualRow:   baseVRow + fIdx,
				LogicalLine: logIdx,
				Offset:      int64(frag.ByteOffsetStart),
				Text:        text,
			})
			if len(rows) >= height {
				break
			}
		}
	}
	return rows
}

func (ev *EditorView) semanticAutocomplete() map[string]any {
	if !ev.acEnabled || len(ev.acMatches) == 0 || ev.acCurrentIdx < 0 || ev.acCurrentIdx >= len(ev.acMatches) {
		return nil
	}
	match := ev.acMatches[ev.acCurrentIdx]
	if len(match) <= len(ev.acPrefix) {
		return nil
	}
	return map[string]any{
		"prefix": ev.acPrefix,
		"tail":   match[len(ev.acPrefix):],
		"index":  ev.acCurrentIdx,
	}
}
