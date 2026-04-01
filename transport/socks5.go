// Package socks5 provides SOCKS5 client functionalities.
// Implements RFC 1928 (SOCKS Protocol Version 5) and RFC 1929 (Username/Password Authentication).
package socks5

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
)

// Pre-defined errors for SOCKS5 operations
var (
	ErrVersionMismatch    = errors.New("socks version mismatched")
	ErrAuthRequired       = errors.New("auth required")
	ErrAuthTooLong        = errors.New("auth username/password too long")
	ErrAuthRejected       = errors.New("rejected username/password")
	ErrUnsupportedMethod  = errors.New("unsupported method")
	ErrInvalidAddressType = errors.New("invalid address type")
	ErrInsufficientBuffer = errors.New("insufficient buffer")
	ErrFragmentedPayload  = errors.New("discarding fragmented payload")
	ErrAddressNil         = errors.New("socks5 addr is nil")
)

// AuthMethod is the authentication method as defined in RFC 1928 section 3.
type AuthMethod = uint8

// SOCKS authentication methods as defined in RFC 1928 section 3.
const (
	MethodNoAuth   AuthMethod = 0x00
	MethodUserPass AuthMethod = 0x02
)

// Version is the protocol version as defined in RFC 1928 section 4.
const Version = 0x05

// Command is request commands as defined in RFC 1928 section 4.
type Command uint8

// SOCKS request commands as defined in RFC 1928 section 4.
const (
	CmdConnect      Command = 0x01
	CmdBind         Command = 0x02
	CmdUDPAssociate Command = 0x03
)

func (c Command) String() string {
	switch c {
	case CmdConnect:
		return "CONNECT"
	case CmdBind:
		return "BIND"
	case CmdUDPAssociate:
		return "UDP ASSOCIATE"
	default:
		return "UNDEFINED"
	}
}

type Atyp = uint8

// SOCKS address types as defined in RFC 1928 section 5.
const (
	AtypIPv4       Atyp = 0x01
	AtypDomainName Atyp = 0x03
	AtypIPv6       Atyp = 0x04
)

// Reply field as defined in RFC 1928 section 6.
type Reply uint8

func (r Reply) String() string {
	switch r {
	case 0x00:
		return "succeeded"
	case 0x01:
		return "general SOCKS server failure"
	case 0x02:
		return "connection not allowed by ruleset"
	case 0x03:
		return "network unreachable"
	case 0x04:
		return "host unreachable"
	case 0x05:
		return "connection refused"
	case 0x06:
		return "TTL expired"
	case 0x07:
		return "command not supported"
	case 0x08:
		return "address type not supported"
	default:
		return fmt.Sprintf("unassigned <%#02x>", uint8(r))
	}
}

// MaxAddrLen is the maximum size of SOCKS address in bytes.
const MaxAddrLen = 1 + 1 + 255 + 2

// MaxAuthLen is the maximum size of user/password field in SOCKS auth.
const MaxAuthLen = 255

// Addr represents a SOCKS address as defined in RFC 1928 section 5.
type Addr []byte

func (a Addr) Valid() bool {
	if len(a) < 1+1+2 /* minimum length */ {
		return false
	}

	switch a[0] {
	case AtypDomainName:
		if len(a) < 1+1+int(a[1])+2 {
			return false
		}
	case AtypIPv4:
		if len(a) < 1+net.IPv4len+2 {
			return false
		}
	case AtypIPv6:
		if len(a) < 1+net.IPv6len+2 {
			return false
		}
	}
	return true
}

// String returns string of socks5.Addr.
func (a Addr) String() string {
	if !a.Valid() {
		return ""
	}

	var host, port string
	switch a[0] {
	case AtypDomainName:
		hostLen := int(a[1])
		host = string(a[2 : 2+hostLen])
		port = strconv.Itoa(int(binary.BigEndian.Uint16(a[2+hostLen:])))
	case AtypIPv4:
		host = net.IP(a[1 : 1+net.IPv4len]).String()
		port = strconv.Itoa(int(binary.BigEndian.Uint16(a[1+net.IPv4len:])))
	case AtypIPv6:
		host = net.IP(a[1 : 1+net.IPv6len]).String()
		port = strconv.Itoa(int(binary.BigEndian.Uint16(a[1+net.IPv6len:])))
	}
	return net.JoinHostPort(host, port)
}

// UDPAddr converts a socks5.Addr to *net.UDPAddr.
func (a Addr) UDPAddr() *net.UDPAddr {
	if !a.Valid() {
		return nil
	}

	switch a[0] {
	case AtypDomainName /* unsupported */ :
		return nil
	case AtypIPv4:
		var ip [net.IPv4len]byte
		copy(ip[:], a[1:1+net.IPv4len])
		port := int(binary.BigEndian.Uint16(a[1+net.IPv4len:]))
		return &net.UDPAddr{IP: ip[:], Port: port}
	case AtypIPv6:
		var ip [net.IPv6len]byte
		copy(ip[:], a[1:1+net.IPv6len])
		port := int(binary.BigEndian.Uint16(a[1+net.IPv6len:]))
		return &net.UDPAddr{IP: ip[:], Port: port}
	default:
		return nil
	}
}

