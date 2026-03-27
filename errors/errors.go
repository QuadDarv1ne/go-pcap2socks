// Package errors provides structured error handling for go-pcap2socks.
// This package offers categorized errors with context support for better debugging.
package errors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// ErrorCategory represents the classification of an error for better monitoring and alerting.
type ErrorCategory int

const (
	// CategoryUnknown is the default category for unclassified errors
	CategoryUnknown ErrorCategory = iota
	// CategoryNetwork for network-related errors (connections, timeouts, etc.)
	CategoryNetwork
	// CategoryProxy for proxy-specific errors (SOCKS5, HTTP, etc.)
	CategoryProxy
	// CategoryConfig for configuration errors
	CategoryConfig
	// CategoryDNS for DNS resolution errors
	CategoryDNS
	// CategoryDHCP for DHCP server/client errors
	CategoryDHCP
	// CategoryRouting for routing table and rule errors
	CategoryRouting
	// CategoryAuth for authentication errors
	CategoryAuth
	// CategoryTimeout for timeout errors
	CategoryTimeout
	// CategoryResource for resource exhaustion (memory, file descriptors, etc.)
	CategoryResource
)

func (c ErrorCategory) String() string {
	switch c {
	case CategoryNetwork:
		return "network"
	case CategoryProxy:
		return "proxy"
	case CategoryConfig:
		return "config"
	case CategoryDNS:
		return "dns"
	case CategoryDHCP:
		return "dhcp"
	case CategoryRouting:
		return "routing"
	case CategoryAuth:
		return "auth"
	case CategoryTimeout:
		return "timeout"
	case CategoryResource:
		return "resource"
	default:
		return "unknown"
	}
}

// Error is the main structured error type with rich context.
type Error struct {
	Category  ErrorCategory
	Code      string
	Message   string
	Cause     error
	Context   map[string]interface{}
	Timestamp time.Time
	Retryable bool
}

func (e *Error) Error() string {
	var sb strings.Builder
	sb.WriteString("[")
	sb.WriteString(e.Category.String())
	if e.Code != "" {
		sb.WriteString(":")
		sb.WriteString(e.Code)
	}
	sb.WriteString("] ")
	sb.WriteString(e.Message)
	if e.Cause != nil {
		sb.WriteString(": ")
		sb.WriteString(e.Cause.Error())
	}
	return sb.String()
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Category == t.Category && e.Code == t.Code
}

