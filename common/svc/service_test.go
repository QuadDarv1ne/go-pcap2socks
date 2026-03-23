package svc

import (
	"testing"
	"time"
)

func TestNewService(t *testing.T) {
	service := NewService("test-service", "Test Service", "Test service description")

	if service == nil {
		t.Fatal("NewService returned nil")
	}

	if service.Name != "test-service" {
		t.Errorf("Expected name 'test-service', got %s", service.Name)
	}

	if service.DisplayName != "Test Service" {
		t.Errorf("Expected displayName 'Test Service', got %s", service.DisplayName)
	}

	if service.Description != "Test service description" {
		t.Errorf("Expected description 'Test service description', got %s", service.Description)
	}

	if service.onStart == nil {
		t.Error("Expected onStart callback to be set")
	}

	if service.onStop == nil {
		t.Error("Expected onStop callback to be set")
	}
}

func TestServiceCallbacks(t *testing.T) {
	startCalled := false
	stopCalled := false

	service := NewService("test-service", "Test Service", "Test description")
	
	service.SetCallbacks(
		func() error {
			startCalled = true
			return nil
		},
		func() error {
			stopCalled = true
			return nil
		},
	)

	// Test start callback
	err := service.onStart()
	if err != nil {
		t.Errorf("onStart returned error: %v", err)
	}

	if !startCalled {
		t.Error("Expected start callback to be called")
	}

	// Test stop callback
	err = service.onStop()
	if err != nil {
		t.Errorf("onStop returned error: %v", err)
	}

	if !stopCalled {
		t.Error("Expected stop callback to be called")
	}
}

func TestServiceCallbacksWithError(t *testing.T) {
	expectedError := "test error"

	service := NewService("test-service", "Test Service", "Test description")
	
	service.SetCallbacks(
		func() error {
			return &testError{msg: expectedError}
		},
		func() error {
			return &testError{msg: expectedError}
		},
	)

	// Test start callback with error
	err := service.onStart()
	if err == nil {
		t.Error("Expected error from onStart, got nil")
	}

	// Test stop callback with error
	err = service.onStop()
	if err == nil {
		t.Error("Expected error from onStop, got nil")
	}
}

func TestServiceState(t *testing.T) {
	service := NewService("test-service", "Test Service", "Test description")

	// Initial state should be stopped
	if service.IsRunning() {
		t.Error("Expected service to be stopped initially")
	}

	// Simulate start
	service.running = true
	if !service.IsRunning() {
		t.Error("Expected service to be running after setting running=true")
	}

	// Simulate stop
	service.running = false
	if service.IsRunning() {
		t.Error("Expected service to be stopped after setting running=false")
	}
}

func TestServiceConcurrentAccess(t *testing.T) {
	service := NewService("test-service", "Test Service", "Test description")

	// Start goroutines to test concurrent access
	done := make(chan bool)
	
	// Goroutine 1: Set running state
	go func() {
		for i := 0; i < 100; i++ {
			service.running = true
			time.Sleep(time.Millisecond)
			service.running = false
		}
		done <- true
	}()

	// Goroutine 2: Read running state
	go func() {
		for i := 0; i < 100; i++ {
			_ = service.IsRunning()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done
}

func TestServiceMultipleCallbacks(t *testing.T) {
	service := NewService("test-service", "Test Service", "Test description")

	startCount := 0
	stopCount := 0

	service.SetCallbacks(
		func() error {
			startCount++
			return nil
		},
		func() error {
			stopCount++
			return nil
		},
	)

	// Call callbacks multiple times
	for i := 0; i < 5; i++ {
		_ = service.onStart()
		_ = service.onStop()
	}

	if startCount != 5 {
		t.Errorf("Expected startCount=5, got %d", startCount)
	}

	if stopCount != 5 {
		t.Errorf("Expected stopCount=5, got %d", stopCount)
	}
}

func TestServiceCallbackReplacement(t *testing.T) {
	service := NewService("test-service", "Test Service", "Test description")

	firstStartCalled := false
	secondStartCalled := false

	// Set first callback
	service.SetCallbacks(
		func() error {
			firstStartCalled = true
			return nil
		},
		func() error { return nil },
	)

	// Replace with second callback
	service.SetCallbacks(
		func() error {
			secondStartCalled = true
			return nil
		},
		func() error { return nil },
	)

	// Call callback - should only call the second one
	_ = service.onStart()

	if firstStartCalled {
		t.Error("First callback should not be called after replacement")
	}

	if !secondStartCalled {
		t.Error("Second callback should be called")
	}
}

func TestServiceNilCallbacks(t *testing.T) {
	service := NewService("test-service", "Test Service", "Test description")

	// Should not panic with nil callbacks
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Service panicked with nil callbacks: %v", r)
		}
	}()

	// These should not panic even with nil callbacks
	_ = service.onStart()
	_ = service.onStop()
}

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
