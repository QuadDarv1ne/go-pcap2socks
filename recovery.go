package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// RecoveryState tracks restart attempts and timing
type RecoveryState struct {
	RestartCount    int       `json:"restart_count"`
	LastRestartTime time.Time `json:"last_restart_time"`
	FirstFailure    time.Time `json:"first_failure"`
}

const (
	recoveryStateFile   = "recovery_state.json"
	maxRestarts         = 5                    // Maximum restart attempts before giving up
	initialBackoff      = 5 * time.Second      // Initial backoff duration
	maxBackoff          = 60 * time.Second     // Maximum backoff duration
	stabilityThreshold  = 5 * time.Minute      // Reset counter if app runs stable for this long
)

// handleRecoveryWithBackoff implements exponential backoff with restart limits
func handleRecoveryWithBackoff(recover interface{}, stack []byte) error {
	// Load or initialize recovery state
	state := loadRecoveryState()
	
	// Check if we've been running stable (reset counter)
	if time.Since(state.LastRestartTime) > stabilityThreshold && state.RestartCount > 0 {
		slog.Info("Application stable for extended period, resetting restart counter",
			"uptime", time.Since(state.LastRestartTime).Round(time.Second))
		state.RestartCount = 0
		state.FirstFailure = time.Now()
	}
	
	// Increment restart counter
	state.RestartCount++
	state.LastRestartTime = time.Now()
	
	if state.FirstFailure.IsZero() {
		state.FirstFailure = time.Now()
	}
	
	// Check if we've exceeded maximum restart attempts
	if state.RestartCount > maxRestarts {
		totalDowntime := time.Since(state.FirstFailure).Round(time.Second)
		slog.Error("Maximum restart attempts exceeded",
			"attempts", state.RestartCount,
			"total_downtime", totalDowntime,
			"first_failure", state.FirstFailure.Format(time.RFC3339))
		
		// Save state for diagnostics
		state.Save()
		
		// Notify user (if notification system available)
		notifyUserAboutMaxRestarts(state)
		
		return fmt.Errorf("maximum restart attempts (%d) exceeded after %s", maxRestarts, totalDowntime)
	}
	
	// Calculate exponential backoff
	backoff := calculateBackoff(state.RestartCount)
	
	slog.Info("Scheduling automatic restart",
		"attempt", state.RestartCount,
		"max_attempts", maxRestarts,
		"backoff", backoff.Round(time.Second),
		"next_restart", time.Now().Add(backoff).Format(time.RFC3339))
	
	// Save state before waiting
	state.Save()
	
	// Wait with backoff (but allow early exit via signal)
	// Note: _gracefulCtx might be nil if panic happens early, so check first
	if _gracefulCtx != nil {
		select {
		case <-time.After(backoff):
			// Proceed with restart
		case <-_gracefulCtx.Done():
			slog.Info("Shutdown requested during backoff, canceling restart")
			return fmt.Errorf("shutdown requested during backoff")
		}
	} else {
		// No graceful context yet, just wait
		time.Sleep(backoff)
	}
	
	// Perform the restart
	return restartApplication()
}

// calculateBackoff computes exponential backoff duration
func calculateBackoff(attempt int) time.Duration {
	// Exponential: 5s, 10s, 20s, 40s, 60s (capped)
	backoff := initialBackoff * time.Duration(1<<(attempt-1))
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	return backoff
}

// restartApplication performs the actual restart
func restartApplication() error {
	executable, err := os.Executable()
	if err != nil {
		slog.Error("Failed to get executable path", "err", err)
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	
	slog.Info("Restarting application", "executable", executable)
	
	cmd := exec.Command(executable, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Start(); err != nil {
		slog.Error("Failed to restart application", "err", err)
		return fmt.Errorf("failed to restart: %w", err)
	}
	
	// Exit current process to let new process take over
	os.Exit(0)
	return nil // Never reached, but satisfies compiler
}

// loadRecoveryState loads state from file or creates new
func loadRecoveryState() RecoveryState {
	data, err := os.ReadFile(recoveryStateFile)
	if err != nil {
		// File doesn't exist or can't be read - fresh state
		return RecoveryState{
			RestartCount: 0,
			FirstFailure: time.Now(),
		}
	}
	
	var state RecoveryState
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Warn("Failed to parse recovery state, creating fresh state", "err", err)
		return RecoveryState{
			RestartCount: 0,
			FirstFailure: time.Now(),
		}
	}
	
	return state
}

// Save persists recovery state to file
func (s *RecoveryState) Save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		slog.Error("Failed to marshal recovery state", "err", err)
		return err
	}
	
	if err := os.WriteFile(recoveryStateFile, data, 0644); err != nil {
		slog.Error("Failed to save recovery state", "err", err)
		return err
	}
	
	return nil
}

// notifyUserAboutMaxRestarts sends notification when max restarts reached
func notifyUserAboutMaxRestarts(state RecoveryState) {
	msg := fmt.Sprintf("go-pcap2socks превысил лимит перезапусков (%d попыток). Требуется ручное вмешательство.", maxRestarts)
	
	// Try to show notification via OS
	if err := showNotification(msg); err != nil {
		slog.Warn("Failed to show notification", "err", err)
	}
	
	// Also log prominently
	slog.Error("!!! ATTENTION: Application exceeded maximum restart attempts !!!",
		"message", msg,
		"restart_count", state.RestartCount,
		"first_failure", state.FirstFailure.Format(time.RFC3339))
}

// showNotification displays a user notification (cross-platform)
func showNotification(message string) error {
	// Try PowerShell on Windows
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`[System.Reflection.Assembly]::LoadWithPartialName("System.Windows.Forms"); [System.Windows.Forms.MessageBox]::Show("%s", "go-pcap2socks Error")`, message))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
