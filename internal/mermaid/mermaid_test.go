package mermaid

import (
	"strings"
	"testing"
)

func TestFlowchart(t *testing.T) {
	got, ok := Flowchart("graph TD\n  A[Start] --> B{Ok?}\n  B -->|yes| C(Do it)\n  B -- no --> D((Stop))\n  E")
	want := strings.Join([]string{
		"[Start] ──▶ {Ok?}",
		"{Ok?} ── yes ─▶ (Do it)",
		"{Ok?} ── no ─▶ ((Stop))",
		"[E]",
	}, "\n")
	if !ok || got != want {
		t.Errorf("basic:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	got, ok = Flowchart("flowchart LR\nA-->B-->C")
	if !ok || got != "[A] ──▶ [B]\n[B] ──▶ [C]" {
		t.Errorf("chain: %q %v", got, ok)
	}
	got, ok = Flowchart("flowchart TD\nA[\"Quoted [text]\"] -.-> B([Round])\nB ==> C[[Sub]]\nC --- D\nD <--> A\nA -. why .-> C")
	want = strings.Join([]string{
		"[Quoted [text]] ┄┄▶ ([Round])",
		"([Round]) ━━▶ [[Sub]]",
		"[[Sub]] ─── [D]",
		"[D] ◀──▶ [Quoted [text]]",
		"[Quoted [text]] ┄┄ why ┄▶ [[Sub]]",
	}, "\n")
	if !ok || got != want {
		t.Errorf("styles:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	got, ok = Flowchart("graph TD\nA[One<br/>two] --> B")
	if !ok || got != "[One two] ──▶ [B]" {
		t.Errorf("br: %q %v", got, ok)
	}
}

func TestFlowchartRefusals(t *testing.T) {
	for name, src := range map[string]string{
		"sequence":  "sequenceDiagram\nA->>B: hi",
		"empty":     "",
		"header":    "graph TD",
		"subgraph":  "graph TD\nsubgraph X\nA-->B\nend",
		"ampersand": "graph TD\nA & B --> C",
		"class":     "graph TD\nA:::red --> B",
		"unclosed":  "graph TD\nA[oops --> B",
		"garbage":   "graph TD\nA --> ",
		"nodeless":  "graph TD\n--> B",
		"too long":  "graph TD\n" + strings.Repeat("A-->B\n", MaxLines),
	} {
		if got, ok := Flowchart(src); ok {
			t.Errorf("%s: converted to %q", name, got)
		}
	}
}
