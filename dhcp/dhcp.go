// Package dhcp provides DHCP server functionality for go-pcap2socks
package dhcp

import (
	"encoding/binary"
	"net"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
)

// DHCP message types
const (
	DHCPDiscover = 1
	DHCPOffer    = 2
	DHCPRequest  = 3
	DHCPDecline  = 4
	DHCPAck      = 5
	DHCPNak      = 6
	DHCPRelease  = 7
	DHCPInform   = 8
)

// DHCP options
const (
	OptionSubnetMask       = 1
	OptionRouter           = 3
	OptionDNSServer        = 6
	OptionHostName         = 12
	OptionRequestedIP      = 50
	OptionLeaseTime        = 51
	OptionDHCPMessageType  = 53
	OptionServerIdentifier = 54
	OptionMessage          = 56
	OptionMaxMessageSize   = 57
	OptionVendorClassID    = 60
	OptionClientID         = 61
	OptionEnd              = 255
)

// DHCPMessage represents a DHCP packet
type DHCPMessage struct {
	OpCode         uint8
	HardwareType   uint8
	HardwareLength uint8
	Hops           uint8
	TransactionID  uint32
	Seconds        uint16
	Flags          uint16
	ClientIP       net.IP
	YourIP         net.IP
	ServerIP       net.IP
	GatewayIP      net.IP
	ClientHardware net.HardwareAddr
	ServerHostname string
	BootFileName   string
	Options        map[uint8][]byte
}

// NewDHCPMessage creates a new DHCP message
func NewDHCPMessage() *DHCPMessage {
	return &DHCPMessage{
		OpCode:         2, // BOOTREPLY
		HardwareType:   1, // Ethernet
		HardwareLength: 6,
		Hops:           0,
		TransactionID:  0,
		Seconds:        0,
		Flags:          0,
		ClientIP:       net.IPv4zero,
		YourIP:         net.IPv4zero,
		ServerIP:       net.IPv4zero,
		GatewayIP:      net.IPv4zero,
		ClientHardware: make(net.HardwareAddr, 6),
		ServerHostname: "",
		BootFileName:   "",
		Options:        make(map[uint8][]byte),
	}
}

// dhcpBufPool is a pool for DHCP message buffers to reduce allocations
var dhcpBufPool = pool.NewAllocator()

// Marshal serializes the DHCP message to bytes
// Returns a newly allocated slice that caller owns
func (m *DHCPMessage) Marshal() []byte {
	// Estimate total size: 240 (header) + options
	estimatedSize := 240
	for code, value := range m.Options {
		if code == OptionEnd {
			continue
		}
		estimatedSize += 2 + len(value) // code + length + value
	}
	estimatedSize += 1 // End option

	// Get buffer from pool
	buf := pool.Get(estimatedSize)
	defer pool.Put(buf)

	// Zero out the buffer
	for i := range buf[:estimatedSize] {
		buf[i] = 0
	}

	// Fixed header (240 bytes)
	// Bytes 0-3: OpCode, HardwareType, HardwareLength, Hops
	buf[0] = m.OpCode
	buf[1] = m.HardwareType
	buf[2] = m.HardwareLength
	buf[3] = m.Hops
	// Bytes 4-7: TransactionID
	binary.BigEndian.PutUint32(buf[4:8], m.TransactionID)
	// Bytes 8-9: Seconds
	binary.BigEndian.PutUint16(buf[8:10], m.Seconds)
	// Bytes 10-11: Flags
	binary.BigEndian.PutUint16(buf[10:12], m.Flags)

	// IP addresses (12-27)
	copy(buf[12:16], m.ClientIP.To4())
	copy(buf[16:20], m.YourIP.To4())
	copy(buf[20:24], m.ServerIP.To4())
	copy(buf[24:28], m.GatewayIP.To4())

	// Client hardware address (28-33)
	copy(buf[28:34], m.ClientHardware)

	// Server hostname (64 bytes starting at 44)
	if m.ServerHostname != "" {
		copy(buf[44:], []byte(m.ServerHostname))
	}

	// Boot file name (128 bytes starting at 108)
	if m.BootFileName != "" {
		copy(buf[108:], []byte(m.BootFileName))
	}

	// Magic cookie (mandatory for DHCP, bytes 236-239)
	buf[236] = 99
	buf[237] = 130
	buf[238] = 83
	buf[239] = 99

	// Options starting at 240
	optionPos := 240

	// Add options in deterministic order for reliability
	// Order: Message Type → Server ID → Subnet Mask → Router → DNS → Lease Time → Requested IP → Others
	
	// Helper function to add an option
	addOption := func(code uint8, value []byte) {
		buf[optionPos] = code
		optionPos++
		buf[optionPos] = byte(len(value))
		optionPos++
		copy(buf[optionPos:optionPos+len(value)], value)
		optionPos += len(value)
	}
	
	// Add mandatory and common options in fixed order
	if msgType, ok := m.Options[OptionDHCPMessageType]; ok {
		addOption(OptionDHCPMessageType, msgType)
	}
	if serverID, ok := m.Options[OptionServerIdentifier]; ok {
		addOption(OptionServerIdentifier, serverID)
	}
	if subnetMask, ok := m.Options[OptionSubnetMask]; ok {
		addOption(OptionSubnetMask, subnetMask)
	}
	if router, ok := m.Options[OptionRouter]; ok {
		addOption(OptionRouter, router)
	}
	if dnsServers, ok := m.Options[OptionDNSServer]; ok {
		addOption(OptionDNSServer, dnsServers)
	}
	if leaseTime, ok := m.Options[OptionLeaseTime]; ok {
		addOption(OptionLeaseTime, leaseTime)
	}
	if requestedIP, ok := m.Options[OptionRequestedIP]; ok {
		addOption(OptionRequestedIP, requestedIP)
	}

	// Track handled options to avoid duplicates
	handledOptions := map[uint8]bool{
		OptionDHCPMessageType:  true,
		OptionServerIdentifier: true,
		OptionSubnetMask:       true,
		OptionRouter:           true,
		OptionDNSServer:        true,
		OptionLeaseTime:        true,
		OptionRequestedIP:      true,
	}

	// Add any remaining options not handled above
	for code, value := range m.Options {
		if handledOptions[code] || code == OptionEnd {
			continue
		}
		addOption(code, value)
	}

	// End option
	buf[optionPos] = OptionEnd

	// Return actual size slice - caller owns this memory
	result := make([]byte, optionPos+1)
	copy(result, buf[:optionPos+1])

	return result
}

