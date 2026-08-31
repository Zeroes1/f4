package main

import (
	"bytes"
	"testing"
)

func TestHostRenderStreamUsesOnlyHostCRLF(t *testing.T) {
	input := []byte("a\nb\r\n\x1b[8;25;80tC\r\n")
	var stream hostRenderStream
	for i := range input {
		stream.Feed(input[i : i+1])
	}
	lines := stream.Lines()
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !bytes.Equal(lines[0].Bytes, []byte("a\nb")) {
		t.Fatalf("first line = %q", lines[0].Bytes)
	}
	if !bytes.Equal(lines[1].Bytes, []byte("\x1b[8;25;80tC")) {
		t.Fatalf("second line = %q", lines[1].Bytes)
	}
	frames := stream.Frames()
	if len(frames) != 1 || frames[0].Width != 80 || frames[0].Height != 25 {
		t.Fatalf("frames = %#v", frames)
	}
}

func TestParseResizeFrame(t *testing.T) {
	w, h, n, ok := parseResizeFrame([]byte("\x1b[8;40;121tmore"))
	if !ok || w != 121 || h != 40 || n != len("\x1b[8;40;121t") {
		t.Fatalf("got %d,%d,%d,%v", w, h, n, ok)
	}
}
