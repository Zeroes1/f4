package editor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checkConfigs is a catalog with one colour style, "default", and no file
// types of its own.
func checkConfigs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("base/catalog.xml", `<?xml version="1.0" encoding="UTF-8"?>
<catalog xmlns="http://colorer.github.io/schema/v1/catalog">
  <hrc-sets/>
  <hrd-sets>
    <hrd class="rgb" name="default" description="Default">
      <location link="hrd/default.hrd"/>
    </hrd>
  </hrd-sets>
</catalog>
`)
	write("base/hrd/default.hrd", `<hrd xmlns="http://colorer.sf.net/2003/hrd"/>`)
	return dir
}

func writeUserHRC(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const checkUserType = `<?xml version="1.0" encoding="UTF-8"?>
<hrc version="take5" xmlns="http://colorer.sf.net/2003/hrc">
  <prototype name="checktest" group="user" description="Check test">
    <location link="checktest.hrc"/>
    <filename>/\.checktest$/</filename>
  </prototype>
  <type name="checktest">
    <scheme name="checktest"/>
  </type>
</hrc>
`

func TestCheckColorerSource_LoadsEveryType(t *testing.T) {
	user := t.TempDir()
	writeUserHRC(t, user, "checktest.hrc", checkUserType)
	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRC: user}

	var labels []string
	check := CheckColorerSource(context.Background(), src, "", true, func(done, total int, label string) {
		labels = append(labels, label)
	})
	if !check.Clean() {
		t.Fatalf("check = %+v, want a clean one", check)
	}
	if check.Types != 1 || len(labels) != 1 || labels[0] != "user: Check test" {
		t.Errorf("types = %d, progress = %q; want the user's one type", check.Types, labels)
	}
}

// Issue #277: a user scheme Colorer cannot load has to be reported with its
// file, before an editor quietly falls back to another highlighter.
func TestCheckColorerSource_BrokenUserSchemeFails(t *testing.T) {
	// The prototype parses; the type it points to does not. Colorer reads the
	// type only when it is loaded, so only the full check can find it.
	user := t.TempDir()
	writeUserHRC(t, user, "proto.hrc", `<?xml version="1.0" encoding="UTF-8"?>
<hrc version="take5" xmlns="http://colorer.sf.net/2003/hrc">
  <prototype name="checktest" group="user" description="Check test">
    <location link="checktest-type.hrc"/>
    <filename>/\.checktest$/</filename>
  </prototype>
</hrc>
`)
	writeUserHRC(t, user, "checktest-type.hrc", `<hrc version="take5" xmlns="http://colorer.sf.net/2003/hrc"><type name="checktest">`)
	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRC: filepath.Join(user, "proto.hrc")}

	if quick := CheckColorerSource(context.Background(), src, "", false, nil); quick.Err != nil {
		t.Fatalf("the quick check loads no type schemes, yet failed: %v", quick.Err)
	}
	check := CheckColorerSource(context.Background(), src, "", true, nil)
	if check.Err == nil || !strings.Contains(check.Err.Error(), "checktest") {
		t.Fatalf("Err = %v, want the broken type named", check.Err)
	}
	if !strings.Contains(strings.Join(check.Reports, "\n"), "checktest-type.hrc") {
		t.Errorf("reports %q do not name the broken file", check.Reports)
	}
}

func TestCheckColorerSource_ReportsWhatDidNotStopIt(t *testing.T) {
	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRD: filepath.Join(t.TempDir(), "missing")}
	check := CheckColorerSource(context.Background(), src, "", false, nil)
	if check.Err != nil {
		t.Fatalf("a missing user path failed the check: %v", check.Err)
	}
	if !strings.Contains(strings.Join(check.Reports, "\n"), "user colour styles not loaded") {
		t.Errorf("reports %q do not mention the skipped path", check.Reports)
	}
}

func TestCheckColorerSource_UnknownStyleFails(t *testing.T) {
	check := CheckColorerSource(context.Background(), ColorerSource{ConfigsDir: checkConfigs(t)}, "no-such-style", false, nil)
	if check.Err == nil || !strings.Contains(check.Err.Error(), "no-such-style") {
		t.Errorf("Err = %v, want the unknown colour style named", check.Err)
	}
}
