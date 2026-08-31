//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	pinnedOpenConsoleSHA256  = "14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e"
	pinnedOpenConsoleVersion = "1.12.220408003-release1.12"
)

type pinnedHostIdentity = nativeHostProof

func verifyPinnedHost(path string) (pinnedHostIdentity, error) {
	if path == "" || !strings.EqualFold(filepath.Base(path), "OpenConsole.exe") {
		return pinnedHostIdentity{}, fmt.Errorf("refusing non-pinned OpenConsole host %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return pinnedHostIdentity{}, fmt.Errorf("pinned OpenConsole.exe is unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return pinnedHostIdentity{}, fmt.Errorf("pinned OpenConsole host is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return pinnedHostIdentity{}, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return pinnedHostIdentity{}, err
	}
	actualSHA := hex.EncodeToString(hash.Sum(nil))
	identity := pinnedHostIdentity{Path: path, SHA256: actualSHA}
	if actualSHA != pinnedOpenConsoleSHA256 {
		return identity, fmt.Errorf("pinned OpenConsole SHA-256 mismatch: got %s want %s", actualSHA, pinnedOpenConsoleSHA256)
	}
	version, err := readHostProductVersion(path)
	if err != nil {
		return identity, err
	}
	identity.Version = version
	if version != pinnedOpenConsoleVersion {
		return identity, fmt.Errorf("pinned OpenConsole version mismatch: got %q want %q", version, pinnedOpenConsoleVersion)
	}
	return identity, nil
}
