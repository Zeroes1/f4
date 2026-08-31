package main

import (
	"bytes"
	"testing"
)

func TestLogicalLineStreamUsesOnlyExplicitLF(t *testing.T) {
	want := []byte("ascii\r\nlong: " + string(bytes.Repeat([]byte{'C'}, 257)) + "\nrepeat\nrepeat\n")
	var stream logicalLineStream
	for i := 0; i < len(want); i++ {
		stream.Feed(want[i : i+1])
	}
	lines := stream.Lines()
	if len(lines) != 4 {
		t.Fatalf("got %d complete lines, want 4", len(lines))
	}
	if got := stream.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("round trip changed bytes: got %d want %d", len(got), len(want))
	}
	if got := len(lines[1].Bytes); got != 263 {
		t.Fatalf("long logical line length = %d, want 263", got)
	}
	if !bytes.Equal(lines[0].Terminator, []byte{'\r', '\n'}) || !bytes.Equal(lines[1].Terminator, []byte{'\n'}) {
		t.Fatal("line terminators were not preserved")
	}
}

func TestReflowDoesNotChangeLogicalLines(t *testing.T) {
	line := logicalLine{Bytes: bytes.Repeat([]byte{'C'}, 257), Terminator: []byte{'\n'}}
	for _, width := range []int{1, 79, 80, 81, 161} {
		rows := reflowLogicalLines([]logicalLine{line}, width)
		var joined []byte
		for _, row := range rows {
			joined = append(joined, row...)
		}
		if !bytes.Equal(joined, line.Bytes) {
			t.Fatalf("width %d changed logical bytes", width)
		}
	}
}
