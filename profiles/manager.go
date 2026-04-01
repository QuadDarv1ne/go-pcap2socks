// Package profiles provides profile management for go-pcap2socks configurations.
package profiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
)

// Pre-defined errors for profile operations
var (
	ErrProfileNotFound   = errors.New("profile not found")
	ErrProfileSaveFailed = errors.New("failed to save profile")
	ErrProfileLoadFailed = errors.New("failed to load profile")
	ErrInvalidProfile    = errors.New("invalid profile name")
)

// Manager handles profile storage and retrieval
type Manager struct {
	mu          sync.RWMutex
	profilesDir string
	current     string
}

type Profile struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Config      interface{} `json:"config"`
}

func NewManager() (*Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}

	profilesDir := path.Join(path.Dir(executable), "profiles")
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return nil, err
	}

	return &Manager{
		profilesDir: profilesDir,
		current:     "default",
	}, nil
}

func (m *Manager) ListProfiles() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, err := os.ReadDir(m.profilesDir)
	if err != nil {
		return nil, err
	}

	profiles := []string{}
	for _, f := range files {
		if !f.IsDir() && path.Ext(f.Name()) == ".json" {
			name := f.Name()[:len(f.Name())-5] // Remove .json
			profiles = append(profiles, name)
		}
	}

	return profiles, nil
}

func (m *Manager) SaveProfile(name string, config interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := path.Join(m.profilesDir, name+".json")

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

func (m *Manager) LoadProfile(name string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filePath := path.Join(m.profilesDir, name+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config, nil
}

func (m *Manager) DeleteProfile(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if name == "default" {
		return fmt.Errorf("cannot delete default profile")
	}

	filePath := path.Join(m.profilesDir, name+".json")
	return os.Remove(filePath)
}

func (m *Manager) SwitchProfile(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	filePath := path.Join(m.profilesDir, name+".json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("profile '%s' does not exist", name)
	}

	// Copy profile to config.json
	executable, err := os.Executable()
	if err != nil {
		return err
	}

	configPath := path.Join(path.Dir(executable), "config.json")

	source, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, source, 0644)
}

func (m *Manager) GetCurrentProfile() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *Manager) SetCurrentProfile(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = name
}

func (m *Manager) ExportProfile(name string, outputPath string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sourcePath := path.Join(m.profilesDir, name+".json")

	// Read source
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// Write to output
	return os.WriteFile(outputPath, data, 0644)
}

func (m *Manager) ImportProfile(inputPath, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Read source
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	// Validate JSON
	var config interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("invalid config file: %w", err)
	}

	// Write to profiles directory
	filePath := path.Join(m.profilesDir, name+".json")
	return os.WriteFile(filePath, data, 0644)
}

// Predefined profiles
var DefaultProfiles = map[string]interface{}{
	"gaming": map[string]interface{}{
		"name":        "Gaming",
		"description": "Optimized for gaming with low latency",
		"config": map[string]interface{}{
			"pcap": map[string]interface{}{
				"mtu": 1472,
			},
		},
	},
	"streaming": map[string]interface{}{
		"name":        "Streaming",
		"description": "Optimized for streaming with high bandwidth",
		"config": map[string]interface{}{
			"pcap": map[string]interface{}{
				"mtu": 1500,
			},
		},
	},
	"default": map[string]interface{}{
		"name":        "Default",
		"description": "Default balanced configuration",
		"config":      map[string]interface{}{},
	},
}

func (m *Manager) CreateDefaultProfiles() error {
	for name, profile := range DefaultProfiles {
		filePath := path.Join(m.profilesDir, name+".json")

		// Skip if already exists
		if _, err := os.Stat(filePath); err == nil {
			continue
		}

		data, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return err
		}
	}

	return nil
}
