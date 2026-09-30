//go:build !windows

package svcmgr

type platformDetailer struct{}

func (platformDetailer) Details(string) (serviceDetails, error) {
	return serviceDetails{}, errUnsupported
}
