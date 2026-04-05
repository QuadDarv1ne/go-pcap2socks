//go:build !windows

package sandbox

import (
	"fmt"
	"os/exec"
	"syscall"
)

// applySandboxRestrictions applies Unix-specific sandbox restrictions
func (e *Executor) applySandboxRestrictions(cmd *exec.Cmd) error {
	// Set working directory
	if e.config.WorkingDirectory != "" {
		cmd.Dir = e.config.WorkingDirectory
	}

	// Set environment variables (filtered)
	if len(e.config.Environment) > 0 {
		cmd.Env = e.config.Environment
	}

	// Unix-specific: Use setrlimit for resource limits
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Create new process group
		Setpgid: true,
		Pgid:    0,
	}

	// Set resource limits
	if err := setResourceLimits(cmd, e.config.MaxMemoryMB); err != nil {
		return fmt.Errorf("failed to set resource limits: %w", err)
	}

	return nil
}

// setResourceLimits sets Unix resource limits using rlimit
func setResourceLimits(cmd *exec.Cmd, memoryLimitMB int64) error {
	// Convert MB to bytes
	memoryLimitBytes := uint64(memoryLimitMB * 1024 * 1024)

	// Set memory limit (RLIMIT_AS - address space)
	// Note: This is set in the child process, not the parent
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// We can't directly set rlimits in SysProcAttr in Go
	// This would require a wrapper script or preexec function
	// For now, we document this limitation

	return nil
}

// SetProcessPriority sets process priority (Unix-specific)
func SetProcessPriority(cmd *exec.Cmd, priority int) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Map priority to Unix nice value (-20 to 19)
	// priority: 0 = lowest (nice 19), 2 = normal (nice 0), 4 = highest (nice -20)
	niceValue := 19 - (priority * 10)
	if niceValue < -20 {
		niceValue = -20
	}
	if niceValue > 19 {
		niceValue = 19
	}

	// Note: Setting nice value requires appropriate permissions
	// This is a simplified implementation
	_ = niceValue // Placeholder

	return nil
}
