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
	expectedStream.Feed(expected)
	lines := expectedStream.Lines()
	printable := printableStream(raw)
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

func assertionFailures(assertions []payloadAssertion) []string {
	var failures []string
	for _, assertion := range assertions {
		if assertion.Status != "passed" {
			failures = append(failures, fmt.Sprintf("%s=%s: %s", assertion.Name, assertion.Status, assertion.Detail))
		}
	}
	return failures
}
