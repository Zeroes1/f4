package vfs

import (
	"bytes"
	"strings"
	"testing"
)

// The .reg file a key is exported as reads back to the same keys and values:
// the way back for editing a key as .reg text (f4#242).
func TestDecodeRegFileReadsWhatEncodeWrites_Issue242(t *testing.T) {
	long := bytes.Repeat([]byte{0xab, 0x01, 0x7f}, 60)
	keys := []regExportKey{
		{Path: `HKEY_CURRENT_USER\Software\f4`, Values: []regExportValue{
			{Name: "", Type: regTypeSZ, Data: regUTF16("Привет\x00")},
			{Name: `Path "x" \ y`, Type: regTypeSZ, Data: regUTF16(`C:\Temp` + "\x00")},
			{Name: "Count", Type: regTypeDWORD, Data: []byte{0x2a, 0, 0, 0}},
			{Name: "Blob", Type: regTypeBinary, Data: long},
			{Name: "Empty", Type: regTypeBinary, Data: nil},
			{Name: "Expand", Type: regTypeExpandSZ, Data: regUTF16("%TEMP%\x00")},
			{Name: "Multi", Type: regTypeMultiSZ, Data: regUTF16("a\x00b\x00\x00")},
			{Name: "Big", Type: regTypeQWORD, Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}},
			{Name: "Lines", Type: regTypeSZ, Data: regUTF16("one\r\ntwo\x00")},
		}},
		{Path: `HKEY_CURRENT_USER\Software\f4\Sub key`},
	}
	got, err := decodeRegFile(encodeRegFile(keys))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(keys) {
		t.Fatalf("%d keys read back, want %d", len(got), len(keys))
	}
	for i, key := range keys {
		if got[i].Path != key.Path || got[i].Delete {
			t.Fatalf("key %d = %q (delete %v), want %q", i, got[i].Path, got[i].Delete, key.Path)
		}
		if len(got[i].Values) != len(key.Values) {
			t.Fatalf("key %q: %d values, want %d", key.Path, len(got[i].Values), len(key.Values))
		}
		for j, want := range key.Values {
			v := got[i].Values[j]
			if v.Name != want.Name || v.Type != want.Type || v.Delete || !bytes.Equal(v.Data, want.Data) {
				t.Errorf("value %q = {%q type %d % x}, want type %d % x", want.Name, v.Name, v.Type, v.Data, want.Type, want.Data)
			}
		}
	}
}

func TestDecodeRegFileRemovalsAndComments_Issue242(t *testing.T) {
	text := "\uFEFFWindows Registry Editor Version 5.00\r\n\r\n" +
		"; a comment\r\n" +
		"[-HKEY_CURRENT_USER\\Software\\gone]\r\n\r\n" +
		"[HKEY_CURRENT_USER\\Software\\f4]\r\n" +
		"\"old\"=-\r\n" +
		"\"n\" = dword:0000000A\r\n" +
		"@=hex:01,\\\r\n    02,03\r\n"
	got, err := decodeRegFile([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Delete || got[0].Path != `HKEY_CURRENT_USER\Software\gone` {
		t.Fatalf("first key = %+v, want the removal of ...\\gone", got)
	}
	values := got[1].Values
	if len(values) != 3 {
		t.Fatalf("values = %+v, want three", values)
	}
	if !values[0].Delete || values[0].Name != "old" {
		t.Errorf("removal = %+v", values[0])
	}
	if values[1].Type != regTypeDWORD || !bytes.Equal(values[1].Data, []byte{10, 0, 0, 0}) {
		t.Errorf("dword = %+v", values[1])
	}
	if values[2].Name != "" || values[2].Type != regTypeBinary || !bytes.Equal(values[2].Data, []byte{1, 2, 3}) {
		t.Errorf("default hex value = %+v", values[2])
	}
}

func TestDecodeRegFileRejectsWhatItCannotRead_Issue242(t *testing.T) {
	head := "Windows Registry Editor Version 5.00\r\n\r\n"
	for name, text := range map[string]string{
		"no header":         "[HKEY_CURRENT_USER\\x]\r\n",
		"REGEDIT4":          "REGEDIT4\r\n\r\n[HKEY_CURRENT_USER\\x]\r\n",
		"value outside key": head + "\"a\"=\"b\"\r\n",
		"bad dword":         head + "[HKEY_CURRENT_USER\\x]\r\n\"a\"=dword:xyz\r\n",
		"bad hex":           head + "[HKEY_CURRENT_USER\\x]\r\n\"a\"=hex:1,zz\r\n",
		"open quote":        head + "[HKEY_CURRENT_USER\\x]\r\n\"a=\"b\"\r\n",
		"open bracket":      head + "[HKEY_CURRENT_USER\\x\r\n",
		"unknown kind":      head + "[HKEY_CURRENT_USER\\x]\r\n\"a\"=str:b\r\n",
	} {
		if _, err := decodeRegFile([]byte(text)); err == nil {
			t.Errorf("%s: no error", name)
		} else if !strings.Contains(err.Error(), ".reg") {
			t.Errorf("%s: error %q does not say it is about the .reg file", name, err)
		}
	}
}
