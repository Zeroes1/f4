package testutil

import (
	"fmt"
	"strings"
)

// openFile is one regular file or socket the process has open.
type openFile struct {
	fd   int
	kind string // "file" or "socket"
	path string // what the descriptor names, where the OS can say
}

func (f openFile) String() string {
	if f.path == "" {
		return fmt.Sprintf("fd %d (%s)", f.fd, f.kind)
	}
	return fmt.Sprintf("fd %d (%s) %s", f.fd, f.kind, f.path)
}

// leakedFiles is what after holds that before did not: a descriptor number
// that was free before, or one that now names something of another kind.
// Descriptor numbers are reused, so a file closed and another opened on the
// same number look alike; the count per kind catches that case, and the
// listing then shows every descriptor of that kind.
func leakedFiles(before, after []openFile) []openFile {
	known := make(map[int]string, len(before))
	countBefore := map[string]int{}
	for _, f := range before {
		known[f.fd] = f.kind
		countBefore[f.kind]++
	}
	var leaked []openFile
	countAfter := map[string]int{}
	for _, f := range after {
		countAfter[f.kind]++
		if kind, ok := known[f.fd]; !ok || kind != f.kind {
			leaked = append(leaked, f)
		}
	}
	if len(leaked) > 0 {
		return leaked
	}
	for kind, n := range countAfter {
		if n > countBefore[kind] {
			for _, f := range after {
				if f.kind == kind {
					leaked = append(leaked, f)
				}
			}
		}
	}
	return leaked
}

// describeFiles lists files one per line for a failure message.
func describeFiles(files []openFile) string {
	var b strings.Builder
	for _, f := range files {
		b.WriteString("  ")
		b.WriteString(f.String())
		b.WriteByte('\n')
	}
	return b.String()
}
