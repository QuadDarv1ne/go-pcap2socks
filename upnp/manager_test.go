//go:build ignore

package upnp

import (
	"testing"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

func TestNewManager(t *testing.T) {
	tests := []struct {
		name       string
		config     *cfg.UPnP
		internalIP string
		wantNil    bool
	}{
		{
			name:       "nil config",
			config:     nil,
			internalIP: "192.168.1.1",
			wantNil:    true,
		},
		{
			name: "disabled config",
			config: &cfg.UPnP{
				Enabled: false,
			},
			internalIP: "192.168.1.1",
			wantNil:    true,
		},
		{
			name: "valid config",
			config: &cfg.UPnP{
				Enabled:     true,
				AutoForward: true,
			},
			internalIP: "192.168.1.1",
			wantNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(tt.config, tt.internalIP)
			if tt.wantNil && mgr != nil {
				t.Errorf("NewManager() = %v, want nil", mgr)
			}
			if !tt.wantNil && mgr == nil {
				t.Errorf("NewManager() = nil, want non-nil")
			}
		})
	}
}

func TestManager_GetActiveMappings(t *testing.T) {
	t.Run("nil manager returns 0", func(t *testing.T) {
		var mgr *Manager
		if got := mgr.GetActiveMappings(); got != 0 {
			t.Errorf("GetActiveMappings() = %d, want 0", got)
		}
	})

	t.Run("empty manager returns 0", func(t *testing.T) {
		mgr := &Manager{}
		if got := mgr.GetActiveMappings(); got != 0 {
			t.Errorf("GetActiveMappings() = %d, want 0", got)
		}
	})
}

func TestManager_GetConfig(t *testing.T) {
	t.Run("nil manager returns nil", func(t *testing.T) {
		var mgr *Manager
		if got := mgr.GetConfig(); got != nil {
			t.Errorf("GetConfig() = %v, want nil", got)
		}
	})

	t.Run("returns config", func(t *testing.T) {
		expectedConfig := &cfg.UPnP{
			Enabled:     true,
			AutoForward: true,
		}
		mgr := &Manager{
			config: expectedConfig,
		}
		if got := mgr.GetConfig(); got != expectedConfig {
			t.Errorf("GetConfig() = %v, want %v", got, expectedConfig)
		}
	})
}

func TestGetGamePresetPorts(t *testing.T) {
	tests := []struct {
		game string
		want []int
	}{
		{
			game: "ps4",
			want: []int{3478, 3479, 3480},
		},
		{
			game: "ps5",
			want: []int{3478, 3479, 3480},
		},
		{
			game: "xbox",
			want: []int{3074, 3075, 3478, 3479, 3480},
		},
		{
			game: "switch",
			want: []int{12400, 12401, 12402, 6657, 6667},
		},
		{
			game: "unknown",
			want: []int{},
		},
		{
			game: "",
			want: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.game, func(t *testing.T) {
			got := GetGamePresetPorts(tt.game)
			if len(got) != len(tt.want) {
				t.Errorf("GetGamePresetPorts(%q) = %v, want %v", tt.game, got, tt.want)
				return
			}
			for i, port := range got {
				if port != tt.want[i] {
					t.Errorf("GetGamePresetPorts(%q)[%d] = %d, want %d", tt.game, i, port, tt.want[i])
				}
			}
		})
	}
}

func TestManager_AddDynamicMapping(t *testing.T) {
	t.Run("nil manager returns error", func(t *testing.T) {
		var mgr *Manager
		err := mgr.AddDynamicMapping("TCP", 8080, 8080, "test")
		if err == nil {
			t.Error("AddDynamicMapping() = nil, want error")
		}
	})

	t.Run("nil config uses default lease", func(t *testing.T) {
		mgr := &Manager{
			upnp:       New(),
			config:     nil,
			internalIP: "192.168.1.1",
		}
		// Should not panic, uses default lease duration
		err := mgr.AddDynamicMapping("TCP", 8080, 8080, "test")
		// Error expected since UPnP is not initialized, but should not panic
		if err == nil {
			t.Log("AddDynamicMapping succeeded (unexpected)")
		}
	})
}

func TestManager_RemoveDynamicMapping(t *testing.T) {
	t.Run("nil manager returns error", func(t *testing.T) {
		var mgr *Manager
		err := mgr.RemoveDynamicMapping("TCP", 8080)
		if err == nil {
			t.Error("RemoveDynamicMapping() = nil, want error")
		}
	})
}

func TestManager_RefreshMappings(t *testing.T) {
	t.Run("nil manager returns nil", func(t *testing.T) {
		var mgr *Manager
		err := mgr.RefreshMappings()
		if err != nil {
			t.Errorf("RefreshMappings() = %v, want nil", err)
		}
	})
}
