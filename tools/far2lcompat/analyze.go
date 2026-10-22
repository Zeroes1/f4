package main

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The sequences the kit looks for in what a manager writes to its terminal:
// private modes, the kitty keyboard flags, the far2l activation and OSC 52.
var sequenceRE = regexp.MustCompile(
	`\x1b\[\?[0-9;]+[hl]` + // DEC private modes: 1049, 2004, 9001, 1000-1006, 1004
		`|\x1b\[[>=<][0-9;]*u|\x1b\[\?u` + // kitty keyboard protocol
		`|\x1b_far2l[01]\x1b\\` + // far2l extensions on and off
		`|\x1b\[5n|\x1b\[6n|\x1b\[c` + // status reports the terminal has to answer
		`|\x1b\]52;[^\x07\x1b]*(?:\x07|\x1b\\)`) // OSC 52

// exitLimit is how long a manager may take to exit after the quit keys: the
// 5 seconds of P-06.
var exitLimit = 5 * time.Second

// settleQuiet and settleLimit: the quit keys are sent when the program has written
// nothing for settleQuiet, and in any case after settleLimit more.
const (
	settleQuiet = 700 * time.Millisecond
	settleLimit = 20 * time.Second
)

// Options say how the kit plays the terminal.
type Options struct {
	Far2l     bool // acknowledge ESC _ far2l1 with ESC _ far2lok, as far2l does
	MinOutput int  // the quit keys wait until the program has written this many bytes (a drawn screen)
	SilentExt bool // acknowledge it and answer nothing else; checks P-06 (implies Far2l)
}

// Recording is what Record returns.
type Recording struct {
	Data     []byte
	Mark     int           // length of Data when the quit keys were sent
	ExitTook time.Duration // from the quit keys to the end of the program
	Killed   bool          // the program was still running after exitLimit
}

// Sequence is one control sequence found in the recording.
type Sequence struct {
	Offset int
	Text   string
	Exit   bool // after the quit keys were sent
}

// Find returns the sequences of rec in order; startLen is the length of the
// recording when the quit keys were sent.
func Find(rec []byte, startLen int) []Sequence {
	var out []Sequence
	for _, m := range sequenceRE.FindAllIndex(rec, -1) {
		out = append(out, Sequence{Offset: m[0], Text: string(rec[m[0]:m[1]]), Exit: m[0] >= startLen})
	}
	return out
}

// Result is the verdict on one requirement of docs/FAR2L_COMPAT.md.
type Result struct {
	ID     string
	Result string // pass, fail, partial, unknown
	Note   string
}

func has(seqs []Sequence, text string, exit bool) bool {
	for _, s := range seqs {
		if s.Text == text && s.Exit == exit {
			return true
		}
	}
	return false
}

func hasPrefix(seqs []Sequence, prefix string, exit bool) bool {
	for _, s := range seqs {
		if strings.HasPrefix(s.Text, prefix) && s.Exit == exit {
			return true
		}
	}
	return false
}

func show(s string) string {
	return strings.NewReplacer("\x1b", "^[", "\\", `\`).Replace(s)
}

// Check applies the recording checks of section 4: P-01, P-02 (a), P-03, P-04
// and the activation half of P-05.
func Check(seqs []Sequence) []Result {
	var out []Result
	add := func(id string, ok bool, pass, fail string) {
		if ok {
			out = append(out, Result{id, "pass", pass})
		} else {
			out = append(out, Result{id, "fail", fail})
		}
	}
	add("P-01", has(seqs, "\x1b[?2004h", false) && has(seqs, "\x1b[?2004l", true),
		"?2004h at the start, ?2004l at the exit", "no ?2004h at the start or no ?2004l at the exit")
	kittyOn := hasPrefix(seqs, "\x1b[>", false) || hasPrefix(seqs, "\x1b[=", false)
	kittyOff := has(seqs, "\x1b[<u", true) || has(seqs, "\x1b[<1u", true) || hasPrefix(seqs, "\x1b[=0;1u", true)
	add("P-03", kittyOn && kittyOff, "kitty flags asked for at the start and given back at the exit",
		"kitty keyboard flags are not asked for at the start or not restored at the exit")
	add("P-04", has(seqs, "\x1b[?9001h", false) && has(seqs, "\x1b[?9001l", true),
		"?9001h at the start, ?9001l at the exit", "no ?9001h at the start or no ?9001l at the exit")
	// P-02 (a): every mode that was switched on is switched off at the exit.
	var left []string
	for _, s := range seqs {
		if s.Exit || !strings.HasPrefix(s.Text, "\x1b[?") || !strings.HasSuffix(s.Text, "h") {
			continue
		}
		off := strings.TrimSuffix(s.Text, "h") + "l"
		if !has(seqs, off, true) && !has(seqs, off, false) {
			left = append(left, show(s.Text))
		}
	}
	if len(left) == 0 {
		out = append(out, Result{"P-02", "pass", "(a) every mode switched on is switched off (b, c are not checked)"})
	} else {
		out = append(out, Result{"P-02", "fail", "(a) not switched off: " + strings.Join(left, " ")})
	}
	add("P-05", has(seqs, "\x1b_far2l1\x1b\\", false) && has(seqs, "\x1b_far2l0\x1b\\", true),
		"far2l1 at the start, far2l0 at the exit (the events are not checked)", "no far2l1 at the start or no far2l0 at the exit")
	return out
}

// far2lKey is a key event of the far2l extensions, ESC _ f2l<base64> BEL
// (docs/FAR2L_COMPAT.md, P-05): the fields of the stack in the order far2l
// pushes them, little endian, the command ('K' down, 'k' up) last.
func far2lKey(vk, scan uint16, char uint32, down bool) string {
	var b []byte
	b = binary.LittleEndian.AppendUint16(b, 1) // repeat count
	b = binary.LittleEndian.AppendUint16(b, vk)
	b = binary.LittleEndian.AppendUint16(b, scan)
	b = binary.LittleEndian.AppendUint32(b, 0) // modifiers
	b = binary.LittleEndian.AppendUint32(b, char)
	if down {
		b = append(b, 'K')
	} else {
		b = append(b, 'k')
	}
	return "\x1b_f2l" + base64.StdEncoding.EncodeToString(b) + "\a"
}

// quitKeys are F10 and, after a pause so that the confirmation can open, Enter:
// plain sequences, or events of the extensions when they are on, because a
// manager that has them on may read keys from the events only.
func quitKeys(far2l bool) (f10, enter string) {
	if !far2l {
		return "\x1b[21~", "\r"
	}
	return far2lKey(0x79, 0x44, 0, true) + far2lKey(0x79, 0x44, 0, false),
		far2lKey(0x0D, 0x1c, '\r', true) + far2lKey(0x0D, 0x1c, '\r', false)
}

// CheckExit gives the verdict on P-06 for a recording made with SilentExt:
// the terminal acknowledged the extensions and answered nothing else, and the
// manager still has to quit on F10 within the limit.
func CheckExit(rec Recording) Result {
	if rec.Killed || rec.ExitTook > 5*time.Second {
		return Result{"P-06", "fail", "the manager did not quit within 5 s of F10 after the extensions were acknowledged and nothing else was answered"}
	}
	return Result{"P-06", "pass", fmt.Sprintf("quit %.1f s after F10 with a silent far2l terminal", rec.ExitTook.Seconds())}
}

// Format prints the results as rows of the report of section 9.
func Format(rs []Result) string {
	var b strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&b, "| %s | %s | — | %s |\n", r.ID, r.Result, r.Note)
	}
	return b.String()
}
