package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
)

func TestServerRoutes(t *testing.T) {
	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil)

	tests := []struct {
		method       string
		path         string
		expectedCode int
	}{
		{"GET", "/api/status", 200},
		{"GET", "/api/traffic", 200},
		{"GET", "/api/devices", 200},
		{"POST", "/api/start", 200},
		{"POST", "/api/stop", 200},
		{"GET", "/api/profiles", 200},
		{"GET", "/api/upnp", 200},
		{"GET", "/nonexistent", 404},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestStatusHandler(t *testing.T) {
	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil)

	// Set running state
	SetIsRunningFn(func() bool { return true })
	SetStartTime(time.Now().Add(-5 * time.Minute))

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	if running, ok := data["running"].(bool); !ok || !running {
		t.Error("Expected running to be true")
	}
}

func TestTrafficHandler(t *testing.T) {
	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/traffic", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	// Check traffic fields exist
	expectedFields := []string{"total_bytes", "upload_bytes", "download_bytes", "packets"}
	for _, field := range expectedFields {
		if _, exists := data[field]; !exists {
			t.Errorf("Expected field %s in response", field)
		}
	}
}

func TestDevicesHandler(t *testing.T) {
	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil)

	// Add a test device
	statsStore.RecordTraffic("192.168.137.100", "aa:bb:cc:dd:ee:ff", 1024, true)

	req := httptest.NewRequest("GET", "/api/devices", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.([]interface{})
	if !ok {
		t.Fatal("Expected data to be an array")
	}

	// Should have at least one device
	if len(data) == 0 {
		t.Error("Expected at least one device in response")
	}
}

func TestServiceStartHandler(t *testing.T) {
	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil)

	startCalled := false
	SetServiceCallbacks(
		func() error {
			startCalled = true
			return nil
		},
		func() error { return nil },
	)

	req := httptest.NewRequest("POST", "/api/start", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !startCalled {
		t.Error("Expected start callback to be called")
	}
}

func TestServiceStopHandler(t *testing.T) {
	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil)

	stopCalled := false
	SetServiceCallbacks(
		func() error { return nil },
		func() error {
			stopCalled = true
			return nil
		},
	)

	req := httptest.NewRequest("POST", "/api/stop", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if !stopCalled {
		t.Error("Expected stop callback to be called")
	}
}

func TestProfilesHandler(t *testing.T) {
	statsStore := stats.NewStore()

	// Create a profile manager for testing
	profileMgr, err := profiles.NewManager()
	if err != nil {
		t.Skipf("Skipping test: profile manager creation failed: %v", err)
		return
	}

	// Create default profiles
	if err := profileMgr.CreateDefaultProfiles(); err != nil {
		t.Skipf("Skipping test: failed to create default profiles: %v", err)
		return
	}

	server := NewServer(statsStore, profileMgr, nil, nil)

	req := httptest.NewRequest("GET", "/api/profiles", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response APIResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}
}

func TestUPnPHandler(t *testing.T) {
	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/upnp", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}
}

func TestSetStartTime(t *testing.T) {
	testTime := time.Now().Add(-10 * time.Minute)
	SetStartTime(testTime)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil)
	server.ServeHTTP(w, req)

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	startTimeStr, ok := data["start_time"].(string)
	if !ok {
		t.Fatal("Expected start_time in response")
	}

	// Parse the start time and verify it's approximately correct
	parsedTime, err := time.Parse(time.RFC3339Nano, startTimeStr)
	if err != nil {
		t.Fatalf("Failed to parse start_time: %v", err)
	}

	// Allow 1 second tolerance
	if parsedTime.Sub(testTime) > time.Second {
		t.Errorf("Expected start_time %v, got %v", testTime, parsedTime)
	}
}

func TestSetIsRunningFn(t *testing.T) {
	SetIsRunningFn(func() bool { return false })

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil)
	server.ServeHTTP(w, req)

	var response APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	running, ok := data["running"].(bool)
	if !ok {
		t.Fatal("Expected running field in response")
	}

	if running {
		t.Error("Expected running to be false")
	}
}

func TestAPIResponse(t *testing.T) {
	// Test success response
	successResp := SuccessResponse(map[string]string{"key": "value"})
	
	data, err := json.Marshal(successResp)
	if err != nil {
		t.Fatalf("Failed to marshal success response: %v", err)
	}

	var parsed APIResponse
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("Failed to unmarshal success response: %v", err)
	}

	if !parsed.Success {
		t.Error("Expected success to be true")
	}

	if parsed.Error != "" {
		t.Errorf("Expected empty error, got %s", parsed.Error)
	}

	// Test error response
	errorResp := ErrorResponse("test error")
	
	data, err = json.Marshal(errorResp)
	if err != nil {
		t.Fatalf("Failed to marshal error response: %v", err)
	}

	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if parsed.Success {
		t.Error("Expected success to be false")
	}

	if parsed.Error != "test error" {
		t.Errorf("Expected error 'test error', got %s", parsed.Error)
	}
}
