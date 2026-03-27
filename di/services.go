// Package di provides dependency injection container for go-pcap2socks.
package di

import (
	"github.com/QuadDarv1ne/go-pcap2socks/interfaces"
)

// ServiceCollection defines the core services for go-pcap2socks.
// This type is deprecated - use Container directly with Register methods.
type ServiceCollection struct {
	Container *Container
}

// NewServiceCollection creates a new service collection.
func NewServiceCollection() *ServiceCollection {
	return &ServiceCollection{
		Container: NewContainer(),
	}
}

// ConfigureServices configures all services for the application.
// This is a template showing how to wire up services using DI.
// Adapt this function to your actual configuration structure.
func (sc *ServiceCollection) ConfigureServices(config interface{}) error {
	builder := NewContainerBuilder()

	// Example configuration - adapt to actual config structure
	// DNS Resolver
	builder.AddSingleton((*interfaces.DNSResolver)(nil), func() (interfaces.DNSResolver, error) {
		return nil, nil // Replace with actual DNS resolver implementation
	})

	// DHCP Server
	builder.AddSingleton((*interfaces.DHCPServer)(nil), func(resolver interfaces.DNSResolver) (interfaces.DHCPServer, error) {
		return nil, nil // Replace with actual DHCP server implementation
	})

	// Router
	builder.AddSingleton((*interfaces.Router)(nil), func(dialer interfaces.Dialer) (interfaces.Router, error) {
		return nil, nil // Replace with actual router implementation
	})

	container, err := builder.Build()
	if err != nil {
		return err
	}

	sc.Container = container
	return nil
}

// GetResolver returns the DNS resolver service.
func (sc *ServiceCollection) GetResolver() (interfaces.DNSResolver, error) {
	instance, err := sc.Container.Resolve((*interfaces.DNSResolver)(nil))
	if err != nil {
		return nil, err
	}
	return instance.(interfaces.DNSResolver), nil
}

// GetDHCPServer returns the DHCP server service.
func (sc *ServiceCollection) GetDHCPServer() (interfaces.DHCPServer, error) {
	instance, err := sc.Container.Resolve((*interfaces.DHCPServer)(nil))
	if err != nil {
		return nil, err
	}
	return instance.(interfaces.DHCPServer), nil
}

// GetRouter returns the router service.
func (sc *ServiceCollection) GetRouter() (interfaces.Router, error) {
	instance, err := sc.Container.Resolve((*interfaces.Router)(nil))
	if err != nil {
		return nil, err
	}
	return instance.(interfaces.Router), nil
}
