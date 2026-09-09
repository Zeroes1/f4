package editor

import (
	"errors"
	"testing"

	"github.com/unxed/f4/internal/piecetable"
)

func selectEditorBytes(ev *EditorView, end int) {
	ev.SelActive = true
	ev.SelAnchorOffset = 0
	ev.CursorLine = ev.Li.GetLineAtOffset(end)
	ev.CursorPos = end - ev.Li.GetLineOffset(ev.CursorLine)
}

func TestEditorBase64EncodeAndDecodeSelection(t *testing.T) {
	ev := NewEditorView(piecetable.New([]byte("hello world")), nil, "test.txt")
	defer ev.Close()

	selectEditorBytes(ev, len("hello world"))
	if err := ev.TransformBase64Selection(true); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := ev.GetText(); got != "aGVsbG8gd29ybGQ=" {
		t.Fatalf("encoded text = %q", got)
	}

	selectEditorBytes(ev, len("aGVsbG8gd29ybGQ="))
	if err := ev.TransformBase64Selection(false); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := ev.GetText(); got != "hello world" {
		t.Fatalf("decoded text = %q", got)
	}
}

func TestEditorBase64DecodeAcceptsWhitespaceAndRejectsInvalidInput(t *testing.T) {
	ev := NewEditorView(piecetable.New([]byte("aGVs\n bG8=")), nil, "test.txt")
	defer ev.Close()

	selectEditorBytes(ev, len("aGVs\n bG8="))
	if err := ev.TransformBase64Selection(false); err != nil {
		t.Fatalf("decode wrapped Base64: %v", err)
	}
	if got := ev.GetText(); got != "hello" {
		t.Fatalf("wrapped decode = %q", got)
	}

	ev.SetText("not Base64!")
	selectEditorBytes(ev, len("not Base64!"))
	before := ev.GetText()
	if err := ev.TransformBase64Selection(false); err == nil {
		t.Fatal("invalid Base64 was accepted")
	}
	if got := ev.GetText(); got != before {
		t.Fatalf("invalid decode changed text to %q", got)
	}
}

func TestEditorBase64DecodeSelectionWithTrailingNewline(t *testing.T) {
	const input = "aGVsbG8gd29ybGQ=\n"
	ev := NewEditorView(piecetable.New([]byte(input)), nil, "test.txt")
	defer ev.Close()

	selectEditorBytes(ev, len(input))
	if err := ev.TransformBase64Selection(false); err != nil {
		t.Fatalf("decode trailing-newline selection: %v", err)
	}
	if got := ev.GetText(); got != "hello world" {
		t.Fatalf("trailing-newline decode = %q", got)
	}
}

func TestEditorBase64RejectsRectangularSelection(t *testing.T) {
	ev := NewEditorView(piecetable.New([]byte("hello")), nil, "test.txt")
	defer ev.Close()
	ev.RectSelActive = true

	err := ev.TransformBase64Selection(true)
	if err == nil || errors.Is(err, errBase64NoSelection) {
		t.Fatalf("rectangular selection error = %v", err)
	}
}
