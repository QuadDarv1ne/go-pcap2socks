package device

import (
	"context"
	"net"

	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// Device is the interface that implemented by network layer devices (e.g. tun),
// and easy to use as stack.LinkEndpoint.
type Device interface {
	stack.LinkEndpoint

	// Name returns the current name of the device.
	Name() string

	// Type returns the driver type of the device.
	Type() string

	// Stop gracefully stops the device with context-based timeout
	Stop(ctx context.Context) error
}

// NetworkConfig holds the parsed network configuration
type NetworkConfig struct {
	Network  *net.IPNet
	LocalIP  net.IP
	LocalMAC net.HardwareAddr
	MTU      uint32
}

// DHCPServer defines the interface for DHCP server implementations
type DHCPServer interface {
	HandleRequest(data []byte) ([]byte, error)
	Start() error
	Stop()
}
