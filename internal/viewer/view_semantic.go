package viewer

import (
	"fmt"
	"unicode/utf8"

	runewidth "github.com/mattn/go-runewidth"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

// HandleSemanticAction runs a native GUI action addressed at this viewer.
func (vv *ViewerView) HandleSemanticAction(action map[string]any) bool {
	target := semantic.String(action["target"])
	if vtui.SemanticID(vv) != target {
		return false
	}

	switch semantic.String(action["action"]) {
	case "viewer.scroll":
		offset := int64(semantic.Int(action["offset"]))
		if offset < 0 {
			offset = 0
		}
		if offset > vv.Backend.Size() {
			offset = vv.Backend.Size()
		}
		if vv.HexMode {
			offset &= ^int64(0xF)
		} else {
			offset = vv.Backend.FindLineStart(offset)
		}
		vv.TopOffset = offset
		return true
	case "control.focus":
		vv.SetFocus(true)
		return true
	}
	return false
}
func (vv *ViewerView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	rows := vv.semanticRows()
	mode := "text"
	if vv.HexMode {
		mode = "hex"
	}

	surface := extui.SurfaceModel{
		ID:        vtui.SemanticID(vv),
		Kind:      "viewer",
		Title:     vv.GetTitle(),
		Path:      vv.Path,
		BaseName:  semantic.BaseName(vv.VFS, vv.Path),
		Mode:      mode,
		HexMode:   vv.HexMode,
		WrapMode:  vv.WrapMode,
		Busy:      vv.Busy,
		TopOffset: vv.TopOffset,
		Size:      vv.Backend.Size(),
		Rows:      rows,
	}
	return surface.ToMap()
}
func (vv *ViewerView) semanticRows() []extui.TextRowModel {
	if vv.Backend == nil {
		return nil
	}
	width := vv.X2 - vv.X1 + 1
	if vv.ScrollBar != nil {
		width--
	}
	contentHeight := vv.Y2 - vv.Y1
	if width <= 0 || contentHeight <= 0 {
		return nil
	}
	if vv.Busy {
		return []extui.TextRowModel{{Index: 0, Text: " [ Loading... ] "}}
	}
	var rows []extui.TextRowModel
	if vv.HexMode {
		currOffset := vv.TopOffset &^ 0xF
		for y := 0; y < contentHeight && currOffset < vv.Backend.Size(); y++ {
			data, err := vv.Backend.ReadAt(currOffset, 16)
			if err != nil && err != piecetable.ErrLoading {
				break
			}
			rows = append(rows, extui.TextRowModel{
				Index:  y,
				Offset: currOffset,
				Text:   semanticHexLine(currOffset, data),
			})
			currOffset += 16
		}
		return rows
	}

	currOffset := vv.TopOffset
	for y := 0; y < contentHeight; y++ {
		if currOffset >= vv.Backend.Size() {
			break
		}
		data, err := vv.Backend.ReadAt(currOffset, width*4)
		if err == piecetable.ErrLoading {
			rows = append(rows, extui.TextRowModel{Index: y, Offset: currOffset, Text: " [ Loading... ] "})
			break
		}
		if err != nil || len(data) == 0 {
			break
		}
		lineLen, textLen := semanticViewerLineLen(data, width, vv.WrapMode)
		rows = append(rows, extui.TextRowModel{Index: y, Offset: currOffset, Text: string(data[:textLen])})
		if lineLen <= 0 {
			break
		}
		currOffset += int64(lineLen)
	}
	return rows
}

func semanticHexLine(offset int64, data []byte) string {
	hexPart := ""
	asciiPart := ""
	for i := 0; i < 16; i++ {
		if i < len(data) {
			hexPart += fmt.Sprintf("%02X ", data[i])
			r := rune(data[i])
			if r < 32 || r > 126 {
				r = '.'
			}
			asciiPart += string(r)
		} else {
			hexPart += "   "
		}
		if i == 7 {
			hexPart += " "
		}
	}
	return fmt.Sprintf("%010X: %s | %s", offset, hexPart, asciiPart)
}
func semanticViewerLineLen(data []byte, width int, wrap bool) (lineLen int, textLen int) {
	visualWidth := 0
	tabSize := 8
	if config.App.EditorTabSize > 0 {
		tabSize = config.App.EditorTabSize
	}
	for lineLen < len(data) {
		r, size := utf8.DecodeRune(data[lineLen:])
		if r == '\n' {
			lineLen += size
			return lineLen, textLen
		}
		if r == '\r' {
			lineLen += size
			continue
		}
		var rw int
		if r == '\t' {
			rw = tabSize - (visualWidth % tabSize)
		} else {
			rw = runewidth.RuneWidth(r)
			if rw <= 0 {
				rw = 1
			}
		}
		if wrap && visualWidth+rw > width {
			return lineLen, textLen
		}
		visualWidth += rw
		lineLen += size
		textLen = lineLen
		if !wrap && visualWidth >= width {
			return lineLen, textLen
		}
	}
	return lineLen, textLen
}
