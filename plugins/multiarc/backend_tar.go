package multiarc

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// tarBackend wraps the system tar binary. GNU tar and BSD tar (libarchive)
// both auto-detect gzip/bzip2/xz/zstd/lzip compression from the archive's
// own content when reading, so one backend covers every .tar.* variant in
// detectFormat without being told which compressor produced it.
type tarBackend struct{}

func (tarBackend) id() string { return "tar" }

func tarAvailable() bool { return toolAvailable("tar") }

// tarForceLocalArgs is "--force-local" when the tar on PATH is GNU tar,
// which otherwise reads an absolute path starting "C:\" (any Windows drive
// letter) as a "host:path" remote-archive spec and fails to open a plain
// local file; bsdtar and BusyBox tar have no such heuristic, and no such
// flag, so they get nothing added.
func tarForceLocalArgs(ctx context.Context) []string {
	if probeTar(ctx).flavor == tarGNU {
		return []string{"--force-local"}
	}
	return nil
}

func (tarBackend) list(ctx context.Context, localPath string) ([]entry, error) {
	args := append(tarForceLocalArgs(ctx), "-tf", localPath)
	out, errOut, err := runTool(ctx, "tar", args...)
	if err != nil {
		return nil, fmt.Errorf("multiarc: tar %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(errOut)))
	}
	return parseBareNameListing(out), nil
}

func (tarBackend) extractAll(ctx context.Context, localPath, destDir string) error {
	args := append(tarForceLocalArgs(ctx), "-xf", localPath, "-C", destDir)
	_, errOut, err := runTool(ctx, "tar", args...)
	if err != nil {
		return fmt.Errorf("multiarc: tar %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(errOut)))
	}
	return nil
}

func (tarBackend) extractOne(ctx context.Context, localPath, destDir, member string) error {
	if member == "" {
		return errors.New("multiarc: extractOne needs a member path")
	}
	args := append(tarForceLocalArgs(ctx), "-xf", localPath, "-C", destDir, "--", member)
	_, errOut, err := runTool(ctx, "tar", args...)
	if err != nil {
		return fmt.Errorf("multiarc: tar %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(errOut)))
	}
	return nil
}
