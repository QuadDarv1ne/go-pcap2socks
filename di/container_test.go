// Package di provides dependency injection container tests.
package di

import (
	"context"
	"errors"
	"testing"
)

// Test services for DI tests
type TestService struct {
	Name string
}

type TestService2 struct {
	Service *TestService
	Value   string
}

type TestServiceWithCtx struct {
	Context context.Context
}

// TestNewContainer tests container creation
func TestNewContainer(t *testing.T) {
	c := NewContainer()
	if c == nil {
		t.Fatal("NewContainer() returned nil")
	}
	if c.services == nil {
		t.Error("services map not initialized")
	}
	if c.instances == nil {
		t.Error("instances map not initialized")
	}
}

// TestRegisterSingleton tests singleton registration
func TestRegisterSingleton(t *testing.T) {
	c := NewContainer()
	
	err := c.RegisterSingleton((*TestService)(nil), func() *TestService {
		return &TestService{Name: "singleton"}
	})
	
	if err != nil {
		t.Fatalf("RegisterSingleton() error = %v", err)
	}
	
	// Resolve twice - should get same instance
	inst1, err := c.Resolve((*TestService)(nil))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	
	inst2, err := c.Resolve((*TestService)(nil))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	
	if inst1 != inst2 {
		t.Error("Singleton should return same instance")
	}
}

// TestRegisterTransient tests transient registration
func TestRegisterTransient(t *testing.T) {
	c := NewContainer()
	
	err := c.RegisterTransient((*TestService)(nil), func() *TestService {
		return &TestService{Name: "transient"}
	})
	
	if err != nil {
		t.Fatalf("RegisterTransient() error = %v", err)
	}
	
	// Resolve twice - should get different instances
	inst1, _ := c.Resolve((*TestService)(nil))
	inst2, _ := c.Resolve((*TestService)(nil))
	
	if inst1 == inst2 {
		t.Error("Transient should return different instances")
	}
}

// TestRegisterInstance tests instance registration
func TestRegisterInstance(t *testing.T) {
	c := NewContainer()
	
	instance := &TestService{Name: "pre-created"}
	
	err := c.RegisterInstance((*TestService)(nil), instance)
	if err != nil {
		t.Fatalf("RegisterInstance() error = %v", err)
	}
	
	resolved, err := c.Resolve((*TestService)(nil))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	
	if resolved != instance {
		t.Error("Should return the same registered instance")
	}
}

// TestResolveDependency tests dependency resolution
func TestResolveDependency(t *testing.T) {
	c := NewContainer()
	
	// Register TestService first
	c.RegisterSingleton((*TestService)(nil), func() *TestService {
		return &TestService{Name: "dependency"}
	})
	
	// Resolve TestService and use it to create TestService2
	ts := c.MustResolve((*TestService)(nil)).(*TestService)
	
	c.RegisterSingleton((*TestService2)(nil), func() *TestService2 {
		return &TestService2{
			Service: ts,
			Value:   "test",
		}
	})
	
	inst, err := c.Resolve((*TestService2)(nil))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	
	s2 := inst.(*TestService2)
	if s2.Service == nil {
		t.Error("Dependency should be resolved")
	}
	if s2.Service.Name != "dependency" {
		t.Errorf("Dependency name = %v, want 'dependency'", s2.Service.Name)
	}
}

// TestResolveWithContext tests context-aware resolution
func TestResolveWithContext(t *testing.T) {
	c := NewContainer()
	
	ctx := context.WithValue(context.Background(), "key", "value")
	
	c.RegisterSingleton((*TestServiceWithCtx)(nil), func(ctx context.Context) *TestServiceWithCtx {
		return &TestServiceWithCtx{Context: ctx}
	})
	
	inst, err := c.ResolveContext(ctx, (*TestServiceWithCtx)(nil))
	if err != nil {
		t.Fatalf("ResolveContext() error = %v", err)
	}
	
	s := inst.(*TestServiceWithCtx)
	if s.Context == nil {
		t.Error("Context should be passed")
	}
	
	val := s.Context.Value("key")
	if val != "value" {
		t.Errorf("Context value = %v, want 'value'", val)
	}
}

