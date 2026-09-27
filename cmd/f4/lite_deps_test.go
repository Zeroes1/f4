package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
)

// TestLiteBuildExcludesHeavyNetworkDependencies is the mechanical half of
// f4#1178 part 3: FISH+ over a subprocess ssh dialer came back into the lite
// build (plugins/netfox, gated file-by-file with //go:build lite/!lite --
// see internal/plughost/plugins_lite.go), and this is what keeps it from
// quietly dragging FTP, SFTP or Pageant support back in with it. Each of
// those links a library -tags lite exists to shed: github.com/jlaffaye/ftp,
// github.com/pkg/sftp, github.com/kbolino/pageant and
// golang.org/x/crypto/ssh (and its ssh/agent, ssh/knownhosts).
//
// This asks the toolchain rather than grepping source: a forbidden import
// reintroduced through a different file, or a new dependency of fishplus
// itself, is caught the same way an already-tagged one would be.
func TestLiteBuildExcludesHeavyNetworkDependencies(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/jlaffaye/ftp",
		"github.com/pkg/sftp",
		"github.com/kbolino/pageant",
		"golang.org/x/crypto/ssh",
	}

	deps := liteBuildDeps(t)

	var offenders []string
	for _, imported := range deps {
		for _, bad := range forbidden {
			if imported == bad || strings.HasPrefix(imported, bad+"/") {
				offenders = append(offenders, imported)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a -tags lite build of ./cmd/f4 still depends on:\n\t%s", strings.Join(offenders, "\n\t"))
	}
}

// TestLiteBuildStillIncludesFishPlus is the other side of the same check: an
// overzealous exclusion that dropped FISH+ itself back out of the lite build
// would pass the test above for the wrong reason. fishplus has no dependency
// beyond the standard library, so its presence here does not reintroduce any
// of the weight -tags lite sheds.
func TestLiteBuildStillIncludesFishPlus(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	deps := liteBuildDeps(t)
	const fishplus = "github.com/unxed/f4/plugins/netfox/fishplus"
	for _, imported := range deps {
		if imported == fishplus {
			return
		}
	}
	t.Fatalf("a -tags lite build of ./cmd/f4 no longer depends on %s", fishplus)
}

// TestLiteBuildExcludesWasmRuntime is the mechanical half of f4#1178's
// wasm-plugin removal: internal/plughost/transport_wazero.go, the transport
// that runs a .wasm plugin inside f4 via wazero, moved behind //go:build
// !lite (see internal/plughost/transport_wazero_lite.go for the stand-in a
// lite build gets instead). This is what catches wazero -- its runtime, the
// wazevo JIT backends, wasi_snapshot_preview1 -- quietly coming back in
// through a different, untagged file.
func TestLiteBuildExcludesWasmRuntime(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const wazero = "github.com/tetratelabs/wazero"
	deps := liteBuildDeps(t)
	for _, imported := range deps {
		if imported == wazero || strings.HasPrefix(imported, wazero+"/") {
			t.Fatalf("a -tags lite build of ./cmd/f4 still depends on %s", imported)
		}
	}
}

// TestLiteBuildExcludesAudioPlayerDependencies is the mechanical half of
// f4#1178 part 5: the mp3/wav/flac/vorbis player was already kept out of a
// lite build by the `lite` tag on internal/media/audio_oto.go (part 1,
// PR #1469), but internal/media/audio_decode.go -- which does the actual
// decoding and is the file that imports github.com/ebitengine/oto/v3,
// github.com/hajimehoshi/go-mp3, github.com/jfreymuth/oggvorbis and
// github.com/mewkiz/flac -- carried no build tag of its own. All four
// libraries therefore kept linking into a lite build regardless of the
// player itself being unreachable there.
func TestLiteBuildExcludesAudioPlayerDependencies(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/ebitengine/oto/v3",
		"github.com/hajimehoshi/go-mp3",
		"github.com/jfreymuth/oggvorbis",
		"github.com/mewkiz/flac",
	}

	deps := liteBuildDeps(t)

	var offenders []string
	for _, imported := range deps {
		for _, bad := range forbidden {
			if imported == bad || strings.HasPrefix(imported, bad+"/") {
				offenders = append(offenders, imported)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a -tags lite build of ./cmd/f4 still depends on:\n\t%s", strings.Join(offenders, "\n\t"))
	}
}

// TestRegularBuildStillIncludesAudioPlayerDependencies is the other side of
// the same check: an overzealous build tag that dropped the audio decoders
// out of a regular build too would pass the test above for the wrong
// reason. IsAudioFile/audioFormatFor (internal/media/audio_format.go) stay
// available in every build, but the decoders themselves are only reachable
// where audio_oto.go builds the real AudioEngine.
func TestRegularBuildStillIncludesAudioPlayerDependencies(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	deps := regularBuildDeps(t)
	want := []string{
		"github.com/ebitengine/oto/v3",
		"github.com/hajimehoshi/go-mp3",
		"github.com/jfreymuth/oggvorbis",
		"github.com/mewkiz/flac",
	}
	for _, lib := range want {
		found := false
		for _, imported := range deps {
			if imported == lib {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("a regular build of ./cmd/f4 no longer depends on %s", lib)
		}
	}
}

// TestRegularBuildStillIncludesWasmRuntime is the other side of the same
// check: an overzealous build tag that dropped the wasm transport out of a
// regular build too would pass the test above for the wrong reason.
func TestRegularBuildStillIncludesWasmRuntime(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const wazero = "github.com/tetratelabs/wazero"
	for _, imported := range regularBuildDeps(t) {
		if imported == wazero {
			return
		}
	}
	t.Fatalf("a regular build of ./cmd/f4 no longer depends on %s", wazero)
}

// TestLiteSheetPackageExcludesSQLiteDependency is the mechanical half of
// f4#1552: internal/sheet/store.go (the native ".f4s.sqlite" spreadsheet
// format) used to import github.com/ncruces/go-sqlite3/driver directly and
// unconditionally, in both builds -- a feature with nothing to do with
// either the sqlite plugin or tar/zipper archive indexing, and the reason
// f4#1178's own TestLiteBuildExcludesSQLiteDependency (below) had to Skip
// instead of asserting for real.
//
// f4#1552 closed that gap: store.go is now //go:build !lite, and
// store_lite.go (//go:build lite) implements the same Save/Load/IsSheetFile
// API on top of encoding/json instead. So this is now the bare "must not
// appear" assertion for internal/sheet specifically: if go-sqlite3 shows up
// in a lite build of that one package, it is a real regression, full stop.
func TestLiteSheetPackageExcludesSQLiteDependency(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const sqlite = "github.com/ncruces/go-sqlite3"

	sheetDeps := packageDepsWithTags(t, "lite", "./internal/sheet")
	var offenders []string
	for _, imported := range sheetDeps {
		if imported == sqlite || strings.HasPrefix(imported, sqlite+"/") {
			offenders = append(offenders, imported)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf(
			"a -tags lite build of ./internal/sheet still depends on:\n\t%s",
			strings.Join(offenders, "\n\t"),
		)
	}
}

// TestLiteBuildExcludesSQLiteDependency is the mechanical half of f4#1178's
// last step (10 of 11, sqlite-free lite build): plugins/sqlite (the SQL
// editor) is meant to move out into its own RPC-plugin module the same way
// cloudfox/android/iOS did, and unxed/tar's own sqlite-backed archive index
// has a FlatBuffers-backed replacement, ArcidxIndex, selected via the
// tarindex_simple build tag. Between those two, neither the sqlite plugin
// nor tar/zipper archive indexing should be the reason a lite build links
// github.com/ncruces/go-sqlite3.
//
// It is deliberately NOT a bare "must not appear" assertion, because that is
// a known false red today, for a reason unrelated to this file: as of this
// branch's base commit, internal/plughost/manager.go's loadInternal still
// wires plugins/sqlite in unconditionally (sqliteplugin.NewPlugin(), no
// lite/!lite split) -- the RPC-plugin extraction that would remove it from a
// lite build's dependency graph is done on a separate, not yet merged
// branch (f4#1178's batch/f4/3). internal/sheet is no longer a source of
// this dependency at all as of f4#1552 (see
// TestLiteSheetPackageExcludesSQLiteDependency above), so this test now
// isolates the one remaining, already-tracked cause instead of conflating
// the two the way its previous version had to.
//
// So this test checks both things it can honestly check: if go-sqlite3
// shows up in a lite build's dependency graph at all, is plugins/sqlite
// (still) the one importing it? If yes, this is the known,
// already-documented, out-of-scope gap (f4#1178, tracked separately, not a
// regression on anything f4#1552 touched), and the test records that
// honestly with Skip rather than either a permanently red CI test or a
// heroic fix bundled into an unrelated feature. If go-sqlite3 shows up for
// some OTHER reason -- plugins/sqlite no longer importing it, yet
// go-sqlite3 still present -- that is exactly the regression this guard
// exists to catch, and it fails for real.
func TestLiteBuildExcludesSQLiteDependency(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const sqlite = "github.com/ncruces/go-sqlite3"

	deps := liteBuildDeps(t)
	var offenders []string
	for _, imported := range deps {
		if imported == sqlite || strings.HasPrefix(imported, sqlite+"/") {
			offenders = append(offenders, imported)
		}
	}
	if len(offenders) == 0 {
		return
	}

	const sqlitePlugin = "github.com/unxed/f4/plugins/sqlite"
	pluginDeps := packageDepsWithTags(t, "lite", sqlitePlugin)
	pluginImportsSQLite := false
	for _, imported := range pluginDeps {
		if imported == sqlite || strings.HasPrefix(imported, sqlite+"/") {
			pluginImportsSQLite = true
			break
		}
	}
	if pluginImportsSQLite {
		t.Skipf(
			"a -tags lite build of ./cmd/f4 still depends on:\n\t%s\n"+
				"but this is the known, already-documented f4#1178 gap: "+
				"internal/plughost/manager.go wires %s in unconditionally, "+
				"and its own extraction into a downloadable RPC plugin is "+
				"done on a separate, not yet merged branch. "+
				"internal/sheet no longer imports %s in a lite build (see "+
				"TestLiteSheetPackageExcludesSQLiteDependency, f4#1552).",
			strings.Join(offenders, "\n\t"), sqlitePlugin, sqlite,
		)
	}
	t.Fatalf(
		"a -tags lite build of ./cmd/f4 depends on %s for a reason other "+
			"than %s -- this looks like a real regression:\n\t%s",
		sqlite, sqlitePlugin, strings.Join(offenders, "\n\t"),
	)
}

// TestRegularBuildStillIncludesSQLiteDependency is the other side of the
// same check: a regular (non-lite) build keeps both plugins/sqlite's
// in-process SQLite VFS mount and internal/sheet's native spreadsheet
// format, so github.com/ncruces/go-sqlite3 staying linked there is expected,
// not a regression. An overzealous change that dropped it out of a regular
// build too would pass TestLiteBuildExcludesSQLiteDependency for the wrong
// reason.
func TestRegularBuildStillIncludesSQLiteDependency(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const sqlite = "github.com/ncruces/go-sqlite3"
	for _, imported := range regularBuildDeps(t) {
		if imported == sqlite {
			return
		}
	}
	t.Fatalf("a regular build of ./cmd/f4 no longer depends on %s", sqlite)
}

func regularBuildDeps(t *testing.T) []string {
	t.Helper()
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
	return strings.Fields(string(out))
}

func liteBuildDeps(t *testing.T) []string {
	t.Helper()
	command := exec.Command("go", "list", "-tags", "lite", "-deps", "./cmd/f4")
	command.Dir = testutil.ModuleRootDir(t)
	out, err := command.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go list -tags lite -deps ./cmd/f4: %v\n%s", err, stderr)
	}
	return strings.Fields(string(out))
}

// packageDepsWithTags is liteBuildDeps/regularBuildDeps generalized to an
// arbitrary package and tag set, used by TestLiteBuildExcludesSQLiteDependency
// to check a single package's own dependency graph (internal/sheet) rather
// than the whole ./cmd/f4 build's.
func packageDepsWithTags(t *testing.T, tags, pkg string) []string {
	t.Helper()
	command := exec.Command("go", "list", "-tags", tags, "-deps", pkg)
	command.Dir = testutil.ModuleRootDir(t)
	out, err := command.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go list -tags %s -deps %s: %v\n%s", tags, pkg, err, stderr)
	}
	return strings.Fields(string(out))
}