// ParseDHCPMessage parses a DHCP message from bytes
func ParseDHCPMessage(data []byte) (*DHCPMessage, error) {
	if len(data) < 240 {
		return nil, ErrInvalidDHCPMessage
	}

	msg := &DHCPMessage{
		OpCode:         data[0],
		HardwareType:   data[1],
		HardwareLength: data[2],
		Hops:           data[3],
		TransactionID:  binary.BigEndian.Uint32(data[4:8]),
		Seconds:        binary.BigEndian.Uint16(data[8:10]),
		Flags:          binary.BigEndian.Uint16(data[10:12]),
		ClientIP:       net.IP(data[12:16]).To4(),
		YourIP:         net.IP(data[16:20]).To4(),
		ServerIP:       net.IP(data[20:24]).To4(),
		GatewayIP:      net.IP(data[24:28]).To4(),
		ClientHardware: net.HardwareAddr(data[28:34]),
		Options:        make(map[uint8][]byte),
	}

	// Parse options (starting at byte 240)
	pos := 240
	for pos < len(data) {
		if data[pos] == OptionEnd {
			break
		}
		if data[pos] == 0 { // Padding
			pos++
			continue
		}
		code := data[pos]
		length := int(data[pos+1])
		if pos+2+length > len(data) {
			break
		}
		value := make([]byte, length)
		copy(value, data[pos+2:pos+2+length])
		msg.Options[code] = value
		pos += 2 + length
	}

	return msg, nil
}

// DHCP transaction state
type DHCPLease struct {
	IP          net.IP
	MAC         net.HardwareAddr
	Hostname    string
	ExpiresAt   time.Time
	Transaction uint32
}

// ServerConfig holds DHCP server configuration
type ServerConfig struct {
	ServerIP      net.IP
	ServerMAC     net.HardwareAddr
	Network       *net.IPNet
	LeaseDuration time.Duration
	FirstIP       net.IP // First IP in pool
	LastIP        net.IP // Last IP in pool
	DNSServers    []net.IP
}

// Error types
type DHCPError string

func (e DHCPError) Error() string { return string(e) }

const (
	ErrInvalidDHCPMessage = DHCPError("invalid DHCP message")
	ErrNoAvailableIPs     = DHCPError("no available IPs in pool")
	ErrInvalidMessageType = DHCPError("invalid DHCP message type")
)
