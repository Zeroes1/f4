//go:build !lite

package netfox

import (
	"fmt"

	"github.com/unxed/f4/vfs"
)

// registerOptionalURIProviders registers the URI-string forms that only the
// full build has a backend for. See netfox_uri_lite.go for the lite build's
// (currently empty) counterpart.
func registerOptionalURIProviders(api vfs.HostAPI) error {
	if err := api.RegisterURIProvider(&sftpURIProvider{}); err != nil {
		return fmt.Errorf("NetFox: register sftp URI provider: %w", err)
	}
	// scp:// opens the same SFTP backend (f4#187), see sftpURIProvider.
	if err := api.RegisterURIProvider(&sftpURIProvider{alias: "scp"}); err != nil {
		return fmt.Errorf("NetFox: register scp URI provider: %w", err)
	}
	// smb:// (f4#188) is registered by smb_register.go, which only builds
	// with -tags smb: the SMB client is switched off for now (see there).
	return registerSMBURIProvider(api)
}
