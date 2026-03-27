// Package di provides dependency injection container for go-pcap2socks.
package di

// ServiceCollection defines the core services for go-pcap2socks.
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
func (sc *ServiceCollection) ConfigureServices(config interface{}) error {
	builder := NewContainerBuilder()

	// Example configuration - adapt to actual config structure
	// DNS Resolver
	builder.AddSingleton((*DNSResolver)(nil), func() (DNSResolver, error) {
		return NewCachedDNSResolver(), nil
	})

	// DHCP Server
	builder.AddSingleton((*DHCPServer)(nil), func(resolver DNSResolver) (DHCPServer, error) {
		return NewDHCPServerWithResolver(resolver), nil
	})

	// Router
	builder.AddSingleton((*Router)(nil), func(dialer Dialer) (Router, error) {
		return NewRuleRouter(), nil
	})

	container, err := builder.Build()
	if err != nil {
		return err
	}

	sc.Container = container
	return nil
}

// Interface stubs for compilation
type DNSResolver interface{}
type DHCPServer interface{}
type Router interface{}
type Dialer interface{}

// Constructor stubs
func NewCachedDNSResolver() DNSResolver     { return nil }
func NewDHCPServerWithResolver(DNSResolver) DHCPServer { return nil }
func NewRuleRouter() Router                 { return nil }

// GetResolver returns the DNS resolver service.
func (sc *ServiceCollection) GetResolver() (DNSResolver, error) {
	instance, err := sc.Container.Resolve((*DNSResolver)(nil))
	if err != nil {
		return nil, err
	}
	return instance.(DNSResolver), nil
}

// GetDHCPServer returns the DHCP server service.
func (sc *ServiceCollection) GetDHCPServer() (DHCPServer, error) {
	instance, err := sc.Container.Resolve((*DHCPServer)(nil))
	if err != nil {
		return nil, err
	}
	return instance.(DHCPServer), nil
}

// GetRouter returns the router service.
func (sc *ServiceCollection) GetRouter() (Router, error) {
	instance, err := sc.Container.Resolve((*Router)(nil))
	if err != nil {
		return nil, err
	}
	return instance.(Router), nil
}
