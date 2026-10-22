// Command far2lcompat is the self-check kit of docs/FAR2L_COMPAT.md, section 10.
// The "record" subcommand runs a program in a pseudo-terminal 80x25, plays the
// part of a terminal that answers the status reports, sends the quit keys after
// a pause and prints the control sequences the program wrote, together with the
// verdicts on the requirements that a recording can decide.
package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "record" {
		fmt.Fprintln(os.Stderr, "usage: far2lcompat record [--wait 2s] -- <command> [args...]")
		os.Exit(2)
	}
	args := os.Args[2:]
	wait := 2 * time.Second
	if len(args) >= 2 && args[0] == "--wait" {
		d, err := time.ParseDuration(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "far2lcompat:", err)
			os.Exit(2)
		}
		wait, args = d, args[2:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "far2lcompat: no command")
		os.Exit(2)
	}
	rec, mark, err := Record(args, wait)
	if err != nil {
		fmt.Fprintln(os.Stderr, "far2lcompat:", err)
		os.Exit(1)
	}
	seqs := Find(rec, mark)
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
	fmt.Print(Format(Check(seqs)))
}
