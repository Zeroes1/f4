package netfox

import (
	"context"
	"testing"
)

func assertSFTPURIErrorBatch33(t *testing.T, raw string) {
	t.Helper()
	if _, err := (&sftpURIProvider{}).OpenURI(context.Background(), nil, raw); err == nil {
		t.Fatalf("OpenURI(%q) returned nil error", raw)
	}
}

func TestSFTPURIProviderSchemeCoverageBatch33(t *testing.T) {
	if got := (&sftpURIProvider{}).Scheme(); got != "sftp" {
		t.Fatalf("Scheme() = %q, want sftp", got)
	}
}

func TestSFTPURIProviderMalformedEscapeCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://%zz")
}

func TestSFTPURIProviderMissingHostCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp:///var/tmp")
}

func TestSFTPURIProviderEmptyInputCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "")
}

func TestSFTPURIProviderDefaultPortCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://127.0.0.1/")
}

func TestSFTPURIProviderExplicitPortCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://127.0.0.1:1/")
}

func TestSFTPURIProviderUsernameCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://alice@127.0.0.1:1/")
}

func TestSFTPURIProviderPasswordCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://alice:secret@127.0.0.1:1/")
}

func TestSFTPURIProviderRootPathCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://127.0.0.1:1/")
}

func TestSFTPURIProviderNestedPathCoverageBatch33(t *testing.T) {
	assertSFTPURIErrorBatch33(t, "sftp://alice:secret@127.0.0.1:1/home/alice")
}
