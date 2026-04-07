//go:build !integration

package sandbox

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestExecuteOnStart_Empty(t *testing.T) {
	config := ExecuteOnStartConfig{
		Commands: []string{},
	}

	result, err := ExecuteOnStart(context.Background(), config)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if result.SuccessCount != 0 {
		t.Errorf("Expected 0 success, got %d", result.SuccessCount)
	}

	if result.FailureCount != 0 {
		t.Errorf("Expected 0 failures, got %d", result.FailureCount)
	}
}

func TestExecuteOnStart_SingleCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var cmd string
	if runtime.GOOS == "windows" {
		// Use ping instead of ipconfig /? as it's more reliable
		cmd = "ping -n 1 127.0.0.1"
	} else {
		cmd = "ping -c 1 127.0.0.1"
	}

	config := ExecuteOnStartConfig{
		Commands:        []string{cmd},
		Timeout:         5 * time.Second,
		MaxMemoryMB:     128,
		ContinueOnError: false,
	}

	result, err := ExecuteOnStart(context.Background(), config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", result.SuccessCount)
	}

	if result.FailureCount != 0 {
		t.Errorf("Expected 0 failures, got %d", result.FailureCount)
	}

	if len(result.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result.Results))
	}
}

func TestExecuteOnStart_MultipleCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var commands []string
	if runtime.GOOS == "windows" {
		commands = []string{
			"ping -n 1 127.0.0.1",
			"ping -n 1 127.0.0.1",
		}
	} else {
		commands = []string{
			"ping -c 1 127.0.0.1",
			"ping -c 1 127.0.0.1",
		}
	}

	config := ExecuteOnStartConfig{
		Commands:        commands,
		Timeout:         5 * time.Second,
		MaxMemoryMB:     128,
		ContinueOnError: true,
	}

	result, err := ExecuteOnStart(context.Background(), config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(result.Results) != len(commands) {
		t.Errorf("Expected %d results, got %d", len(commands), len(result.Results))
	}

	if result.TotalDuration <= 0 {
		t.Error("Expected positive total duration")
	}
}

func TestExecuteOnStart_InvalidCommand(t *testing.T) {
	config := ExecuteOnStartConfig{
		Commands:        []string{"invalid-command-xyz"},
		Timeout:         5 * time.Second,
		ContinueOnError: false,
	}

	result, err := ExecuteOnStart(context.Background(), config)
	if err == nil {
		t.Error("Expected error for invalid command")
	}

	if result.FailureCount != 1 {
		t.Errorf("Expected 1 failure, got %d", result.FailureCount)
	}
}

func TestExecuteOnStart_ContinueOnError(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var validCmd string
	if runtime.GOOS == "windows" {
		validCmd = "ping -n 1 127.0.0.1"
	} else {
		validCmd = "ping -c 1 127.0.0.1"
	}

	config := ExecuteOnStartConfig{
		Commands: []string{
			"invalid-command-xyz",
			validCmd,
		},
		Timeout:         5 * time.Second,
		ContinueOnError: true,
	}

	result, err := ExecuteOnStart(context.Background(), config)
	if err != nil {
		t.Errorf("Expected no error with ContinueOnError=true, got: %v", err)
	}

	if result.FailureCount != 1 {
		t.Errorf("Expected 1 failure, got %d", result.FailureCount)
	}

	if result.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", result.SuccessCount)
	}
}

func TestParseCommandLine(t *testing.T) {
	tests := []struct {
		input       string
		expectedCmd string
		expectedLen int
	}{
		{"ping 127.0.0.1", "ping", 1},
		{"netsh interface show", "netsh", 2},
		{"ipconfig /all", "ipconfig", 1},
		{"echo hello world", "echo", 2},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, args, err := parseCommandLine(tt.input)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if cmd != tt.expectedCmd {
				t.Errorf("Expected command %q, got %q", tt.expectedCmd, cmd)
			}

			if len(args) != tt.expectedLen {
				t.Errorf("Expected %d args, got %d", tt.expectedLen, len(args))
			}
		})
	}
}

func TestParseCommandLine_Quotes(t *testing.T) {
	tests := []struct {
		input       string
		expectedCmd string
		expectedLen int
	}{
		{`echo "hello world"`, "echo", 1},
		{`echo 'hello world'`, "echo", 1},
		{`cmd /c "echo test"`, "cmd", 2},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, args, err := parseCommandLine(tt.input)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if cmd != tt.expectedCmd {
				t.Errorf("Expected command %q, got %q", tt.expectedCmd, cmd)
			}

			if len(args) != tt.expectedLen {
				t.Errorf("Expected %d args, got %d", tt.expectedLen, len(args))
			}
		})
	}
}

func TestParseCommandLine_Empty(t *testing.T) {
	_, _, err := parseCommandLine("")
	if err == nil {
		t.Error("Expected error for empty command line")
	}
}

func TestSplitCommandLine(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"ping 127.0.0.1", []string{"ping", "127.0.0.1"}},
		{`echo "hello world"`, []string{"echo", "hello world"}},
		{`echo 'hello world'`, []string{"echo", "hello world"}},
		{"cmd /c echo test", []string{"cmd", "/c", "echo", "test"}},
		{"  spaces  around  ", []string{"spaces", "around"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitCommandLine(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d parts, got %d", len(tt.expected), len(result))
				return
			}

			for i, part := range result {
				if part != tt.expected[i] {
					t.Errorf("Part %d: expected %q, got %q", i, tt.expected[i], part)
				}
			}
		})
	}
}

func TestValidateExecuteOnStartCommands(t *testing.T) {
	allowedCommands := map[string]bool{
		"ping":     true,
		"ipconfig": true,
		"netsh":    true,
	}

	tests := []struct {
		name        string
		commands    []string
		shouldError bool
	}{
		{
			name:        "Valid commands",
			commands:    []string{"ping 127.0.0.1", "ipconfig /all"},
			shouldError: false,
		},
		{
			name:        "Invalid command",
			commands:    []string{"malicious-cmd"},
			shouldError: true,
		},
		{
			name:        "Dangerous arguments",
			commands:    []string{"ping 127.0.0.1; rm -rf /"},
			shouldError: true,
		},
		{
			name:        "Empty list",
			commands:    []string{},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExecuteOnStartCommands(tt.commands, allowedCommands)
			if tt.shouldError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func BenchmarkExecuteOnStart(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo test"
	} else {
		cmd = "echo test"
	}

	config := ExecuteOnStartConfig{
		Commands:    []string{cmd},
		Timeout:     5 * time.Second,
		MaxMemoryMB: 128,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExecuteOnStart(context.Background(), config)
	}
}

func BenchmarkParseCommandLine(b *testing.B) {
	cmdLine := "netsh interface ip set dns name=Ethernet static 8.8.8.8"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = parseCommandLine(cmdLine)
	}
}
