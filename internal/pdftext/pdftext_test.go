package pdftext

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func stream(dict, body string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(body), body)
}

func flate(t *testing.T, s string) string {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// build lays objects out as numbered indirect objects with a trailer.
func build(objs map[int]string, trailer string) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.5\n")
	for n := 1; n <= len(objs); n++ {
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, objs[n])
	}
	fmt.Fprintf(&b, "trailer\n%s\n%%%%EOF\n", trailer)
	return []byte(b.String())
}

func extract(t *testing.T, data []byte) (*Result, error) {
	t.Helper()
	return Extract(bytes.NewReader(data), int64(len(data)))
}

func simplePDF(t *testing.T, content string, compress bool) []byte {
	t.Helper()
	filter := ""
	if compress {
		content = flate(t, content)
		filter = "/Filter /FlateDecode"
	}
	return build(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 2 /Resources << /Font << /F1 5 0 R >> >> >>",
		3: "<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>",
		4: stream(filter, content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
		6: "<< /Type /Page /Parent 2 0 R /Contents [7 0 R] >>",
		7: stream("", "BT /F1 12 Tf (Second \\(page\\)) Tj ET"),
	}, "<< /Root 1 0 R /Size 8 >>")
}

func TestExtractSimplePages(t *testing.T) {
	content := "BT /F1 12 Tf 72 700 Td (Hello, World) Tj 0 -14 Td [(Foo) -300 (bar) 20 (baz)] TJ T* (caf\\351 \\223q\\224) Tj ET"
	for _, compress := range []bool{false, true} {
		res, err := extract(t, simplePDF(t, content, compress))
		if err != nil {
			t.Fatalf("compress=%v: %v", compress, err)
		}
		if len(res.Pages) != 2 {
			t.Fatalf("pages = %d", len(res.Pages))
		}
		want := "Hello, World\nFoo barbaz\ncafé “q”"
		if res.Pages[0] != want {
			t.Errorf("compress=%v page 1 = %q, want %q", compress, res.Pages[0], want)
		}
		if res.Pages[1] != "Second (page)" {
			t.Errorf("page 2 = %q", res.Pages[1])
		}
	}
}

func TestExtractObjectStreamAndToUnicode(t *testing.T) {
	// The catalog, page tree and page live in an object stream; the font is
	// Type0 with a ToUnicode CMap, codes of two bytes.
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
	}
	var body strings.Builder
	var header strings.Builder
	for i, o := range objs {
		fmt.Fprintf(&header, "%d %d ", i+1, body.Len())
		body.WriteString(o + " ")
	}
	objStm := header.String() + body.String()
	cmap := "/CIDInit begincmap 1 begincodespacerange <0000> <FFFF> endcodespacerange " +
		"2 beginbfchar <0001> <0048> <0002> <0069> endbfchar " +
		"1 beginbfrange <0010> <0012> <0061> endbfrange endcmap"
	comp := flate(t, objStm)
	data := []byte("%PDF-1.5\n" +
		"6 0 obj\n" + fmt.Sprintf("<< /Type /ObjStm /N 3 /First %d /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream", header.Len(), len(comp), comp) + "\nendobj\n" +
		"4 0 obj\n" + stream("", "BT /F1 10 Tf <00010002> Tj <001000110012> Tj ET") + "\nendobj\n" +
		"7 0 obj\n" + stream("", cmap) + "\nendobj\n" +
		"5 0 obj\n<< /Type /Font /Subtype /Type0 /BaseFont /X /Encoding /Identity-H /ToUnicode 7 0 R >>\nendobj\n" +
		"trailer\n<< /Root 1 0 R >>\n")
	res, err := extract(t, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pages) != 1 || res.Pages[0] != "Hiabc" {
		t.Fatalf("pages = %q", res.Pages)
	}
	if res.Lost != 0 {
		t.Errorf("Lost = %d", res.Lost)
	}
}

