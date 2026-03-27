// Package di provides dependency injection container for go-pcap2socks.
// This package offers a lightweight DI container with support for different lifecycles.
package di

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

// Lifecycle defines the lifetime of a service.
type Lifecycle int

const (
	// Singleton: One shared instance for the entire application lifetime
	Singleton Lifecycle = iota
	// Transient: New instance every time Resolve is called
	Transient
	// Scoped: One instance per scope (not yet implemented, behaves as Singleton)
	Scoped
)

func (l Lifecycle) String() string {
	switch l {
	case Singleton:
		return "singleton"
	case Transient:
		return "transient"
	case Scoped:
		return "scoped"
	default:
		return "unknown"
	}
}

// Container is the dependency injection container.
// It is safe for concurrent use.
type Container struct {
	mu         sync.RWMutex
	services   map[reflect.Type]*serviceDescriptor
	instances  map[reflect.Type]interface{}
	resolving  map[reflect.Type]bool
	isDisposed bool
}

type serviceDescriptor struct {
	serviceType reflect.Type
	lifecycle   Lifecycle
	constructor interface{}
	instance    interface{}
	initialized bool
}

// NewContainer creates a new DI container.
func NewContainer() *Container {
	return &Container{
		services:  make(map[reflect.Type]*serviceDescriptor),
		instances: make(map[reflect.Type]interface{}),
		resolving: make(map[reflect.Type]bool),
	}
}

// RegisterSingleton registers a singleton service.
// The same instance will be returned for all Resolve calls.
func (c *Container) RegisterSingleton(serviceType interface{}, constructor interface{}) error {
	return c.Register(serviceType, constructor, Singleton)
}

// RegisterTransient registers a transient service.
// A new instance will be created for each Resolve call.
func (c *Container) RegisterTransient(serviceType interface{}, constructor interface{}) error {
	return c.Register(serviceType, constructor, Transient)
}

// Register registers a service with the specified lifecycle.
func (c *Container) Register(serviceType interface{}, constructor interface{}, lifecycle Lifecycle) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isDisposed {
		return fmt.Errorf("container is disposed")
	}

	typ := reflect.TypeOf(serviceType)
	if typ == nil {
		return fmt.Errorf("serviceType must be a non-nil pointer")
	}

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct && typ.Kind() != reflect.Interface {
		return fmt.Errorf("serviceType must be a pointer to struct or interface, got %v", typ.Kind())
	}

	ctorType := reflect.TypeOf(constructor)
	if ctorType == nil || ctorType.Kind() != reflect.Func {
		return fmt.Errorf("constructor must be a function")
	}

	c.services[typ] = &serviceDescriptor{
		serviceType: typ,
		lifecycle:   lifecycle,
		constructor: constructor,
	}

	return nil
}

// RegisterInstance registers an existing instance as a singleton.
// Useful for registering external dependencies or pre-created objects.
func (c *Container) RegisterInstance(serviceType interface{}, instance interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isDisposed {
		return fmt.Errorf("container is disposed")
	}

	typ := reflect.TypeOf(serviceType)
	if typ == nil {
		return fmt.Errorf("serviceType must be a non-nil pointer")
	}

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	c.services[typ] = &serviceDescriptor{
		serviceType: typ,
		lifecycle:   Singleton,
		instance:    instance,
		initialized: true,
	}

	c.instances[typ] = instance
	return nil
}

// Resolve retrieves a service instance.
func (c *Container) Resolve(serviceType interface{}) (interface{}, error) {
	return c.ResolveContext(context.Background(), serviceType)
}

// ResolveContext retrieves a service instance with context support.
func (c *Container) ResolveContext(ctx context.Context, serviceType interface{}) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isDisposed {
		return nil, fmt.Errorf("container is disposed")
	}

	typ := reflect.TypeOf(serviceType)
	if typ == nil {
		return nil, fmt.Errorf("serviceType must be a non-nil pointer")
	}

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	return c.resolveType(ctx, typ)
}

// MustResolve resolves a service and panics if not found.
// Useful for initialization code where missing dependencies are fatal.
func (c *Container) MustResolve(serviceType interface{}) interface{} {
	instance, err := c.Resolve(serviceType)
	if err != nil {
		panic(fmt.Sprintf("DI resolve error: %v", err))
	}
	return instance
}

// ResolveTyped is a convenience method that returns a typed result.
// Usage: router, err := container.ResolveTyped((*Router)(nil))
func (c *Container) ResolveTyped(serviceType interface{}) (interface{}, error) {
	return c.Resolve(serviceType)
}

