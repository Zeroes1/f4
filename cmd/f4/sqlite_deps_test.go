package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
)

// TestFullBuildExcludesSQLitePlugin is the mechanical half of f4#1178's
// SQLite extraction (part 1 of 4 of the same plan as cloudfox_deps_test.go,
// android_deps_test.go and ios_deps_test.go): plugins/sqlite (the panel
// command, the mounted-database VFS, the interactive client) no longer
// links into f4 at all, in either the full or the lite build. It is its own
// module now (plugins/sqlite/go.mod) built as a separate subprocess RPC
// plugin binary (plugins/sqlite/cmd/sqlite-plugin), which `go list -deps` on
// this module cannot even see, let alone report as a dependency -- but this
// test still asks the toolchain rather than trusting that fact to stay
// true, the same way the other three deps tests do for their own
// extraction.
//
// Unlike cloudfox, the forbidden list below is just the package path
// itself, the same shape android_deps_test.go uses and for the same
// reason: plugins/sqlite carried no *unique* heavy third-party dependency
// of its own to shed. Its SQL engine, github.com/ncruces/go-sqlite3, is not
// forbidden here, because forbidding it would be false: internal/sheet/
// store.go (the native ".f4s.sqlite" spreadsheet format, an unrelated
// feature) imports that very same driver package directly and
// unconditionally, in both builds, so it stays in ./cmd/f4's dependency
// list regardless of this extraction. See plugins/sqlite/rpc_plugin.go's
// package comment for the same point made where the extraction itself
// lives.
func TestFullBuildExcludesSQLitePlugin(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/unxed/f4/plugins/sqlite",
	}

	command := exec.Command("go", "list", "-deps", "./cmd/f4")
	command.Dir = testutil.ModuleRootDir(t)
	out, err := command.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go list -deps ./cmd/f4: %v\n%s", err, stderr)
	}
	deps := strings.Fields(string(out))

	var offenders []string
	for _, imported := range deps {
		for _, bad := range forbidden {
			if imported == bad || strings.HasPrefix(imported, bad+"/") {
				offenders = append(offenders, imported)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a regular build of ./cmd/f4 still depends on:\n\t%s", strings.Join(offenders, "\n\t"))
	}
}
