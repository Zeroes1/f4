//go:build windows

package svcmgr

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

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

// SetStartType changes only the start type: every other field of the
// configuration is left as it is (SERVICE_NO_CHANGE, nil). The delayed flag is
// a separate setting of the manager (SERVICE_CONFIG_DELAYED_AUTO_START_INFO),
// written after the start type; it is cleared for every type but a delayed
// automatic one.
func (platformController) SetStartType(name string, startType uint32, delayed bool) error {
	return withService(name, windows.SERVICE_CHANGE_CONFIG, func(h windows.Handle) error {
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
