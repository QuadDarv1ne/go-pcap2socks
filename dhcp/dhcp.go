// Package dhcp provides DHCP server functionality for go-pcap2socks
package dhcp

import (
	"encoding/binary"
	"net"
	"time"
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

// Marshal serializes the DHCP message to bytes
func (m *DHCPMessage) Marshal() []byte {
	packet := make([]byte, 240) // Minimum DHCP packet size

	// Fixed header
	packet[0] = m.OpCode
	packet[1] = m.HardwareType
	packet[2] = m.HardwareLength
	packet[3] = m.Hops
	binary.BigEndian.PutUint32(packet[4:8], m.TransactionID)
	binary.BigEndian.PutUint16(packet[8:10], m.Seconds)
	binary.BigEndian.PutUint16(packet[10:12], m.Flags)

	// IP addresses
	copy(packet[12:16], m.ClientIP.To4())
	copy(packet[16:20], m.YourIP.To4())
	copy(packet[20:24], m.ServerIP.To4())
	copy(packet[24:28], m.GatewayIP.To4())

	// Client hardware address
	copy(packet[28:34], m.ClientHardware)

	// Options
	optionPos := 240
	packet[optionPos] = OptionDHCPMessageType
	optionPos++
	packet[optionPos] = 1
	optionPos++
	packet[optionPos] = m.Options[OptionDHCPMessageType][0]
	optionPos++

	for code, value := range m.Options {
		if code == OptionDHCPMessageType {
			continue // Already added
		}
		if code == OptionEnd {
			continue // Add at the end
		}
		packet[optionPos] = code
		optionPos++
		packet[optionPos] = byte(len(value))
		optionPos++
		copy(packet[optionPos:optionPos+len(value)], value)
		optionPos += len(value)
	}

	// End option
	packet[optionPos] = OptionEnd

	return packet
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
