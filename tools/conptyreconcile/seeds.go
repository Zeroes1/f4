package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// seedWorkload is generated deterministically and contains no expected screen
// model. It is only the byte payload authored by the child process.
func seedWorkload(seed uint64, width int) []byte {
	if width < 1 {
		width = 80
	}
	rng := rand.New(rand.NewSource(int64(seed)))
	begin := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_BEGIN__", seed)
	end := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_END__", seed)
	var b strings.Builder
	b.WriteString(begin)
	b.WriteString("\r\n")
	b.WriteString(fmt.Sprintf("ascii: seed-%016x\r\n", seed))
	b.WriteString("edge: ")
	b.WriteString(strings.Repeat("X", maxInt(0, width-6)))
	b.WriteString("\r\n")
	b.WriteString("repeat: SAME\r\nrepeat: SAME\r\nrepeat: SAME\r\n")
	b.WriteString("unicode: 漢字 e\u0301 😀 👩‍💻 אבג العربية\r\n")
	b.WriteString("long: ")
	b.WriteString(strings.Repeat("C", width+177+rng.Intn(83)))
	b.WriteString("\r\n")
	b.WriteString(end)
	b.WriteString("\r\n")
	return []byte(b.String())
}
