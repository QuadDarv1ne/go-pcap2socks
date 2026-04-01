// Package errors provides structured error tests.
package errors

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

// TestErrorCategoryString tests category string conversion
func TestErrorCategoryString(t *testing.T) {
	tests := []struct {
		category ErrorCategory
		want     string
	}{
		{CategoryUnknown, "unknown"},
		{CategoryNetwork, "network"},
		{CategoryProxy, "proxy"},
		{CategoryConfig, "config"},
		{CategoryDNS, "dns"},
		{CategoryDHCP, "dhcp"},
		{CategoryRouting, "routing"},
		{CategoryAuth, "auth"},
		{CategoryTimeout, "timeout"},
		{CategoryResource, "resource"},
		{ErrorCategory(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.category.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestErrorError tests error message formatting
func TestErrorError(t *testing.T) {
	err := &Error{
		Category: CategoryNetwork,
		Code:     "TEST",
		Message:  "test error",
	}

	want := "[network:TEST] test error"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}

// TestErrorErrorWithCause tests error with cause
func TestErrorErrorWithCause(t *testing.T) {
	cause := errors.New("underlying cause")
	err := &Error{
		Category: CategoryProxy,
		Code:     "CONNECT",
		Message:  "connection failed",
		Cause:    cause,
	}

	want := "[proxy:CONNECT] connection failed: underlying cause"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %v, want %v", got, want)
	}
}

// TestErrorUnwrap tests error unwrapping
func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("cause")
	err := &Error{
		Cause: cause,
	}

	if unwrapped := err.Unwrap(); unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}
}

// TestErrorIs tests error comparison
func TestErrorIs(t *testing.T) {
	err1 := &Error{
		Category: CategoryNetwork,
		Code:     "TIMEOUT",
	}

	err2 := &Error{
		Category: CategoryNetwork,
		Code:     "TIMEOUT",
	}

	err3 := &Error{
		Category: CategoryNetwork,
		Code:     "UNREACHABLE",
	}

	if !err1.Is(err2) {
		t.Error("Same category and code should be equal")
	}

	if err1.Is(err3) {
		t.Error("Different code should not be equal")
	}

	regularErr := errors.New("regular")
	if err1.Is(regularErr) {
		t.Error("Should not match non-Error type")
	}
}

// TestErrorWithContext tests context addition
func TestErrorWithContext(t *testing.T) {
	err := New(CategoryProxy, "TEST", "test error")
	err = err.WithContext("key", "value")
	err = err.WithContext("number", 42)

	if err.Context == nil {
		t.Fatal("Context should not be nil")
	}

	if v, ok := err.Context["key"]; !ok || v != "value" {
		t.Errorf("Context[key] = %v, want 'value'", v)
	}

	if v, ok := err.Context["number"]; !ok || v != 42 {
		t.Errorf("Context[number] = %v, want 42", v)
	}
}

// TestErrorWithRetryable tests retryable flag
func TestErrorWithRetryable(t *testing.T) {
	err := New(CategoryNetwork, "TIMEOUT", "timeout")

	if err.Retryable {
		t.Error("Retryable should be false initially")
	}

	err = err.WithRetryable()

	if !err.Retryable {
		t.Error("Retryable should be true after WithRetryable")
	}
}

// TestNew tests error creation
func TestNew(t *testing.T) {
	err := New(CategoryDNS, "NOT_FOUND", "domain not found")

	if err.Category != CategoryDNS {
		t.Errorf("Category = %v, want %v", err.Category, CategoryDNS)
	}
	if err.Code != "NOT_FOUND" {
		t.Errorf("Code = %v, want 'NOT_FOUND'", err.Code)
	}
	if err.Message != "domain not found" {
		t.Errorf("Message = %v, want 'domain not found'", err.Message)
	}
	if err.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
	if err.Cause != nil {
		t.Error("Cause should be nil")
	}
}

// TestWrap tests error wrapping
func TestWrap(t *testing.T) {
	cause := errors.New("underlying")
	err := Wrap(cause, CategoryProxy, "AUTH", "auth failed")

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
	if err.Category != CategoryProxy {
		t.Errorf("Category = %v, want %v", err.Category, CategoryProxy)
	}
}

// TestWrapf tests error wrapping with formatting
func TestWrapf(t *testing.T) {
	cause := errors.New("underlying")
	err := Wrapf(cause, CategoryNetwork, "CONN", "connection to %s failed", "example.com")

	if err.Message != "connection to example.com failed" {
		t.Errorf("Message = %v, want 'connection to example.com failed'", err.Message)
	}
}

// TestNewf tests error creation with formatting
func TestNewf(t *testing.T) {
	err := Newf(CategoryConfig, "INVALID", "invalid config for %s", "proxy")

	if err.Message != "invalid config for proxy" {
		t.Errorf("Message = %v, want 'invalid config for proxy'", err.Message)
	}
}

// TestPredefinedErrors tests predefined error values
func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		err      *Error
		category ErrorCategory
	}{
		{ErrProxyNotSet, CategoryProxy},
		{ErrProxyTimeout, CategoryProxy},
		{ErrNetworkTimeout, CategoryNetwork},
		{ErrDNSNotFound, CategoryDNS},
		{ErrDHCPNoIPs, CategoryDHCP},
		{ErrRouteNotFound, CategoryRouting},
	}

	for _, tt := range tests {
		t.Run(tt.err.Code, func(t *testing.T) {
			if tt.err.Category != tt.category {
				t.Errorf("Category = %v, want %v", tt.err.Category, tt.category)
			}
		})
	}
}

