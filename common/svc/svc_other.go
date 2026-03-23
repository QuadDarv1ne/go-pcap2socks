//go:build !windows

package svc

// IsAnInteractiveSession returns true for non-Windows platforms
func IsAnInteractiveSession() (bool, error) {
	return true, nil
}
