package vfs

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// regImportKey is one key section of a .reg file as read back: the key with
// its values, or, for "[-path]", the removal of the key and its subkeys.
type regImportKey struct {
	Path   string
	Delete bool
	Values []regImportValue
}

// regImportValue is one value line. Delete is set for "name"=-, which
// removes the value; Type and Data are meaningful only otherwise.
type regImportValue struct {
	Name   string
	Delete bool
	Type   uint32
	Data   []byte
}

// decodeRegFile reads a .reg file of the format encodeRegFile writes and
// regedit exports: UTF-16LE with a byte order mark (or UTF-8), the header
// "Windows Registry Editor Version 5.00", then key sections. It is the way
// back from the .reg text a key is edited as (f4#242). Lines that end with a
// backslash go on in the next line; lines starting with ';' are comments. A
// REG_SZ written as "text" comes back as UTF-16LE with its terminating zero,
// as the registry stores it.
func decodeRegFile(data []byte) ([]regImportKey, error) {
	text, err := regFileText(data)
	if err != nil {
		return nil, err
	}
	lines := regLogicalLines(text)
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i].text) == "" {
		i++
	}
	if i == len(lines) || strings.TrimSpace(lines[i].text) != regFileHeader {
		return nil, fmt.Errorf(".reg: the first line is not %q", regFileHeader)
	}
	var keys []regImportKey
	for _, line := range lines[i+1:] {
		s := strings.TrimSpace(line.text)
		if s == "" || strings.HasPrefix(s, ";") {
			continue
		}
		if strings.HasPrefix(s, "[") {
			if !strings.HasSuffix(s, "]") {
				return nil, fmt.Errorf(".reg line %d: a key without its closing bracket", line.number)
			}
			path := s[1 : len(s)-1]
			key := regImportKey{Path: path}
			if strings.HasPrefix(path, "-") {
				key.Path, key.Delete = path[1:], true
			}
			if key.Path == "" {
				return nil, fmt.Errorf(".reg line %d: an empty key path", line.number)
			}
			keys = append(keys, key)
			continue
		}
		if len(keys) == 0 || keys[len(keys)-1].Delete {
			return nil, fmt.Errorf(".reg line %d: a value outside a key", line.number)
		}
		value, err := regParseValueLine(s)
		if err != nil {
			return nil, fmt.Errorf(".reg line %d: %w", line.number, err)
		}
		last := &keys[len(keys)-1]
		last.Values = append(last.Values, value)
	}
	return keys, nil
}

// regFileText decodes the bytes of a .reg file to text.
func regFileText(data []byte) (string, error) {
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		body := data[2:]
		if len(body)%2 != 0 {
			return "", fmt.Errorf(".reg: UTF-16 text of odd length")
		}
		words := make([]uint16, len(body)/2)
		for i := range words {
			words[i] = binary.LittleEndian.Uint16(body[2*i:])
		}
		return string(utf16.Decode(words)), nil
	}
	return strings.TrimPrefix(string(data), "\uFEFF"), nil
}

type regLine struct {
	number int
	text   string
}

// regLogicalLines splits the text into lines and joins a line that ends with
// a backslash with the next one, dropping the next one's leading blanks; the
// number is that of the first physical line.
func regLogicalLines(text string) []regLine {
	physical := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var out []regLine
	for i := 0; i < len(physical); i++ {
		line := regLine{number: i + 1, text: physical[i]}
		for regContinues(line.text, i, physical) {
			trimmed := strings.TrimRight(line.text, " \t")
			i++
			line.text = trimmed[:len(trimmed)-1] + strings.TrimLeft(physical[i], " \t")
		}
		out = append(out, line)
	}
	return out
}

