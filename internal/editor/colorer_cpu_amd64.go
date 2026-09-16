//go:build amd64

package editor

import "golang.org/x/sys/cpu"

func colorerCPUSupportsPOPCNT() bool {
	return cpu.X86.HasPOPCNT
}
