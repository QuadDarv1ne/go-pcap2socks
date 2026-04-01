//go:build windows

package auto

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

// MEMORYSTATUSEX structure for Windows API
type MEMORYSTATUSEX struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// getTotalMemory returns total system memory on Windows
func getTotalMemory() uint64 {
	memStatus := &MEMORYSTATUSEX{
		Length: uint32(unsafe.Sizeof(MEMORYSTATUSEX{})),
	}

	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(memStatus)))
	if ret == 0 {
		// Fallback to default
		return 4 * GB
	}

	return memStatus.TotalPhys
}

// estimateNetworkSpeed on Windows
func estimateNetworkSpeed() int64 {
	// Default to 100 Mbps
	// Could be enhanced with actual network adapter speed detection
	return 100
}
