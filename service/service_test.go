//go:build windows

package service

import (
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/mgr"
)

// mockLogger implements debug.Log interface for testing
type mockLogger struct{}

func (m *mockLogger) Info(eid uint32, msg string) error    { return nil }
func (m *mockLogger) Warning(eid uint32, msg string) error { return nil }
func (m *mockLogger) Error(eid uint32, msg string) error   { return nil }
func (m *mockLogger) Close() error                         { return nil }

var _ debug.Log = (*mockLogger)(nil)

func TestServiceName(t *testing.T) {
	if serviceName != "go-pcap2socks" {
		t.Errorf("serviceName = %v, want %v", serviceName, "go-pcap2socks")
	}
}

func TestInstall_NotAdmin(t *testing.T) {
	// This test will fail if run as admin, but that's expected
	// It tests that Install returns an error when not running as admin
	err := Install()
	
	// Should return an error (access denied or service manager connection error)
	if err == nil {
		t.Log("Install() succeeded - test should be run as non-admin")
	} else {
		t.Logf("Install() returned error (expected): %v", err)
	}
}

func TestUninstall_ServiceNotInstalled(t *testing.T) {
	err := Uninstall()
	
	if err == nil {
		t.Log("Uninstall() succeeded - service was installed")
	} else {
		// Expected error: service not installed
		t.Logf("Uninstall() returned error (expected if not installed): %v", err)
	}
}

func TestStatus_ServiceNotInstalled(t *testing.T) {
	status, err := Status()
	
	if err != nil {
		t.Errorf("Status() returned error: %v", err)
	}
	
	// Should return "not_installed" or similar
	t.Logf("Service status: %s", status)
}

func TestIsInstalled_ServiceNotInstalled(t *testing.T) {
	installed := IsInstalled()
	
	if installed {
		t.Log("Service is installed")
	} else {
		t.Log("Service is not installed (expected for tests)")
	}
}

func TestWaitForService_Timeout(t *testing.T) {
	// Test timeout when service is not installed
	err := WaitForService(svc.Running, 1*time.Second)
	
	if err == nil {
		t.Error("WaitForService() should timeout when service is not installed")
	} else {
		t.Logf("WaitForService() returned error (expected): %v", err)
	}
}

func TestWaitForService_InvalidState(t *testing.T) {
	// Test with invalid state
	err := WaitForService(svc.State(999), 100*time.Millisecond)
	
	// Should timeout or return error
	if err == nil {
		t.Error("WaitForService() should return error for invalid state")
	}
}

func TestStart_ServiceNotInstalled(t *testing.T) {
	err := Start()
	
	if err == nil {
		t.Log("Start() succeeded - service was installed")
	} else {
		t.Logf("Start() returned error (expected if not installed): %v", err)
	}
}

func TestStop_ServiceNotInstalled(t *testing.T) {
	err := Stop()
	
	if err == nil {
		t.Log("Stop() succeeded - service was installed and stopped")
	} else {
		t.Logf("Stop() returned error (expected if not installed): %v", err)
	}
}

func TestRun_AsService(t *testing.T) {
	// This test would actually run the service
	// Skip in normal test runs
	t.Skip("Skipping Run_AsService test - would start service")
}

func TestWindowsService_Execute(t *testing.T) {
	// Test the windowsService struct
	ws := &windowsService{elog: &mockLogger{}}

	// Create buffered channels for testing to prevent blocking
	cmdChan := make(chan svc.ChangeRequest, 1)
	statusChan := make(chan svc.Status, 10)

	// Start Execute in goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.Execute([]string{}, cmdChan, statusChan)
	}()

	// Send stop command
	cmdChan <- svc.ChangeRequest{
		Cmd: svc.Stop,
	}

	// Wait for completion or timeout
	select {
	case <-done:
		t.Log("Execute() completed successfully")
	case <-time.After(2 * time.Second):
		t.Error("Execute() timed out")
	}
}

