//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// session is a manager running in a pseudo-terminal that the kit plays.
type session struct {
	master *os.File
	cmd    *exec.Cmd
	home   string
	mu     sync.Mutex
	rec    bytes.Buffer
}

func startSession(argv []string) (*session, error) {
	home, err := os.MkdirTemp("", "f2c")
	if err != nil {
		return nil, err
	}
	work := filepath.Join(home, "work")
	if err := os.MkdirAll(work, 0o700); err != nil { //nolint:gosec // paths are inside the temporary directory of the probe
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("a\n"), 0o600); err != nil {
		return nil, err
	}
	master, slave, err := openPTY(30, 100)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // running the command given on the command line is the point of the tool
	cmd.Dir = work
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.Env = append(os.Environ(), "HOME="+home, "TMPDIR="+home, "XDG_CONFIG_HOME="+home+"/.config",
		"TERM=xterm-256color", "LANG=C.UTF-8", "SHELL=/bin/bash", "OUT="+home+"/fc")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return nil, err
	}
	_ = slave.Close()
	s := &session{master: master, cmd: cmd, home: home}
	go func() { _ = cmd.Wait() }()
	go func() {
		var tail []byte
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.rec.Write(buf[:n])
				tail = answerChunk(master, tail, buf[:n], answers)
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return s, nil
}

// snap saves the recording so far for the diagnostic F2C_DUMP, to be rendered later.
func (s *session) snap(label string) {
	dump := os.Getenv("F2C_DUMP")
	if dump == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.WriteFile(dump+"."+label, s.rec.Bytes(), 0o600) //nolint:gosec // a diagnostic file named by the user
}

func (s *session) out(name string) string { return filepath.Join(s.home, "fc", name) }

func (s *session) send(text string) { _, _ = s.master.Write([]byte(text)) }

func (s *session) close() {
	_ = s.cmd.Process.Kill()
	_ = s.master.Close()
	_ = os.RemoveAll(s.home) //nolint:gosec // paths are inside the temporary directory of the probe
}

// waitFile waits for a file to appear.
func (s *session) waitFile(name string, limit time.Duration) bool {
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if _, err := os.Stat(s.out(name)); err == nil { //nolint:gosec // names are constants of the probes
			return true
		}
	}
	return false
}

// settle waits until the manager has written nothing for a moment.
func (s *session) settle(limit time.Duration) {
	last, since := -1, time.Now()
	for end := time.Now().Add(limit); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		s.mu.Lock()
		n := s.rec.Len()
		s.mu.Unlock()
		if n != last {
			last, since = n, time.Now()
		} else if time.Since(since) > 700*time.Millisecond {
			return
		}
	}
}

// typeCommand types a command into the command line of the manager and runs it.
func (s *session) typeCommand(cmd string) {
	s.send(cmd)
	time.Sleep(300 * time.Millisecond)
	s.send("\r")
}

// HostProbes runs the probes of section 6 against the manager started by argv.
func HostProbes(argv []string, opt HostOptions) ([]Result, error) {
	files := map[string]string{}
	keyOut := map[string]string{}

	// Phase 1: one script in the terminal of the manager, no keys.
	s, err := startSession(argv)
	if err != nil {
		return nil, err
	}
	s.settle(20 * time.Second)
	if err := os.MkdirAll(s.out(""), 0o700); err != nil { //nolint:gosec // paths are inside the temporary directory of the probe
		s.close()
		return nil, err
	}
	script := filepath.Join(s.home, "probe1.sh")
	if err := os.WriteFile(script, []byte(hostProbe1), 0o700); err != nil { //nolint:gosec // a script to run
		s.close()
		return nil, err
	}
	s.typeCommand("bash " + script)
	if s.waitFile("DONE", 40*time.Second) {
		for _, p := range queryProbes {
			if b, err := os.ReadFile(s.out(p.file + ".out")); err == nil { //nolint:gosec // names are constants of the probes
				files[p.file] = string(b)
			}
		}
	}
	s.close()

	// Phase 2: a fresh manager for every probe that needs keys.
	for _, kp := range keyProbes {
		ks, err := startSession(argv)
		if err != nil {
			return nil, err
		}
		ks.settle(20 * time.Second)
		if err := os.MkdirAll(ks.out(""), 0o700); err != nil { //nolint:gosec // paths are inside the temporary directory of the probe
			ks.close()
			return nil, err
		}
		script := filepath.Join(ks.home, "probe2.sh")
		if err := os.WriteFile(script, []byte(hostProbe2), 0o700); err != nil { //nolint:gosec // a script to run
			ks.close()
			return nil, err
		}
		if kp.pre != "" {
			ks.typeCommand(kp.pre)
			time.Sleep(2 * time.Second)
		}
		ks.typeCommand("bash " + script + " " + kp.name)
		if ks.waitFile(kp.name+".ready", 30*time.Second) {
			time.Sleep(500 * time.Millisecond)
			ks.snap(kp.name + ".a")
			if opt.TermKey != "" {
				ks.send(opt.TermKey) // show the terminal
			}
			time.Sleep(1500 * time.Millisecond)
			ks.snap(kp.name + ".b")
			ks.send(kp.keys)
			time.Sleep(500 * time.Millisecond)
			ks.snap(kp.name + ".c")
			if ks.waitFile(kp.name+".done", 15*time.Second) {
				if b, err := os.ReadFile(ks.out(kp.name + ".out")); err == nil { //nolint:gosec // names are constants of the probes
					keyOut[kp.name] = string(b)
				}
			}
		}
		if dump := os.Getenv("F2C_DUMP"); dump != "" {
			ks.mu.Lock()
			_ = os.WriteFile(dump+"."+kp.name, ks.rec.Bytes(), 0o600) //nolint:gosec // a diagnostic file named by the user
			ks.mu.Unlock()
		}
		if os.Getenv("F2C_DEBUG") != "" {
			ks.mu.Lock()
			fmt.Fprintf(os.Stderr, "DEBUG %s: output file %q, screen text:\n%s\n", kp.name, keyOut[kp.name], visibleText(ks.rec.Bytes()))
			ks.mu.Unlock()
		}
		ks.close()
	}
	return hostResults(files, keyOut), nil
}
