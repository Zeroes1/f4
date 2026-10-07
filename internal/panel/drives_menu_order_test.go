package panel

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/sysinfo"
)

func TestOrderDriveMenuPlatformDrivesPutsPhysicalEntriesFirst(t *testing.T) {
	drives := []sysinfo.DriveEntry{
		{Name: "/ Root"},
		{Name: "Physical Disks (/dev)"},
		{Name: "~ Home"},
		{Name: "Physical Disks"},
		{Name: "USB (sdb1)"},
	}

	got := orderDriveMenuPlatformDrives(drives)
	want := []string{
		"Physical Disks (/dev)",
		"Physical Disks",
		"/ Root",
		"~ Home",
		"USB (sdb1)",
	}
	names := make([]string, len(got))
	for i, drv := range got {
		names[i] = drv.Name
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("platform drive order = %v, want %v", names, want)
	}
}

func TestOrderDriveMenuPlatformDrivesKeepsInputSlice(t *testing.T) {
	drives := []sysinfo.DriveEntry{{Name: "A"}, {Name: "Physical Disks"}, {Name: "B"}}
	want := []string{"A", "Physical Disks", "B"}
	orderDriveMenuPlatformDrives(drives)
	for i, drv := range drives {
		if drv.Name != want[i] {
			t.Fatalf("input slice changed at %d: got %q, want %q", i, drv.Name, want[i])
		}
	}
}