// regContinues reports whether a value line goes on in the next physical
// line: it ends with a backslash outside a quoted string.
func regContinues(text string, i int, physical []string) bool {
	if i+1 >= len(physical) {
		return false
	}
	trimmed := strings.TrimRight(text, " \t")
	if !strings.HasSuffix(trimmed, `\`) {
		return false
	}
	return !regEndsInsideQuotes(trimmed)
}

// regEndsInsideQuotes reports whether the last character of s is inside a
// quoted string, where a backslash is an escape and not a continuation.
func regEndsInsideQuotes(s string) bool {
	inside := false
	for i := 0; i < len(s); i++ {
		switch {
		case inside && s[i] == '\\':
			if i == len(s)-1 {
				return true
			}
			i++
		case s[i] == '"':
			inside = !inside
		}
	}
	return inside
}

// regParseValueLine reads one value line: @ or "name", =, then "text",
// dword:XXXXXXXX, hex:.., hex(N):.. or - for a removal.
func regParseValueLine(s string) (regImportValue, error) {
	var value regImportValue
	rest := s
	switch {
	case strings.HasPrefix(rest, "@"):
		rest = rest[1:]
	case strings.HasPrefix(rest, `"`):
		name, n, err := regUnquote(rest)
		if err != nil {
			return value, err
		}
		value.Name, rest = name, rest[n:]
	default:
		return value, fmt.Errorf("a value line starts with @ or a quoted name: %q", s)
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "=") {
		return value, fmt.Errorf("no = after the value name")
	}
	rest = strings.TrimSpace(rest[1:])
	switch {
	case rest == "-":
		value.Delete = true
	case strings.HasPrefix(rest, `"`):
		text, n, err := regUnquote(rest)
		if err != nil {
			return value, err
		}
		if strings.TrimSpace(rest[n:]) != "" {
			return value, fmt.Errorf("text after the closing quote")
		}
		value.Type = regTypeSZ
		value.Data = append(regUTF16LE(text), 0, 0)
	case strings.HasPrefix(strings.ToLower(rest), "dword:"):
		n, err := strconv.ParseUint(strings.TrimSpace(rest[len("dword:"):]), 16, 32)
		if err != nil {
			return value, fmt.Errorf("a dword that is not 8 hex digits: %q", rest)
		}
		value.Type = regTypeDWORD
		value.Data = binary.LittleEndian.AppendUint32(nil, uint32(n))
	case strings.HasPrefix(strings.ToLower(rest), "hex"):
		kind, data, ok := strings.Cut(rest, ":")
		if !ok {
			return value, fmt.Errorf("hex without a colon: %q", rest)
		}
		value.Type = regTypeBinary
		if kind = strings.ToLower(strings.TrimSpace(kind)); kind != "hex" {
			inner, isTyped := strings.CutPrefix(kind, "hex(")
			inner, closed := strings.CutSuffix(inner, ")")
			if !isTyped || !closed {
				return value, fmt.Errorf("unknown value kind %q", kind)
			}
			t, err := strconv.ParseUint(inner, 16, 32)
			if err != nil {
				return value, fmt.Errorf("unknown value kind %q", kind)
			}
			value.Type = uint32(t)
		}
		bytes, err := regHexBytes(data)
		if err != nil {
			return value, err
		}
		value.Data = bytes
	default:
		return value, fmt.Errorf("unknown value data %q", rest)
	}
	return value, nil
}

// regUnquote reads a quoted string at the start of s, with \\ and \" as
// escapes, and returns it with the number of bytes it took.
func regUnquote(s string) (string, int, error) {
	var out strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i++
				out.WriteByte(s[i])
				continue
			}
		case '"':
			return out.String(), i + 1, nil
		}
		out.WriteByte(s[i])
	}
	return "", 0, fmt.Errorf("a quoted string without its closing quote")
}

// regHexBytes reads comma separated hex pairs; an empty list is no bytes.
func regHexBytes(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]byte, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		b, err := hex.DecodeString(part)
		if err != nil || len(b) != 1 {
			return nil, fmt.Errorf("a byte that is not two hex digits: %q", part)
		}
		out = append(out, b[0])
	}
	return out, nil
}

func regUTF16LE(s string) []byte {
	words := utf16.Encode([]rune(s))
	out := make([]byte, 2*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint16(out[2*i:], w)
	}
	return out
}
