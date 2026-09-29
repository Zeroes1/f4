package dotnet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

type builder struct{ bytes.Buffer }

func (b *builder) u16(v int) { _ = binary.Write(b, binary.LittleEndian, uint16(v)) }
func (b *builder) u32(v int) { _ = binary.Write(b, binary.LittleEndian, uint32(v)) }
func (b *builder) pad(to int) {
	for b.Len()%to != 0 {
		b.WriteByte(0)
	}
}

// heap builds a #Strings heap and hands out indexes into it.
type heap struct {
	data []byte
}

func (h *heap) add(s string) int {
	if len(h.data) == 0 {
		h.data = []byte{0}
	}
	at := len(h.data)
	h.data = append(h.data, s...)
	h.data = append(h.data, 0)
	return at
}

// sampleMetadata is a small but real metadata root: one module, three
// TypeDefs (<Module>, My.Ns.Foo, and Nested inside Foo), one assembly, one
// reference and one resource.
func sampleMetadata() []byte {
	var h heap
	h.add("")
	modName := h.add("Sample.dll")
	moduleType := h.add("<Module>")
	foo := h.add("Foo")
	ns := h.add("My.Ns")
	nested := h.add("Nested")
	asmName := h.add("Sample")
	refName := h.add("System.Runtime")
	resName := h.add("Sample.strings.resources")

	var t builder
	// Module: Generation, Name, Mvid, EncId, EncBaseId.
	t.u16(0)
	t.u16(modName)
	t.u16(0)
	t.u16(0)
	t.u16(0)
	// TypeDef x3: Flags, Name, Namespace, Extends, FieldList, MethodList.
	for _, row := range [][2]int{{moduleType, 0}, {foo, ns}, {nested, 0}} {
		t.u32(0)
		t.u16(row[0])
		t.u16(row[1])
		t.u16(0)
		t.u16(1)
		t.u16(1)
	}
	// Assembly: HashAlgId, version, Flags, PublicKey, Name, Culture.
	t.u32(0x8004)
	t.u16(1)
	t.u16(2)
	t.u16(3)
	t.u16(4)
	t.u32(0)
	t.u16(0)
	t.u16(asmName)
	t.u16(0)
	// AssemblyRef: version, Flags, PublicKeyOrToken, Name, Culture, HashValue.
	t.u16(8)
	t.u16(0)
	t.u16(0)
	t.u16(0)
	t.u32(0)
	t.u16(0)
	t.u16(refName)
	t.u16(0)
	t.u16(0)
	// ManifestResource: Offset, Flags, Name, Implementation.
	t.u32(0)
	t.u32(1)
	t.u16(resName)
	t.u16(0)
	// NestedClass: NestedClass, EnclosingClass.
	t.u16(3)
	t.u16(2)

	var s builder
	s.u32(0)
	s.WriteByte(2)
	s.WriteByte(0)
	s.WriteByte(0)
	s.WriteByte(1)
	valid := uint64(1<<0 | 1<<2 | 1<<0x20 | 1<<0x23 | 1<<0x28 | 1<<0x29)
	_ = binary.Write(&s, binary.LittleEndian, valid)
	_ = binary.Write(&s, binary.LittleEndian, uint64(0))
	for _, n := range []int{1, 3, 1, 1, 1, 1} {
		s.u32(n)
	}
	s.Write(t.Bytes())
	stream := s.Bytes()

	const version = "v4.0.30319\x00\x00"
	head := 16 + len(version) + 4
	dir1 := 8 + 4
	dir2 := 8 + 12
	tableOff := head + dir1 + dir2
	strOff := tableOff + len(stream)

	var r builder
	r.u32(0x424A5342)
	r.u16(1)
	r.u16(1)
	r.u32(0)
	r.u32(len(version))
	r.WriteString(version)
	r.u16(0)
	r.u16(2)
	r.u32(tableOff)
	r.u32(len(stream))
	r.WriteString("#~\x00\x00")
	r.u32(strOff)
	r.u32(len(h.data))
	r.WriteString("#Strings\x00\x00\x00\x00")
	r.Write(stream)
	r.Write(h.data)
	return r.Bytes()
}

// wrapPE puts metadata into a minimal PE32 image with a CLR header.
func wrapPE(metadata []byte, clr bool) []byte {
	const (
		peOff    = 0x40
		optOff   = peOff + 4 + 20
		optSize  = 224
		secOff   = optOff + optSize
		rawOff   = 0x200
		virtAddr = 0x2000
		corSize  = 72
		metaOff  = corSize
	)
	image := make([]byte, rawOff+corSize+len(metadata))
	le := binary.LittleEndian
	image[0], image[1] = 'M', 'Z'
	le.PutUint32(image[0x3c:], peOff)
	copy(image[peOff:], "PE\x00\x00")
	le.PutUint16(image[peOff+4:], 0x14c)
	le.PutUint16(image[peOff+6:], 1)
	le.PutUint16(image[peOff+20:], optSize)
	le.PutUint16(image[peOff+22:], 0x2102)
	le.PutUint16(image[optOff:], 0x10b)
	le.PutUint32(image[optOff+92:], 16)
	if clr {
		le.PutUint32(image[optOff+96+14*8:], virtAddr)
		le.PutUint32(image[optOff+96+14*8+4:], corSize)
	}
	copy(image[secOff:], ".text")
	le.PutUint32(image[secOff+8:], uint32(corSize+len(metadata)))
	le.PutUint32(image[secOff+12:], virtAddr)
	le.PutUint32(image[secOff+16:], uint32(corSize+len(metadata)))
	le.PutUint32(image[secOff+20:], rawOff)
	le.PutUint32(image[rawOff:], corSize)
	le.PutUint32(image[rawOff+8:], virtAddr+metaOff)
	le.PutUint32(image[rawOff+12:], uint32(len(metadata)))
	copy(image[rawOff+corSize:], metadata)
	return image
}

