//go:build windows

package main

// fileVersionOf and its VS_FIXEDFILEINFO plumbing are taken verbatim from
// this repository's tools/termprobe/conpty_windows.go, which already reads the
// host version this way. A log that does not record which console host it
// measured is worth very little later, which this project learned the hard
// way: an entire session of ports was made against the wrong conhost.

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	version                     = syscall.NewLazyDLL("version.dll")
	procGetFileVersionInfoSizeW = version.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = version.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = version.NewProc("VerQueryValueW")
)

type vsFixedFileInfo struct {
	Signature        uint32
	StrucVersion     uint32
	FileVersionMS    uint32
	FileVersionLS    uint32
	ProductVersionMS uint32
	ProductVersionLS uint32
	FileFlagsMask    uint32
	FileFlags        uint32
	FileOS           uint32
	FileType         uint32
	FileSubtype      uint32
	FileDateMS       uint32
	FileDateLS       uint32
}

func fileVersionOf(path string) string {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "?"
	}
	size, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 {
		return "no version resource"
	}
	buf := make([]byte, size)
	if r, _, _ := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return "unreadable"
	}
	sub, _ := syscall.UTF16PtrFromString(`\`)
	var info *vsFixedFileInfo
	var l uint32
	if r, _, _ := procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(sub)),
		uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&l))); r == 0 || info == nil {
		return "no fixed info"
	}
	return fmt.Sprintf("%d.%d.%d.%d",
		info.FileVersionMS>>16, info.FileVersionMS&0xffff,
		info.FileVersionLS>>16, info.FileVersionLS&0xffff)
}
