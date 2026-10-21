package f4plugin

// ProfileInfo identifies the active host profile. A subprocess's executable
// directory does not identify f4's portable or custom profile.
type ProfileInfo struct {
	Dir      string
	Portable bool
}

// Profile returns the profile selected by f4, including portable mode.
func (h *Host) Profile() (ProfileInfo, error) {
	var response ProfileInfo
	err := h.sess.Call("Host.GetProfile", nil, &response)
	return response, err
}
