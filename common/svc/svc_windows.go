//go:build windows

package svc

import (
	"golang.org/x/sys/windows/svc"
)

// IsAnInteractiveSession checks if running in interactive session
func IsAnInteractiveSession() (bool, error) {
	return svc.IsAnInteractiveSession()
}
