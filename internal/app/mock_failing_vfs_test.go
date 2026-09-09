package app

// A VFS whose Create refuses, for the callers that check what a failed save
// does. internal/editor keeps its own copy: a mock is scaffolding, and a
// package that cannot import the other's tests has to declare its own.

import (
	"context"
	"io"
	"os"

	"github.com/unxed/f4/vfs"
)

type mockFailingVFS struct {
	vfs.VFS
	failCreate bool
}

func (m *mockFailingVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	if m.failCreate {
		return nil, os.ErrPermission
	}
	return m.VFS.Create(ctx, path)
}
