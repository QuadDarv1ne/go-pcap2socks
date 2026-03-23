package cfg

import (
	"testing"
)

func TestPortRange_Contains(t *testing.T) {
	tests := []struct {
		name     string
		pr       PortRange
		port     uint16
		expected bool
	}{
		{"single port match", PortRange{80, 80}, 80, true},
		{"single port no match", PortRange{80, 80}, 443, false},
		{"range start", PortRange{8000, 9000}, 8000, true},
		{"range end", PortRange{8000, 9000}, 9000, true},
		{"range middle", PortRange{8000, 9000}, 8500, true},
		{"range below", PortRange{8000, 9000}, 7999, false},
		{"range above", PortRange{8000, 9000}, 9001, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pr.Contains(tt.port); got != tt.expected {
				t.Errorf("PortRange.Contains() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewPortMatcher(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantErr bool
	}{
		{"empty", "", false},
		{"single port", "80", false},
		{"multiple ports", "80,443,8080", false},
		{"range", "8000-9000", false},
		{"mixed", "80,443,8000-9000", false},
		{"with spaces", " 80 , 443 , 8000 - 9000 ", false},
		{"invalid format", "80-90-100", true},
		{"invalid port", "abc", true},
		{"invalid range start", "abc-100", true},
		{"invalid range end", "80-abc", true},
		{"reversed range", "9000-8000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPortMatcher(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPortMatcher() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && pm == nil {
				t.Error("NewPortMatcher() returned nil without error")
			}
		})
	}
}

func TestPortMatcher_Matches(t *testing.T) {
	tests := []struct {
		name     string
		spec     string
		port     uint16
		expected bool
	}{
		{"empty matcher", "", 80, false},
		{"single port match", "80", 80, true},
		{"single port no match", "80", 443, false},
		{"multiple ports match first", "80,443,8080", 80, true},
		{"multiple ports match middle", "80,443,8080", 443, true},
		{"multiple ports match last", "80,443,8080", 8080, true},
		{"multiple ports no match", "80,443,8080", 22, false},
		{"range match start", "8000-9000", 8000, true},
		{"range match end", "8000-9000", 9000, true},
		{"range match middle", "8000-9000", 8500, true},
		{"range no match", "8000-9000", 7999, false},
		{"mixed match port", "80,443,8000-9000", 80, true},
		{"mixed match range", "80,443,8000-9000", 8500, true},
		{"mixed no match", "80,443,8000-9000", 22, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPortMatcher(tt.spec)
			if err != nil {
				t.Fatalf("NewPortMatcher() error = %v", err)
			}
			if got := pm.Matches(tt.port); got != tt.expected {
				t.Errorf("PortMatcher.Matches(%d) = %v, want %v", tt.port, got, tt.expected)
			}
		})
	}
}

func TestPortMatcher_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		spec     string
		expected bool
	}{
		{"empty spec", "", true},
		{"single port", "80", false},
		{"range", "8000-9000", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPortMatcher(tt.spec)
			if err != nil {
				t.Fatalf("NewPortMatcher() error = %v", err)
			}
			if got := pm.IsEmpty(); got != tt.expected {
				t.Errorf("PortMatcher.IsEmpty() = %v, want %v", got, tt.expected)
			}
		})
	}
}
