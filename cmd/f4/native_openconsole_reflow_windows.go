//go:build windows

package main

import "os"

func terminalReflowDefaultEnabled() bool {
	return os.Getenv("F4_NATIVE_OPENCONSOLE") == "1"
}
