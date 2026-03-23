//go:build !windows

package service

// Install is a stub for non-Windows platforms
func Install() error {
	return nil
}

// Uninstall is a stub for non-Windows platforms
func Uninstall() error {
	return nil
}

// Start is a stub for non-Windows platforms
func Start() error {
	return nil
}

// Stop is a stub for non-Windows platforms
func Stop() error {
	return nil
}

// Status is a stub for non-Windows platforms
func Status() (string, error) {
	return "not_installed", nil
}

// Run is a stub for non-Windows platforms
func Run() {
}
