package terminal

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func (tv *TerminalView) SemanticModel(ctx *vtui.SemanticContext) *extui.TerminalModel {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	buf := tv.Lines
	if tv.UseAltScreen {
		buf = tv.AltLines
	}
	offset := 0
	if !tv.UseAltScreen {
		lowestRow := 0
		for y := tv.Height - 1; y >= 0; y-- {
			if tv.rowHasText(y) {
				lowestRow = y
				break
			}
		}
		if tv.CursorY > lowestRow {
			lowestRow = tv.CursorY
		}
		if lowestRow < tv.Height-1 {
			offset = (tv.Height - 1) - lowestRow
		}
	}

	var rows []extui.TextRowModel
	for y := 0; y < tv.Height && y < len(buf); y++ {
		drawY := y + offset
		if tv.UseAltScreen {
			drawY = y
		}
		if drawY < 0 || drawY >= tv.Height {
			continue
		}
		rows = append(rows, extui.TextRowModel{
			Index: drawY,
			Runs:  semantic.RunsFromCells(buf[y]),
		})
	}

	return &extui.TerminalModel{
		ID:        vtui.SemanticID(tv),
		Title:     tv.Title,
		Visible:   tv.IsVisible(),
		Focused:   tv.IsFocused(),
		AltScreen: tv.UseAltScreen,
		Busy:      tv.Muted,
		CursorX:   tv.CursorX,
		CursorY:   tv.CursorY + offset,
		Rows:      rows,
	}
}
