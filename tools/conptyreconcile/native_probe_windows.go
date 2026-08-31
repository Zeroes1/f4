//go:build windows

package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const (
	probeBundleURL   = "https://github.com/microsoft/terminal/releases/download/v1.12.10982.0/Microsoft.WindowsTerminal_Win11_1.12.10983.0_8wekyb3d8bbwe.msixbundle"
	probePackageName = "CascadiaPackage_1.12.10983.0_x64.msix"
)

type probeResize struct {
	At     time.Time `json:"at"`
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Error  string    `json:"error,omitempty"`
}

type nativeProbeSession struct {
	InitialWidth  int              `json:"initial_width"`
	InitialHeight int              `json:"initial_height"`
	Command       string           `json:"command"`
	HostCommand   string           `json:"host_command"`
	StartedAt     time.Time        `json:"started_at"`
	FinishedAt    time.Time        `json:"finished_at"`
	ExitCode      uint32           `json:"exit_code"`
	Resizes       []probeResize    `json:"resizes"`
	Markers       []string         `json:"markers"`
	RawSHA256     string           `json:"raw_sha256"`
	RawOutput     []byte           `json:"raw_output"`
	Events        []streamEvent    `json:"events"`
	Error         string           `json:"error,omitempty"`
}

type nativeProbeReport struct {
	Mode          string             `json:"mode"`
	Host          pinnedHostIdentity  `json:"host"`
	BundleURL     string             `json:"bundle_url"`
	Package       string             `json:"package"`
	GOOS          string             `json:"goos"`
	GOARCH        string             `json:"goarch"`
	WorkingDir    string             `json:"working_dir"`
	Executable    string             `json:"probe_executable"`
	Environment   map[string]string  `json:"environment"`
	ExpectedInput []byte             `json:"expected_input"`
	Sessions      []nativeProbeSession `json:"sessions"`
	CompletedAt   time.Time          `json:"completed_at"`
}

func runNativeProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join(filepath.Dir(os.Args[0]), "native-openconsole-probe.json")
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return fmt.Errorf("create native probe report directory: %w", err)
	}
	resolved, err := ensureProbeHost(hostPath)
	if err != nil {
		return err
	}
	identity, err := verifyPinnedHost(resolved)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate probe executable: %w", err)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	report := nativeProbeReport{
		Mode: "native-openconsole-probe", Host: identity, BundleURL: probeBundleURL,
		Package: probePackageName, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		WorkingDir: workingDir, Executable: executable, Environment: probeEnvironment(), ExpectedInput: []byte(probeWorkload()),
	}
	for _, dimensions := range [][2]int{{80, 25}, {1, 1}, {121, 40}} {
		session, runErr := runNativeProbeSession(resolved, executable, dimensions[0], dimensions[1])
		report.Sessions = append(report.Sessions, session)
		artifactDir := reportPath + ".sessions"
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			return fmt.Errorf("create native probe artifact directory: %w", err)
		}
		artifact := filepath.Join(artifactDir, fmt.Sprintf("%dx%d.raw", dimensions[0], dimensions[1]))
		if err := os.WriteFile(artifact, session.RawOutput, 0o644); err != nil {
			return fmt.Errorf("write native probe raw output: %w", err)
		}
		if runErr != nil {
			_ = writeJSON(reportPath, report)
			return fmt.Errorf("native probe %dx%d: %w", dimensions[0], dimensions[1], runErr)
		}
	}
	report.CompletedAt = time.Now().UTC()
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	fmt.Printf("native OpenConsole probe complete: %s\n", reportPath)
	return nil
}

func probeEnvironment() map[string]string {
	result := make(map[string]string)
	for _, name := range []string{"WT_SESSION", "WT_PROFILE_ID", "TERM", "TERM_PROGRAM", "WSLENV", "ConEmuANSI", "PROMPT", "CHCP"} {
		if value, ok := os.LookupEnv(name); ok {
			result[name] = value
		}
	}
	return result
}

