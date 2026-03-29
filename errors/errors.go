// Package errors provides structured error handling for go-pcap2socks.
package errors

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

// ErrorCategory represents the classification of an error.
type ErrorCategory int

const (
	CategoryUnknown ErrorCategory = iota
	CategoryNetwork
	CategoryProxy
	CategoryConfig
	CategoryDNS
	CategoryDHCP
	CategoryRouting
	CategoryAuth
	CategoryTimeout
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

// Error is the main structured error type.
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

func New(category ErrorCategory, code, message string) *Error {
	return &Error{
		Category:  category,
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
	}
}

func Wrap(cause error, category ErrorCategory, code, message string) *Error {
	return &Error{
		Category:  category,
		Code:      code,
		Message:   message,
		Cause:     cause,
		Timestamp: time.Now(),
	}
}

// Predefined errors
var (
	ErrProxyNotSet      = New(CategoryProxy, "NOT_SET", "proxy not set")
	ErrProxyTimeout     = New(CategoryProxy, "TIMEOUT", "proxy connection timeout")
	ErrProxyAuth        = New(CategoryProxy, "AUTH_FAILED", "proxy authentication failed")
	ErrNetworkTimeout   = New(CategoryNetwork, "TIMEOUT", "network operation timeout")
	ErrDNSNotFound      = New(CategoryDNS, "NOT_FOUND", "domain not found")
	ErrDHCPNoIPs        = New(CategoryDHCP, "NO_IPS", "no available IP addresses")
	ErrRouteNotFound    = New(CategoryRouting, "NOT_FOUND", "no matching route")
	ErrRouteMACFilter   = New(CategoryRouting, "MAC_FILTER", "blocked by MAC filter")
)

func IsRetryable(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Retryable
	}
	return false
}

func NewNetworkError(op string, addr net.Addr, cause error) *Error {
	err := Wrap(cause, CategoryNetwork, "CONNECTION_FAILED",
		fmt.Sprintf("%s failed", op))
	if addr != nil {
		err = err.WithContext("address", addr.String())
	}
	return err
}

func NewProxyError(tag, addr string, cause error) *Error {
	err := Wrap(cause, CategoryProxy, "PROXY_ERROR",
		"proxy operation failed")
	err = err.WithContext("tag", tag)
	err = err.WithContext("address", addr)
	return err
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

// ToLogAttr converts error to slog.Attr for structured logging
func (e *Error) ToLogAttr() slog.Attr {
	return slog.Group("error",
		slog.String("category", e.Category.String()),
		slog.String("code", e.Code),
		slog.String("message", e.Message),
		slog.Bool("retryable", e.Retryable),
		slog.Time("timestamp", e.Timestamp),
	)
}

// LogAttrs returns slog.Attr slice for error with context
func (e *Error) LogAttrs() []slog.Attr {
	attrs := make([]slog.Attr, 0, 6+len(e.Context))
	attrs = append(attrs,
		slog.String("error.category", e.Category.String()),
		slog.String("error.code", e.Code),
		slog.String("error.message", e.Message),
		slog.Bool("error.retryable", e.Retryable),
		slog.Time("error.timestamp", e.Timestamp),
	)
	for k, v := range e.Context {
		attrs = append(attrs, slog.Any("error."+k, v))
	}
	return attrs
}

// LogError logs error with structured context using slog
func LogError(logger *slog.Logger, err error, msg string) {
	if logger == nil {
		logger = slog.Default()
	}

	var e *Error
	if errors.As(err, &e) {
		args := make([]any, 0, 6+len(e.Context))
		for _, attr := range e.LogAttrs() {
			args = append(args, attr.Key, attr.Value.Any())
		}
		args = append(args, "message", msg)
		logger.Error(msg, args...)
	} else {
		logger.Error(msg, slog.Any("error", err))
	}
}

// LogWarn logs warning with structured context using slog
func LogWarn(logger *slog.Logger, err error, msg string) {
	if logger == nil {
		logger = slog.Default()
	}

	var e *Error
	if errors.As(err, &e) {
		args := make([]any, 0, 6+len(e.Context))
		for _, attr := range e.LogAttrs() {
			args = append(args, attr.Key, attr.Value.Any())
		}
		args = append(args, "message", msg)
		logger.Warn(msg, args...)
	} else {
		logger.Warn(msg, slog.Any("error", err))
	}
}
