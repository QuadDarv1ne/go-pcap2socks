package auto

import (
	"testing"
)

func TestDetectByMAC(t *testing.T) {
	tests := []struct {
		name     string
		mac      string
		wantType DeviceType
		wantMfr  string
	}{
		{"PS4", "00:9D:6B:12:34:56", DevicePS4, "Sony"},
		{"PS4 Alt", "00:D9:D1:AB:CD:EF", DevicePS4, "Sony"},
		{"PS5", "34:CD:66:11:22:33", DevicePS5, "Sony"},
		{"PS5 Alt", "0C:DB:00:AA:BB:CC", DevicePS5, "Sony"},
		{"Xbox Series X", "E8:4E:22:12:34:56", DeviceXboxSX, "Microsoft"},
		{"Xbox One", "B4:7C:9C:AB:CD:EF", DeviceXboxOne, "Microsoft"},
		{"Xbox 360", "00:25:5C:11:22:33", DeviceXbox, "Microsoft"},
		{"Switch", "F8:89:32:12:34:56", DeviceSwitch, "Nintendo"},
		{"Switch Alt", "04:94:53:AB:CD:EF", DeviceSwitch, "Nintendo"},
		{"Dell PC", "A4:BB:6D:12:34:56", DevicePC, "Dell"},
		{"HP PC", "B8:07:51:AB:CD:EF", DevicePC, "HP"},
		{"iPhone", "A4:83:E7:12:34:56", DevicePhone, "Apple"},
		{"Samsung", "44:7E:CA:AB:CD:EF", DevicePhone, "Samsung"},
		{"iRobot", "00:BB:3A:12:34:56", DeviceRobot, "iRobot"},
		{"Unknown", "FF:FF:FF:12:34:56", DeviceUnknown, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectByMAC(tt.mac)
			if got.Type != tt.wantType {
				t.Errorf("DetectByMAC(%q) type = %v, want %v", tt.mac, got.Type, tt.wantType)
			}
			if got.Manufacturer != tt.wantMfr {
				t.Errorf("DetectByMAC(%q) manufacturer = %q, want %q", tt.mac, got.Manufacturer, tt.wantMfr)
			}
		})
	}
}

func TestDetectByMAC_DifferentFormats(t *testing.T) {
	tests := []struct {
		name string
		mac  string
	}{
		{"Colon uppercase", "00:9D:6B:12:34:56"},
		{"Colon lowercase", "00:9d:6b:12:34:56"},
		{"Dash uppercase", "00-9D-6B-12-34-56"},
		{"Dash lowercase", "00-9d-6b-12-34-56"},
		{"Mixed", "00:9D-6B:12-34:56"},
	}

	wantType := DevicePS4
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectByMAC(tt.mac)
			if got.Type != wantType {
				t.Errorf("DetectByMAC(%q) type = %v, want %v", tt.mac, got.Type, wantType)
			}
		})
	}
}

func TestDeviceProfile_IsGamingDevice(t *testing.T) {
	tests := []struct {
		name  string
		profile DeviceProfile
		want  bool
	}{
		{"PS4", DeviceProfile{Type: DevicePS4}, true},
		{"PS5", DeviceProfile{Type: DevicePS5}, true},
		{"Xbox", DeviceProfile{Type: DeviceXbox}, true},
		{"Xbox One", DeviceProfile{Type: DeviceXboxOne}, true},
		{"Xbox Series X", DeviceProfile{Type: DeviceXboxSX}, true},
		{"Switch", DeviceProfile{Type: DeviceSwitch}, true},
		{"PC", DeviceProfile{Type: DevicePC}, false},
		{"Phone", DeviceProfile{Type: DevicePhone}, false},
		{"Unknown", DeviceProfile{Type: DeviceUnknown}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.IsGamingDevice(); got != tt.want {
				t.Errorf("IsGamingDevice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeviceProfile_IsMobileDevice(t *testing.T) {
	tests := []struct {
		name  string
		profile DeviceProfile
		want  bool
	}{
		{"Phone", DeviceProfile{Type: DevicePhone}, true},
		{"Tablet", DeviceProfile{Type: DeviceTablet}, true},
		{"PC", DeviceProfile{Type: DevicePC}, false},
		{"PS4", DeviceProfile{Type: DevicePS4}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.IsMobileDevice(); got != tt.want {
				t.Errorf("IsMobileDevice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeviceProfile_IsRobot(t *testing.T) {
	tests := []struct {
		name  string
		profile DeviceProfile
		want  bool
	}{
		{"Robot", DeviceProfile{Type: DeviceRobot}, true},
		{"PC", DeviceProfile{Type: DevicePC}, false},
		{"Phone", DeviceProfile{Type: DevicePhone}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.IsRobot(); got != tt.want {
				t.Errorf("IsRobot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDefaultProfile(t *testing.T) {
	profile := GetDefaultProfile()

	if profile.Type != DeviceUnknown {
		t.Errorf("Default profile type = %v, want %v", profile.Type, DeviceUnknown)
	}
	if profile.Manufacturer != "Unknown" {
		t.Errorf("Default profile manufacturer = %q, want %q", profile.Manufacturer, "Unknown")
	}
	if profile.MTU != 1500 {
		t.Errorf("Default profile MTU = %d, want %d", profile.MTU, 1500)
	}
	if profile.Priority != 5 {
		t.Errorf("Default profile priority = %d, want %d", profile.Priority, 5)
	}
}

func TestGenerateDeviceName(t *testing.T) {
	tests := []struct {
		name    string
		profile DeviceProfile
		mac     string
		want    string
	}{
		{
			"PS4 with MAC",
			DeviceProfile{Type: DevicePS4, Manufacturer: "Sony"},
			"00:9D:6B:12:34:56",
			"Sony ps4 -:34:56",
		},
		{
			"Unknown device",
			DeviceProfile{Type: DeviceUnknown, Manufacturer: "Unknown"},
			"FF:FF:FF:12:34:56",
			"Unknown Device (FF:FF:FF:12:34:56)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateDeviceName(tt.profile, tt.mac)
			if got != tt.want {
				t.Errorf("GenerateDeviceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetDeviceIcon(t *testing.T) {
	tests := []struct {
		name    string
		profile DeviceProfile
		want    string
	}{
		{"PS4", DeviceProfile{Type: DevicePS4}, "gamepad"},
		{"PS5", DeviceProfile{Type: DevicePS5}, "gamepad"},
		{"Xbox", DeviceProfile{Type: DeviceXbox}, "xbox"},
		{"Switch", DeviceProfile{Type: DeviceSwitch}, "switch"},
		{"PC", DeviceProfile{Type: DevicePC}, "desktop"},
		{"Phone", DeviceProfile{Type: DevicePhone}, "mobile"},
		{"Tablet", DeviceProfile{Type: DeviceTablet}, "tablet"},
		{"Robot", DeviceProfile{Type: DeviceRobot}, "robot"},
		{"Unknown", DeviceProfile{Type: DeviceUnknown}, "device"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDeviceIcon(tt.profile)
			if got != tt.want {
				t.Errorf("GetDeviceIcon() = %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkDetectByMAC(b *testing.B) {
	mac := "00:9D:6B:12:34:56"
	for i := 0; i < b.N; i++ {
		DetectByMAC(mac)
	}
}

func BenchmarkDetectByMAC_Unknown(b *testing.B) {
	mac := "FF:FF:FF:12:34:56"
	for i := 0; i < b.N; i++ {
		DetectByMAC(mac)
	}
}
