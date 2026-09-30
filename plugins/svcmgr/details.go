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
	// DependsOn names the services and service groups this one needs
	// running first.
	DependsOn []string
	// Recovery is what the manager does after the first, second and later
	// failures of the service; empty when none is configured.
	Recovery []recoveryAction
}

// recoveryAction is one step of a service's failure recovery.
type recoveryAction struct {
	Type     uint32 // SC_ACTION_*, see recoveryName
	DelaySec uint32
}

// The SC_ACTION_* values of winsvc.h.
const (
	recoverNone    = 0
	recoverRestart = 1
	recoverReboot  = 2
	recoverCommand = 3
)

// recoveryName is one recovery step as text, with its delay when it has one.
func recoveryName(a recoveryAction) string {
	var name string
	switch a.Type {
	case recoverNone:
		return "Do nothing"
	case recoverRestart:
		name = "Restart the service"
	case recoverReboot:
		name = "Restart the computer"
	case recoverCommand:
		name = "Run a program"
	default:
		name = strconv.FormatUint(uint64(a.Type), 10)
	}
	if a.DelaySec == 0 {
		return name
	}
	return name + " after " + strconv.FormatUint(uint64(a.DelaySec), 10) + " s"
}

// startUnknown marks a start type that could not be read.
const startUnknown = 0xFFFFFFFF

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
	case startUnknown:
		return ""
	}
	return strconv.FormatUint(uint64(t), 10)
}

// detailer reads a service's configuration by name; the Windows one opens the
// service with the query-configuration right only.
type detailer interface {
	Details(name string) (serviceDetails, error)
}

// serviceDetailer makes the detailer for a computer (empty: this one); a test
// replaces it.
var serviceDetailer = func(machine string) detailer { return platformDetailer{machine: machine} }
