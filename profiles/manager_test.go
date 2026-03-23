package profiles

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	manager, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	// Verify profiles directory is set
	if manager.profilesDir == "" {
		t.Error("Expected profilesDir to be set")
	}

	if manager.current != "default" {
		t.Errorf("Expected current profile 'default', got %s", manager.current)
	}
}

func TestListProfiles(t *testing.T) {
	manager, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create default profiles
	err = manager.CreateDefaultProfiles()
	if err != nil {
		t.Fatalf("CreateDefaultProfiles failed: %v", err)
	}

	// List profiles
	profiles, err := manager.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles failed: %v", err)
	}
	
	if len(profiles) < 3 {
		t.Errorf("Expected at least 3 default profiles, got %d", len(profiles))
	}

	// Check for expected profiles
	expectedNames := []string{"default", "gaming", "streaming"}
	for _, expected := range expectedNames {
		found := false
		for _, name := range profiles {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected profile %s not found in list", expected)
		}
	}
}

func TestSaveAndLoadProfile(t *testing.T) {
	manager, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create test config
	testConfig := map[string]interface{}{
		"name":        "test-profile",
		"description": "Test profile for unit testing",
		"pcap": map[string]interface{}{
			"interfaceGateway": "192.168.137.1",
			"network":          "192.168.137.0/24",
			"localIP":          "192.168.137.1",
			"mtu":              1486,
		},
	}

	err = manager.SaveProfile("test-profile", testConfig)
	if err != nil {
		t.Fatalf("SaveProfile failed: %v", err)
	}

	// Load profile
	loaded, err := manager.LoadProfile("test-profile")
	if err != nil {
		t.Fatalf("LoadProfile failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("LoadProfile returned nil")
	}
}

func TestLoadProfileNotFound(t *testing.T) {
	manager, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	_, err = manager.LoadProfile("nonexistent-profile")
	if err == nil {
		t.Error("Expected error for non-existent profile, got nil")
	}
}

func TestGetCurrentProfile(t *testing.T) {
	manager, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	current := manager.GetCurrentProfile()
	if current != "default" {
		t.Errorf("Expected current profile 'default', got %s", current)
	}
}

func TestSetCurrentProfile(t *testing.T) {
	manager, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create default profiles first
	err = manager.CreateDefaultProfiles()
	if err != nil {
		t.Fatalf("CreateDefaultProfiles failed: %v", err)
	}

	manager.SetCurrentProfile("direct")
	
	current := manager.GetCurrentProfile()
	if current != "direct" {
		t.Errorf("Expected current profile 'direct', got %s", current)
	}
}
