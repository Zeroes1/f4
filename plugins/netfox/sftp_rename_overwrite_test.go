package netfox

import (
	"context"
	"io"
	"testing"

	"github.com/pkg/sftp"

	"github.com/unxed/f4/vfs"
)

func sftpPutFile(t *testing.T, c *sftp.Client, name, body string) {
	t.Helper()
	f, err := c.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func sftpGetFile(t *testing.T, c *sftp.Client, name string) string {
	t.Helper()
	f, err := c.Open(name)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func sftpNames(t *testing.T, c *sftp.Client) []string {
	t.Helper()
	infos, err := c.ReadDir("/")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, fi := range infos {
		names = append(names, fi.Name())
	}
	return names
}

// f4#1716: saving an edited file over SFTP renames the staged temp file onto
// the existing one. The editor asks for replacement through the context; a
// plain SSH_FXP_RENAME refuses an existing target (SSH_FX_FAILURE).
func TestSFTPRenameOverwriteReplacesExistingTarget(t *testing.T) {
	c, _ := startTestSFTPServer(t)
	v := &SFTPVFS{client: c}
	sftpPutFile(t, c, "/doc.txt", "old")
	sftpPutFile(t, c, "/doc.txt.tmp", "new")

	ctx := vfs.WithDestinationOverwrite(context.Background(), true)
	if err := v.Rename(ctx, "/doc.txt.tmp", "/doc.txt"); err != nil {
		t.Fatalf("rename over an existing target with overwrite: %v", err)
	}
	if got := sftpGetFile(t, c, "/doc.txt"); got != "new" {
		t.Fatalf("target = %q, want %q", got, "new")
	}
	if _, err := c.Stat("/doc.txt.tmp"); err == nil {
		t.Fatal("staged file still present after rename")
	}
}

// Without an explicit overwrite decision the old behaviour stays: refuse.
func TestSFTPRenameWithoutOverwriteRefusesExistingTarget(t *testing.T) {
	c, _ := startTestSFTPServer(t)
	v := &SFTPVFS{client: c}
	sftpPutFile(t, c, "/doc.txt", "old")
	sftpPutFile(t, c, "/doc.txt.tmp", "new")

	for name, ctx := range map[string]context.Context{
		"unknown": context.Background(),
		"false":   vfs.WithDestinationOverwrite(context.Background(), false),
	} {
		if err := v.Rename(ctx, "/doc.txt.tmp", "/doc.txt"); err == nil {
			t.Fatalf("%s: rename onto an existing target must fail", name)
		}
		if got := sftpGetFile(t, c, "/doc.txt"); got != "old" {
			t.Fatalf("%s: target changed to %q", name, got)
		}
	}
}

// Servers without posix-rename@openssh.com take the backup-swap path.
func TestSFTPRenameSwapReplacesExistingTarget(t *testing.T) {
	c, _ := startTestSFTPServer(t)
	v := &SFTPVFS{client: c}
	sftpPutFile(t, c, "/doc.txt", "old")
	sftpPutFile(t, c, "/doc.txt.tmp", "new")

	if err := v.renameSwap("/doc.txt.tmp", "/doc.txt"); err != nil {
		t.Fatal(err)
	}
	if got := sftpGetFile(t, c, "/doc.txt"); got != "new" {
		t.Fatalf("target = %q, want %q", got, "new")
	}
	if names := sftpNames(t, c); len(names) != 1 || names[0] != "doc.txt" {
		t.Fatalf("leftovers after swap: %v", names)
	}
}

func TestSFTPRenameSwapToAbsentTargetIsPlainRename(t *testing.T) {
	c, _ := startTestSFTPServer(t)
	v := &SFTPVFS{client: c}
	sftpPutFile(t, c, "/doc.txt.tmp", "new")

	if err := v.renameSwap("/doc.txt.tmp", "/doc.txt"); err != nil {
		t.Fatal(err)
	}
	if got := sftpGetFile(t, c, "/doc.txt"); got != "new" {
		t.Fatalf("target = %q, want %q", got, "new")
	}
}

// A failing move must give the old target back and leave no backup behind.
func TestSFTPRenameSwapRestoresTargetWhenMoveFails(t *testing.T) {
	c, _ := startTestSFTPServer(t)
	v := &SFTPVFS{client: c}
	sftpPutFile(t, c, "/doc.txt", "old")

	if err := v.renameSwap("/missing.tmp", "/doc.txt"); err == nil {
		t.Fatal("moving a missing source must fail")
	}
	if got := sftpGetFile(t, c, "/doc.txt"); got != "old" {
		t.Fatalf("target = %q, want the original %q", got, "old")
	}
	if names := sftpNames(t, c); len(names) != 1 || names[0] != "doc.txt" {
		t.Fatalf("backup left behind: %v", names)
	}
}
