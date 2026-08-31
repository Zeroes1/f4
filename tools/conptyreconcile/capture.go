package main

import "sync"

type streamKind uint8

const (
	streamInput streamKind = iota
	streamObservedOutput
	streamResize
)

type streamEvent struct {
	Kind         streamKind `json:"kind"`
	Bytes        []byte     `json:"bytes,omitempty"`
	OutputOffset int        `json:"output_offset,omitempty"`
	Width        int        `json:"width,omitempty"`
	Height       int        `json:"height,omitempty"`
	Cause        string     `json:"cause,omitempty"`
}

type capture struct {
	Seed       int64         `json:"seed,omitempty"`
	HostPath   string        `json:"host_path,omitempty"`
	HostSHA256 string        `json:"host_sha256,omitempty"`
	Events     []streamEvent `json:"events"`
}

func (c *capture) append(kind streamKind, data []byte, cause string) {
	c.Events = append(c.Events, streamEvent{Kind: kind, Bytes: append([]byte(nil), data...), Cause: cause})
}

type hostCaptureRecorder struct {
	mu      sync.Mutex
	capture capture
	width   int
	height  int
	outputBytes int
}

func newHostCaptureRecorder(_ int64, width, height int) *hostCaptureRecorder {
	return &hostCaptureRecorder{width: width, height: height}
}

func (r *hostCaptureRecorder) append(kind streamKind, data []byte, cause string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capture.append(kind, data, cause)
	if kind == streamObservedOutput {
		r.outputBytes += len(data)
	}
}

func (r *hostCaptureRecorder) resize(width, height int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.width, r.height = width, height
	r.capture.Events = append(r.capture.Events, streamEvent{Kind: streamResize, OutputOffset: r.outputBytes, Width: width, Height: height, Cause: "pinned-host-resize"})
}

func (r *hostCaptureRecorder) snapshot() capture {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := capture{Events: make([]streamEvent, len(r.capture.Events))}
	copy(result.Events, r.capture.Events)
	return result
}
