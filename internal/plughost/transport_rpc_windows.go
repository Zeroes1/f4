package plughost

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PlugRing entrypoints are platform-neutral, but Windows requires .exe when
// resolving the process image. Repair the extensionless PE files shipped by
// older plugin archives as well as accepting correctly packaged .exe files.
func prepareRPCExecutable(path string) (string, error) {
	if filepath.Ext(path) != "" {
		return path, nil
	}
	exePath := path + ".exe"
	if info, err := os.Stat(exePath); err == nil && !info.IsDir() {
		return exePath, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	var magic [2]byte
	_, readErr := io.ReadFull(file, magic[:])
	closeErr := file.Close()
	if readErr != nil || magic != [2]byte{'M', 'Z'} {
		return path, nil
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Rename(path, exePath); err != nil {
		return "", fmt.Errorf("add .exe extension to %s: %w", path, err)
	}
	return exePath, nil
}
