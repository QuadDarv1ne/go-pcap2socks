package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/sandbox"
)

// executeCommandsWithSandbox выполняет команды из executeOnStart с использованием sandbox
func executeCommandsWithSandbox(ctx context.Context, commands []string) error {
	if len(commands) == 0 {
		return nil
	}

	slog.Info("Executing commands with sandbox protection",
		"count", len(commands),
		"timeout", 30*time.Second)

	// Конфигурация sandbox
	config := sandbox.ExecuteOnStartConfig{
		Commands:        commands,
		Timeout:         30 * time.Second,
		MaxMemoryMB:     256,
		MaxCPUPercent:   50,
		ContinueOnError: true, // Продолжаем выполнение при ошибках
	}

	// Выполнение в sandbox
	result, err := sandbox.ExecuteOnStart(ctx, config)
	if err != nil {
		slog.Error("Sandbox execution failed", "error", err)
		return err
	}

	// Логирование результатов
	slog.Info("Sandbox execution completed",
		"total_commands", len(commands),
		"success", result.SuccessCount,
		"failed", result.FailureCount,
		"duration", result.TotalDuration)

	// Детальное логирование каждой команды
	for i, execResult := range result.Results {
		if execResult.Error != nil || execResult.ExitCode != 0 {
			slog.Warn("Command execution issue",
				"index", i+1,
				"command", commands[i],
				"exit_code", execResult.ExitCode,
				"error", execResult.Error,
				"stderr", execResult.Stderr,
				"timed_out", execResult.TimedOut)
		} else {
			slog.Debug("Command executed successfully",
				"index", i+1,
				"command", commands[i],
				"duration", execResult.Duration,
				"stdout", execResult.Stdout)
		}
	}

	return nil
}

// validateCommandsWithSandbox валидирует команды перед выполнением
func validateCommandsWithSandbox(commands []string) error {
	if len(commands) == 0 {
		return nil
	}

	slog.Debug("Validating commands with sandbox", "count", len(commands))

	// Используем sandbox валидацию вместо старой validateExecuteOnStart
	err := sandbox.ValidateExecuteOnStartCommands(commands, nil)
	if err != nil {
		slog.Error("Command validation failed", "error", err)
		return err
	}

	slog.Info("All commands validated successfully", "count", len(commands))
	return nil
}

// getSandboxConfig возвращает конфигурацию sandbox на основе настроек приложения
func getSandboxConfig() sandbox.Config {
	return sandbox.Config{
		MaxExecutionTime: 30 * time.Second,
		MaxMemoryMB:      256,
		MaxCPUPercent:    50,
		AllowedCommands:  allowedCommands,
		AllowedPaths: []string{
			// Windows paths
			"C:\\Windows\\System32\\",
			"C:\\Program Files\\",
			// Unix paths
			"/usr/bin/",
			"/usr/sbin/",
			"/bin/",
			"/sbin/",
		},
	}
}

// createSandboxExecutor создает executor с настройками по умолчанию
func createSandboxExecutor() *sandbox.Executor {
	config := getSandboxConfig()
	return sandbox.NewExecutor(config)
}