func TestExtractFormXObjectAndLostGlyphs(t *testing.T) {
	data := build(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /XObject << /Fm0 5 0 R >> /Font << /G 6 0 R >> >> >>",
		4: stream("", "/Fm0 Do BT /G 9 Tf <00410042> Tj ET"),
		5: stream("/Type /XObject /Subtype /Form /Resources << /Font << /F1 7 0 R >> >>", "BT /F1 9 Tf (inside form) Tj ET"),
		6: "<< /Type /Font /Subtype /Type0 /Encoding /Identity-H >>",
		7: "<< /Type /Font /Subtype /Type1 >>",
	}, "<< /Root 1 0 R >>")
	res, err := extract(t, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Pages[0], "inside form") {
		t.Errorf("form text missing: %q", res.Pages[0])
	}
	if res.Lost != 2 || !strings.Contains(res.Pages[0], "\uFFFD\uFFFD") {
		t.Errorf("Lost = %d, page %q", res.Lost, res.Pages[0])
	}
}

func TestExtractRefusals(t *testing.T) {
	if _, err := extract(t, []byte("just some text")); !errors.Is(err, ErrNotPDF) {
		t.Errorf("text: %v", err)
	}
	enc := build(map[int]string{1: "<< /Type /Catalog >>"}, "<< /Root 1 0 R /Encrypt 9 0 R >>")
	if _, err := extract(t, enc); !errors.Is(err, ErrEncrypted) {
		t.Errorf("encrypted: %v", err)
	}
	if _, err := extract(t, []byte("%PDF-1.4\n")); err == nil {
		t.Error("a PDF with no catalog was accepted")
	}
	if _, err := Extract(bytes.NewReader(nil), -1); err == nil {
		t.Error("a negative size was accepted")
	}
}

func TestExtractSurvivesDamage(t *testing.T) {
	src := simplePDF(t, "BT /F1 12 Tf (Hello) Tj ET", true)
	for cut := 0; cut < len(src); cut += 3 {
		_, _ = extract(t, src[:cut])
	}
	for i := 0; i < len(src); i += 2 {
		damaged := append([]byte(nil), src...)
		damaged[i] ^= 0x55
		_, _ = extract(t, damaged)
	}
	// Hostile nesting and a self-referencing page tree end without a hang.
	deep := "%PDF-1.4\n1 0 obj\n" + strings.Repeat("[", 5000) + "\nendobj\ntrailer\n<< /Root 1 0 R >>"
	_, _ = extract(t, []byte(deep))
	loop := build(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [2 0 R 2 0 R] >>",
	}, "<< /Root 1 0 R >>")
	res, err := extract(t, loop)
	if err != nil || len(res.Pages) != 0 {
		t.Errorf("loop: %v %v", res, err)
	}
}

func TestFiltersAndParserCorners(t *testing.T) {
	if got := asciiHex([]byte("48 65 6C6c 6F7>")); string(got) != "Hello\x70" {
		t.Errorf("asciiHex = %q", got)
	}
	got, err := ascii85([]byte("9jqo^~>"))
	if err != nil || string(got) != "Man " {
		t.Errorf("ascii85 = %q, %v", got, err)
	}
	if got, err := ascii85([]byte("z~>")); err != nil || !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Errorf("ascii85 z = %v, %v", got, err)
	}
	if _, err := ascii85([]byte("\x01")); err == nil {
		t.Error("bad ASCII85 accepted")
	}
	ps := &parser{b: []byte(`<< /A#20B (x\101\n) /C <4a4> /D [1 -2.5 (n(e)st)] /E 12 0 R >> true`), refs: true}
	v, err := ps.value()
	if err != nil {
		t.Fatal(err)
	}
	dict := v.(Dict)
	if string(dict["A B"].(Str)) != "xA\n" || string(dict["C"].(Str)) != "J@" {
		t.Errorf("dict = %#v", dict)
	}
	if arr := dict["D"].(Array); len(arr) != 3 || arr[1].(float64) != -2.5 || string(arr[2].(Str)) != "n(e)st" {
		t.Errorf("array = %#v", arr)
	}
	if dict["E"] != (Ref{Num: 12}) {
		t.Errorf("ref = %#v", dict["E"])
	}
	if f := fenceFor("a ``` b"); f != "````" {
		t.Errorf("fence = %q", f)
	}
	if utf16String(Str{0xD8, 0x3D, 0xDE, 0x00}) != "😀" {
		t.Error("surrogate pair not decoded")
	}
}

func TestReport(t *testing.T) {
	res := &Result{Pages: []string{"one ``` two", ""}, Lost: 1, Truncated: true}
	text := Report(res, "a`b.pdf")
	for _, want := range []string{"`a'b.pdf`", "````\none ``` two\n````"} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}
}
