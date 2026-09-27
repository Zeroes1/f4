//go:build darwin

package proclist

import (
	"bytes"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// Supported reports whether this build can collect a real process list.
func Supported() bool { return true }

// collect enumerates every process the kernel reports through
// kern.proc.all and returns one sample per process. kern.proc.all's
// kinfo_proc gives a reliable PID and name (that is its actual purpose),
// but its memory/CPU accounting fields are widely known to be stale
// BSD-compatibility leftovers on modern XNU -- x/sys/unix's own KinfoProc
// even names the embedded Vmspace fields "Dummy" -- so CPU% and memory come
// from libproc's proc_pidinfo(PROC_PIDTASKINFO) instead, the same
// documented API Activity Monitor and ps use. A process this one lacks the
// entitlement to query still gets listed by PID and name, with zeroed CPU%
// and memory, the same way collect_windows.go handles a process it could
// not open.
func (c *collector) collect() ([]sample, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("proclist: sysctl kern.proc.all: %w", err)
	}

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	seen := make(map[int]bool, len(procs))
	samples := make([]sample, 0, len(procs))
	for _, kp := range procs {
		pid := int(kp.Proc.P_pid)
		if pid <= 0 {
			continue
		}
		name := commString(kp.Proc.P_comm[:])
		seen[pid] = true

		info, ok := readDarwinTaskInfo(pid)
		cpuPercent := 0.0
		var rssKiB uint64
		if ok {
			rssKiB = info.ResidentSize / 1024
			// ticks is in nanoseconds here (libproc's own unit); the divisor
			// below reflects that, not USER_HZ or FILETIME's 100ns.
			nanos := info.TotalUser + info.TotalSystem
			if prev, hasPrev := c.prev[pid]; hasPrev && nanos >= prev.ticks {
				if dt := now.Sub(prev.at).Seconds(); dt > 0 {
					cpuPercent = float64(nanos-prev.ticks) / 1e9 / dt * 100
				}
			}
			c.prev[pid] = tickSample{ticks: nanos, at: now}
		}
		samples = append(samples, sample{pid: pid, name: name, rssKiB: rssKiB, cpuPercent: cpuPercent})
	}

	// Bound memory: forget the running total for processes that no longer exist.
	for pid := range c.prev {
		if !seen[pid] {
			delete(c.prev, pid)
		}
	}
	return samples, nil
}

// commString trims a null-terminated comm buffer (kinfo_proc's P_comm) to a
// Go string.
func commString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// procTaskInfo mirrors <sys/proc_info.h>'s struct proc_taskinfo: 6 uint64
// fields followed by 12 int32 fields, 96 bytes on both amd64 and arm64 (no
// padding: the leading uint64 run keeps 8-byte alignment throughout, and 12
// is even, so the struct's own size is already a multiple of 8). Only the
// fields this package reads are named individually; readDarwinTaskInfo
// checks the byte count proc_pidinfo actually filled against
// unsafe.Sizeof(procTaskInfo{}) before trusting any of them, so a future
// ABI change that shrinks or reorders the struct is refused rather than
// silently misread.
type procTaskInfo struct {
	VirtualSize   uint64
	ResidentSize  uint64
	TotalUser     uint64
	TotalSystem   uint64
	ThreadsUser   uint64
	ThreadsSystem uint64
	_             [12]int32
}

// procPidTaskInfo is PROC_PIDTASKINFO from <sys/proc_info.h>.
const procPidTaskInfo = 4

var (
	libprocOnce sync.Once
	libprocOK   bool
	procPidinfo func(pid int32, flavor int32, arg uint64, buffer *procTaskInfo, buffersize int32) int32
)

// initLibproc loads libproc's proc_pidinfo lazily. Failure is silent and
// leaves libprocOK false: readDarwinTaskInfo then reports every process as
// having no task info, and collect still lists PID and name for all of
// them, the same graceful degradation cpu_windows.go uses for its optional
// PDH counters.
func initLibproc() {
	libprocOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				libprocOK = false
			}
		}()
		// libproc.dylib has had no on-disk file of its own since the dyld
		// shared cache (macOS 11+); dlopen still resolves this and every
		// other historical system library path through the cache, which is
		// exactly what cpu_windows.go's initPDH relies on for pdh.dll.
		h, err := purego.Dlopen("/usr/lib/libproc.dylib", purego.RTLD_LAZY)
		if err != nil || h == 0 {
			return
		}
		purego.RegisterLibFunc(&procPidinfo, h, "proc_pidinfo")
		libprocOK = true
	})
}

func readDarwinTaskInfo(pid int) (procTaskInfo, bool) {
	initLibproc()
	if !libprocOK {
		return procTaskInfo{}, false
	}
	var info procTaskInfo
	size := int32(unsafe.Sizeof(info))
	// #nosec G115 -- pid comes from kinfo_proc's own P_pid (int32) and is bounds-checked non-negative by the caller.
	n := procPidinfo(int32(pid), procPidTaskInfo, 0, &info, size)
	if n != size {
		return procTaskInfo{}, false
	}
	return info, true
}
