//go:build windows

package svcmgr

import "golang.org/x/sys/windows"

type platformController struct{}

// withService opens the named service with the given access right and hands
// it to fn; the service and the manager are closed afterwards.
func withService(name string, access uint32, fn func(h windows.Handle) error) error {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseServiceHandle(manager) }()
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	h, err := windows.OpenService(manager, namePtr, access)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseServiceHandle(h) }()
	return fn(h)
}

func control(name string, access, code uint32) error {
	return withService(name, access, func(h windows.Handle) error {
		var status windows.SERVICE_STATUS
		return windows.ControlService(h, code, &status)
	})
}

func (platformController) Start(name string) error {
	return withService(name, windows.SERVICE_START, func(h windows.Handle) error {
		return windows.StartService(h, 0, nil)
	})
}

func (platformController) Stop(name string) error {
	return control(name, windows.SERVICE_STOP, windows.SERVICE_CONTROL_STOP)
}

func (platformController) Pause(name string) error {
	return control(name, windows.SERVICE_PAUSE_CONTINUE, windows.SERVICE_CONTROL_PAUSE)
}

func (platformController) Resume(name string) error {
	return control(name, windows.SERVICE_PAUSE_CONTINUE, windows.SERVICE_CONTROL_CONTINUE)
}
