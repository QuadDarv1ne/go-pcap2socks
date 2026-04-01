//go:build linux || darwin

package auto

import (
	"golang.org/x/sys/unix"
)

// getTotalMemory returns total system memory on Unix-like systems
func getTotalMemory() uint64 {
	// Use sysconf for page size and page count
	pageSize := uint64(unix.Getpagesize())
	pageCount := getPhysPages()

	if pageCount > 0 {
		return pageSize * pageCount
	}

	// Fallback based on common configurations
	return 8 * GB
}

// getPhysPages returns number of physical pages
func getPhysPages() uint64 {
	pages, err := unix.Sysconf(unix._SC_PHYS_PAGES)
	if err != nil {
		return 0
	}
	return pages
}

// estimateNetworkSpeed on Unix
func estimateNetworkSpeed() int64 {
	// Default to 100 Mbps
	return 100
}
