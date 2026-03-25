//go:build windows

package api

import "github.com/QuadDarv1ne/go-pcap2socks/hotkey"

// getHotkeysList returns hotkeys list for Windows
func (s *Server) getHotkeysList() []map[string]interface{} {
	if s.hotkeyManager == nil {
		return nil
	}

	hotkeys := s.hotkeyManager.GetRegisteredHotkeys()

	type HotkeyInfo struct {
		Name     string `json:"name"`
		Shortcut string `json:"shortcut"`
		Enabled  bool   `json:"enabled"`
	}

	hotkeyList := make([]map[string]interface{}, 0, len(hotkeys))
	for _, hk := range hotkeys {
		modifiers := ""
		if hk.Modifiers&hotkey.MOD_CTRL != 0 {
			modifiers += "Ctrl+"
		}
		if hk.Modifiers&hotkey.MOD_ALT != 0 {
			modifiers += "Alt+"
		}
		if hk.Modifiers&hotkey.MOD_SHIFT != 0 {
			modifiers += "Shift+"
		}
		if hk.Modifiers&hotkey.MOD_WIN != 0 {
			modifiers += "Win+"
		}

		// Convert virtual key to string
		keyStr := keyToString(hk.VirtualKey)

		hotkeyList = append(hotkeyList, map[string]interface{}{
			"name":     hk.Name,
			"shortcut": modifiers + keyStr,
			"enabled":  true,
		})
	}

	return hotkeyList
}

// keyToString converts virtual key code to string representation (Windows)
func keyToString(vk int) string {
	switch vk {
	case hotkey.VK_P:
		return "P"
	case hotkey.VK_R:
		return "R"
	case hotkey.VK_S:
		return "S"
	case hotkey.VK_L:
		return "L"
	default:
		return "?"
	}
}
