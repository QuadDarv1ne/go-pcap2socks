package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ExecuteOnStartConfig defines configuration for executeOnStart commands
type ExecuteOnStartConfig struct {
	// Commands to execute
	Commands []string
	// Timeout for each command
	Timeout time.Duration
	// MaxMemoryMB per command
	MaxMemoryMB int64
	// MaxCPUPercent per command
	MaxCPUPercent int
	// AllowedCommands whitelist
	AllowedCommands map[string]bool
	// ContinueOnError - if true, continue executing remaining commands on error
	ContinueOnError bool
}

// ExecuteOnStartResult contains results of all executed commands
type ExecuteOnStartResult struct {
	Results       []*ExecutionResult
	TotalDuration time.Duration
	SuccessCount  int
	FailureCount  int
}

// ExecuteOnStart executes a list of commands with sandbox restrictions
func ExecuteOnStart(ctx context.Context, config ExecuteOnStartConfig) (*ExecuteOnStartResult, error) {
	if len(config.Commands) == 0 {
		return &ExecuteOnStartResult{}, nil
	}

	startTime := time.Now()
	result := &ExecuteOnStartResult{
		Results: make([]*ExecutionResult, 0, len(config.Commands)),
	}

	// Create sandbox executor
	sandboxConfig := Config{
		MaxExecutionTime: config.Timeout,
		MaxMemoryMB:      config.MaxMemoryMB,
		MaxCPUPercent:    config.MaxCPUPercent,
		AllowedCommands:  config.AllowedCommands,
	}

	executor := NewExecutor(sandboxConfig)

	// Execute each command
	for i, cmdLine := range config.Commands {
		slog.Info("Executing command in sandbox",
			"index", i+1,
			"total", len(config.Commands),
			"command", cmdLine)

		// Parse command line
		cmd, args, err := parseCommandLine(cmdLine)
		if err != nil {
			slog.Error("Failed to parse command", "error", err, "command", cmdLine)
			execResult := &ExecutionResult{
				Error:    err,
				ExitCode: -1,
			}
			result.Results = append(result.Results, execResult)
			result.FailureCount++

			if !config.ContinueOnError {
				return result, fmt.Errorf("failed to parse command %d: %w", i+1, err)
			}
			continue
		}

		// Execute in sandbox
		execResult, err := executor.Execute(ctx, cmd, args...)
		result.Results = append(result.Results, execResult)

		if err != nil || execResult.ExitCode != 0 {
			result.FailureCount++
			slog.Error("Command execution failed",
				"command", cmdLine,
				"exit_code", execResult.ExitCode,
				"error", err,
				"stderr", execResult.Stderr)

			if !config.ContinueOnError {
				return result, fmt.Errorf("command %d failed: %w", i+1, err)
			}
		} else {
			result.SuccessCount++
			slog.Info("Command executed successfully",
				"command", cmdLine,
				"duration", execResult.Duration,
				"stdout", execResult.Stdout)
		}
	}

	result.TotalDuration = time.Since(startTime)

	slog.Info("ExecuteOnStart completed",
		"total_commands", len(config.Commands),
		"success", result.SuccessCount,
		"failure", result.FailureCount,
		"duration", result.TotalDuration)

	return result, nil
}

// parseCommandLine splits a command line into command and arguments
func parseCommandLine(cmdLine string) (string, []string, error) {
	if cmdLine == "" {
		return "", nil, fmt.Errorf("empty command line")
	}

	// Basic parsing - split by spaces, respects quoted strings
	parts := splitCommandLine(cmdLine)
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("invalid command line: %s", cmdLine)
	}

	return parts[0], parts[1:], nil
}

// splitCommandLine splits a command line respecting quotes
func splitCommandLine(cmdLine string) []string {
	var parts []string
	var current string
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range cmdLine {
		switch {
		case ch == '"' || ch == '\'':
			if inQuote {
				if ch == quoteChar {
					inQuote = false
					quoteChar = 0
				} else {
					current += string(ch)
				}
			} else {
				inQuote = true
				quoteChar = ch
			}
		case ch == ' ' && !inQuote:
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		default:
			current += string(ch)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// ValidateExecuteOnStartCommands validates commands before execution
func ValidateExecuteOnStartCommands(commands []string, allowedCommands map[string]bool) error {
	if len(commands) == 0 {
		return nil
	}

	executor := NewExecutor(Config{
		AllowedCommands: allowedCommands,
	})

	for i, cmdLine := range commands {
		cmd, args, err := parseCommandLine(cmdLine)
		if err != nil {
			return fmt.Errorf("command %d: %w", i+1, err)
		}

		if err := executor.validateCommand(cmd, args); err != nil {
			return fmt.Errorf("command %d (%s): %w", i+1, cmdLine, err)
		}
	}

	return nil
}
