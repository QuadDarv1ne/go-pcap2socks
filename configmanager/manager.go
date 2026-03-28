// Package configmanager provides atomic configuration management with rollback.
package configmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// ConfigManager handles atomic configuration updates with rollback
type ConfigManager struct {
	mu            sync.RWMutex
	configPath    string
	backupPath    string
	currentConfig []byte
	validator     ConfigValidator
}

// ConfigValidator is a function that validates configuration
type ConfigValidator func(config []byte) error

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string, validator ConfigValidator) *ConfigManager {
	backupPath := configPath + ".backup"

	return &ConfigManager{
		configPath: configPath,
		backupPath: backupPath,
		validator:  validator,
	}
}

// LoadConfig loads and validates the current configuration
func (m *ConfigManager) LoadConfig() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Validate
	if m.validator != nil {
		if err := m.validator(data); err != nil {
			return nil, fmt.Errorf("config validation failed: %w", err)
		}
	}

	m.currentConfig = data
	slog.Info("Configuration loaded", "path", m.configPath, "size", len(data))
	return data, nil
}

// UpdateConfig atomically updates the configuration with rollback on failure
func (m *ConfigManager) UpdateConfig(newConfig []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate new configuration first (dry-run)
	if m.validator != nil {
		if err := m.validator(newConfig); err != nil {
			return fmt.Errorf("config validation failed (dry-run): %w", err)
		}
	}

	// Create backup of current config
	if err := m.createBackup(); err != nil {
		slog.Warn("Failed to create config backup", "error", err)
		// Continue anyway - backup is nice to have but not critical
	}

	// Write to temp file first
	tempFile := m.configPath + ".tmp"
	if err := os.WriteFile(tempFile, newConfig, 0644); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}

	// Rename temp file to actual file (atomic on most filesystems)
	if err := os.Rename(tempFile, m.configPath); err != nil {
		// Rollback: restore from backup
		if rollbackErr := m.restoreBackup(); rollbackErr != nil {
			slog.Error("Failed to restore backup after failed update", "error", rollbackErr)
			return fmt.Errorf("config update failed and backup restore failed: %w", rollbackErr)
		}
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	// Update current config
	m.currentConfig = newConfig

	slog.Info("Configuration updated", "path", m.configPath, "size", len(newConfig))
	return nil
}

// UpdateConfigFromFile updates configuration from a file with validation
func (m *ConfigManager) UpdateConfigFromFile(newConfigPath string) error {
	newConfig, err := os.ReadFile(newConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read new config: %w", err)
	}

	return m.UpdateConfig(newConfig)
}

// Rollback restores the previous configuration
func (m *ConfigManager) Rollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.restoreBackup()
}

// createBackup creates a backup of the current configuration
// Requires m.mu to be held by caller
func (m *ConfigManager) createBackup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentConfig == nil {
		// Load current config first
		data, err := os.ReadFile(m.configPath)
		if err != nil {
			return err
		}
		m.currentConfig = data
	}

	// Write backup
	if err := os.WriteFile(m.backupPath, m.currentConfig, 0644); err != nil {
		return err
	}

	slog.Debug("Config backup created", "path", m.backupPath)
	return nil
}

// restoreBackup restores configuration from backup
func (m *ConfigManager) restoreBackup() error {
	// Check if backup exists
	if _, err := os.Stat(m.backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file does not exist: %s", m.backupPath)
	}

	// Read backup
	backupData, err := os.ReadFile(m.backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Write to temp file
	tempFile := m.configPath + ".tmp"
	if err := os.WriteFile(tempFile, backupData, 0644); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}

	// Rename to actual file
	if err := os.Rename(tempFile, m.configPath); err != nil {
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	m.currentConfig = backupData

	slog.Warn("Configuration rolled back from backup", "path", m.configPath)
	return nil
}

// GetBackupPath returns the backup file path
func (m *ConfigManager) GetBackupPath() string {
	return m.backupPath
}

// HasBackup returns true if a backup exists
func (m *ConfigManager) HasBackup() bool {
	_, err := os.Stat(m.backupPath)
	return err == nil
}

// ValidateConfig validates configuration without applying it
func (m *ConfigManager) ValidateConfig(config []byte) error {
	if m.validator == nil {
		return nil
	}
	return m.validator(config)
}

// GetConfigPath returns the config file path
func (m *ConfigManager) GetConfigPath() string {
	return m.configPath
}

// ConfigDiff represents differences between two configurations
type ConfigDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

// DiffConfigs compares two configurations and returns differences
func DiffConfigs(oldConfig, newConfig []byte) (*ConfigDiff, error) {
	var oldMap, newMap map[string]any

	if err := json.Unmarshal(oldConfig, &oldMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal old config: %w", err)
	}

	if err := json.Unmarshal(newConfig, &newMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal new config: %w", err)
	}

	diff := &ConfigDiff{
		Added:   make([]string, 0),
		Removed: make([]string, 0),
		Changed: make([]string, 0),
	}

	// Find added and changed keys
	for key, newValue := range newMap {
		oldValue, exists := oldMap[key]
		if !exists {
			diff.Added = append(diff.Added, key)
		} else if !bytes.Equal(mustJSON(oldValue), mustJSON(newValue)) {
			diff.Changed = append(diff.Changed, key)
		}
	}

	// Find removed keys
	for key := range oldMap {
		if _, exists := newMap[key]; !exists {
			diff.Removed = append(diff.Removed, key)
		}
	}

	return diff, nil
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

// ConfigChange represents a configuration change event
type ConfigChange struct {
	Timestamp time.Time   `json:"timestamp"`
	OldSize   int         `json:"old_size"`
	NewSize   int         `json:"new_size"`
	Diff      *ConfigDiff `json:"diff"`
	Success   bool        `json:"success"`
	Error     string      `json:"error,omitempty"`
}

// LogConfigChange logs a configuration change
func LogConfigChange(oldConfig, newConfig []byte, success bool, err error) {
	diff, diffErr := DiffConfigs(oldConfig, newConfig)
	if diffErr != nil {
		slog.Warn("Failed to compute config diff", "error", diffErr)
		return
	}

	change := ConfigChange{
		Timestamp: time.Now(),
		OldSize:   len(oldConfig),
		NewSize:   len(newConfig),
		Diff:      diff,
		Success:   success,
	}

	if err != nil {
		change.Error = err.Error()
	}

	slog.Info("Configuration change",
		"added", len(diff.Added),
		"removed", len(diff.Removed),
		"changed", len(diff.Changed),
		"success", success)
}
