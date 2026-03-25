//go:build !windows

package api

// getHotkeysList returns empty hotkeys list for non-Windows platforms
func (s *Server) getHotkeysList() []map[string]interface{} {
	return nil
}
