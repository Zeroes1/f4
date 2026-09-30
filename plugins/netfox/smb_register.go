//go:build smb && !lite

package netfox

import (
	"fmt"

	"github.com/unxed/f4/vfs"
)

// smbBuilt reports whether this build carries the SMB client.
const smbBuilt = true

// registerSMBURIProvider registers smb:// (f4#188).
//
// The SMB client (github.com/hirochachacha/go-smb2) is built only with
// -tags smb, and no release or CI build sets it. SMB is switched off for now
// because of GO-2026-5051 (out-of-bounds read and panic in go-smb2's ReadDir
// on a hostile server's directory listing), which has no fix in that module;
// govulncheck reaches it through smbClient.ReadDir. Turning it back on means
// a fixed go-smb2 and dropping the tag.
func registerSMBURIProvider(api vfs.HostAPI) error {
	if err := api.RegisterURIProvider(&smbURIProvider{}); err != nil {
		return fmt.Errorf("NetFox: register smb URI provider: %w", err)
	}
	return nil
}
