// Command far2lcompat is the self-check kit of docs/FAR2L_COMPAT.md, section 10.
// The "record" subcommand runs a program in a pseudo-terminal 80x25, plays the
// part of a terminal that answers the status reports, sends the quit keys after
// a pause and prints the control sequences the program wrote, together with the
// verdicts on the requirements that a recording can decide.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "record" {
		fmt.Fprintln(os.Stderr, "usage: far2lcompat record [--wait 2s] [--far2l] [--silent-ext] -- <command> [args...]")
		os.Exit(2)
	}
	args := os.Args[2:]
	wait := 2 * time.Second
	opt := Options{MinOutput: 1500} // a drawn screen is more than that
	for len(args) > 0 && strings.HasPrefix(args[0], "--") && args[0] != "--" {
		switch args[0] {
		case "--wait":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "far2lcompat: --wait needs a duration")
				os.Exit(2)
			}
			d, err := time.ParseDuration(args[1])
			if err != nil {
				fmt.Fprintln(os.Stderr, "far2lcompat:", err)
				os.Exit(2)
			}
			wait, args = d, args[1:]
		case "--exit-limit": // how long to wait for the exit before the kill (default 5s, the limit of P-06)
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "far2lcompat: --exit-limit needs a duration")
				os.Exit(2)
			}
			d, err := time.ParseDuration(args[1])
			if err != nil {
				fmt.Fprintln(os.Stderr, "far2lcompat:", err)
				os.Exit(2)
			}
			exitLimit, args = d, args[1:]
		case "--far2l":
			opt.Far2l = true
		case "--silent-ext":
			opt.Far2l, opt.SilentExt = true, true
		default:
			fmt.Fprintln(os.Stderr, "far2lcompat: unknown option", args[0])
			os.Exit(2)
		}
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "far2lcompat: no command")
		os.Exit(2)
	}
	rec, err := Record(args, wait, opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "far2lcompat:", err)
		os.Exit(1)
	}
	seqs := Find(rec.Data, rec.Mark)
	fmt.Println("Sequences (offset, phase, text):")
	for _, s := range seqs {
		phase := "start"
		if s.Exit {
			phase = "exit"
		}
		fmt.Printf("  %6d %-5s %s\n", s.Offset, phase, show(s.Text))
	}
	fmt.Println()
	fmt.Println("| ID | Result | Env | Note |")
	fmt.Println("|---|---|---|---|")
	if rec.Killed {
		fmt.Println("The program did not exit; what it wrote after the quit keys (stacks, if it is a Go program):")
		tail := rec.Data[rec.Mark:]
		if len(tail) > 20000 {
			tail = tail[len(tail)-20000:]
		}
		fmt.Printf("%s\n", strings.ToValidUTF8(string(tail), "?"))
	}
	fmt.Printf("Exit: %.1f s after the quit keys, killed: %v\n\n", rec.ExitTook.Seconds(), rec.Killed)
	results := Check(seqs)
	if opt.SilentExt {
		results = append(results, CheckExit(rec))
	}
	fmt.Print(Format(results))
}
