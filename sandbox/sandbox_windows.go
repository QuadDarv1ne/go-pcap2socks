package sandbox

import (
	"fmt"
	"os/exec"
	"syscall"
)

// applySandboxRestrictions applies Windows-specific sandbox restrictions
func (e *Executor) applySandboxRestrictions(cmd *exec.Cmd) error {
	// Set working directory
	if e.config.WorkingDirectory != "" {
		cmd.Dir = e.config.WorkingDirectory
	}

	// Set environment variables (filtered)
	if len(e.config.Environment) > 0 {
		cmd.Env = e.config.Environment
	}

	// Windows-specific: Create process in a job object for resource limits
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		// Hide window for GUI applications
		HideWindow: true,
	}

	return nil
}

// SetProcessPriority sets process priority (Windows-specific)
func SetProcessPriority(cmd *exec.Cmd, priority int) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Map priority to Windows priority class
	// priority: 0 = idle, 1 = below normal, 2 = normal, 3 = above normal, 4 = high
	var priorityClass uint32
	switch priority {
	case 0:
		priorityClass = 0x00000040 // IDLE_PRIORITY_CLASS
	case 1:
		priorityClass = 0x00004000 // BELOW_NORMAL_PRIORITY_CLASS
	case 2:
		priorityClass = 0x00000020 // NORMAL_PRIORITY_CLASS
	case 3:
		priorityClass = 0x00008000 // ABOVE_NORMAL_PRIORITY_CLASS
	case 4:
		priorityClass = 0x00000080 // HIGH_PRIORITY_CLASS
	default:
		return fmt.Errorf("invalid priority: %d (must be 0-4)", priority)
	}

	cmd.SysProcAttr.CreationFlags |= priorityClass
	return nil
}

// CreateJobObject creates a Windows job object for resource limits
// This is a placeholder - full implementation would use Windows API
func CreateJobObject(memoryLimitMB int64, cpuPercent int) error {
	// TODO: Implement using Windows Job Objects API
	// This requires CGO and Windows API calls
	// For now, we rely on process priority and timeout
	return nil
}
