//go:build windows

package vfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// procRegSetValueEx writes a value of any type with its exact bytes; the
// registry package only offers setters for the common types.
var procRegSetValueEx = windows.NewLazySystemDLL("advapi32.dll").NewProc("RegSetValueExW")

// applyRegFile writes the keys of a .reg file into the registry the way
// regedit imports one (f4#242): every key is created if missing and its
// values are set; "[-key]" removes the key with its subkeys and "name"=-
// removes a value. Keys and values the file does not mention are left alone.
// It stops at the first error; what was applied before it stays.
func applyRegFile(ctx context.Context, keys []regImportKey) error {
	for _, k := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		hive, sub, err := regImportTarget(k.Path)
		if err != nil {
			return err
		}
		if k.Delete {
			if err := deleteRegistryTree(hive, sub); err != nil && !errors.Is(err, registry.ErrNotExist) {
				return fmt.Errorf("[-%s]: %w", k.Path, err)
			}
			continue
		}
		key, _, err := registry.CreateKey(hive, sub, registry.SET_VALUE|registry.QUERY_VALUE)
		if err != nil {
			return fmt.Errorf("[%s]: %w", k.Path, err)
		}
		err = applyRegValues(key, k.Values)
		closeErr := key.Close()
		if err != nil {
			return fmt.Errorf("[%s]: %w", k.Path, err)
		}
		if closeErr != nil {
			return fmt.Errorf("[%s]: %w", k.Path, closeErr)
		}
	}
	return nil
}

func applyRegValues(key registry.Key, values []regImportValue) error {
	for _, v := range values {
		if v.Delete {
			if err := key.DeleteValue(v.Name); err != nil && !errors.Is(err, registry.ErrNotExist) {
				return fmt.Errorf("%q=-: %w", v.Name, err)
			}
			continue
		}
		if err := setRawRegistryValue(key, v.Name, v.Type, v.Data); err != nil {
			return fmt.Errorf("%q: %w", v.Name, err)
		}
	}
	return nil
}

// regImportTarget splits the path of a .reg key into its hive and the path
// below it. A hive itself is refused: neither a hive nor its removal is
// something a .reg edit may do.
func regImportTarget(path string) (registry.Key, string, error) {
	hiveName, sub, _ := strings.Cut(path, `\`)
	hive, ok := registryHiveByName(hiveName)
	if !ok {
		return 0, "", fmt.Errorf("[%s]: not a registry hive: %q", path, hiveName)
	}
	sub = strings.Trim(sub, `\`)
	if sub == "" {
		return 0, "", fmt.Errorf("[%s]: a whole hive cannot be changed from a .reg file", path)
	}
	return hive.key, sub, nil
}

// deleteRegistryTree removes the key at sub under parent with all its
// subkeys, deepest first, as RegDeleteKey removes only a key without any.
func deleteRegistryTree(parent registry.Key, sub string) error {
	key, err := registry.OpenKey(parent, sub, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	names, err := key.ReadSubKeyNames(-1)
	closeErr := key.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	for _, name := range names {
		if err := deleteRegistryTree(parent, sub+`\`+name); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
	}
	return registry.DeleteKey(parent, sub)
}

// setRawRegistryValue writes the value with its type and exact bytes.
func setRawRegistryValue(key registry.Key, name string, valueType uint32, data []byte) error {
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var buf *byte
	if len(data) > 0 {
		buf = &data[0]
	}
	r, _, _ := procRegSetValueEx.Call(uintptr(key), uintptr(unsafe.Pointer(p)), 0, uintptr(valueType),
		uintptr(unsafe.Pointer(buf)), uintptr(len(data)))
	if r != 0 {
		return os.NewSyscallError("RegSetValueEx", syscall.Errno(r))
	}
	return nil
}

var _ TransferContentImporter = (*RegistryVFS)(nil)

// ImportContent applies the .reg text a key was edited as (F4 on the key,
// f4#242), as a regedit import does. Every key of the text must be the
// edited key or below it: a section that names another branch is refused
// before anything is written.
func (v *RegistryVFS) ImportContent(ctx context.Context, p string, content []byte) error {
	segments, err := registryPathSegments(p)
	if err != nil {
		return err
	}
	if len(segments) < 2 {
		return fmt.Errorf("registry: a whole hive cannot be edited as .reg")
	}
	hive, ok := registryHiveByName(segments[0])
	if !ok {
		return os.ErrNotExist
	}
	root := strings.Join(append([]string{hive.name}, segments[1:]...), `\`)
	keys, err := decodeRegFile(content)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if !regPathWithin(k.Path, root) {
			return fmt.Errorf(".reg: [%s] is outside the edited key %s", k.Path, root)
		}
	}
	return applyRegFile(ctx, keys)
}

// regPathWithin reports whether path is root or a key below it; registry
// names compare without regard to case.
func regPathWithin(path, root string) bool {
	path, root = strings.ToLower(strings.Trim(path, `\`)), strings.ToLower(root)
	return path == root || strings.HasPrefix(path, root+`\`)
}
