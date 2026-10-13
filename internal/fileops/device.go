package fileops

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

// DeviceType represents the type of storage device.
type DeviceType int

const (
	DeviceUnknown DeviceType = iota
	DeviceHDD
	DeviceSSD
	DeviceNetwork
	DeviceVirtual // archives, synthetic filesystems, etc.
)

// DeviceInfo holds classification information for a device.
type DeviceInfo struct {
	Type       DeviceType
	DeviceID   uint64    // stable device identifier from FileIdentifier
	MountPoint string    // path used for classification
	Display    string    // human-readable description
}

// deviceClassifierCache caches classification results per device ID.
var deviceClassifierCache = struct {
	mu    sync.Mutex
	cache map[uint64]DeviceInfo
	ttl   time.Duration
}{
	cache: make(map[uint64]DeviceInfo),
	ttl:   5 * time.Minute,
}

// ClassifyDevice determines the device type for a VFS at a given path.
// It uses FileIdentifier for local filesystems and heuristics for remote ones.
// Results are cached per device ID for 5 minutes.
func ClassifyDevice(ctx context.Context, v vfs.VFS, path string) DeviceInfo {
	// Try to get device ID via FileIdentifier
	var deviceID uint64
	if idf, ok := v.(vfs.FileIdentifier); ok {
		if dev, _, ok := idf.FileIdentity(ctx, path); ok {
			deviceID = dev
		}
	}

	// Check cache
	if deviceID != 0 {
		deviceClassifierCache.mu.Lock()
		if cached, ok := deviceClassifierCache.cache[deviceID]; ok {
			deviceClassifierCache.mu.Unlock()
			return cached
		}
		deviceClassifierCache.mu.Unlock()
	}

	// Classify based on VFS type and platform
	info := classifyDeviceImpl(ctx, v, path, deviceID)

	// Cache result
	if deviceID != 0 {
		deviceClassifierCache.mu.Lock()
		deviceClassifierCache.cache[deviceID] = info
		deviceClassifierCache.mu.Unlock()
	}

	return info
}

func classifyDeviceImpl(ctx context.Context, v vfs.VFS, path string, deviceID uint64) DeviceInfo {
	// Check for OSVFS (local filesystem)
	if osvfs, ok := v.(*vfs.OSVFS); ok {
		return classifyOSVFS(ctx, osvfs, path, deviceID)
	}

	// Check for remote VFS types
	switch v.(type) {
	case interface{ RemoteTransfer() bool }:
		if v.(interface{ RemoteTransfer() bool }).RemoteTransfer() {
			return DeviceInfo{
				Type:       DeviceNetwork,
				DeviceID:   deviceID,
				MountPoint: path,
				Display:    "Network storage",
			}
		}
	}

	// Check for archive VFS (has ParentVFS)
	if v.ParentVFS() != nil {
		return DeviceInfo{
			Type:       DeviceVirtual,
			DeviceID:   deviceID,
			MountPoint: path,
			Display:    "Archive or virtual filesystem",
		}
	}

	// Default to unknown
	return DeviceInfo{
		Type:       DeviceUnknown,
		DeviceID:   deviceID,
		MountPoint: path,
		Display:    "Unknown device type",
	}
}

func classifyOSVFS(ctx context.Context, v *vfs.OSVFS, path string, deviceID uint64) DeviceInfo {
	// Try to get mount point for display
	mountPoint := path
	if abs, err := v.Abs(path); err == nil {
		mountPoint = abs
	}

	// Platform-specific detection
	var dtype DeviceType
	var display string

	if runtime.GOOS == "windows" {
		dtype, display = classifyWindowsDevice(ctx, v, path, deviceID)
	} else {
		dtype, display = classifyUnixDevice(ctx, v, path, deviceID)
	}

	return DeviceInfo{
		Type:       dtype,
		DeviceID:   deviceID,
		MountPoint: mountPoint,
		Display:    display,
	}
}

func classifyUnixDevice(ctx context.Context, v *vfs.OSVFS, path string, deviceID uint64) (DeviceType, string) {
	// On Linux, check /sys/block/*/queue/rotational
	// For now, use a simple heuristic: if we can't determine, assume SSD
	// A more thorough implementation would read /sys/block or use lsblk

	// Try to get the block device for the mount point
	// This is a simplified version; real implementation would parse /proc/mounts
	// and check /sys/block/<dev>/queue/rotational

	// For now, default to SSD on modern systems
	// TODO: Implement proper detection via /sys/block or statx
	return DeviceSSD, "Local SSD (assumed)"
}

func classifyWindowsDevice(ctx context.Context, v *vfs.OSVFS, path string, deviceID uint64) (DeviceType, string) {
	// On Windows, we can use GetDiskFreeSpaceEx or query the volume
	// For now, default to SSD
	// TODO: Implement proper detection via Windows APIs
	return DeviceSSD, "Local SSD (assumed)"
}

// ShouldUseParallelCopy determines if parallel copying is beneficial for a source→dest pair.
// Returns (shouldParallel, recommendedThreads, reason).
func ShouldUseParallelCopy(srcInfo, dstInfo DeviceInfo) (bool, int, string) {
	// Same physical device - no benefit, potential harm (seek thrashing)
	if srcInfo.DeviceID != 0 && dstInfo.DeviceID != 0 && srcInfo.DeviceID == dstInfo.DeviceID {
		return false, 1, "source and destination are on the same physical device"
	}

	// Virtual filesystems (archives) - use bulk copy instead
	if srcInfo.Type == DeviceVirtual || dstInfo.Type == DeviceVirtual {
		return false, 1, "virtual filesystem (archive) - bulk copy preferred"
	}

	// HDD as source or destination - avoid parallelism
	if srcInfo.Type == DeviceHDD || dstInfo.Type == DeviceHDD {
		return false, 1, "HDD detected - parallelism may reduce performance"
	}

	// Network involved - limited parallelism beneficial
	if srcInfo.Type == DeviceNetwork || dstInfo.Type == DeviceNetwork {
		return true, 2, "network transfer - limited parallelism helps"
	}

	// SSD to SSD (different devices) - high parallelism beneficial
	if srcInfo.Type == DeviceSSD && dstInfo.Type == DeviceSSD {
		return true, 4, "SSD to SSD - high parallelism beneficial"
	}

	// Unknown types - conservative
	if srcInfo.Type == DeviceUnknown || dstInfo.Type == DeviceUnknown {
		return true, 2, "unknown device types - conservative parallelism"
	}

	// Default conservative
	return true, 2, "default conservative parallelism"
}