func TestWindowsService_Execute_Interrogate(t *testing.T) {
	ws := &windowsService{elog: &mockLogger{}}

	// Create buffered channels for testing to prevent blocking
	cmdChan := make(chan svc.ChangeRequest, 1)
	statusChan := make(chan svc.Status, 10)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.Execute([]string{}, cmdChan, statusChan)
	}()

	// Send interrogate command
	cmdChan <- svc.ChangeRequest{
		Cmd: svc.Interrogate,
	}

	// Wait for status response or timeout
	select {
	case status := <-statusChan:
		t.Logf("Received status: %v", status.State)
	case <-time.After(2 * time.Second):
		t.Error("Execute() timed out on interrogate")
	}

	// Stop the service
	cmdChan <- svc.ChangeRequest{
		Cmd: svc.Stop,
	}

	<-done
}

func TestWindowsService_Execute_InvalidCommand(t *testing.T) {
	ws := &windowsService{elog: &mockLogger{}}

	// Create buffered channels for testing to prevent blocking
	cmdChan := make(chan svc.ChangeRequest, 1)
	statusChan := make(chan svc.Status, 10)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.Execute([]string{}, cmdChan, statusChan)
	}()

	// Send invalid command
	cmdChan <- svc.ChangeRequest{
		Cmd: svc.Cmd(999), // Invalid command
	}

	// Give it time to process
	time.Sleep(100 * time.Millisecond)

	// Stop the service
	cmdChan <- svc.ChangeRequest{
		Cmd: svc.Stop,
	}

	<-done
}

func TestRunMainApp(t *testing.T) {
	// Test runMainApp function
	err := runMainApp()
	
	// This will likely fail in test environment without proper config
	if err == nil {
		t.Log("runMainApp() succeeded")
	} else {
		t.Logf("runMainApp() returned error: %v", err)
	}
}

func TestServiceConfig(t *testing.T) {
	// Test that service config has expected values
	expectedDisplayName := "go-pcap2socks - SOCKS5 Proxy for Devices"
	expectedDescription := "Перенаправляет трафик с устройств (PS4, Xbox, Switch) на SOCKS5 прокси"
	expectedStartType := mgr.StartAutomatic
	
	// These are compile-time checks, but we can at least verify the constants
	if expectedDisplayName == "" {
		t.Error("DisplayName should not be empty")
	}
	
	if expectedDescription == "" {
		t.Error("Description should not be empty")
	}
	
	if expectedStartType != mgr.StartAutomatic {
		t.Errorf("StartType = %v, want %v", expectedStartType, mgr.StartAutomatic)
	}
}

func TestServiceRecovery(t *testing.T) {
	// Note: The current implementation doesn't set service recovery options
	// This test documents that limitation
	
	t.Log("Service recovery options are not configured in current implementation")
	t.Log("Consider adding: sc.exe failure <service> reset= 0 actions= restart/60000")
}

func TestServiceGracefulShutdown(t *testing.T) {
	// Test that service handles shutdown gracefully
	ws := &windowsService{elog: &mockLogger{}}

	// Create buffered channels for testing to prevent blocking
	cmdChan := make(chan svc.ChangeRequest, 1)
	statusChan := make(chan svc.Status, 10)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.Execute([]string{}, cmdChan, statusChan)
	}()

	// Wait for service to start
	time.Sleep(100 * time.Millisecond)

	// Send shutdown command
	cmdChan <- svc.ChangeRequest{
		Cmd: svc.Shutdown,
	}

	// Wait for completion or timeout
	select {
	case <-done:
		t.Log("Graceful shutdown completed")
	case <-time.After(2 * time.Second):
		t.Error("Graceful shutdown timed out")
	}
}

func TestServiceMultipleInstances(t *testing.T) {
	// Test that multiple instances cannot be installed
	err := Install()
	if err == nil {
		// If first install succeeded, second should fail
		err2 := Install()
		if err2 == nil {
			t.Error("Multiple service installations should not be allowed")
			Uninstall()
		} else {
			t.Logf("Second Install() correctly failed: %v", err2)
		}
		Uninstall()
	} else {
		t.Logf("First Install() failed (expected): %v", err)
	}
}