func (c *Container) resolveType(ctx context.Context, typ reflect.Type) (interface{}, error) {
	if c.resolving[typ] {
		return nil, fmt.Errorf("circular dependency detected for type %v", typ)
	}

	desc, exists := c.services[typ]
	if !exists {
		return nil, fmt.Errorf("service not registered: %v", typ)
	}

	if desc.lifecycle == Singleton && desc.initialized {
		return desc.instance, nil
	}

	c.resolving[typ] = true
	defer func() { delete(c.resolving, typ) }()

	instance, err := c.createInstance(ctx, desc)
	if err != nil {
		return nil, err
	}

	if desc.lifecycle == Singleton {
		desc.instance = instance
		desc.initialized = true
		c.instances[typ] = instance
	}

	return instance, nil
}

func (c *Container) createInstance(ctx context.Context, desc *serviceDescriptor) (interface{}, error) {
	ctorValue := reflect.ValueOf(desc.constructor)
	ctorType := ctorValue.Type()

	// Check if constructor requires context
	args := make([]reflect.Value, 0, ctorType.NumIn())

	for i := 0; i < ctorType.NumIn(); i++ {
		paramType := ctorType.In(i)

		// Handle context.Context parameter
		if paramType == reflect.TypeOf((*context.Context)(nil)).Elem() {
			args = append(args, reflect.ValueOf(ctx))
			continue
		}

		// Resolve dependency
		dep, err := c.resolveType(ctx, paramType)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve dependency %v: %w", paramType, err)
		}
		args = append(args, reflect.ValueOf(dep))
	}

	results := ctorValue.Call(args)

	if len(results) == 0 {
		return nil, fmt.Errorf("constructor must return at least one value")
	}

	instance := results[0].Interface()

	// Handle error return value
	if len(results) > 1 {
		if err, ok := results[1].Interface().(error); ok && err != nil {
			return nil, err
		}
	}

	return instance, nil
}

// Dispose disposes the container and all disposable services.
// Services implementing Disposable interface will have their Dispose method called.
func (c *Container) Dispose() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isDisposed {
		return nil
	}

	c.isDisposed = true

	// Dispose all singleton instances that implement Disposable
	for typ, instance := range c.instances {
		if disposable, ok := instance.(Disposable); ok {
			if err := disposable.Dispose(); err != nil {
				return fmt.Errorf("failed to dispose service %v: %w", typ, err)
			}
		}
	}

	c.services = nil
	c.instances = nil
	c.resolving = nil

	return nil
}

// Disposable is implemented by services that need cleanup.
type Disposable interface {
	Dispose() error
}

// ContainerBuilder provides a fluent interface for building containers.
type ContainerBuilder struct {
	container *Container
	errors    []error
}

// NewContainerBuilder creates a new container builder.
func NewContainerBuilder() *ContainerBuilder {
	return &ContainerBuilder{
		container: NewContainer(),
	}
}

// AddSingleton adds a singleton service.
func (b *ContainerBuilder) AddSingleton(serviceType interface{}, constructor interface{}) *ContainerBuilder {
	if err := b.container.RegisterSingleton(serviceType, constructor); err != nil {
		b.errors = append(b.errors, err)
	}
	return b
}

// AddTransient adds a transient service.
func (b *ContainerBuilder) AddTransient(serviceType interface{}, constructor interface{}) *ContainerBuilder {
	if err := b.container.RegisterTransient(serviceType, constructor); err != nil {
		b.errors = append(b.errors, err)
	}
	return b
}

// AddInstance adds an existing instance.
func (b *ContainerBuilder) AddInstance(serviceType interface{}, instance interface{}) *ContainerBuilder {
	if err := b.container.RegisterInstance(serviceType, instance); err != nil {
		b.errors = append(b.errors, err)
	}
	return b
}

// Build builds and returns the container.
func (b *ContainerBuilder) Build() (*Container, error) {
	if len(b.errors) > 0 {
		return nil, fmt.Errorf("errors during container build: %v", b.errors)
	}
	return b.container, nil
}

// MustBuild builds the container and panics on error.
func (b *ContainerBuilder) MustBuild() *Container {
	c, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("DI container build error: %v", err))
	}
	return c
}

// GetServiceCount returns the number of registered services.
func (c *Container) GetServiceCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.services)
}

// IsRegistered checks if a service is registered.
func (c *Container) IsRegistered(serviceType interface{}) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	typ := reflect.TypeOf(serviceType)
	if typ == nil {
		return false
	}

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	_, exists := c.services[typ]
	return exists
}
