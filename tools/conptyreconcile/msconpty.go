//go:build windows

// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of the pseudoconsole creation path of microsoft/terminal
// src/winconpty/winconpty.cpp together with the two helpers it calls,
// src/server/DeviceHandle.cpp (_CreateHandle, CreateServerHandle,
// CreateClientHandle), at tag v1.12.10982.0 -- the version pinned in
// docs/PINNED_CONSOLE.md and the version of the OpenConsole.exe shipped
// beside this executable.
//
// Why this exists at all. kernel32's CreatePseudoConsole always starts the
// inbox conhost.exe of whatever Windows the probe happens to run on, so every
// measurement made through it describes that machine and nothing else. The
// pinned host has to be the one we bundle, or the measurements answer a
// question nobody asked. winconpty.cpp already contains exactly this logic:
// _ConsoleHostPath() prefers an OpenConsole.exe sitting next to the module and
// falls back to the inbox conhost, and the host is started with
// "--headless --width W --height H --signal 0xS --server 0xV" over a ConDrv
// server handle. Ported, that makes the bundled host the default here too,
// with no flag to remember and nothing to install.
//
// Mechanical transformations required by Go, and nothing else:
//   - wil::unique_handle -> explicit CloseHandle on the failure paths
//   - the __INSIDE_WINDOWS branch of _ConsoleHostPath is not taken: this is
//     not a Windows-internal build, so the OpenConsole-beside-the-module
//     branch is the live one, exactly as it is for Windows Terminal
//   - the BUILD_WOW6432 filesystem-redirection scope is not taken (no 32-bit
//     build of this probe exists)
//   - CreateProcessAsUserW branch: the probe never passes a token, so only
//     the CreateProcessW branch is ported; hToken is absent from the signature
//     rather than accepted and ignored
//
// Recorded non-ports: PSEUDOCONSOLE_INHERIT_CURSOR / _WIN32_INPUT_MODE /
// _RESIZE_QUIRK flags (the probe passes none, and the format string's
// corresponding fragments are empty in that case), ClosePseudoConsole's
// wait-for-exit path beyond killing the process, and the PTY reference
// handle's use by console API clients -- it is created because the original
// creates it and the host expects it, but this probe never calls console APIs
// through it.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	ntdll                     = syscall.NewLazyDLL("ntdll.dll")
	procNtOpenFile            = ntdll.NewProc("NtOpenFile")
	procRtlNtStatusToDosError = ntdll.NewProc("RtlNtStatusToDosError")
)

type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

type objectAttributes struct {
	Length                   uint32
	RootDirectory            syscall.Handle
	ObjectName               *unicodeString
	Attributes               uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}

type ioStatusBlock struct {
	Status      uintptr
	Information uintptr
}

const (
	objCaseInsensitive = 0x00000040
	objInherit         = 0x00000002

	fileShareAll                = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE
	fileSynchronousIoNonalert   = 0x00000020
	genericAll                  = 0x10000000
	genericReadWriteSynchronize = syscall.GENERIC_READ | syscall.GENERIC_WRITE | syscall.SYNCHRONIZE
)

// DeviceHandle::_CreateHandle
func deviceCreateHandle(deviceName string, desiredAccess uint32, parent syscall.Handle,
	inheritable bool, openOptions uint32) (syscall.Handle, error) {

	flags := uint32(objCaseInsensitive)
	if inheritable {
		flags |= objInherit
	}

	buf, err := syscall.UTF16FromString(deviceName)
	if err != nil {
		return 0, err
	}
	// UNICODE_STRING counts bytes and excludes the terminator from Length.
	name := unicodeString{
		Length:        uint16((len(buf) - 1) * 2),
		MaximumLength: uint16(len(buf) * 2),
		Buffer:        &buf[0],
	}

	oa := objectAttributes{
		Length:        uint32(unsafe.Sizeof(objectAttributes{})),
		RootDirectory: parent,
		ObjectName:    &name,
		Attributes:    flags,
	}

	var iosb ioStatusBlock
	var h syscall.Handle
	st, _, _ := procNtOpenFile.Call(
		uintptr(unsafe.Pointer(&h)),
		uintptr(desiredAccess),
		uintptr(unsafe.Pointer(&oa)),
		uintptr(unsafe.Pointer(&iosb)),
		uintptr(fileShareAll),
		uintptr(openOptions))
	if st != 0 {
		code, _, _ := procRtlNtStatusToDosError.Call(st)
		return 0, fmt.Errorf("NtOpenFile(%s) NTSTATUS 0x%08x: %w",
			deviceName, uint32(st), syscall.Errno(code))
	}
	return h, nil
}

