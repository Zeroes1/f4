package panel

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// watchDriveListGoroutines counts the drive-menu watch goroutines alive now.
func watchDriveListGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "watchDriveList.func1")
}

// The watch ends itself through a task on the frame manager it was started
// on. A manager that was shut down runs no task, so without a check of its
// own the ticker goroutine would read the drive list every period for the
// rest of the process (found by goleak over this package's tests).
func TestDriveMenuWatchEndsWhenFrameManagerShutsDown(t *testing.T) {
	oldEvery, oldDrives := driveMenuWatchEvery, driveMenuPlatformDrives
	t.Cleanup(func() { driveMenuWatchEvery, driveMenuPlatformDrives = oldEvery, oldDrives })
	driveMenuWatchEvery = 5 * time.Millisecond
	driveMenuPlatformDrives = func() []sysinfo.DriveEntry {
		return []sysinfo.DriveEntry{{Name: "Alpha", Factory: func() vfs.VFS { return nil }}}
	}

	oldManager := vtui.FrameManager
	manager := vtui.NewFrameManager()
	manager.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager = manager
	t.Cleanup(func() { vtui.FrameManager = oldManager })

	before := watchDriveListGoroutines()
	menu := vtui.NewVMenu(" Drives ")
	manager.Push(&DriveMenuFrame{VMenu: menu})
	pf := &PanelsFrame{}
	pf.watchDriveList(0, menu, "")
	if got := watchDriveListGoroutines(); got != before+1 {
		t.Fatalf("watch goroutines after start: %d, want %d", got, before+1)
	}

	manager.Shutdown()

	deadline := time.Now().Add(2 * time.Second)
	for watchDriveListGoroutines() > before {
		if !time.Now().Before(deadline) {
			t.Fatal("the drive menu watch outlived the frame manager it posts to")
		}
		time.Sleep(time.Millisecond)
	}
}