// TestCircularDependency tests circular dependency detection
func TestCircularDependency(t *testing.T) {
	c := NewContainer()
	
	// Create circular dependency: A -> B -> A
	c.RegisterSingleton((*TestService)(nil), func(s2 *TestService2) *TestService {
		return &TestService{Name: "A"}
	})
	
	c.RegisterSingleton((*TestService2)(nil), func(s *TestService) *TestService2 {
		return &TestService2{Service: s}
	})
	
	_, err := c.Resolve((*TestService)(nil))
	if err == nil {
		t.Error("Expected circular dependency error")
	}
}

// TestServiceNotFound tests error on missing service
func TestServiceNotFound(t *testing.T) {
	c := NewContainer()
	
	_, err := c.Resolve((*TestService)(nil))
	if err == nil {
		t.Error("Expected error for unregistered service")
	}
}

// TestMustResolve tests MustResolve panic
func TestMustResolve(t *testing.T) {
	c := NewContainer()
	
	// Should panic for unregistered service
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustResolve should panic for unregistered service")
		}
	}()
	
	c.MustResolve((*TestService)(nil))
}

// TestDispose tests container disposal
func TestDispose(t *testing.T) {
	c := NewContainer()
	
	c.RegisterSingleton((*TestService)(nil), func() *TestService {
		return &TestService{Name: "test"}
	})
	
	// Resolve to create instance
	c.Resolve((*TestService)(nil))
	
	// Dispose
	err := c.Dispose()
	if err != nil {
		t.Errorf("Dispose() error = %v", err)
	}
	
	// Should error on resolve after dispose
	_, err = c.Resolve((*TestService)(nil))
	if err == nil {
		t.Error("Expected error after dispose")
	}
}

// TestContainerBuilder tests fluent builder
func TestContainerBuilder(t *testing.T) {
	builder := NewContainerBuilder()
	
	c := builder.
		AddSingleton((*TestService)(nil), func() *TestService {
			return &TestService{Name: "built"}
		}).
		AddTransient((*TestService2)(nil), func(s *TestService) *TestService2 {
			return &TestService2{Service: s, Value: "transient"}
		}).
		MustBuild()
	
	inst, err := c.Resolve((*TestService)(nil))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	
	if inst.(*TestService).Name != "built" {
		t.Errorf("Service name = %v, want 'built'", inst.(*TestService).Name)
	}
}

// TestIsRegistered tests service registration check
func TestIsRegistered(t *testing.T) {
	c := NewContainer()
	
	if c.IsRegistered((*TestService)(nil)) {
		t.Error("Service should not be registered initially")
	}
	
	c.RegisterSingleton((*TestService)(nil), func() *TestService {
		return &TestService{}
	})
	
	if !c.IsRegistered((*TestService)(nil)) {
		t.Error("Service should be registered")
	}
}

// TestGetServiceCount tests service count
func TestGetServiceCount(t *testing.T) {
	c := NewContainer()
	
	if count := c.GetServiceCount(); count != 0 {
		t.Errorf("GetServiceCount() = %v, want 0", count)
	}
	
	c.RegisterSingleton((*TestService)(nil), func() *TestService { return &TestService{} })
	c.RegisterSingleton((*TestService2)(nil), func() *TestService2 { return &TestService2{} })
	
	if count := c.GetServiceCount(); count != 2 {
		t.Errorf("GetServiceCount() = %v, want 2", count)
	}
}

// TestConstructorError tests constructor error handling
func TestConstructorError(t *testing.T) {
	c := NewContainer()
	
	testErr := errors.New("constructor error")
	
	c.RegisterSingleton((*TestService)(nil), func() (*TestService, error) {
		return nil, testErr
	})
	
	_, err := c.Resolve((*TestService)(nil))
	if err != testErr {
		t.Errorf("Expected constructor error, got %v", err)
	}
}