func TestReadAssembly(t *testing.T) {
	image := wrapPE(sampleMetadata(), true)
	info, err := Read(bytes.NewReader(image), int64(len(image)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if info.RuntimeVersion != "v4.0.30319" {
		t.Errorf("RuntimeVersion = %q", info.RuntimeVersion)
	}
	if info.Name != "Sample" || info.Version.String() != "1.2.3.4" {
		t.Errorf("assembly = %q %s", info.Name, info.Version)
	}
	if len(info.References) != 1 || info.References[0].Name != "System.Runtime" || info.References[0].Version.String() != "8.0.0.0" {
		t.Errorf("References = %+v", info.References)
	}
	// <Module> and the nested type are left out.
	if info.TypeCount != 1 || len(info.Types["My.Ns"]) != 1 || info.Types["My.Ns"][0] != "Foo" {
		t.Errorf("Types = %v (%d)", info.Types, info.TypeCount)
	}
	if got := info.Namespaces(); len(got) != 1 || got[0] != "My.Ns" {
		t.Errorf("Namespaces = %v", got)
	}
	if len(info.Resources) != 1 || info.Resources[0] != "Sample.strings.resources" {
		t.Errorf("Resources = %v", info.Resources)
	}
}

func TestReadRejectsWhatIsNotAnAssembly(t *testing.T) {
	native := wrapPE(sampleMetadata(), false)
	for name, data := range map[string][]byte{
		"text":   []byte("hello, world"),
		"native": native,
		"empty":  {},
	} {
		if _, err := Read(bytes.NewReader(data), int64(len(data))); !errors.Is(err, ErrNotAssembly) {
			t.Errorf("%s: err = %v, want ErrNotAssembly", name, err)
		}
	}
}

func TestReadSurvivesDamage(t *testing.T) {
	meta := sampleMetadata()
	// Every truncation and every single-byte corruption must end in an error
	// or a result, never a panic or a runaway allocation.
	for cut := 0; cut < len(meta); cut++ {
		image := wrapPE(meta[:cut], true)
		_, _ = Read(bytes.NewReader(image), int64(len(image)))
	}
	for i := range meta {
		damaged := append([]byte(nil), meta...)
		damaged[i] ^= 0xff
		image := wrapPE(damaged, true)
		_, _ = Read(bytes.NewReader(image), int64(len(image)))
	}
	short := wrapPE(sampleMetadata(), true)
	if _, err := Read(bytes.NewReader(short), int64(len(short)-10)); err == nil {
		t.Error("a file shorter than its metadata was accepted")
	}
}

func TestReportListsWhatWasRead(t *testing.T) {
	image := wrapPE(sampleMetadata(), true)
	info, err := Read(bytes.NewReader(image), int64(len(image)))
	if err != nil {
		t.Fatal(err)
	}
	text := Report(info, "Sample.dll")
	for _, want := range []string{"`Sample.dll`", "`Sample`", "`1.2.3.4`", "`System.Runtime`", "`My.Ns`", "`Foo`", "`Sample.strings.resources`", "`v4.0.30319`"} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Nested") {
		t.Error("a nested type was listed")
	}
	if code("a`b") != "`a'b`" {
		t.Error("a backtick inside a code span was not neutralized")
	}
}

func TestReportCapsLongLists(t *testing.T) {
	info := &Info{RuntimeVersion: "v4", Types: map[string][]string{}}
	for i := 0; i < maxReportRefs+5; i++ {
		info.References = append(info.References, Ref{Name: "R"})
	}
	for i := 0; i < maxReportTypes+5; i++ {
		info.Types["N"] = append(info.Types["N"], "T")
		info.TypeCount++
	}
	for i := 0; i < maxReportResources+5; i++ {
		info.Resources = append(info.Resources, "res")
	}
	text := Report(info, "big.dll")
	if n := strings.Count(text, "- `R`"); n != maxReportRefs {
		t.Errorf("references listed = %d", n)
	}
	if n := strings.Count(text, "- `T`"); n != maxReportTypes {
		t.Errorf("types listed = %d", n)
	}
	if n := strings.Count(text, "- `res`"); n != maxReportResources {
		t.Errorf("resources listed = %d", n)
	}
}