// TestNewNetworkError tests network error helper
func TestNewNetworkError(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	cause := errors.New("connection refused")

	err := NewNetworkError("dial", addr, cause)

	if err.Category != CategoryNetwork {
		t.Errorf("Category = %v, want %v", err.Category, CategoryNetwork)
	}
	if err.Code != "CONNECTION_FAILED" {
		t.Errorf("Code = %v, want 'CONNECTION_FAILED'", err.Code)
	}
	if err.Context["address"] != "127.0.0.1:8080" {
		t.Errorf("Context[address] = %v, want '127.0.0.1:8080'", err.Context["address"])
	}
}

// TestNewProxyError tests proxy error helper
func TestNewProxyError(t *testing.T) {
	cause := errors.New("timeout")

	err := NewProxyError("socks5", "proxy.example.com:1080", cause)

	if err.Category != CategoryProxy {
		t.Errorf("Category = %v, want %v", err.Category, CategoryProxy)
	}
	if err.Context["tag"] != "socks5" {
		t.Errorf("Context[tag] = %v, want 'socks5'", err.Context["tag"])
	}
	if err.Context["address"] != "proxy.example.com:1080" {
		t.Errorf("Context[address] = %v, want 'proxy.example.com:1080'", err.Context["address"])
	}
}

// TestIsRetryable tests retryable detection
func TestIsRetryable(t *testing.T) {
	retryableErr := New(CategoryNetwork, "TIMEOUT", "timeout").WithRetryable()
	nonRetryableErr := New(CategoryConfig, "INVALID", "invalid")

	if !IsRetryable(retryableErr) {
		t.Error("Should be retryable")
	}

	if IsRetryable(nonRetryableErr) {
		t.Error("Should not be retryable")
	}

	regularErr := errors.New("regular")
	if IsRetryable(regularErr) {
		t.Error("Regular error should not be retryable")
	}
}

// TestIsCategory tests category check
func TestIsCategory(t *testing.T) {
	err := New(CategoryDNS, "NOT_FOUND", "not found")

	if !IsCategory(err, CategoryDNS) {
		t.Error("Should be DNS category")
	}

	if IsCategory(err, CategoryProxy) {
		t.Error("Should not be Proxy category")
	}

	regularErr := errors.New("regular")
	if IsCategory(regularErr, CategoryDNS) {
		t.Error("Regular error should not match any category")
	}
}

// TestGetCategory tests category extraction
func TestGetCategory(t *testing.T) {
	err := New(CategoryAuth, "FAILED", "auth failed")

	if got := GetCategory(err); got != CategoryAuth {
		t.Errorf("GetCategory() = %v, want %v", got, CategoryAuth)
	}

	regularErr := errors.New("regular")
	if got := GetCategory(regularErr); got != CategoryUnknown {
		t.Errorf("GetCategory() = %v, want %v", got, CategoryUnknown)
	}
}

// TestGetContext tests context extraction
func TestGetContext(t *testing.T) {
	err := New(CategoryProxy, "TEST", "test").WithContext("key", "value")

	ctx := GetContext(err)
	if ctx == nil {
		t.Fatal("Context should not be nil")
	}
	if v, ok := ctx["key"]; !ok || v != "value" {
		t.Errorf("Context[key] = %v, want 'value'", v)
	}

	regularErr := errors.New("regular")
	if ctx := GetContext(regularErr); ctx != nil {
		t.Errorf("GetContext() = %v, want nil", ctx)
	}
}

// TestErrorAs tests errors.As compatibility
func TestErrorAs(t *testing.T) {
	var err error = New(CategoryNetwork, "TEST", "test error")

	var structuredErr *Error
	if !errors.As(err, &structuredErr) {
		t.Fatal("Should unwrap to *Error")
	}

	if structuredErr.Category != CategoryNetwork {
		t.Errorf("Category = %v, want %v", structuredErr.Category, CategoryNetwork)
	}
}

// TestWrappedErrorAs tests errors.As with wrapped errors
func TestWrappedErrorAs(t *testing.T) {
	baseErr := New(CategoryProxy, "BASE", "base error")
	wrapped := fmt.Errorf("wrapped: %w", baseErr)

	var structuredErr *Error
	if !errors.As(wrapped, &structuredErr) {
		t.Fatal("Should unwrap to *Error")
	}

	if structuredErr.Code != "BASE" {
		t.Errorf("Code = %v, want 'BASE'", structuredErr.Code)
	}
}

// BenchmarkErrorCreation benchmarks error creation
func BenchmarkErrorCreation(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		New(CategoryNetwork, "TEST", "test error")
	}
}

// BenchmarkErrorWrap benchmarks error wrapping
func BenchmarkErrorWrap(b *testing.B) {
	b.ReportAllocs()
	baseErr := errors.New("base error")
	for i := 0; i < b.N; i++ {
		Wrap(baseErr, CategoryProxy, "TEST", "wrapped")
	}
}

// BenchmarkErrorWithContext benchmarks context addition
func BenchmarkErrorWithContext(b *testing.B) {
	b.ReportAllocs()
	err := New(CategoryNetwork, "TEST", "test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err.WithContext("key", "value")
	}
}
