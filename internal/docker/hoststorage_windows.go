//go:build windows

package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func detectDockerDesktopHostStorage(host string) HostStorageInfo {
	info := HostStorageInfo{
		Label: "Docker Desktop VHDX",
		Kind:  "Docker Desktop WSL2 virtual disk",
	}

	if !localDockerHost(host) {
		info.Unavailable = "local Docker Desktop VHDX is not shown for remote Docker hosts"
		return info
	}

	for _, path := range dockerDesktopVHDXPaths(os.Getenv("LOCALAPPDATA"), os.Getenv("USERPROFILE")) {
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		info.Path = path
		allocatedBytes, err := fileAllocatedBytes(path)
		if err != nil {
			allocatedBytes = uint64(fi.Size())
		}
		info.AllocatedMB = bytesToMB(int64(allocatedBytes))
		info.HostFreeMB, _ = freeSpaceMB(path)
		info.Available = true
		return info
	}

	info.Unavailable = "Docker Desktop WSL2 VHDX was not found on this host"
	return info
}

func freeSpaceMB(path string) (float64, error) {
	volume := filepath.VolumeName(path)
	if volume != "" {
		path = volume + `\`
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	var freeBytesAvailable uint64
	ret, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		0,
		0,
	)
	if ret == 0 {
		return 0, fmt.Errorf("GetDiskFreeSpaceExW failed: %w", callErr)
	}
	return bytesToMB(int64(freeBytesAvailable)), nil
}

func fileAllocatedBytes(path string) (uint64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetCompressedFileSizeW")
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	var high uint32
	low, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&high)),
	)
	if low == 0xffffffff {
		if errno, ok := callErr.(syscall.Errno); ok && errno != 0 {
			return 0, fmt.Errorf("GetCompressedFileSizeW failed: %w", callErr)
		}
	}
	return (uint64(high) << 32) + uint64(low), nil
}