// DeviceHandle::CreateServerHandle
func createServerHandle(inheritable bool) (syscall.Handle, error) {
	return deviceCreateHandle(`\Device\ConDrv\Server`, genericAll, 0, inheritable, 0)
}

// DeviceHandle::CreateClientHandle
func createClientHandle(server syscall.Handle, name string, inheritable bool) (syscall.Handle, error) {
	return deviceCreateHandle(name, genericReadWriteSynchronize, server, inheritable,
		fileSynchronousIoNonalert)
}

// _InboxConsoleHostPath
func inboxConsoleHostPath() string {
	return `\\?\` + systemDirectory() + `\conhost.exe`
}

// _ConsoleHostPath: "Returns the path to either conhost.exe or the
// side-by-side OpenConsole, depending on whether this module is building with
// Windows and OpenConsole could be found."
func consoleHostPath() string {
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "OpenConsole.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return inboxConsoleHostPath()
}

// bundledHostInUse reports whether the bundled OpenConsole was found, for the
// log header. Not part of the original.
func bundledHostInUse() bool {
	return filepath.Base(consoleHostPath()) == "OpenConsole.exe"
}

type pseudoConsole struct {
	hSignal        syscall.Handle
	hPtyReference  syscall.Handle
	hConPtyProcess syscall.Handle
}

// _CreatePseudoConsole
func createPseudoConsoleViaHost(width, height int, hInput, hOutput syscall.Handle) (*pseudoConsole, error) {
	if width == 0 || height == 0 {
		return nil, fmt.Errorf("invalid size %dx%d", width, height)
	}

	serverHandle, err := createServerHandle(true)
	if err != nil {
		return nil, err
	}

	var signalPipeConhostSide, signalPipeOurSide syscall.Handle
	sa := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{}))}
	// "Mark inheritable for signal handle when creating. It'll have the same
	// value on the other side."
	sa.InheritHandle = 0
	if err := syscall.CreatePipe(&signalPipeConhostSide, &signalPipeOurSide, &sa, 0); err != nil {
		syscall.CloseHandle(serverHandle)
		return nil, err
	}
	if err := syscall.SetHandleInformation(signalPipeConhostSide,
		syscall.HANDLE_FLAG_INHERIT, syscall.HANDLE_FLAG_INHERIT); err != nil {
		syscall.CloseHandle(serverHandle)
		return nil, err
	}

	hostPath := consoleHostPath()
	// GH4061: the path is quoted so C:\Program.exe cannot collide with
	// C:\Program Files.
	cmd := fmt.Sprintf(`"%s" --headless --width %d --height %d --signal 0x%x --server 0x%x`,
		hostPath, uint16(width), uint16(height), uintptr(signalPipeConhostSide), uintptr(serverHandle))

	// Only pass the handles we actually want the conhost to know about to it.
	inherited := []syscall.Handle{serverHandle, hInput, hOutput, signalPipeConhostSide}

	var listSize uintptr
	procInitAttrList.Call(0, 1, 0, uintptr(unsafe.Pointer(&listSize)))
	attrList := make([]byte, listSize)
	if r, _, e := procInitAttrList.Call(uintptr(unsafe.Pointer(&attrList[0])), 1, 0,
		uintptr(unsafe.Pointer(&listSize))); r == 0 {
		syscall.CloseHandle(serverHandle)
		return nil, fmt.Errorf("InitializeProcThreadAttributeList: %w", e)
	}
	defer procDeleteAttrList.Call(uintptr(unsafe.Pointer(&attrList[0])))

	if r, _, e := procUpdateAttr.Call(uintptr(unsafe.Pointer(&attrList[0])), 0,
		procThreadAttributeHandleList,
		uintptr(unsafe.Pointer(&inherited[0])),
		uintptr(len(inherited))*unsafe.Sizeof(syscall.Handle(0)), 0, 0); r == 0 {
		syscall.CloseHandle(serverHandle)
		return nil, fmt.Errorf("UpdateProcThreadAttribute: %w", e)
	}

	var siEx conptyStartupInfoEx
	siEx.Cb = uint32(unsafe.Sizeof(siEx))
	siEx.StdInput = hInput
	siEx.StdOutput = hOutput
	siEx.StdErr = hOutput
	siEx.Flags |= syscall.STARTF_USESTDHANDLES
	siEx.AttributeList = uintptr(unsafe.Pointer(&attrList[0]))

	appName, err := syscall.UTF16PtrFromString(hostPath)
	if err != nil {
		syscall.CloseHandle(serverHandle)
		return nil, err
	}
	cmdLine, err := syscall.UTF16PtrFromString(cmd)
	if err != nil {
		syscall.CloseHandle(serverHandle)
		return nil, err
	}

	var pi syscall.ProcessInformation
	if err := syscall.CreateProcess(appName, cmdLine, nil, nil, true,
		extendedStartupInfoPresent, nil, nil, &siEx.StartupInfo, &pi); err != nil {
		syscall.CloseHandle(serverHandle)
		return nil, fmt.Errorf("CreateProcess(%s): %w", hostPath, err)
	}
	syscall.CloseHandle(pi.Thread)

	pty := &pseudoConsole{hConPtyProcess: pi.Process}

	ref, err := createClientHandle(serverHandle, `\Reference`, false)
	if err != nil {
		syscall.CloseHandle(serverHandle)
		return nil, err
	}
	pty.hPtyReference = ref
	pty.hSignal = signalPipeOurSide

	syscall.CloseHandle(serverHandle)
	syscall.CloseHandle(signalPipeConhostSide)
	return pty, nil
}

// ResizePseudoConsole: the signal packet the original writes down the signal
// pipe (winconpty.cpp, ConptyResizePseudoConsole).
func (p *pseudoConsole) resize(width, height int) error {
	packet := struct {
		code uint16
		x    uint16
		y    uint16
	}{PTY_SIGNAL_RESIZE_WINDOW, uint16(width), uint16(height)}
	var written uint32
	buf := (*[6]byte)(unsafe.Pointer(&packet))[:]
	return syscall.WriteFile(p.hSignal, buf, &written, nil)
}

const PTY_SIGNAL_RESIZE_WINDOW = 8

// ClosePseudoConsole, reduced to what this probe needs (see the header).
func (p *pseudoConsole) close() {
	if p.hSignal != 0 {
		syscall.CloseHandle(p.hSignal)
	}
	if p.hPtyReference != 0 {
		syscall.CloseHandle(p.hPtyReference)
	}
	if p.hConPtyProcess != 0 {
		syscall.CloseHandle(p.hConPtyProcess)
	}
}

const procThreadAttributeHandleList = 0x00020002

type conptyStartupInfoEx struct {
	syscall.StartupInfo
	AttributeList uintptr
}

// GetSystemDirectoryW, which Go's syscall package does not wrap.
var procGetSystemDirectoryW = kernel32.NewProc("GetSystemDirectoryW")

func systemDirectory() string {
	buf := make([]uint16, 260)
	n, _, _ := procGetSystemDirectoryW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || int(n) > len(buf) {
		return `C:\Windows\System32`
	}
	return syscall.UTF16ToString(buf[:n])
}

// hostKind names the host in the log header: measurements made against the
// inbox conhost describe the machine they ran on, not the host f4 ships, and
// a log that does not say which one it used cannot be trusted later.
func hostKind() string {
	if bundledHostInUse() {
		return "bundled OpenConsole " + fileVersionOf(consoleHostPath())
	}
	return "INBOX conhost -- NOT the pinned host; drop OpenConsole.exe beside this exe"
}
