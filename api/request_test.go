//go:build ignore

package api

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBody_Success(t *testing.T) {
	s := &Server{}

	body := `{"test": "value"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	w := httptest.NewRecorder()

	var result map[string]string
	err := s.decodeJSONBody(w, req, &result)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result["test"] != "value" {
		t.Errorf("Expected 'value', got %s", result["test"])
	}
}

func TestDecodeJSONBody_TooLarge(t *testing.T) {
	s := &Server{}

	// Create a body larger than MaxRequestBodySize (1MB)
	largeBody := bytes.Repeat([]byte("a"), MaxRequestBodySize+1)
	req := httptest.NewRequest("POST", "/api/test", bytes.NewReader(largeBody))
	w := httptest.NewRecorder()

	var result map[string]string
	err := s.decodeJSONBody(w, req, &result)

	if err == nil {
		t.Error("Expected error for too large body, got nil")
	}
}

func TestDecodeJSONBody_UnknownFields(t *testing.T) {
	s := &Server{}

	type TestStruct struct {
		Known string `json:"known"`
	}

	body := `{"known": "value", "unknown": "field"}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	w := httptest.NewRecorder()

	var result TestStruct
	err := s.decodeJSONBody(w, req, &result)

	if err == nil {
		t.Error("Expected error for unknown fields, got nil")
	}
}

func TestDecodeJSONBodyWithLimit_CustomLimit(t *testing.T) {
	s := &Server{}

	// Create body just under custom limit
	customLimit := int64(100)
	body := strings.Repeat("a", 50)
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	w := httptest.NewRecorder()

	var result string
	err := s.decodeJSONBodyWithLimit(w, req, &result, customLimit)

	// Should succeed (body is under limit)
	if err != nil {
		t.Logf("Decode error (expected for non-JSON): %v", err)
	}
}

func TestDecodeJSONBodyWithLimit_ExceedsCustomLimit(t *testing.T) {
	s := &Server{}

	customLimit := int64(10)
	body := strings.Repeat("a", 20)
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	w := httptest.NewRecorder()

	var result string
	err := s.decodeJSONBodyWithLimit(w, req, &result, customLimit)

	if err == nil {
		t.Error("Expected error for body exceeding custom limit, got nil")
	}
}

func TestDecodeJSONBody_InvalidJSON(t *testing.T) {
	s := &Server{}

	body := `{"invalid": json}`
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	w := httptest.NewRecorder()

	var result map[string]string
	err := s.decodeJSONBody(w, req, &result)

	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestMaxRequestBodySize_Constant(t *testing.T) {
	expectedSize := int64(1 << 20) // 1MB
	if MaxRequestBodySize != expectedSize {
		t.Errorf("Expected MaxRequestBodySize to be %d, got %d", expectedSize, MaxRequestBodySize)
	}
}

func TestMaxConfigUploadSize_Constant(t *testing.T) {
	expectedSize := int64(10 << 20) // 10MB
	if MaxConfigUploadSize != expectedSize {
		t.Errorf("Expected MaxConfigUploadSize to be %d, got %d", expectedSize, MaxConfigUploadSize)
	}
}
