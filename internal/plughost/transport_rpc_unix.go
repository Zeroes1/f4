//go:build !windows

package plughost

func prepareRPCExecutable(path string) (string, error) {
	return path, nil
}
