package core

import (
	"github.com/QuadDarv1ne/go-pcap2socks/core/option"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

func withRouteTable(nicID tcpip.NICID) option.Option {
	return func(s *stack.Stack) error {
		// Enable IP forwarding for IPv4 and IPv6
		// This allows gvisor to forward packets between endpoints
		s.SetForwardingDefaultAndAllNICs(header.IPv4ProtocolNumber, true)
		s.SetForwardingDefaultAndAllNICs(header.IPv6ProtocolNumber, true)
		
		s.SetRouteTable([]tcpip.Route{
			{
				Destination: header.IPv4EmptySubnet,
				NIC:         nicID,
			},
			{
				Destination: header.IPv6EmptySubnet,
				NIC:         nicID,
			},
		})
		return nil
	}
}
