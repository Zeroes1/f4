//go:build !amd64

package editor

func colorerCPUSupportsPOPCNT() bool {
	return true
}
