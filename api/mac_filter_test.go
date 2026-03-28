package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

func TestMACFilterAPI_NewMACFilterAPI(t *testing.T) {
	api := NewMACFilterAPI(nil, "", nil)
	if api == nil {
		t.Fatal("NewMACFilterAPI returned nil")
	}
}

func TestMACFilterAPI_GetMode(t *testing.T) {
	// Test with nil config
	api := NewMACFilterAPI(nil, "", nil)
	mode := api.GetMode()
	if mode != "disabled" {
		t.Errorf("Expected mode 'disabled', got '%s'", mode)
	}

	// Test with config
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterWhitelist,
		List: []string{},
	}
	api = NewMACFilterAPI(filter, "", nil)
	mode = api.GetMode()
	if mode != "whitelist" {
		t.Errorf("Expected mode 'whitelist', got '%s'", mode)
	}
}

func TestMACFilterAPI_GetList(t *testing.T) {
	// Test with nil config
	api := NewMACFilterAPI(nil, "", nil)
	list := api.GetList()
	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d items", len(list))
	}

	// Test with config
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterWhitelist,
		List: []string{"AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"},
	}
	api = NewMACFilterAPI(filter, "", nil)
	list = api.GetList()
	if len(list) != 2 {
		t.Errorf("Expected 2 items, got %d", len(list))
	}
}

func TestMACFilterAPI_HandleGet(t *testing.T) {
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterBlacklist,
		List: []string{"AA:BB:CC:DD:EE:FF"},
	}
	api := NewMACFilterAPI(filter, "", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/macfilter", nil)
	w := httptest.NewRecorder()

	api.HandleGet(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp MACFilterResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Mode != "blacklist" {
		t.Errorf("Expected mode 'blacklist', got '%s'", resp.Mode)
	}
	if resp.Count != 1 {
		t.Errorf("Expected count 1, got %d", resp.Count)
	}
}

func TestMACFilterAPI_HandlePost(t *testing.T) {
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterWhitelist,
		List: []string{},
	}
	api := NewMACFilterAPI(filter, "", func(f *cfg.MACFilter) error {
		filter = f
		return nil
	})

	body := bytes.NewReader([]byte(`{"mac":"AA:BB:CC:DD:EE:FF"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/macfilter", body)
	w := httptest.NewRecorder()

	api.HandlePost(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp MACFilterResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Count != 1 {
		t.Errorf("Expected count 1, got %d", resp.Count)
	}
}

func TestMACFilterAPI_HandlePost_Duplicate(t *testing.T) {
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterWhitelist,
		List: []string{"AA:BB:CC:DD:EE:FF"},
	}
	api := NewMACFilterAPI(filter, "", nil)

	body := bytes.NewReader([]byte(`{"mac":"AA:BB:CC:DD:EE:FF"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/macfilter", body)
	w := httptest.NewRecorder()

	api.HandlePost(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", w.Code)
	}
}

func TestMACFilterAPI_HandleDelete(t *testing.T) {
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterWhitelist,
		List: []string{"AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"},
	}
	api := NewMACFilterAPI(filter, "", func(f *cfg.MACFilter) error {
		filter = f
		return nil
	})

	body := bytes.NewReader([]byte(`{"mac":"AA:BB:CC:DD:EE:FF"}`))
	req := httptest.NewRequest(http.MethodDelete, "/api/macfilter", body)
	w := httptest.NewRecorder()

	api.HandleDelete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp MACFilterResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Count != 1 {
		t.Errorf("Expected count 1, got %d", resp.Count)
	}
}

func TestMACFilterAPI_HandleDelete_NotFound(t *testing.T) {
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterWhitelist,
		List: []string{"AA:BB:CC:DD:EE:FF"},
	}
	api := NewMACFilterAPI(filter, "", nil)

	body := bytes.NewReader([]byte(`{"mac":"11:22:33:44:55:66"}`))
	req := httptest.NewRequest(http.MethodDelete, "/api/macfilter", body)
	w := httptest.NewRecorder()

	api.HandleDelete(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestMACFilterAPI_HandleCheck(t *testing.T) {
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterWhitelist,
		List: []string{"AA:BB:CC:DD:EE:FF"},
	}
	api := NewMACFilterAPI(filter, "", nil)

	// Check allowed MAC
	body := bytes.NewReader([]byte(`{"mac":"AA:BB:CC:DD:EE:FF"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/macfilter/check", body)
	w := httptest.NewRecorder()

	api.HandleCheck(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp MACFilterResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !resp.Allowed {
		t.Error("Expected MAC to be allowed")
	}

	// Check blocked MAC (new request)
	body2 := bytes.NewReader([]byte(`{"mac":"11:22:33:44:55:66"}`))
	req2 := httptest.NewRequest(http.MethodPost, "/api/macfilter/check", body2)
	w2 := httptest.NewRecorder()

	api.HandleCheck(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp2 MACFilterResponse
	if err := json.NewDecoder(w2.Body).Decode(&resp2); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp2.Allowed {
		t.Error("Expected MAC to be blocked")
	}
}

func TestMACFilterAPI_HandleMode(t *testing.T) {
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterDisabled,
		List: []string{},
	}
	api := NewMACFilterAPI(filter, "", func(f *cfg.MACFilter) error {
		filter = f
		return nil
	})

	body := bytes.NewReader([]byte(`{"mode":"whitelist"}`))
	req := httptest.NewRequest(http.MethodPut, "/api/macfilter/mode", body)
	w := httptest.NewRecorder()

	api.HandleMode(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	if filter.Mode != cfg.MACFilterWhitelist {
		t.Errorf("Expected mode 'whitelist', got '%s'", filter.Mode)
	}
}

func TestMACFilterAPI_HandleClear(t *testing.T) {
	filter := &cfg.MACFilter{
		Mode: cfg.MACFilterWhitelist,
		List: []string{"AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"},
	}
	api := NewMACFilterAPI(filter, "", func(f *cfg.MACFilter) error {
		filter = f
		return nil
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/macfilter/clear", nil)
	w := httptest.NewRecorder()

	api.HandleClear(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp MACFilterResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Count != 0 {
		t.Errorf("Expected count 0, got %d", resp.Count)
	}
}

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"AA:BB:CC:DD:EE:FF", "AA:BB:CC:DD:EE:FF"},
		{"aa:bb:cc:dd:ee:ff", "AA:BB:CC:DD:EE:FF"},
		{"AA-BB-CC-DD-EE-FF", "AA:BB:CC:DD:EE:FF"},
		{"aabbccddeeff", "AA:BB:CC:DD:EE:FF"},
		{"AABBCCDDEEFF", "AA:BB:CC:DD:EE:FF"},
		{"invalid", ""},
		{"GG:GG:GG:GG:GG:GG", ""},
		{"AA:BB:CC:DD:EE", ""},
	}

	for _, tt := range tests {
		result := normalizeMAC(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeMAC(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