// User provides basic socks5 auth functionality.
type User struct {
	Username string
	Password string
}

// ClientHandshake fast-tracks SOCKS initialization to get target address to connect on client side.
func ClientHandshake(rw io.ReadWriter, addr Addr, command Command, user *User) (Addr, error) {
	buf := pool.GetAddr()
	defer pool.PutAddr(buf)

	var method uint8
	if user != nil {
		method = MethodUserPass /* USERNAME/PASSWORD */
	} else {
		method = MethodNoAuth /* NO AUTHENTICATION REQUIRED */
	}

	// VER, NMETHODS, METHODS
	if _, err := rw.Write([]byte{Version, 0x01 /* NMETHODS */, method}); err != nil {
		return nil, err
	}

	// VER, METHOD
	if _, err := io.ReadFull(rw, buf[:2]); err != nil {
		return nil, err
	}

	if buf[0] != Version {
		return nil, ErrVersionMismatch
	}

	if buf[1] == MethodUserPass /* USERNAME/PASSWORD */ {
		if user == nil {
			return nil, ErrAuthRequired
		}

		uLen := len(user.Username)
		pLen := len(user.Password)

		// Both ULEN and PLEN are limited to the range [1, 255].
		if uLen == 0 || pLen == 0 {
			return nil, ErrAuthTooLong
		} else if uLen > MaxAuthLen || pLen > MaxAuthLen {
			return nil, ErrAuthTooLong
		}

		authMsgLen := 1 + 1 + uLen + 1 + pLen

		// password protocol version
		authMsg := bytes.NewBuffer(make([]byte, 0, authMsgLen))
		authMsg.WriteByte(0x01 /* VER */)
		authMsg.WriteByte(byte(uLen) /* ULEN */)
		authMsg.WriteString(user.Username /* UNAME */)
		authMsg.WriteByte(byte(pLen) /* PLEN */)
		authMsg.WriteString(user.Password /* PASSWD */)

		if _, err := rw.Write(authMsg.Bytes()); err != nil {
			return nil, err
		}

		if _, err := io.ReadFull(rw, buf[:2]); err != nil {
			return nil, err
		}

		if buf[1] != 0x00 /* STATUS of SUCCESS */ {
			return nil, ErrAuthRejected
		}

	} else if buf[1] != MethodNoAuth /* NO AUTHENTICATION REQUIRED */ {
		return nil, ErrUnsupportedMethod
	}

	// VER, CMD, RSV, ADDR
	if _, err := rw.Write(bytes.Join([][]byte{{Version, byte(command), 0x00 /* RSV */}, addr}, nil)); err != nil {
		return nil, err
	}

	// VER, REP, RSV
	if _, err := io.ReadFull(rw, buf[:3]); err != nil {
		return nil, err
	}

	if rep := Reply(buf[1]); rep != 0x00 /* SUCCEEDED */ {
		return nil, fmt.Errorf("%s: %s", command, rep)
	}

	return ReadAddr(rw, buf)
}

func ReadAddr(r io.Reader, b []byte) (Addr, error) {
	if len(b) < MaxAddrLen {
		return nil, io.ErrShortBuffer
	}

	// read 1st byte for address type
	if _, err := io.ReadFull(r, b[:1]); err != nil {
		return nil, err
	}

	switch b[0] /* ATYP */ {
	case AtypDomainName:
		// read 2nd byte for domain length
		if _, err := io.ReadFull(r, b[1:2]); err != nil {
			return nil, err
		}
		domainLength := uint16(b[1])
		_, err := io.ReadFull(r, b[2:2+domainLength+2])
		return b[:1+1+domainLength+2], err
	case AtypIPv4:
		_, err := io.ReadFull(r, b[1:1+net.IPv4len+2])
		return b[:1+net.IPv4len+2], err
	case AtypIPv6:
		_, err := io.ReadFull(r, b[1:1+net.IPv6len+2])
		return b[:1+net.IPv6len+2], err
	default:
		return nil, ErrInvalidAddressType
	}
}

// SplitAddr slices a SOCKS address from beginning of b. Returns nil if failed.
func SplitAddr(b []byte) Addr {
	addrLen := 1
	if len(b) < addrLen {
		return nil
	}

	switch b[0] {
	case AtypDomainName:
		if len(b) < 2 {
			return nil
		}
		addrLen = 1 + 1 + int(b[1]) + 2
	case AtypIPv4:
		addrLen = 1 + net.IPv4len + 2
	case AtypIPv6:
		addrLen = 1 + net.IPv6len + 2
	default:
		return nil
	}

	if len(b) < addrLen {
		return nil
	}

	return b[:addrLen]
}

