//go:build windows

package service

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "go-pcap2socks"

// Install installs the Windows service
func Install() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable error: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Check if service already exists
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", serviceName)
	}

	// Create service
	s, err = m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName: "go-pcap2socks - SOCKS5 Proxy for Devices",
		Description: "Перенаправляет трафик с устройств (PS4, Xbox, Switch) на SOCKS5 прокси",
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		return fmt.Errorf("create service error: %w", err)
	}
	defer s.Close()

	// Set service recovery options (restart on failure)
	// This requires additional Windows API calls

	slog.Info("Service installed successfully", "name", serviceName, "path", exePath)
	return nil
}

// Uninstall removes the Windows service
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", serviceName)
	}
	defer s.Close()

	err = s.Delete()
	if err != nil {
		return fmt.Errorf("delete service error: %w", err)
	}

	slog.Info("Service uninstalled successfully", "name", serviceName)
	return nil
}

// Start starts the Windows service
func Start() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", serviceName)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		return fmt.Errorf("start service error: %w", err)
	}

	slog.Info("Service started")
	return nil
}

// Stop stops the Windows service
func Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", serviceName)
	}
	defer s.Close()

	status, err := s.Control(svc.Cmd(svc.Stop))
	if err != nil {
		return fmt.Errorf("stop service error: %w", err)
	}

	slog.Info("Service stopped", "status", status)
	return nil
}

// Status returns the current service status
func Status() (string, error) {
	m, err := mgr.Connect()
	if err != nil {
		return "", fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return "not_installed", nil
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return "unknown", fmt.Errorf("query service error: %w", err)
	}

	switch status.State {
	case svc.Stopped:
		return "stopped", nil
	case svc.Running:
		return "running", nil
	case svc.Paused:
		return "paused", nil
	case svc.StartPending:
		return "starting", nil
	case svc.StopPending:
		return "stopping", nil
	default:
		return "unknown", nil
	}
}

// Run runs the application as a Windows service
func Run() {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		slog.Error("eventlog open error", slog.Any("err", err))
		return
	}
	defer elog.Close()

	err = elog.Info(1, fmt.Sprintf("%s service starting", serviceName))
	if err != nil {
		slog.Error("eventlog write error", slog.Any("err", err))
	}

	err = svc.Run(serviceName, &windowsService{elog: elog})
	if err != nil {
		err = elog.Error(2, fmt.Sprintf("%s service failed: %v", serviceName, err))
		if err != nil {
			slog.Error("eventlog write error", slog.Any("err", err))
		}
	}

	err = elog.Info(3, fmt.Sprintf("%s service stopped", serviceName))
	if err != nil {
		slog.Error("eventlog write error", slog.Any("err", err))
	}
}

type windowsService struct {
	elog debug.Log
}

func (m *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	m.elog.Info(4, "Service started, running main application")

	// Start the main application in a goroutine
	goroutine.SafeGo(func() {
		if err := runMainApp(); err != nil {
			m.elog.Error(5, fmt.Sprintf("Main app error: %v", err))
		}
	})

loop:
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			m.elog.Info(6, "Service stop requested")
			changes <- svc.Status{State: svc.StopPending}
			break loop
		default:
			m.elog.Error(7, fmt.Sprintf("unexpected control request #%d", c))
		}
	}

	changes <- svc.Status{State: svc.Stopped}
	return
}

// runMainApp runs the main application logic
func runMainApp() error {
	// Get config path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable error: %w", err)
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Check if config exists
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		// Create default config
		slog.Info("Config not found, creating default", "file", cfgFile)
		cmd := exec.Command(os.Args[0], "auto-config")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("auto-config error: %w", err)
		}
	}

	// Run the main application
	slog.Info("Starting main application")
	cmd := exec.Command(os.Args[0])
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start main app error: %w", err)
	}

	// Wait for the process
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("main app error: %w", err)
	}

	return nil
}

// IsInstalled checks if the service is installed
func IsInstalled() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return false
	}
	defer s.Close()

	return true
}

// WaitForService waits for the service to reach a specific state
func WaitForService(state svc.State, timeout time.Duration) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed", serviceName)
	}
	defer s.Close()

	start := time.Now()
	for time.Since(start) < timeout {
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("query service error: %w", err)
		}

		if status.State == state {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for service state %v", state)
}