// WithContext adds context to the error for better debugging
func (e *Error) WithContext(key string, value interface{}) *Error {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithRetryable marks the error as retryable
func (e *Error) WithRetryable() *Error {
	e.Retryable = true
	return e
}

// New creates a new structured error
func New(category ErrorCategory, code, message string) *Error {
	return &Error{
		Category:  category,
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
	}
}

// Wrap wraps an existing error with additional context
func Wrap(cause error, category ErrorCategory, code, message string) *Error {
	return &Error{
		Category:  category,
		Code:      code,
		Message:   message,
		Cause:     cause,
		Timestamp: time.Now(),
	}
}

// Wrapf wraps an error with formatting
func Wrapf(cause error, category ErrorCategory, code, format string, args ...interface{}) *Error {
	return &Error{
		Category:  category,
		Code:      code,
		Message:   fmt.Sprintf(format, args...),
		Cause:     cause,
		Timestamp: time.Now(),
	}
}

// Newf creates a new error with formatting
func Newf(category ErrorCategory, code, format string, args ...interface{}) *Error {
	return &Error{
		Category:  category,
		Code:      code,
		Message:   fmt.Sprintf(format, args...),
		Timestamp: time.Now(),
	}
}

// Predefined errors for common scenarios
var (
	// Proxy errors
	ErrProxyNotSet      = New(CategoryProxy, "NOT_SET", "proxy not set")
	ErrProxyTimeout     = New(CategoryProxy, "TIMEOUT", "proxy connection timeout")
	ErrProxyAuth        = New(CategoryProxy, "AUTH_FAILED", "proxy authentication failed")
	ErrProxyConnect     = New(CategoryProxy, "CONNECT_FAILED", "failed to connect to proxy")
	ErrProxyUnsupported = New(CategoryProxy, "UNSUPPORTED", "unsupported proxy type")
	
	// Network errors
	ErrNetworkTimeout   = New(CategoryNetwork, "TIMEOUT", "network operation timeout")
	ErrNetworkUnreachable = New(CategoryNetwork, "UNREACHABLE", "network unreachable")
	ErrConnectionRefused = New(CategoryNetwork, "CONNECTION_REFUSED", "connection refused")
	ErrConnectionReset  = New(CategoryNetwork, "CONNECTION_RESET", "connection reset by peer")
	
	// DNS errors
	ErrDNSNotFound      = New(CategoryDNS, "NOT_FOUND", "domain not found")
	ErrDNSResolution    = New(CategoryDNS, "RESOLUTION_FAILED", "DNS resolution failed")
	
	// DHCP errors
	ErrDHCPNoIPs        = New(CategoryDHCP, "NO_IPS", "no available IP addresses")
	ErrDHCPInvalidRequest = New(CategoryDHCP, "INVALID_REQUEST", "invalid DHCP request")
	
	// Routing errors
	ErrRouteNotFound    = New(CategoryRouting, "NOT_FOUND", "no matching route")
	ErrRouteMACFilter   = New(CategoryRouting, "MAC_FILTER", "blocked by MAC filter")
	ErrRouteInvalid     = New(CategoryRouting, "INVALID", "invalid routing rule")
	
	// Config errors
	ErrConfigInvalid    = New(CategoryConfig, "INVALID", "invalid configuration")
	ErrConfigMissing    = New(CategoryConfig, "MISSING", "required configuration missing")
	
	// Auth errors
	ErrAuthRequired     = New(CategoryAuth, "REQUIRED", "authentication required")
	ErrAuthFailed       = New(CategoryAuth, "FAILED", "authentication failed")
	
	// Timeout errors
	ErrTimeout          = New(CategoryTimeout, "EXCEEDED", "operation timeout exceeded")
	
	// Resource errors
	ErrResourceExhausted = New(CategoryResource, "EXHAUSTED", "resource exhausted")
)

// Helper functions for common error scenarios

// NewNetworkError creates a network error with operation context
func NewNetworkError(op string, addr net.Addr, cause error) *Error {
	err := Wrap(cause, CategoryNetwork, "CONNECTION_FAILED",
		fmt.Sprintf("%s failed", op))
	if addr != nil {
		err.WithContext("address", addr.String())
	}
	return err
}

// NewProxyError creates a proxy error with tag and address context
func NewProxyError(tag, addr string, cause error) *Error {
	err := Wrap(cause, CategoryProxy, "PROXY_ERROR",
		"proxy operation failed")
	err.WithContext("tag", tag)
	err.WithContext("address", addr)
	return err
}

// NewDNSError creates a DNS error with domain context
func NewDNSError(domain string, cause error) *Error {
	err := Wrap(cause, CategoryDNS, "DNS_ERROR",
		"DNS operation failed")
	err.WithContext("domain", domain)
	return err
}

// NewRoutingError creates a routing error with rule context
func NewRoutingError(ruleID string, cause error) *Error {
	err := Wrap(cause, CategoryRouting, "ROUTING_ERROR",
		"routing operation failed")
	err.WithContext("rule_id", ruleID)
	return err
}

// Utility functions

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Retryable
	}
	
	// Check for common retryable errors
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	
	return false
}

// IsCategory checks if an error belongs to a specific category
func IsCategory(err error, category ErrorCategory) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Category == category
	}
	return false
}

// GetCategory returns the category of an error
func GetCategory(err error) ErrorCategory {
	var e *Error
	if errors.As(err, &e) {
		return e.Category
	}
	return CategoryUnknown
}

// GetContext returns the context map from an error if available
func GetContext(err error) map[string]interface{} {
	var e *Error
	if errors.As(err, &e) {
		return e.Context
	}
	return nil
}