func runNativeProbeSession(hostPath, executable string, width, height int) (session nativeProbeSession, runErr error) {
	session.InitialWidth, session.InitialHeight = width, height
	session.Command = fmt.Sprintf("%s -emit-probe", executable)
	session.StartedAt = time.Now().UTC()
	pty, err := createPinnedPseudoConsole(hostPath, width, height)
	if err != nil {
		session.Error = err.Error()
		return session, err
	}
	session.HostCommand = pty.hostCommandLine
	defer pty.close()
	defer pty.closePipes()
	recorder := newHostCaptureRecorder(0, width, height)
	recorder.append(streamInput, []byte(probeWorkload()), "native-probe-child-payload")
	outputReady := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, readErr := readPinnedOutputRecorded(pty, recorder)
		outputReady <- struct {
			data []byte
			err  error
		}{data: data, err: readErr}
	}()
	if err := attachPinnedClient(pty, fmt.Sprintf(`"%s" -emit-probe`, executable)); err != nil {
		session.Error = err.Error()
		return session, err
	}
	resizeSchedule := [][2]int{{1, 1}, {width, height}, {121, 40}, {80, 25}}
	for index, dimensions := range resizeSchedule {
		// The child deliberately writes in short chunks.  These bounded pauses
		// make at least one resize overlap active output without making timing a
		// completion heuristic.
		time.Sleep(time.Duration(2+index*3) * time.Millisecond)
		resize := probeResize{At: time.Now().UTC(), Width: dimensions[0], Height: dimensions[1]}
		if err := resizePinnedPseudoConsole(pty, uint16(dimensions[0]), uint16(dimensions[1])); err != nil {
			resize.Error = err.Error()
			session.Resizes = append(session.Resizes, resize)
			session.Error = err.Error()
			terminatePinnedClient(pty)
			return session, err
		}
		recorder.resize(dimensions[0], dimensions[1])
		session.Resizes = append(session.Resizes, resize)
	}
	exitCode, err := waitProbeClient(pty)
	if err != nil {
		session.Error = err.Error()
		terminatePinnedClient(pty)
		return session, err
	}
	session.ExitCode = exitCode
	if exitCode != 0 {
		err := fmt.Errorf("native probe child exited with code %d", exitCode)
		session.Error = err.Error()
		return session, err
	}
	if pty.input != 0 {
		_ = windows.CloseHandle(pty.input)
		pty.input = 0
	}
	pty.close()
	result := <-outputReady
	if result.err != nil {
		session.Error = result.err.Error()
		return session, result.err
	}
	session.FinishedAt = time.Now().UTC()
	session.RawOutput = result.data
	session.Events = recorder.snapshot().Events
	hash := sha256.Sum256(result.data)
	session.RawSHA256 = hex.EncodeToString(hash[:])
	for _, marker := range probeExpectedMarkers() {
		if bytes.Contains(result.data, []byte(marker)) {
			session.Markers = append(session.Markers, marker)
		} else {
			err := fmt.Errorf("native output does not contain marker %q", marker)
			session.Error = err.Error()
			return session, err
		}
	}
	return session, nil
}

func waitProbeClient(pty *pinnedConPTY) (uint32, error) {
	if pty == nil || pty.childProcess == 0 {
		return 0, fmt.Errorf("native probe child process was not created")
	}
	event, err := windows.WaitForSingleObject(pty.childProcess, maxWaitMilliseconds)
	if err != nil {
		return 0, fmt.Errorf("WaitForSingleObject(native probe child): %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return 0, fmt.Errorf("native probe child did not finish within %d ms", maxWaitMilliseconds)
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(pty.childProcess, &exitCode); err != nil {
		return 0, fmt.Errorf("GetExitCodeProcess(native probe child): %w", err)
	}
	handle := pty.childProcess
	pty.childProcess = 0
	_ = windows.CloseHandle(handle)
	return exitCode, nil
}

func ensureProbeHost(hostPath string) (string, error) {
	if hostPath != "" {
		return filepath.Clean(hostPath), nil
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	if base == "" {
		return "", fmt.Errorf("LOCALAPPDATA is unavailable; cannot choose native host cache")
	}
	root := filepath.Join(base, "f4", "native-conpty", "1.12.10983.0")
	host := filepath.Join(root, strings.TrimSuffix(probePackageName, ".msix"), "OpenConsole.exe")
	if _, err := verifyPinnedHost(host); err == nil {
		return host, nil
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return "", fmt.Errorf("create native host cache: %w", err)
	}
	lock := root + ".lock"
	deadline := time.Now().Add(2 * time.Minute)
	for {
		err := os.Mkdir(lock, 0o755)
		if err == nil {
			break
		}
		if !os.IsExist(err) || time.Now().After(deadline) {
			return "", fmt.Errorf("acquire native host cache lock: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer os.Remove(lock)
	if _, err := verifyPinnedHost(host); err == nil {
		return host, nil
	}
	bundle := root + ".msixbundle"
	if err := downloadProbeBundle(bundle); err != nil {
		return "", err
	}
	tempRoot, err := os.MkdirTemp(filepath.Dir(root), "native-conpty-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempRoot)
	if err := extractProbeZip(bundle, tempRoot); err != nil {
		return "", fmt.Errorf("extract MSIX bundle: %w", err)
	}
	packageFile, err := findProbeFile(tempRoot, probePackageName)
	if err != nil {
		return "", err
	}
	packageDir := filepath.Join(root, strings.TrimSuffix(probePackageName, ".msix"))
	if err := os.RemoveAll(packageDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		return "", err
	}
	if err := extractProbeZip(packageFile, packageDir); err != nil {
		return "", fmt.Errorf("extract x64 package: %w", err)
	}
	if _, err := verifyPinnedHost(host); err != nil {
		return "", fmt.Errorf("downloaded package failed pinned host verification: %w", err)
	}
	return host, nil
}

func downloadProbeBundle(destination string) error {
	part := destination + ".part"
	response, err := http.Get(probeBundleURL)
	if err != nil {
		return fmt.Errorf("download pinned MSIX bundle: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download pinned MSIX bundle: HTTP %s", response.Status)
	}
	file, err := os.Create(part)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(part)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(part)
		return closeErr
	}
	_ = os.Remove(destination)
	if err := os.Rename(part, destination); err != nil {
		_ = os.Remove(part)
		return err
	}
	return nil
}

func extractProbeZip(source, destination string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer archive.Close()
	root, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	for _, entry := range archive.File {
		name := filepath.FromSlash(entry.Name)
		target := filepath.Join(root, name)
		relative, err := filepath.Rel(root, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("zip entry escapes destination: %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		reader, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.Create(target)
		if err == nil {
			_, err = io.Copy(output, reader)
		}
		_ = reader.Close()
		if output != nil {
			_ = output.Close()
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func findProbeFile(root, name string) (string, error) {
	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.EqualFold(info.Name(), name) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("expected exactly one %s in bundle, found %d", name, len(matches))
	}
	return matches[0], nil
}
