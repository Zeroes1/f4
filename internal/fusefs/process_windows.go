package fusefs

import "golang.org/x/sys/windows"

// stillActive is the exit code GetExitCodeProcess reports for a process
// that has not exited.
const stillActive = 259

// processExists reports whether pid names a running process. Opening the PID
// is not enough on Windows: a process that has exited can still be opened
// for as long as anybody holds a handle to it, so it has to report itself
// still active as well.
func processExists(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // G115: a PID from our own registry
	if err != nil {
		// Access denied means a process is there, owned by somebody else.
		return err == windows.ERROR_ACCESS_DENIED
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == stillActive
}
