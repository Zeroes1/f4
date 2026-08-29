//go:build windows

package main

import "fmt"

func (s stepResult) line() string {
	base := fmt.Sprintf("%-22s %-8s %6dms", s.Name, s.Outcome, s.Dur.Milliseconds())
	if s.Summary != "" {
		base += "  " + s.Summary
	}
	if s.Err != "" {
		base += "  <" + s.Err + ">"
	}
	return base
}

func (r rungResult) line() string {
	if !r.CreateOK {
		return fmt.Sprintf("%-7d create FAILED: %s", r.Height, r.CreateNo)
	}
	tag := " "
	if r.Precise {
		tag = "*"
	}
	rejoined := "no"
	switch r.WideLongRows {
	case 1:
		rejoined = "yes"
	case 0:
		rejoined = "n/a"
	}
	return fmt.Sprintf(
		"%-7d%s create %5dms  host %6dKB->%6dKB  fill %d/%d in %5dms  "+
			"reflow %7dB %5dms carrying %d [%d..%d]  wide4000 %6dB %5dms rejoined=%s  alt h/l=%v/%v",
		r.Height, tag, r.CreateMs,
		r.HostRSSAfterCreateKB, r.HostRSSAfterFillKB,
		r.LinesSeen, r.LinesAsked, r.FillMs,
		r.ReflowBytes, r.ReflowMs, r.ReflowMarkers, r.ReflowLowest, r.ReflowHighest,
		r.WideBytes, r.WideMs, rejoined,
		r.AltEnterSeen, r.AltLeaveSeen)
}
