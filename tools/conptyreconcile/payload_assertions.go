package main

import (
	"bytes"
	"fmt"
	"strings"
)

// payloadAssertion is deliberately explicit.  A native transport success is
// not a history success: every assertion below is either passed, failed, or
// deferred to the VT-history consumer.  Deferred assertions make the gate
// fail closed instead of silently treating a partial check as complete.
type payloadAssertion struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	ExpectedCount int    `json:"expected_count,omitempty"`
	ObservedCount int    `json:"observed_count,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

func assertStaticPayload(expected, raw []byte, markers ...string) []payloadAssertion {
	if len(markers) == 0 {
		markers = []string{probeBeginMarker, probeEndMarker}
	}
	var expectedStream logicalLineStream
	expectedStream.Feed(stripCursorVisibilityWrapper(expected))
	lines := expectedStream.Lines()
	stream := parseHostRenderStream(raw, 0)
	var history []byte
	for _, line := range stream.Lines() {
		history = append(history, line.Bytes...)
		history = append(history, line.Terminator...)
	}
	printable := printableStream(history)
	result := make([]payloadAssertion, 0, len(lines)+3)
	lineFrequency := make(map[string]int)
	for _, line := range lines {
		if !bytes.Contains(line.Bytes, []byte{0x1b}) {
			needle := append(append([]byte(nil), line.Bytes...), line.Terminator...)
			lineFrequency[string(needle)]++
		}
	}

	for index, line := range lines {
		name := fmt.Sprintf("line[%d]", index)
		if colon := bytes.IndexByte(line.Bytes, ':'); colon > 0 {
			name = string(line.Bytes[:colon])
		}
		if bytes.Contains(line.Bytes, []byte{0x1b}) || bytes.Contains(line.Bytes, []byte{'\t'}) {
			result = append(result, payloadAssertion{
				Name:   name,
				Status: "deferred",
				Detail: "line contains VT controls; raw transport is not a logical history",
			})
			continue
		}
		needle := append(append([]byte(nil), line.Bytes...), line.Terminator...)
		observed := bytes.Count(printable, needle)
		expectedCount := lineFrequency[string(needle)]
		status := "passed"
		detail := "exact host-emitted logical line found"
		if observed != expectedCount {
			status = "failed"
			detail = "plain logical line count differs"
		}
		result = append(result, payloadAssertion{
			Name: name, Status: status, ExpectedCount: expectedCount,
			ObservedCount: observed, Detail: detail,
		})
	}

	// Marker checks are independent from line checks.  CR/LF inserted between
	// marker bytes by a narrow viewport is not silently removed here: that is
	// precisely a history-layer responsibility.
	withoutNewlines := strings.NewReplacer("\r", "", "\n", "").Replace(string(printable))
	for _, marker := range markers {
		observed := strings.Count(withoutNewlines, marker)
		status := "passed"
		detail := "marker count is exact in the static stream"
		if observed != 1 {
			status = "failed"
			detail = "marker count is not exactly one"
		}
		result = append(result, payloadAssertion{
			Name: marker, Status: status, ExpectedCount: 1,
			ObservedCount: observed, Detail: detail,
		})
	}
	return result
}

// assertAlternatePayload treats text written while the alternate buffer is
// active as deliberately non-history. The handoff markers are still required
// exactly once, while both alternate records must be absent from the live
// logical history. No row shape or content-based deduplication is used.
func assertAlternatePayload(raw []byte, markers ...string) []payloadAssertion {
	stream := parseHostRenderStream(raw, 0)
	var history []byte
	for _, line := range stream.Lines() {
		history = append(history, line.Bytes...)
		history = append(history, line.Terminator...)
	}
	printable := printableStream(history)
	withoutNewlines := strings.NewReplacer("\r", "", "\n", "").Replace(string(printable))
	result := make([]payloadAssertion, 0, len(markers)+2)
	for _, marker := range markers {
		observed := strings.Count(withoutNewlines, marker)
		status := "passed"
		detail := "alternate handoff marker count is exact"
		if observed != 1 {
			status = "failed"
			detail = "alternate handoff marker count is not exactly one"
		}
		result = append(result, payloadAssertion{Name: marker, Status: status, ExpectedCount: 1, ObservedCount: observed, Detail: detail})
	}
	for _, record := range []string{"alternate-end", "alt-screen"} {
		observed := strings.Count(string(printable), record+"\r\n")
		status := "passed"
		detail := "alternate-buffer record is absent from primary history"
		if observed != 0 {
			status = "failed"
			detail = "alternate-buffer record leaked into primary history"
		}
		result = append(result, payloadAssertion{Name: record, Status: status, ExpectedCount: 0, ObservedCount: observed, Detail: detail})
	}
	return result
}

func stripCursorVisibilityWrapper(expected []byte) []byte {
	const hide = "\x1b[?25l"
	const show = "\x1b[?25h"
	if bytes.HasPrefix(expected, []byte(hide)) {
		expected = expected[len(hide):]
	}
	if bytes.HasSuffix(expected, []byte(show)) {
		expected = expected[:len(expected)-len(show)]
	}
	return expected
}

func assertControlPayload(raw []byte, markers ...string) []payloadAssertion {
	history := parseRenderedHistory(raw).Lines()
	result := make([]payloadAssertion, 0, 10)
	want := []struct {
		name string
		line []byte
	}{
		{"control-warmup", []byte("control-warmup")},
		{controlBeginMarker, []byte(controlBeginMarker)},
		{"red", []byte("red")},
		{"rewritten", []byte("rewritten")},
		{"cursor", []byte("twosor: one")},
		{"tabs", append([]byte("tabs:"), append(bytes.Repeat([]byte{' '}, 3), append([]byte("X"), append(bytes.Repeat([]byte{' '}, 7), 'Y')...)...)...)},
		{controlEndMarker, []byte(controlEndMarker)},
	}
	for _, item := range want {
		observed := 0
		for _, line := range history {
			if bytes.Equal(line.Bytes, item.line) {
				observed++
			}
		}
		status := "passed"
		detail := "rendered logical line matches exact expected bytes"
		if observed != 1 {
			status = "failed"
			detail = "rendered logical line count differs"
		}
		result = append(result, payloadAssertion{Name: item.name, Status: status, ExpectedCount: 1, ObservedCount: observed, Detail: detail})
	}
	for _, item := range []struct {
		name string
		seq  []byte
	}{
		{"sgr-red", []byte("\x1b[31m")},
		{"sgr-default", []byte("\x1b[m")},
	} {
		observed := bytes.Count(raw, item.seq)
		status := "passed"
		detail := "host renderer emitted the source-defined SGR sequence"
		if observed != 1 {
			status = "failed"
			detail = "host renderer SGR sequence count differs"
		}
		result = append(result, payloadAssertion{Name: item.name, Status: status, ExpectedCount: 1, ObservedCount: observed, Detail: detail})
	}
	_ = markers
	return result
}

func assertionFailures(assertions []payloadAssertion) []string {
	var failures []string
	for _, assertion := range assertions {
		if assertion.Status != "passed" {
			failures = append(failures, fmt.Sprintf("%s=%s: %s", assertion.Name, assertion.Status, assertion.Detail))
		}
	}
	return failures
}
