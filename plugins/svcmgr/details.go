package svcmgr

import "strconv"

// serviceDetails is what the Service Control Manager keeps about a service
// beyond the list row: how it starts, what it runs and as whom.
type serviceDetails struct {
	StartType   uint32 // SERVICE_*_START, see startTypeName
	Delayed     bool   // automatic start, delayed
	BinaryPath  string
	Account     string
	Description string
}

// The SERVICE_*_START values of winsvc.h.
const (
	startBoot     = 0
	startSystem   = 1
	startAuto     = 2
	startManual   = 3
	startDisabled = 4
)

// startTypeName is the start type as text.
func startTypeName(t uint32, delayed bool) string {
	switch t {
	case startBoot:
		return "Boot"
	case startSystem:
		return "System"
	case startAuto:
		if delayed {
			return "Automatic (delayed start)"
		}
		return "Automatic"
	case startManual:
		return "Manual"
	case startDisabled:
		return "Disabled"
	}
	return strconv.FormatUint(uint64(t), 10)
}

// detailer reads a service's configuration by name; the Windows one opens the
// service with the query-configuration right only.
type detailer interface {
	Details(name string) (serviceDetails, error)
}

// serviceDetailer is the detailer the panel uses unless a test replaces it.
var serviceDetailer detailer = platformDetailer{}
