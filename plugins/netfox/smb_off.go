//go:build !smb || lite

package netfox

import "github.com/unxed/f4/vfs"

// smbBuilt reports whether this build carries the SMB client: it does not,
// see smb_register.go.
const smbBuilt = false

// registerSMBURIProvider registers nothing: SMB is switched off (GO-2026-5051,
// see smb_register.go).
func registerSMBURIProvider(vfs.HostAPI) error { return nil }
