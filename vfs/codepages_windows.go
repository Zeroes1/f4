//go:build windows
// +build windows

package vfs

import (
	"fmt"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

const cpSupported = 2

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	enumSystemCodePagesW = kernel32.NewProc("EnumSystemCodePagesW")
	getCPInfoExW         = kernel32.NewProc("GetCPInfoExW")
	getACP               = kernel32.NewProc("GetACP")
	getOEMCP             = kernel32.NewProc("GetOEMCP")
	wideCharToMultiByte  = kernel32.NewProc("WideCharToMultiByte")
)

// CPINFOEXW is the Win32 structure returned by GetCPInfoExW. Keeping this
// small declaration here avoids depending on a generated API package for the
// codepage enumeration path.
type cpInfoExW struct {
	MaxCharSize        uint32
	DefaultChar        [2]byte
	LeadByte           [12]byte
	UnicodeDefaultChar [2]uint16
	CodePage           uint32
	CodePageName       [260]uint16
}

func platformCodepages() []Codepage {
	var result []Codepage
	callback := windows.NewCallback(func(name *uint16) uintptr {
		cp, err := parseWindowsCodepage(name)
		if err != nil || cp == 0 {
			return 1
		}
		if _, exists := FindCodepage(cp); exists {
			return 1
		}

		info, ok := getWindowsCodepageInfo(cp)
		if !ok {
			return 1
		}
		result = append(result, Codepage{
			ID:    cp,
			Name:  windowsCodepageName(info, cp),
			Enc:   windowsCodepageEncoding(cp),
			group: codepageOther,
		})
		return 1
	})

	if resultCode, _, _ := enumSystemCodePagesW.Call(callback, cpSupported); resultCode == 0 {
		return nil
	}
	return result
}

func parseWindowsCodepage(name *uint16) (int, error) {
	text := windows.UTF16PtrToString(name)
	var cp int
	if _, err := fmt.Sscanf(text, "%d", &cp); err != nil {
		return 0, err
	}
	return cp, nil
}

func getWindowsCodepageInfo(cp int) (cpInfoExW, bool) {
	var info cpInfoExW
	result, _, _ := getCPInfoExW.Call(uintptr(cp), 0, uintptr(unsafe.Pointer(&info)))
	return info, result != 0
}

func windowsCodepageName(info cpInfoExW, cp int) string {
	name := windows.UTF16ToString(info.CodePageName[:])
	if name == "" {
		return fmt.Sprintf("Windows codepage %d", cp)
	}
	return name
}

func windowsSystemCodepage(proc *windows.LazyProc) int {
	cp, _, _ := proc.Call()
	return int(cp)
}

func systemCodepageIDs() (int, int) {
	ansi := windowsSystemCodepage(getACP)
	oem := windowsSystemCodepage(getOEMCP)
	return ansi, oem
}

func systemCodepageNames() (string, string) {
	return fmt.Sprintf("System ANSI (%d)", systemANSI), fmt.Sprintf("System OEM (%d)", systemOEM)
}

type windowsCodepage struct {
	cp uint32
}

func windowsCodepageEncoding(cp int) encoding.Encoding {
	return windowsCodepage{cp: uint32(cp)}
}

func (e windowsCodepage) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: windowsCodepageTransformer{cp: e.cp}}
}

func (e windowsCodepage) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: windowsCodepageTransformer{cp: e.cp, encode: true}}
}

type windowsCodepageTransformer struct {
	cp     uint32
	encode bool
}

func (windowsCodepageTransformer) Reset() {}

func (t windowsCodepageTransformer) Transform(dst, src []byte, atEOF bool) (int, int, error) {
	if len(src) == 0 {
		return 0, 0, nil
	}
	if t.encode {
		return t.encodeBytes(dst, src)
	}
	return t.decodeBytes(dst, src)
}

func (t windowsCodepageTransformer) decodeBytes(dst, src []byte) (int, int, error) {
	wideLen, err := windows.MultiByteToWideChar(t.cp, 0, &src[0], int32(len(src)), nil, 0)
	if err != nil {
		return 0, 0, err
	}
	wide := make([]uint16, wideLen)
	if _, err = windows.MultiByteToWideChar(t.cp, 0, &src[0], int32(len(src)), &wide[0], wideLen); err != nil {
		return 0, 0, err
	}
	out := []byte(string(utf16.Decode(wide)))
	if len(out) > len(dst) {
		return 0, 0, transform.ErrShortDst
	}
	return copy(dst, out), len(src), nil
}

func (t windowsCodepageTransformer) encodeBytes(dst, src []byte) (int, int, error) {
	wide := utf16.Encode([]rune(string(src)))
	defaultChar := byte('?')
	usedDefaultChar := int32(0)
	outLen, err := callWideCharToMultiByte(t.cp, wide, nil, nil, nil)
	if err != nil {
		return 0, 0, err
	}
	out := make([]byte, outLen)
	outLen, err = callWideCharToMultiByte(t.cp, wide, &defaultChar, &usedDefaultChar, out)
	if err != nil {
		return 0, 0, err
	}
	if usedDefaultChar != 0 {
		return 0, 0, fmt.Errorf("character cannot be represented in Windows codepage %d", t.cp)
	}
	if outLen > len(dst) {
		return 0, 0, transform.ErrShortDst
	}
	return copy(dst, out[:outLen]), len(src), nil
}

func callWideCharToMultiByte(cp uint32, wide []uint16, defaultChar *byte, usedDefaultChar *int32, dst []byte) (int, error) {
	var widePtr uintptr
	if len(wide) > 0 {
		widePtr = uintptr(unsafe.Pointer(&wide[0]))
	}
	var defaultPtr uintptr
	if defaultChar != nil {
		defaultPtr = uintptr(unsafe.Pointer(defaultChar))
	}
	var usedPtr uintptr
	if usedDefaultChar != nil {
		usedPtr = uintptr(unsafe.Pointer(usedDefaultChar))
	}
	var dstPtr uintptr
	if len(dst) > 0 {
		dstPtr = uintptr(unsafe.Pointer(&dst[0]))
	}
	result, _, err := wideCharToMultiByte.Call(
		uintptr(cp), 0, widePtr, uintptr(len(wide)), dstPtr, uintptr(len(dst)), defaultPtr, usedPtr,
	)
	if result == 0 {
		return 0, err
	}
	return int(result), nil
}
