// Package di provides service registration helpers for go-pcap2socks.
// This file contains common service registrations for the application.
package di

import (
	"context"
	"log/slog"
	"time"
)

// RegisterCommonServices registers common services used across go-pcap2socks.
// This includes logging, configuration, and core components.
func RegisterCommonServices(builder *ContainerBuilder) *ContainerBuilder {
	return builder.
		AddSingleton((*Logger)(nil), NewLogger).
		AddSingleton((*ConfigService)(nil), NewConfigService).
		AddSingleton((*HealthChecker)(nil), NewHealthChecker)
}

// Logger interface for structured logging
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	With(args ...interface{}) Logger
}

// SlogLogger implements Logger using log/slog
type SlogLogger struct {
	logger *slog.Logger
}

func NewLogger() *SlogLogger {
	return &SlogLogger{
		logger: slog.Default(),
	}
}

func (l *SlogLogger) Debug(msg string, args ...interface{}) {
	l.logger.Debug(msg, args...)
}

func (l *SlogLogger) Info(msg string, args ...interface{}) {
	l.logger.Info(msg, args...)
}

func (l *SlogLogger) Warn(msg string, args ...interface{}) {
	l.logger.Warn(msg, args...)
}

func (l *SlogLogger) Error(msg string, args ...interface{}) {
	l.logger.Error(msg, args...)
}

func (l *SlogLogger) With(args ...interface{}) Logger {
	return &SlogLogger{
		logger: l.logger.With(args...),
	}
}

// ConfigService provides configuration management
type ConfigService interface {
	Get(key string) interface{}
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	GetDuration(key string) time.Duration
	Set(key string, value interface{})
	Load(path string) error
	Save(path string) error
}

// configServiceImpl implements ConfigService
type configServiceImpl struct {
	data map[string]interface{}
}

func NewConfigService() *configServiceImpl {
	return &configServiceImpl{
		data: make(map[string]interface{}),
	}
}

func (c *configServiceImpl) Get(key string) interface{} {
	return c.data[key]
}

func (c *configServiceImpl) GetString(key string) string {
	if v, ok := c.data[key].(string); ok {
		return v
	}
	return ""
}

func (c *configServiceImpl) GetInt(key string) int {
	if v, ok := c.data[key].(int); ok {
		return v
	}
	return 0
}

func (c *configServiceImpl) GetBool(key string) bool {
	if v, ok := c.data[key].(bool); ok {
		return v
	}
	return false
}

func (c *configServiceImpl) GetDuration(key string) time.Duration {
	if v, ok := c.data[key].(time.Duration); ok {
		return v
	}
	return 0
}

func (c *configServiceImpl) Set(key string, value interface{}) {
	c.data[key] = value
}

func (c *configServiceImpl) Load(path string) error {
	// Implementation would load from file
	return nil
}

func (c *configServiceImpl) Save(path string) error {
	// Implementation would save to file
	return nil
}

// HealthChecker monitors service health
type HealthChecker interface {
	Start(ctx context.Context) error
	Stop() error
	RegisterService(name string, check HealthCheckFunc) error
	GetStatus(name string) ServiceStatus
	GetAllStatus() map[string]ServiceStatus
}

// HealthCheckFunc is a function that checks service health
type HealthCheckFunc func(ctx context.Context) error

// ServiceStatus represents the health status of a service
type ServiceStatus struct {
	Name      string
	Healthy   bool
	Error     error
	LastCheck time.Time
	Latency   time.Duration
}

// healthCheckerImpl implements HealthChecker
type healthCheckerImpl struct {
	services map[string]HealthCheckFunc
	status   map[string]ServiceStatus
	stopChan chan struct{}
}

func NewHealthChecker() *healthCheckerImpl {
	return &healthCheckerImpl{
		services: make(map[string]HealthCheckFunc),
		status:   make(map[string]ServiceStatus),
		stopChan: make(chan struct{}),
	}
}

func (h *healthCheckerImpl) Start(ctx context.Context) error {
	go h.runChecks(ctx)
	return nil
}

func (h *healthCheckerImpl) Stop() error {
	close(h.stopChan)
	return nil
}

func (h *healthCheckerImpl) RegisterService(name string, check HealthCheckFunc) error {
	h.services[name] = check
	return nil
}

func (h *healthCheckerImpl) GetStatus(name string) ServiceStatus {
	return h.status[name]
}

func (h *healthCheckerImpl) GetAllStatus() map[string]ServiceStatus {
	result := make(map[string]ServiceStatus, len(h.status))
	for k, v := range h.status {
		result[k] = v
	}
	return result
}

func (h *healthCheckerImpl) runChecks(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkAll(ctx)
		case <-h.stopChan:
			return
		}
	}
}

func (h *healthCheckerImpl) checkAll(ctx context.Context) {
	for name, check := range h.services {
		start := time.Now()
		err := check(ctx)
		latency := time.Since(start)

		h.status[name] = ServiceStatus{
			Name:      name,
			Healthy:   err == nil,
			Error:     err,
			LastCheck: time.Now(),
			Latency:   latency,
		}
	}
}
