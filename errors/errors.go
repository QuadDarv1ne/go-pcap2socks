// Package errors provides structured error handling for go-pcap2socks.
package errors

import (
	"errors"
	"fmt"
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

func (e *Error) WithContext(key string, value interface{}) *Error {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
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
	return Wrap(cause, CategoryNetwork, "CONNECTION_FAILED",
		fmt.Sprintf("%s failed", op))
}

func NewProxyError(tag, addr string, cause error) *Error {
	return Wrap(cause, CategoryProxy, "PROXY_ERROR",
		"proxy operation failed")
}
