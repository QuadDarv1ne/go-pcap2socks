//go:build !integration

package sandbox

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestNewExecutor(t *testing.T) {
	config := Config{
		MaxExecutionTime: 10 * time.Second,
		MaxMemoryMB:      128,
		MaxCPUPercent:    30,
	}

	executor := NewExecutor(config)

	if executor == nil {
		t.Fatal("Expected non-nil executor")
	}

	if executor.config.MaxExecutionTime != 10*time.Second {
		t.Errorf("Expected MaxExecutionTime 10s, got %v", executor.config.MaxExecutionTime)
	}
}

func TestNewExecutor_Defaults(t *testing.T) {
	executor := NewExecutor(Config{})

	if executor.config.MaxExecutionTime != 30*time.Second {
		t.Errorf("Expected default MaxExecutionTime 30s, got %v", executor.config.MaxExecutionTime)
	}

	if executor.config.MaxMemoryMB != 256 {
		t.Errorf("Expected default MaxMemoryMB 256, got %d", executor.config.MaxMemoryMB)
	}

	if executor.config.MaxCPUPercent != 50 {
		t.Errorf("Expected default MaxCPUPercent 50, got %d", executor.config.MaxCPUPercent)
	}
}

func TestExecutor_ValidateCommand_Allowed(t *testing.T) {
	executor := NewExecutor(Config{})

	var testCmd string
	if runtime.GOOS == "windows" {
		testCmd = "ipconfig"
	} else {
		testCmd = "ping"
	}

	err := executor.validateCommand(testCmd, []string{})
	if err != nil {
		t.Errorf("Expected command to be allowed, got error: %v", err)
	}
}

func TestExecutor_ValidateCommand_NotAllowed(t *testing.T) {
	executor := NewExecutor(Config{})

	err := executor.validateCommand("malicious-command", []string{})
	if err == nil {
		t.Error("Expected error for disallowed command")
	}
}

func TestExecutor_ValidateCommand_DangerousArgs(t *testing.T) {
	executor := NewExecutor(Config{})

	dangerousArgs := []string{
		"arg; rm -rf /",
		"arg | cat /etc/passwd",
		"arg && echo hacked",
		"arg $(whoami)",
		"arg `whoami`",
		"arg ../../../etc/passwd",
	}

	for _, arg := range dangerousArgs {
		err := executor.validateCommand("ping", []string{arg})
		if err == nil {
			t.Errorf("Expected error for dangerous argument: %s", arg)
		}
	}
}

func TestExecutor_Execute_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	executor := NewExecutor(Config{
		MaxExecutionTime: 5 * time.Second,
	})

	var cmd string
	var args []string

	if runtime.GOOS == "windows" {
		cmd = "ping"
		args = []string{"-n", "1", "127.0.0.1"}
	} else {
		cmd = "ping"
		args = []string{"-c", "1", "127.0.0.1"}
	}

	result, err := executor.Execute(context.Background(), cmd, args...)
	if err != nil {
		t.Fatalf("Expected successful execution, got error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	if result.TimedOut {
		t.Error("Expected command not to timeout")
	}

	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}
}

func TestExecutor_Execute_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	executor := NewExecutor(Config{
		MaxExecutionTime: 100 * time.Millisecond,
	})

	var cmd string
	var args []string

	if runtime.GOOS == "windows" {
		cmd = "ping"
		args = []string{"-n", "10", "127.0.0.1"}
	} else {
		cmd = "ping"
		args = []string{"-c", "10", "127.0.0.1"}
	}

	result, err := executor.Execute(context.Background(), cmd, args...)
	if err == nil {
		t.Error("Expected timeout error")
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.TimedOut {
		t.Error("Expected command to timeout")
	}
}

func TestExecutor_Execute_InvalidCommand(t *testing.T) {
	executor := NewExecutor(Config{})

	result, err := executor.Execute(context.Background(), "invalid-command-xyz", []string{})
	if err == nil {
		t.Error("Expected error for invalid command")
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Error == nil {
		t.Error("Expected error in result")
	}
}

func TestExecutor_UpdateConfig(t *testing.T) {
	executor := NewExecutor(Config{
		MaxExecutionTime: 10 * time.Second,
	})

	newConfig := Config{
		MaxExecutionTime: 20 * time.Second,
		MaxMemoryMB:      512,
	}

	executor.UpdateConfig(newConfig)

	config := executor.GetConfig()
	if config.MaxExecutionTime != 20*time.Second {
		t.Errorf("Expected MaxExecutionTime 20s, got %v", config.MaxExecutionTime)
	}

	if config.MaxMemoryMB != 512 {
		t.Errorf("Expected MaxMemoryMB 512, got %d", config.MaxMemoryMB)
	}
}

func TestContainsDangerousPattern(t *testing.T) {
	tests := []struct {
		arg      string
		expected bool
	}{
		{"normal-arg", false},
		{"arg;malicious", true},
		{"arg|pipe", true},
		{"arg&&chain", true},
		{"arg$(injection)", true},
		{"arg`backtick`", true},
		{"arg\nNewline", true},
		{"arg../../../etc", true},
		{"192.168.1.1", false},
		{"-flag=value", false},
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			result := containsDangerousPattern(tt.arg)
			if result != tt.expected {
				t.Errorf("containsDangerousPattern(%q) = %v, expected %v", tt.arg, result, tt.expected)
			}
		})
	}
}

func TestGetDefaultAllowedCommands(t *testing.T) {
	commands := getDefaultAllowedCommands()

	if len(commands) == 0 {
		t.Error("Expected non-empty default commands")
	}

	// Check platform-specific commands
	if runtime.GOOS == "windows" {
		if !commands["netsh"] {
			t.Error("Expected netsh to be allowed on Windows")
		}
		if !commands["ipconfig"] {
			t.Error("Expected ipconfig to be allowed on Windows")
		}
		if commands["powershell"] {
			t.Error("Expected powershell to be disabled by default")
		}
	} else {
		if !commands["iptables"] {
			t.Error("Expected iptables to be allowed on Unix")
		}
		if !commands["ip"] {
			t.Error("Expected ip to be allowed on Unix")
		}
		if commands["bash"] {
			t.Error("Expected bash to be disabled by default")
		}
	}
}

func BenchmarkExecutor_ValidateCommand(b *testing.B) {
	executor := NewExecutor(Config{})

	for i := 0; i < b.N; i++ {
		_ = executor.validateCommand("ping", []string{"127.0.0.1"})
	}
}

func BenchmarkContainsDangerousPattern(b *testing.B) {
	testArgs := []string{
		"normal-arg",
		"192.168.1.1",
		"-flag=value",
		"arg;malicious",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, arg := range testArgs {
			_ = containsDangerousPattern(arg)
		}
	}
}
