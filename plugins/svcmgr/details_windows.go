//go:build windows

package svcmgr

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

type platformDetailer struct{}

// Details reads the configuration through mgr.Service.Config, which needs only
// the query-configuration right on the handle it is given.
func (platformDetailer) Details(name string) (serviceDetails, error) {
	var out serviceDetails
	err := withService(name, windows.SERVICE_QUERY_CONFIG, func(h windows.Handle) error {
		cfg, err := (&mgr.Service{Name: name, Handle: h}).Config()
		if err != nil {
			return err
		}
		out = serviceDetails{
			StartType:   cfg.StartType,
			Delayed:     cfg.DelayedAutoStart,
			BinaryPath:  cfg.BinaryPathName,
			Account:     cfg.ServiceStartName,
			Description: cfg.Description,
		}
		return nil
	})
	return out, err
}
