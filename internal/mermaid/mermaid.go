// Package mermaid turns a Mermaid flowchart into plain Unicode text for the
// terminal Markdown viewer (f4#1664). A terminal cannot draw the diagram, so
// the text form lists it: one line per connection, nodes in their own
// brackets, arrows drawn with box characters, edge labels inline. Only the
// flowchart subset that this reader fully understands is converted; anything
// else (subgraphs, other diagram types, unknown syntax, oversized input) is
// refused so the caller keeps the diagram's source, which is exact.
package mermaid

import (
	"regexp"
	"strings"
)

// Limits on what one diagram may hold.
const (
	MaxLines = 400
	maxEdges = 600
)

type node struct {
	open, close, text string
}

type edge struct {
	from, to string
	label    string
	style    byte // '-', '=' or '.'
	arrow    bool
	both     bool
}

// shapes lists node delimiters, longest opening first.
var shapes = []struct{ open, close string }{
	{"((", "))"}, {"([", "])"}, {"[[", "]]"}, {"[(", ")]"}, {"{{", "}}"},
	{"[", "]"}, {"(", ")"}, {"{", "}"}, {">", "]"},
}

var (
	header   = regexp.MustCompile(`^(?:flowchart|graph)(?:\s+(TD|TB|BT|LR|RL))?$`)
	nodeID   = regexp.MustCompile(`^[A-Za-z0-9_]+`)
	labelled = regexp.MustCompile(`^\s*(--|==|-\.)\s+([^-=.|<>][^|]*?)\s+(-->|---|==>|===|\.->|-\.-)`)
	plainOp  = regexp.MustCompile(`^\s*(<-->|-{2,}>|-{3,}|={2,}>|={3,}|-\.+->|-\.+-)(?:\s*\|([^|]*)\|)?`)
	ignored  = regexp.MustCompile(`^(?:style|classDef|class|linkStyle|click|direction)\b`)
	brTag    = regexp.MustCompile(`(?i)<br\s*/?>`)
)

// Flowchart converts the body of a ```mermaid block. ok is false when the
// diagram is not a flowchart this package fully understands.
func Flowchart(source string) (text string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) > MaxLines {
		return "", false
	}
	nodes := make(map[string]*node)
	var order []string
	var edges []edge
	sawHeader := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		line = strings.TrimSuffix(line, ";")
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !sawHeader {
			if !header.MatchString(line) {
				return "", false
			}
			sawHeader = true
			continue
		}
		if ignored.MatchString(line) {
			continue
		}
		if !parseStatement(line, nodes, &order, &edges) || len(edges) > maxEdges {
			return "", false
		}
	}
	if !sawHeader || len(order) == 0 {
		return "", false
	}
	var b strings.Builder
	used := make(map[string]bool)
	for _, e := range edges {
		used[e.from], used[e.to] = true, true
		b.WriteString(display(nodes[e.from], e.from))
		b.WriteString(" " + arrowText(e) + " ")
		b.WriteString(display(nodes[e.to], e.to))
		b.WriteByte('\n')
	}
	for _, id := range order {
		if !used[id] {
			b.WriteString(display(nodes[id], id))
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n"), true
}

func display(n *node, id string) string {
	if n == nil || n.open == "" {
		return "[" + id + "]"
	}
	return n.open + n.text + n.close
}

func arrowText(e edge) string {
	line, head := "─", "▶"
	switch e.style {
	case '=':
		line = "━"
	case '.':
		line = "┄"
	}
	tail := line
	if e.arrow {
		tail = head
	}
	prefix := ""
	if e.both {
		prefix = "◀"
	}
	if e.label == "" {
		return prefix + line + line + tail
	}
	return prefix + line + line + " " + e.label + " " + line + tail
}

// parseStatement reads NODE (EDGE NODE)* from line.
func parseStatement(line string, nodes map[string]*node, order *[]string, edges *[]edge) bool {
	rest := line
	prev, rest, ok := parseNode(rest, nodes, order)
	if !ok {
		return false
	}
	for strings.TrimSpace(rest) != "" {
		e, after, ok := parseEdge(rest)
		if !ok {
			return false
		}
		var next string
		next, rest, ok = parseNode(after, nodes, order)
		if !ok {
			return false
		}
		e.from, e.to = prev, next
		*edges = append(*edges, e)
		prev = next
	}
	return true
}

func parseNode(s string, nodes map[string]*node, order *[]string) (id, rest string, ok bool) {
	s = strings.TrimLeft(s, " \t")
	id = nodeID.FindString(s)
	if id == "" {
		return "", "", false
	}
	rest = s[len(id):]
	n, known := nodes[id]
	if !known {
		n = &node{}
		nodes[id] = n
		*order = append(*order, id)
	}
	for _, sh := range shapes {
		if !strings.HasPrefix(rest, sh.open) {
			continue
		}
		body := rest[len(sh.open):]
		var text, tail string
		if strings.HasPrefix(body, `"`) {
			end := strings.Index(body[1:], `"`)
			if end < 0 || !strings.HasPrefix(body[end+2:], sh.close) {
				return "", "", false
			}
			text, tail = body[1:end+1], body[end+2+len(sh.close):]
		} else {
			end := strings.Index(body, sh.close)
			if end < 0 {
				return "", "", false
			}
			text, tail = body[:end], body[end+len(sh.close):]
		}
		text = strings.TrimSpace(brTag.ReplaceAllString(text, " "))
		if strings.ContainsAny(text, "\n\r") {
			return "", "", false
		}
		if n.open == "" || text != "" {
			n.open, n.close, n.text = sh.open, sh.close, text
		}
		// A shape may be followed by a class shorthand or nothing else.
		if strings.HasPrefix(tail, ":::") {
			return "", "", false
		}
		return id, tail, true
	}
	if strings.HasPrefix(rest, ":::") || strings.HasPrefix(rest, "@") || strings.HasPrefix(strings.TrimSpace(rest), "&") {
		return "", "", false
	}
	return id, rest, true
}

func parseEdge(s string) (edge, string, bool) {
	if m := labelled.FindStringSubmatch(s); m != nil {
		e := edge{label: strings.TrimSpace(m[2]), style: m[1][0]}
		if m[1] == "-." {
			e.style = '.'
		}
		e.arrow = strings.HasSuffix(m[3], ">")
		return e, s[len(m[0]):], true
	}
	if m := plainOp.FindStringSubmatch(s); m != nil {
		op := m[1]
		e := edge{label: strings.TrimSpace(m[2]), style: op[0], arrow: strings.HasSuffix(op, ">"), both: op == "<-->"}
		switch {
		case strings.Contains(op, "."):
			e.style = '.'
		case op[0] == '<':
			e.style = '-'
		}
		return e, s[len(m[0]):], true
	}
	return edge{}, "", false
}
