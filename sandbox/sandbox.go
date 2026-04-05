package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Config defines sandbox execution parameters
type Config struct {
	// MaxExecutionTime limits how long a command can run
	MaxExecutionTime time.Duration
	// MaxMemoryMB limits memory usage (platform-specific)
	MaxMemoryMB int64
	// MaxCPUPercent limits CPU usage (0-100)
	MaxCPUPercent int
	// AllowedCommands whitelist of executable names
	AllowedCommands map[string]bool
	// AllowedPaths whitelist of allowed executable paths
	AllowedPaths []string
	// WorkingDirectory for command execution
	WorkingDirectory string
	// Environment variables (filtered)
	Environment []string
	// DisableNetworkAccess attempts to disable network (platform-specific)
	DisableNetworkAccess bool
}

// Executor manages sandboxed command execution
type Executor struct {
	config Config
	mu     sync.RWMutex
}

// ExecutionResult contains command execution results
type ExecutionResult struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	Duration   time.Duration
	TimedOut   bool
	MemoryUsed int64
	Error      error
}

// NewExecutor creates a new sandbox executor
func NewExecutor(config Config) *Executor {
	// Set defaults
	if config.MaxExecutionTime == 0 {
		config.MaxExecutionTime = 30 * time.Second
	}
	if config.MaxMemoryMB == 0 {
		config.MaxMemoryMB = 256 // 256MB default
	}
	if config.MaxCPUPercent == 0 {
		config.MaxCPUPercent = 50 // 50% CPU default
	}
	if config.AllowedCommands == nil {
		config.AllowedCommands = getDefaultAllowedCommands()
	}

	return &Executor{
		config: config,
	}
}

// Execute runs a command in the sandbox
func (e *Executor) Execute(ctx context.Context, command string, args ...string) (*ExecutionResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := &ExecutionResult{}
	startTime := time.Now()

	// Validate command
	if err := e.validateCommand(command, args); err != nil {
		result.Error = err
		return result, err
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, e.config.MaxExecutionTime)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(execCtx, command, args...)

	// Apply sandbox restrictions
	if err := e.applySandboxRestrictions(cmd); err != nil {
		result.Error = fmt.Errorf("failed to apply sandbox restrictions: %w", err)
		return result, result.Error
	}

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()
	result.Duration = time.Since(startTime)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	// Check if timed out
	if execCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.Error = fmt.Errorf("command timed out after %v", e.config.MaxExecutionTime)
		return result, result.Error
	}

	// Get exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err
			return result, err
		}
	}

	return result, nil
}

// validateCommand checks if command is allowed
func (e *Executor) validateCommand(command string, args []string) error {
	// Check if command is in whitelist
	cmdName := strings.ToLower(command)
	
	// Remove .exe extension for Windows
	cmdName = strings.TrimSuffix(cmdName, ".exe")
	
	// Check simple command name
	if !e.config.AllowedCommands[cmdName] {
		// Check if it's a full path in allowed paths
		allowed := false
		for _, allowedPath := range e.config.AllowedPaths {
			if strings.HasPrefix(command, allowedPath) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("command not allowed: %s", command)
		}
	}

	// Check for dangerous argument patterns
	for _, arg := range args {
		if containsDangerousPattern(arg) {
			return fmt.Errorf("dangerous argument pattern detected: %s", arg)
		}
	}

	return nil
}

// containsDangerousPattern checks for command injection patterns
func containsDangerousPattern(arg string) bool {
	dangerousPatterns := []string{
		";", "|", "&", "$", "`", "\n", "\r",
		"$(", "${", "&&", "||", "../",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(arg, pattern) {
			return true
		}
	}

	return false
}

// getDefaultAllowedCommands returns platform-specific default whitelist
func getDefaultAllowedCommands() map[string]bool {
	commands := make(map[string]bool)

	if runtime.GOOS == "windows" {
		// Windows commands
		commands["netsh"] = true
		commands["ipconfig"] = true
		commands["ping"] = true
		commands["route"] = true
		commands["arp"] = true
		commands["nssm"] = true
		commands["sc"] = true
		commands["powershell"] = false // Disabled by default - too powerful
		commands["cmd"] = false         // Disabled by default - too powerful
	} else {
		// Unix commands
		commands["iptables"] = true
		commands["ip"] = true
		commands["ifconfig"] = true
		commands["ping"] = true
		commands["route"] = true
		commands["arp"] = true
		commands["systemctl"] = true
		commands["sh"] = false   // Disabled by default
		commands["bash"] = false // Disabled by default
	}

	return commands
}

// UpdateConfig updates executor configuration
func (e *Executor) UpdateConfig(config Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config = config
}

// GetConfig returns current configuration
func (e *Executor) GetConfig() Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config
}
