//go:build windows

package svcmgr

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// platformController talks to the Service Control Manager of one computer
// (empty: this one).
type platformController struct{ machine string }

// openManager opens the Service Control Manager of machine (empty: the local
// one) with the given right.
func openManager(machine string, access uint32) (windows.Handle, error) {
	var name *uint16
	if machine != "" {
		var err error
		if name, err = windows.UTF16PtrFromString(machine); err != nil {
			return 0, err
		}
	}
	return windows.OpenSCManager(name, nil, access)
}

// withService opens the named service with the given access right and hands
// it to fn; the service and the manager are closed afterwards.
func withService(machine, name string, access uint32, fn func(h windows.Handle) error) error {
	manager, err := openManager(machine, windows.SC_MANAGER_CONNECT)
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

func (c platformController) control(name string, access, code uint32) error {
	return withService(c.machine, name, access, func(h windows.Handle) error {
		var status windows.SERVICE_STATUS
		return windows.ControlService(h, code, &status)
	})
}

func (c platformController) Start(name string) error {
	return withService(c.machine, name, windows.SERVICE_START, func(h windows.Handle) error {
		return windows.StartService(h, 0, nil)
	})
}

func (c platformController) Stop(name string) error {
	return c.control(name, windows.SERVICE_STOP, windows.SERVICE_CONTROL_STOP)
}

func (c platformController) Pause(name string) error {
	return c.control(name, windows.SERVICE_PAUSE_CONTINUE, windows.SERVICE_CONTROL_PAUSE)
}

func (c platformController) Resume(name string) error {
	return c.control(name, windows.SERVICE_PAUSE_CONTINUE, windows.SERVICE_CONTROL_CONTINUE)
}

// SetStartType changes only the start type: every other field of the
// configuration is left as it is (SERVICE_NO_CHANGE, nil). The delayed flag is
// a separate setting of the manager (SERVICE_CONFIG_DELAYED_AUTO_START_INFO),
// written after the start type; it is cleared for every type but a delayed
// automatic one.
func (c platformController) SetStartType(name string, startType uint32, delayed bool) error {
	return withService(c.machine, name, windows.SERVICE_CHANGE_CONFIG, func(h windows.Handle) error {
		if err := windows.ChangeServiceConfig(h, windows.SERVICE_NO_CHANGE, startType, windows.SERVICE_NO_CHANGE,
			nil, nil, nil, nil, nil, nil, nil); err != nil {
			return err
		}
		info := windows.SERVICE_DELAYED_AUTO_START_INFO{}
		if delayed && startType == startAuto {
			info.IsDelayedAutoStartUp = 1
		}
		return windows.ChangeServiceConfig2(h, windows.SERVICE_CONFIG_DELAYED_AUTO_START_INFO, (*byte)(unsafe.Pointer(&info)))
	})
}
