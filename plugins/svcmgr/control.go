package svcmgr

// controller starts, stops, pauses and resumes services by name. The Windows
// implementation talks to the Service Control Manager, opening each service
// with only the right its action needs, so a user allowed to start a service
// but not to stop it gets exactly that; every other OS reports
// errUnsupported.
type controller interface {
	Start(name string) error
	Stop(name string) error
	Pause(name string) error
	Resume(name string) error
	// SetStartType changes how the service starts: one of the startAuto,
	// startManual and startDisabled values.
	SetStartType(name string, startType uint32) error
}

// serviceController is the controller the panel uses unless a test replaces it.
var serviceController controller = platformController{}
