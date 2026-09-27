package sqlite

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseTyped(t *testing.T) {
	for _, tt := range []struct {
		field string
		want  any
	}{
		{"null:", nil},
		{"integer:2D39303037313939323534373430393933", int64(-9007199254740993)},
		{"real:312E30652B333030", 1e300},
		{"text:D182D0B5D181D1820A1F1E", "тест\n\x1f\x1e"},
		{"text:", ""},
		{"blob:00FF1E1F", []byte{0x00, 0xff, 0x1e, 0x1f}},
		{"blob:", []byte{}},
		{"42", int64(42)},
	} {
		got, err := parseTypedOrInteger(tt.field)
		if err != nil {
			t.Errorf("parseTypedOrInteger(%q): %v", tt.field, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseTypedOrInteger(%q) = %#v, want %#v", tt.field, got, tt.want)
		}
	}
	for _, bad := range []string{"text:zz", "mystery:00", "nocolon"} {
		if _, err := parseTypedOrInteger(bad); err == nil {
			t.Errorf("parseTypedOrInteger(%q) accepted a malformed field", bad)
		}
	}
}

// sqlite3 prefixes its messages with where it failed and echoes the SQL
// under them; the client shows what the driver would have said.
func TestCLIErrorKeepsTheMessage(t *testing.T) {
	for stderr, want := range map[string]string{
		"Parse error near line 3: no such table: t\n  SELECT * FROM t;\n                ^--- error here\n": "no such table: t",
		"Runtime error near line 1: NOT NULL constraint failed: t.a (19)\n":                                "NOT NULL constraint failed: t.a (19)",
		"Error: near line 2: database is locked\n":                                                         "database is locked",
		"Error: unable to open database \"x\": unable to open database file\n":                             "unable to open database \"x\": unable to open database file",
	} {
		if got := cliError(errors.New("exit status 1"), stderr).Error(); got != want {
			t.Errorf("cliError(%q) = %q, want %q", stderr, got, want)
		}
	}
	if got := cliError(errors.New("exit status 1"), "").Error(); got != "sqlite3: exit status 1" {
		t.Errorf("cliError with no stderr = %q", got)
	}
}

func TestSQLTextRefusesNUL(t *testing.T) {
	if got, err := sqlText("O'Brien"); err != nil || got != "'O''Brien'" {
		t.Errorf("sqlText(O'Brien) = %q, %v", got, err)
	}
	if _, err := sqlText("a\x00b"); err == nil {
		t.Error("sqlText accepted a NUL byte")
	}
}

// Quote mode separates fields with commas and records with line breaks,
// except inside a string or a function call, and a field that is not a
// plain literal is kept as sqlite3 printed it.
func TestReadQuoteRecords(t *testing.T) {
	out := []byte("'a','b, c'\n1,'x''y\nz'\nX'00FF',unistr('p\\u000aq, r')\nNULL,-1.5e+300\n")
	records, err := readQuoteRecords(out)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"'a'", "'b, c'"},
		{"1", "'x''y\nz'"},
		{"X'00FF'", "unistr('p\\u000aq, r')"},
		{"NULL", "-1.5e+300"},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v, want %#v", records, want)
	}
	for field, want := range map[string]any{
		"'x''y\nz'": "x'y\nz",
		"X'00FF'":   []byte{0x00, 0xff},
		"NULL":      nil,
		"-1.5e+300": -1.5e300,
		"1":         int64(1),
	} {
		got, ok := parseSQLLiteral(field)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("parseSQLLiteral(%q) = %#v, %v; want %#v", field, got, ok, want)
		}
	}
	for _, field := range []string{"unistr('p\\u000aq, r')", "'a'||'b'", "X'0'"} {
		if _, ok := parseSQLLiteral(field); ok {
			t.Errorf("parseSQLLiteral(%q) took an expression for a literal", field)
		}
	}
}
