//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// The Windows half of layer 1: a Toolhelp snapshot. It is the only part of the
// detector that cannot be tested without Windows, which is why it is this
// small and does nothing but read.

var (
	kernel32Proc         = syscall.NewLazyDLL("kernel32.dll")
	procCreateToolhelp32 = kernel32Proc.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW  = kernel32Proc.NewProc("Process32FirstW")
	procProcess32NextW   = kernel32Proc.NewProc("Process32NextW")
)

const th32csSnapProcess = 0x00000002

type processEntry32 struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

// SnapshotLister implements ProcessLister against the live system.
type SnapshotLister struct{}

func (SnapshotLister) List() ([]ProcessInfo, error) {
	snap, _, err := procCreateToolhelp32.Call(uintptr(th32csSnapProcess), 0)
	if snap == 0 || snap == uintptr(syscall.InvalidHandle) {
		return nil, fmt.Errorf("CreateToolhelp32Snapshot: %v", err)
	}
	defer syscall.CloseHandle(syscall.Handle(snap))

	var e processEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	out := make([]ProcessInfo, 0, 256)
	ret, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&e)))
	for ret != 0 {
		out = append(out, ProcessInfo{
			PID:    e.ProcessID,
			Parent: e.ParentProcessID,
			Name:   syscall.UTF16ToString(e.ExeFile[:]),
		})
		ret, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&e)))
	}
	return out, nil
}
