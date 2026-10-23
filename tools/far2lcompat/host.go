package main

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed host_probe1.sh
var hostProbe1 string

//go:embed host_probe2.sh
var hostProbe2 string

// HostOptions say how the host probes drive the manager.
type HostOptions struct {
	TermKey string // the key that shows the built-in terminal, as the bytes the terminal sends; none by default (f4 shows it while a command runs)
}

// probe is a check of one requirement of section 6 on what the host sent.
type probe struct {
	id, file string
	want     func(got string) (ok bool, note string)
}

func exactly(want string) func(string) (bool, string) {
	return func(got string) (bool, string) { return got == want, "got " + got }
}

func matches(re string) func(string) (bool, string) {
	rx := regexp.MustCompile(re)
	return func(got string) (bool, string) { return rx.MatchString(got), "got " + got }
}

// queryProbes are the answers of phase 1, read from the files the script wrote.
// printf %q shows the answers as $'...'.
var queryProbes = []probe{
	{"T-01", "T01a", exactly(`$'\E[0'`)},
	{"T-01", "T01b", matches(`^\$'\\E\[[0-9]+;[0-9]+'$`)},
	{"T-01", "T01c", matches(`^\$'\\E\[\?[0-9;]*'$`)},
	{"T-03", "T03a", exactly(`$'\E[?1'`)},
	{"T-05", "T05a", exactly(`$'\E_far2lok\a\E[0'`)},
	{"T-10", "T10a", exactly(`''`)},
}

// keyProbe is a probe that needs keys: the driver sets the script up, presses
// the keys and compares what cat -v showed.
type keyProbe struct {
	id, name string
	keys     string // what the terminal sends to the manager
	want     string
	pre      string // a command typed into the manager before the script, "" for none
}

const (
	w32A     = "\x1b[65;30;97;1;0;1_\x1b[65;30;97;0;0;1_"
	w32CtrlA = "\x1b[65;30;1;1;24;1_\x1b[65;30;1;0;24;1_"
	pasteXY  = "\x1b[200~x\ry\x1b[201~"
)

var keyProbes = []keyProbe{
	{"T-02", "T02a", pasteXY, "^[[200~x^My^[[201~", ""},
	{"T-02", "T02b", pasteXY, "x^My", ""},
	{"T-03", "T03b", "\x1b[97;6u", "^[[97;6u", ""},
	{"T-04", "T04c", w32A, "^[[65;30;97;1;0;1_^[[65;30;97;0;0;1_", ""},
	{"T-07", "T07b", w32A + w32CtrlA + "\x1b[200~x\x1b[201~", "a^Ax",
		`printf '\033[?2004h\033[?9001h\033[>1u\033[?1000h'`},
}

// hostResults turns the answers of phase 1 and the output of the key probes
// into rows. Rows of one requirement are joined: it passes when all its probes
// pass.
func hostResults(files map[string]string, keyOut map[string]string) []Result {
	order := []string{}
	byID := map[string]*Result{}
	add := func(id string, ok bool, note string) {
		r, seen := byID[id]
		if !seen {
			r = &Result{ID: id, Result: "pass"}
			byID[id] = r
			order = append(order, id)
		}
		if !ok {
			r.Result = "fail"
		}
		if r.Note != "" {
			r.Note += "; "
		}
		r.Note += note
	}
	for _, p := range queryProbes {
		got, found := files[p.file]
		if !found {
			add(p.id, false, p.file+": no answer file (the script did not finish)")
			continue
		}
		ok, note := p.want(strings.TrimSpace(got))
		if ok {
			note = p.file + " ok"
		} else {
			note = p.file + " " + note
		}
		add(p.id, ok, note)
	}
	for _, p := range keyProbes {
		got, found := keyOut[p.name]
		if !found {
			add(p.id, false, p.name+": no output (the probe did not run)")
			continue
		}
		shown := catV(got)
		if shown == p.want {
			add(p.id, true, p.name+" ok")
		} else {
			add(p.id, false, p.name+" got "+shown+", want "+p.want)
		}
	}
	out := make([]Result, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

func printHost(rs []Result) {
	fmt.Println("| ID | Result | Env | Note |")
	fmt.Println("|---|---|---|---|")
	fmt.Print(Format(rs))
}

var escapeRE = regexp.MustCompile("\x1b\\[[0-9;?<>=:]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|\x1b_[^\x07\x1b]*(?:\x07|\x1b\\\\)")

// visibleText is the text of a recording without control sequences, for the
// diagnostic output of a probe that did not get what it expected.
func visibleText(rec []byte) string {
	text := strings.ToValidUTF8(escapeRE.ReplaceAllString(string(rec), ""), "?")
	if len(text) > 3000 {
		text = text[len(text)-3000:]
	}
	return text
}

// catV shows bytes the way cat -v does: control characters as ^X, DEL as ^?.
func catV(b string) string {
	var out strings.Builder
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == '\n' || c == '\t':
			out.WriteByte(c)
		case c < 0x20:
			out.WriteByte('^')
			out.WriteByte(c + 64)
		case c == 0x7f:
			out.WriteString("^?")
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}
