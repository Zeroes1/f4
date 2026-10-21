//go:build windows

package vfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// A .reg file applied to the registry creates its keys, sets its values with
// their exact types and bytes, and removes what its "-" lines name, as a
// regedit import does (f4#242).
func TestApplyRegFileImportsLikeRegedit_Issue242(t *testing.T) {
	base := fmt.Sprintf(`Software\f4-reg-apply-%d-%d`, time.Now().UnixNano(), os.Getpid())
	t.Cleanup(func() { _ = deleteRegistryTree(registry.CURRENT_USER, base) })
	root := `HKEY_CURRENT_USER\` + base

	ctx := context.Background()
	first := "Windows Registry Editor Version 5.00\r\n\r\n" +
		"[" + root + "]\r\n" +
		"@=\"default\"\r\n" +
		"\"Text\"=\"a \\\"quoted\\\" C:\\\\path\"\r\n" +
		"\"Count\"=dword:0000002a\r\n" +
		"\"Big\"=hex(b):01,02,03,04,05,06,07,08\r\n" +
		"\"Gone\"=dword:00000001\r\n\r\n" +
		"[" + root + "\\Sub\\Deep]\r\n" +
		"\"x\"=hex:de,ad\r\n"
	keys, err := decodeRegFile([]byte(first))
	if err != nil {
		t.Fatal(err)
	}
	if err := applyRegFile(ctx, keys); err != nil {
		t.Fatalf("apply: %v", err)
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, base, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("the key was not created: %v", err)
	}
	if s, _, err := key.GetStringValue(""); err != nil || s != "default" {
		t.Errorf("default value = %q, %v", s, err)
	}
	if s, _, err := key.GetStringValue("Text"); err != nil || s != `a "quoted" C:\path` {
		t.Errorf("Text = %q, %v", s, err)
	}
	if n, _, err := key.GetIntegerValue("Count"); err != nil || n != 42 {
		t.Errorf("Count = %d, %v", n, err)
	}
	if n, typ, err := key.GetIntegerValue("Big"); err != nil || typ != registry.QWORD || n != 0x0807060504030201 {
		t.Errorf("Big = %#x type %d, %v", n, typ, err)
	}
	_ = key.Close()
	deep, err := registry.OpenKey(registry.CURRENT_USER, base+`\Sub\Deep`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("the nested key was not created: %v", err)
	}
	if b, _, err := deep.GetBinaryValue("x"); err != nil || !bytes.Equal(b, []byte{0xde, 0xad}) {
		t.Errorf("x = % x, %v", b, err)
	}
	_ = deep.Close()

	second := "Windows Registry Editor Version 5.00\r\n\r\n" +
		"[" + root + "]\r\n" +
		"\"Gone\"=-\r\n" +
		"\"Missing\"=-\r\n\r\n" +
		"[-" + root + "\\Sub]\r\n"
	keys, err = decodeRegFile([]byte(second))
	if err != nil {
		t.Fatal(err)
	}
	if err := applyRegFile(ctx, keys); err != nil {
		t.Fatalf("apply removals: %v", err)
	}
	key, err = registry.OpenKey(registry.CURRENT_USER, base, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := key.GetIntegerValue("Gone"); !errors.Is(err, registry.ErrNotExist) {
		t.Errorf("Gone after \"Gone\"=-: %v, want it removed", err)
	}
	if _, _, err := key.GetIntegerValue("Count"); err != nil {
		t.Errorf("Count, which the second file does not name, was touched: %v", err)
	}
	_ = key.Close()
	if k, err := registry.OpenKey(registry.CURRENT_USER, base+`\Sub`, registry.QUERY_VALUE); err == nil {
		_ = k.Close()
		t.Error("Sub with its subkey Deep is still there after [-...\\Sub]")
	}
}

func TestApplyRegFileRefusesAHive_Issue242(t *testing.T) {
	for _, path := range []string{`HKEY_CURRENT_USER`, `HKEY_NOWHERE\Software`} {
		err := applyRegFile(context.Background(), []regImportKey{{Path: path, Delete: true}})
		if err == nil {
			t.Errorf("[-%s] was accepted", path)
		}
	}
}
