//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

// nativeOpenConsolePTY is the production f4 transport for the pinned host.
// It owns the ConDrv handles and the attached child; no recorded stream or
// second terminal model is involved.
type nativeOpenConsolePTY struct {
	mu   sync.Mutex
	pty  *pinnedConPTY
	cols int
	rows int
}

func newNativeOpenConsolePTY(cols, rows int) (*nativeOpenConsolePTY, error) {
	host, err := pinnedOpenConsolePath()
	if err != nil {
		return nil, err
	}
	pty, err := createPinnedPseudoConsole(host, cols, rows)
	if err != nil {
		return nil, err
	}
	if _, _, err := verifyPinnedHostProcess(pty.hostProcess, host); err != nil {
		pty.close()
		pty.closePipes()
		return nil, err
	}
	return &nativeOpenConsolePTY{pty: pty, cols: cols, rows: rows}, nil
}

func pinnedOpenConsolePath() (string, error) {
	if candidate := os.Getenv("F4_PINNED_OPENCONSOLE"); candidate != "" {
		if _, err := verifyPinnedHost(candidate); err != nil {
			return "", err
		}
		return filepath.Clean(candidate), nil
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", fmt.Errorf("LOCALAPPDATA is unavailable; pinned OpenConsole is required")
	}
	host := filepath.Join(base, "f4", "native-conpty", "1.12.10983.0", "CascadiaPackage_1.12.10983.0_x64", "OpenConsole.exe")
	if _, err := verifyPinnedHost(host); err != nil {
		return "", fmt.Errorf("pinned OpenConsole cache is unavailable: %w", err)
	}
	return host, nil
}

func (p *nativeOpenConsolePTY) Read(buffer []byte) (int, error) {
	p.mu.Lock()
	pty := p.pty
	p.mu.Unlock()
	if pty == nil || pty.output == 0 {
		return 0, os.ErrClosed
	}
	var read uint32
	err := windows.ReadFile(pty.output, buffer, &read, nil)
	return int(read), err
}

func (p *nativeOpenConsolePTY) Write(data []byte) (int, error) {
	p.mu.Lock()
	pty := p.pty
	p.mu.Unlock()
	if pty == nil || pty.input == 0 {
		return 0, os.ErrClosed
	}
	var written uint32
	if err := windows.WriteFile(pty.input, data, &written, nil); err != nil {
		return int(written), err
	}
	return int(written), nil
}

func (p *nativeOpenConsolePTY) SetSize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pty == nil {
		return
	}
	if err := resizePinnedPseudoConsole(p.pty, uint16(cols), uint16(rows)); err == nil {
		p.cols, p.rows = cols, rows
	}
}

func (p *nativeOpenConsolePTY) Run(name string, args ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pty == nil {
		return os.ErrClosed
	}
	command := name
	if len(args) > 0 {
		command += " " + strings.Join(args, " ")
	}
	return attachPinnedClient(p.pty, command)
}

func (p *nativeOpenConsolePTY) Wait() error {
	p.mu.Lock()
	pty := p.pty
	p.mu.Unlock()
	if pty == nil || pty.childProcess == 0 {
		return nil
	}
	event, err := windows.WaitForSingleObject(pty.childProcess, windows.INFINITE)
	if err != nil {
		return err
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("native OpenConsole child wait returned %d", event)
	}
	return nil
}

func (p *nativeOpenConsolePTY) IsBusy() bool {
	p.mu.Lock()
	pty := p.pty
	p.mu.Unlock()
	if pty == nil || pty.childProcess == 0 {
		return false
	}
	var code uint32
	return windows.GetExitCodeProcess(pty.childProcess, &code) == nil && code == 259
}

func (p *nativeOpenConsolePTY) Close() error {
	p.mu.Lock()
	pty := p.pty
	p.pty = nil
	p.mu.Unlock()
	if pty == nil {
		return nil
	}
	if pty.childProcess != 0 {
		_ = windows.TerminateProcess(pty.childProcess, 1)
		_, _ = windows.WaitForSingleObject(pty.childProcess, 2000)
		_ = windows.CloseHandle(pty.childProcess)
		pty.childProcess = 0
	}
	// _ClosePseudoConsoleMembers closes the signal first and waits for the
	// pinned host to flush. Closing output before that boundary truncates the
	// final ConPTY frame (the live test observed only the first 15 C's).
	pty.close()
	pty.closePipes()
	return nil
}
