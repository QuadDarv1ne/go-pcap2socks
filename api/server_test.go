//go:build ignore

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
)

// createTestServer создаёт тестовый сервер с установленным токеном
func createTestServer() *Server {
	statsStore := stats.NewStore()
	server := NewServer(statsStore, nil, nil, nil, nil)
	server.SetAuthToken("test-token-123")
	return server
}

// createAuthRequest создаёт запрос с заголовком авторизации
func createAuthRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer test-token-123")
	return req
}

func TestServerRoutes(t *testing.T) {
	server := createTestServer()

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
			req := createAuthRequest(tt.method, tt.path)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestStatusHandler(t *testing.T) {
	server := createTestServer()

	// Set running state
	SetIsRunningFn(func() bool { return true })
	SetStartTime(time.Now().Add(-5 * time.Minute))

	req := createAuthRequest("GET", "/api/status")
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
	server := createTestServer()

	req := createAuthRequest("GET", "/api/traffic")
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
	server := createTestServer()

	// Add a test device
	server.statsStore.RecordTraffic("192.168.137.100", "aa:bb:cc:dd:ee:ff", 1024, true)

	req := createAuthRequest("GET", "/api/devices")
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
	server := createTestServer()

	startCalled := false
	SetServiceCallbacks(
		func() error {
			startCalled = true
			return nil
		},
		func() error { return nil },
	)

	req := createAuthRequest("POST", "/api/start")
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
	server := createTestServer()

	stopCalled := false
	SetServiceCallbacks(
		func() error { return nil },
		func() error {
			stopCalled = true
			return nil
		},
	)

	req := createAuthRequest("POST", "/api/stop")
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

	server := createTestServer()
	server.profileMgr = profileMgr

	req := createAuthRequest("GET", "/api/profiles")
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
	server := createTestServer()

	req := createAuthRequest("GET", "/api/upnp")
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

	server := createTestServer()

	req := createAuthRequest("GET", "/api/status")
	w := httptest.NewRecorder()

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

	server := createTestServer()

	req := createAuthRequest("GET", "/api/status")
	w := httptest.NewRecorder()

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

func TestHandleHotkey_Success(t *testing.T) {
	// Test with nil hotkey manager - should return enabled=false
	server := createTestServer()

	req := createAuthRequest(http.MethodGet, "/api/hotkey")
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

	// With nil hotkey manager, enabled should be false
	enabled, ok := data["enabled"].(bool)
	if ok && enabled {
		t.Error("Expected enabled to be false with nil hotkey manager")
	}

	// Hotkeys should be empty or null
	hotkeys, ok := data["hotkeys"].([]interface{})
	if ok && len(hotkeys) > 0 {
		t.Errorf("Expected 0 hotkeys, got %d", len(hotkeys))
	}
}

func TestHandleHotkey_NoHotkeys(t *testing.T) {
	server := createTestServer()

	req := createAuthRequest(http.MethodGet, "/api/hotkey")
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

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	enabled, ok := data["enabled"].(bool)
	if ok && enabled {
		t.Error("Expected enabled to be false when no hotkeys registered")
	}
}

func TestHandleHotkey_MethodNotAllowed(t *testing.T) {
	server := createTestServer()

	req := createAuthRequest(http.MethodPost, "/api/hotkey")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHandleHotkeyToggle_Success(t *testing.T) {
	server := createTestServer()

	body := `{"action": "toggle", "hotkey": "toggle_proxy"}`
	req := httptest.NewRequest(http.MethodPost, "/api/hotkey/toggle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token-123")
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

	if data["action"] != "toggle" {
		t.Errorf("Expected action 'toggle', got %v", data["action"])
	}

	if data["status"] != "acknowledged" {
		t.Errorf("Expected status 'acknowledged', got %v", data["status"])
	}
}

func TestHandleHotkeyToggle_InvalidMethod(t *testing.T) {
	server := createTestServer()

	req := createAuthRequest(http.MethodGet, "/api/hotkey/toggle")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHandleHotkeyToggle_InvalidBody(t *testing.T) {
	server := createTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/hotkey/toggle", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token-123")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestKeyToString(t *testing.T) {
	tests := []struct {
		vk       int
		expected string
	}{
		{hotkey.VK_P, "P"},
		{hotkey.VK_R, "R"},
		{hotkey.VK_S, "S"},
		{hotkey.VK_L, "L"},
		{0xFF, "?"}, // Unknown key
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := keyToString(tt.vk)
			if result != tt.expected {
				t.Errorf("keyToString(%d) = %s, expected %s", tt.vk, result, tt.expected)
			}
		})
	}
}
