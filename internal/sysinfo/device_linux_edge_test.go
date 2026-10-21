//go:build linux

package sysinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// stackedChain builds n stacked devices dm-0 .. dm-(n-1) under the fake
// sysfs, each listing the next as its only slave, with a rotating disk sdz
// behind the last one. dm-0 is registered as major 253, minor 0.
func stackedChain(f *fakeSysfs, n int) {
	leaf := f.disk("devices/pci0/ata1/sdz", "8:0", "1")
	dirs := make([]string, n)
	for i := range dirs {
		dirs[i] = f.mkdir(fmt.Sprintf("devices/virtual/block/dm-%d", i))
		f.mkdir(fmt.Sprintf("devices/virtual/block/dm-%d/slaves", i))
	}
	for i, dir := range dirs {
		target := leaf
		if i+1 < n {
			target = dirs[i+1]
		}
		if err := os.Symlink(target, filepath.Join(dir, "slaves", filepath.Base(target))); err != nil {
			f.t.Fatal(err)
		}
	}
	f.link(dirs[0], "253:0")
}

func TestBlockDeviceFollowsAShortStackToItsDisk(t *testing.T) {
	f := newFakeSysfs(t)
	stackedChain(f, 3)

	got, ok := blockDevice(f.root, 253, 0)
	if !ok || got.Name != "sdz" || got.Class != DeviceHDD || !reflect.DeepEqual(got.Keys, []string{"sdz"}) {
		t.Fatalf("blockDevice(dm-0) = %+v ok=%v, want the HDD sdz at the bottom of the stack", got, ok)
	}
}

func TestBlockDeviceGivesUpOnAStackDeeperThanTheLimit(t *testing.T) {
	f := newFakeSysfs(t)
	stackedChain(f, maxSlaveDepth+4)

	// A loop of slaves (or a pathological stack) must end the walk rather
	// than recurse forever; with no disk found the device does not resolve.
	if got, ok := blockDevice(f.root, 253, 0); ok {
		t.Fatalf("blockDevice resolved %+v through a stack deeper than %d levels", got, maxSlaveDepth)
	}
}

func TestBlockDeviceKeepsASlaveItCannotResolve(t *testing.T) {
	f := newFakeSysfs(t)
	dm := f.mkdir("devices/virtual/block/dm-0")
	f.mkdir("devices/virtual/block/dm-0/slaves")
	// A slave whose sysfs entry has vanished: still listed, no longer there.
	if err := os.Symlink(filepath.Join(f.root, "devices", "gone"), filepath.Join(dm, "slaves", "ghost")); err != nil {
		t.Fatal(err)
	}
	f.link(dm, "253:0")

	got, ok := blockDevice(f.root, 253, 0)
	if !ok || got.Class != DeviceUnknown || got.Name != "ghost" || !reflect.DeepEqual(got.Keys, []string{"ghost"}) {
		t.Fatalf("blockDevice(dm-0) = %+v ok=%v, want the unresolvable slave kept by name with an unknown class", got, ok)
	}
}

func TestDiskClassIgnoresAnUnknownRotationalValue(t *testing.T) {
	f := newFakeSysfs(t)
	for rotational, want := range map[string]DeviceClass{
		"0": DeviceSSD,
		"1": DeviceHDD,
		"2": DeviceUnknown,
		"":  DeviceUnknown,
	} {
		dir := f.mkdir("devices/pci0/ata" + rotational + "x/sdq")
		if rotational != "" {
			f.mkdir("devices/pci0/ata" + rotational + "x/sdq/queue")
			f.write(filepath.Join(dir, "queue", "rotational"), rotational+"\n")
		}
		if got := diskClass(dir); got != want {
			t.Errorf("diskClass with rotational %q = %v, want %v", rotational, got, want)
		}
	}
}