// SerializeAddr serializes destination address and port to Addr.
// If a domain name is provided, AtypDomainName would be used first.
func SerializeAddr(domainName string, dstIP net.IP, dstPort uint16) Addr {
	var addr Addr
	// Use stack-allocated array for port to avoid allocation
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], dstPort)

	if domainName != "" /* Domain Name */ {
		length := len(domainName)
		// ATYP(1) + LEN(1) + domain + port(2)
		addr = make(Addr, 1+1+length+2)
		addr[0] = AtypDomainName
		addr[1] = uint8(length)
		copy(addr[2:], domainName)
		copy(addr[2+length:], portBuf[:])
	} else if dstIP.To4() != nil /* IPv4 */ {
		// ATYP(1) + IPv4(4) + port(2)
		addr = make(Addr, 1+net.IPv4len+2)
		addr[0] = AtypIPv4
		copy(addr[1:], dstIP.To4())
		copy(addr[1+net.IPv4len:], portBuf[:])
	} else /* IPv6 */ {
		// ATYP(1) + IPv6(16) + port(2)
		addr = make(Addr, 1+net.IPv6len+2)
		addr[0] = AtypIPv6
		copy(addr[1:], dstIP.To16())
		copy(addr[1+net.IPv6len:], portBuf[:])
	}
	return addr
}

// ParseAddr parses a socks addr from net.Addr.
// This is a fast path of ParseAddrString(addr.String())
func ParseAddr(addr net.Addr) Addr {
	switch v := addr.(type) {
	case *net.TCPAddr:
		return SerializeAddr("", v.IP, uint16(v.Port))
	case *net.UDPAddr:
		return SerializeAddr("", v.IP, uint16(v.Port))
	default:
		return ParseAddrString(addr.String())
	}
}

// ParseAddrString parses the address in string s to Addr. Returns nil if failed.
func ParseAddrString(s string) Addr {
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return nil
	}

	dstPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil
	}

	if ip := net.ParseIP(host); ip != nil {
		return SerializeAddr("", ip, uint16(dstPort))
	}
	return SerializeAddr(host, nil, uint16(dstPort))
}

// DecodeUDPPacket split `packet` to addr payload, and this function is mutable with `packet`
func DecodeUDPPacket(packet []byte) (addr Addr, payload []byte, err error) {
	if len(packet) < 5 {
		err = ErrInsufficientBuffer
		return
	}

	// packet[0] and packet[1] are reserved
	if !bytes.Equal(packet[:2], []byte{0x00, 0x00}) {
		err = ErrVersionMismatch
		return
	}

	// The FRAG field indicates whether or not this datagram is one of a
	// number of fragments.  If implemented, the high-order bit indicates
	// end-of-fragment sequence, while a value of X'00' indicates that this
	// datagram is standalone.  Values between 1 and 127 indicate the
	// fragment position within a fragment sequence.  Each receiver will
	// have a REASSEMBLY QUEUE and a REASSEMBLY TIMER associated with these
	// fragments.  The reassembly queue must be reinitialized and the
	// associated fragments abandoned whenever the REASSEMBLY TIMER expires,
	// or a new datagram arrives carrying a FRAG field whose value is less
	// than the highest FRAG value processed for this fragment sequence.
	// The reassembly timer MUST be no less than 5 seconds.  It is
	// recommended that fragmentation be avoided by applications wherever
	// possible.
	//
	// Ref: https://datatracker.ietf.org/doc/html/rfc1928#section-7
	if packet[2] != 0x00 /* fragments */ {
		err = ErrFragmentedPayload
		return
	}

	addr = SplitAddr(packet[3:])
	if addr == nil {
		err = ErrAddressNil
	}

	payload = packet[3+len(addr):]
	return
}

// DecodeUDPPacketInPlace parses SOCKS5 UDP header and returns address and payload length
// This is a zero-copy version that doesn't allocate separate payload slice
func DecodeUDPPacketInPlace(packet []byte) (addr Addr, payloadLen int, err error) {
	if len(packet) < 5 {
		err = ErrInsufficientBuffer
		return
	}

	// packet[0] and packet[1] are reserved
	if !bytes.Equal(packet[:2], []byte{0x00, 0x00}) {
		err = ErrVersionMismatch
		return
	}

	// Check fragment field
	if packet[2] != 0x00 {
		err = ErrFragmentedPayload
		return
	}

	addr = SplitAddr(packet[3:])
	if addr == nil {
		err = ErrAddressNil
		return
	}

	// Calculate payload length
	headerLen := 3 + len(addr)
	payloadLen = len(packet) - headerLen

	return
}

func EncodeUDPPacket(addr Addr, payload []byte) (packet []byte, err error) {
	if addr == nil {
		return nil, ErrAddressNil
	}
	// RSV(2) + FRSAG(1) + addr + payload
	headerLen := 3 + len(addr)
	// Use pool for large packets, direct allocation for small ones
	if headerLen+len(payload) > 1024 {
		buf := pool.GetUDP()
		buf = buf[:headerLen+len(payload)]
		copy(buf[3:], addr)
		copy(buf[headerLen:], payload)
		return buf, nil
	}
	packet = make([]byte, headerLen+len(payload))
	copy(packet[3:], addr)
	copy(packet[headerLen:], payload)
	return
}
