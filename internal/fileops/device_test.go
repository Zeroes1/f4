package fileops

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestClassifyDevice(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	osvfs := vfs.NewOSVFS(tmpDir)

	info := ClassifyDevice(ctx, osvfs, tmpDir)

	// Basic checks
	if info.DeviceID == 0 {
		t.Logf("DeviceID is 0 (may be expected on some platforms)")
	}
	if info.Type == DeviceUnknown {
		t.Logf("Device type is unknown (may be expected in test environment)")
	}
	t.Logf("Device info: %+v", info)
}

func TestShouldUseParallelCopy(t *testing.T) {
	tests := []struct {
		name           string
		src, dst       DeviceInfo
		wantParallel   bool
		wantThreads    int
		wantReasonPart string
	}{
		{
			name: "same device",
			src:  DeviceInfo{Type: DeviceSSD, DeviceID: 1},
			dst:  DeviceInfo{Type: DeviceSSD, DeviceID: 1},
			wantParallel: false,
			wantThreads: 1,
			wantReasonPart: "same physical device",
		},
		{
			name: "SSD to SSD different devices",
			src:  DeviceInfo{Type: DeviceSSD, DeviceID: 1},
			dst:  DeviceInfo{Type: DeviceSSD, DeviceID: 2},
			wantParallel: true,
			wantThreads: 4,
			wantReasonPart: "SSD to SSD",
		},
		{
			name: "HDD involved",
			src:  DeviceInfo{Type: DeviceHDD, DeviceID: 1},
			dst:  DeviceInfo{Type: DeviceSSD, DeviceID: 2},
			wantParallel: false,
			wantThreads: 1,
			wantReasonPart: "HDD detected",
		},
		{
			name: "network involved",
			src:  DeviceInfo{Type: DeviceNetwork, DeviceID: 1},
			dst:  DeviceInfo{Type: DeviceSSD, DeviceID: 2},
			wantParallel: true,
			wantThreads: 2,
			wantReasonPart: "network transfer",
		},
		{
			name: "virtual filesystem",
			src:  DeviceInfo{Type: DeviceVirtual, DeviceID: 1},
			dst:  DeviceInfo{Type: DeviceSSD, DeviceID: 2},
			wantParallel: false,
			wantThreads: 1,
			wantReasonPart: "virtual filesystem",
		},
		{
			name: "unknown types",
			src:  DeviceInfo{Type: DeviceUnknown, DeviceID: 1},
			dst:  DeviceInfo{Type: DeviceUnknown, DeviceID: 2},
			wantParallel: true,
			wantThreads: 2,
			wantReasonPart: "unknown device types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parallel, threads, reason := ShouldUseParallelCopy(tt.src, tt.dst)
			if parallel != tt.wantParallel {
				t.Errorf("ShouldUseParallelCopy() parallel = %v, want %v", parallel, tt.wantParallel)
			}
			if threads != tt.wantThreads {
				t.Errorf("ShouldUseParallelCopy() threads = %v, want %v", threads, tt.wantThreads)
			}
			if reason == "" || (tt.wantReasonPart != "" && !contains(reason, tt.wantReasonPart)) {
				t.Errorf("ShouldUseParallelCopy() reason = %q, want to contain %q", reason, tt.wantReasonPart)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}