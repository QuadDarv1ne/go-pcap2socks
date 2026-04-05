//go:build ignore

package socks5

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

func TestCommand_String(t *testing.T) {
	tests := []struct {
		cmd      Command
		expected string
	}{
		{CmdConnect, "CONNECT"},
		{CmdBind, "BIND"},
		{CmdUDPAssociate, "UDP ASSOCIATE"},
		{0xFF, "UNDEFINED"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.cmd.String(); got != tt.expected {
				t.Errorf("Command.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReply_String(t *testing.T) {
	tests := []struct {
		reply    Reply
		expected string
	}{
		{0x00, "succeeded"},
		{0x01, "general SOCKS server failure"},
		{0x02, "connection not allowed by ruleset"},
		{0x03, "network unreachable"},
		{0x04, "host unreachable"},
		{0x05, "connection refused"},
		{0x06, "TTL expired"},
		{0x07, "command not supported"},
		{0x08, "address type not supported"},
		{0xFF, "unassigned <0xff>"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.reply.String(); got != tt.expected {
				t.Errorf("Reply.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAddr_Valid(t *testing.T) {
	tests := []struct {
		name     string
		addr     Addr
		expected bool
	}{
		{
			name:     "valid IPv4",
			addr:     Addr{AtypIPv4, 192, 168, 1, 1, 0, 80},
			expected: true,
		},
		{
			name:     "valid IPv6",
			addr:     append(Addr{AtypIPv6}, make([]byte, net.IPv6len+2)...),
			expected: true,
		},
		{
			name:     "valid domain",
			addr:     Addr{AtypDomainName, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0, 80},
			expected: true,
		},
		{
			name:     "too short",
			addr:     Addr{AtypIPv4, 192},
			expected: false,
		},
		{
			name:     "invalid IPv4 length",
			addr:     Addr{AtypIPv4, 192, 168},
			expected: false,
		},
		{
			name:     "invalid IPv6 length",
			addr:     Addr{AtypIPv6, 0, 0},
			expected: false,
		},
		{
			name:     "invalid domain length",
			addr:     Addr{AtypDomainName, 10, 't', 'e', 's', 't'},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.Valid(); got != tt.expected {
				t.Errorf("Addr.Valid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAddr_String(t *testing.T) {
	tests := []struct {
		name     string
		addr     Addr
		expected string
	}{
		{
			name:     "IPv4",
			addr:     Addr{AtypIPv4, 192, 168, 1, 1, 0, 80},
			expected: "192.168.1.1:80",
		},
		{
			name:     "IPv6",
			addr:     append(Addr{AtypIPv6}, append(net.ParseIP("::1").To16(), []byte{0, 80}...)...),
			expected: "[::1]:80",
		},
		{
			name:     "domain",
			addr:     Addr{AtypDomainName, 9, 'l', 'o', 'c', 'a', 'l', 'h', 'o', 's', 't', 0, 80},
			expected: "localhost:80",
		},
		{
			name:     "invalid",
			addr:     Addr{0xFF},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.String(); got != tt.expected {
				t.Errorf("Addr.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAddr_UDPAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     Addr
		expected *net.UDPAddr
	}{
		{
			name: "IPv4",
			addr: Addr{AtypIPv4, 192, 168, 1, 1, 0, 80},
			expected: &net.UDPAddr{
				IP:   net.ParseIP("192.168.1.1").To4(),
				Port: 80,
			},
		},
		{
			name: "IPv6",
			addr: append(Addr{AtypIPv6}, append(net.ParseIP("::1").To16(), []byte{0, 80}...)...),
			expected: &net.UDPAddr{
				IP:   net.ParseIP("::1").To16(),
				Port: 80,
			},
		},
		{
			name:     "domain unsupported",
			addr:     Addr{AtypDomainName, 4, 't', 'e', 's', 't', 0, 80},
			expected: nil,
		},
		{
			name:     "invalid",
			addr:     Addr{0xFF},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addr.UDPAddr()
			if got == nil && tt.expected == nil {
				return
			}
			if got == nil || tt.expected == nil {
				t.Errorf("Addr.UDPAddr() = %v, want %v", got, tt.expected)
				return
			}
			if !got.IP.Equal(tt.expected.IP) || got.Port != tt.expected.Port {
				t.Errorf("Addr.UDPAddr() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReadAddr(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantErr  bool
		expected Addr
	}{
		{
			name:     "IPv4",
			data:     []byte{AtypIPv4, 192, 168, 1, 1, 0, 80},
			wantErr:  false,
			expected: Addr{AtypIPv4, 192, 168, 1, 1, 0, 80},
		},
		{
			name:     "IPv6",
			data:     append([]byte{AtypIPv6}, append(net.ParseIP("::1").To16(), []byte{0, 80}...)...),
			wantErr:  false,
			expected: append([]byte{AtypIPv6}, append(net.ParseIP("::1").To16(), []byte{0, 80}...)...),
		},
		{
			name:     "domain",
			data:     []byte{AtypDomainName, 4, 't', 'e', 's', 't', 0, 80},
			wantErr:  false,
			expected: Addr{AtypDomainName, 4, 't', 'e', 's', 't', 0, 80},
		},
		{
			name:     "invalid type",
			data:     []byte{0xFF},
			wantErr:  true,
			expected: nil,
		},
		{
			name:     "short buffer",
			data:     []byte{AtypIPv4},
			wantErr:  true,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, MaxAddrLen)
			r := bytes.NewReader(tt.data)
			got, err := ReadAddr(r, buf)

			if (err != nil) != tt.wantErr {
				t.Errorf("ReadAddr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !bytes.Equal(got, tt.expected) {
				t.Errorf("ReadAddr() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSplitAddr(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected Addr
	}{
		{
			name:     "IPv4",
			data:     append([]byte{AtypIPv4, 192, 168, 1, 1, 0, 80}, []byte("extra data")...),
			expected: Addr{AtypIPv4, 192, 168, 1, 1, 0, 80},
		},
		{
			name:     "IPv6",
			data:     append(append([]byte{AtypIPv6}, net.ParseIP("::1").To16()...), append([]byte{0, 80}, []byte("extra")...)...),
			expected: append(append([]byte{AtypIPv6}, net.ParseIP("::1").To16()...), []byte{0, 80}...),
		},
		{
			name:     "domain",
			data:     append([]byte{AtypDomainName, 4, 't', 'e', 's', 't', 0, 80}, []byte("extra")...),
			expected: Addr{AtypDomainName, 4, 't', 'e', 's', 't', 0, 80},
		},
		{
			name:     "too short",
			data:     []byte{AtypIPv4},
			expected: nil,
		},
		{
			name:     "invalid type",
			data:     []byte{0xFF, 1, 2, 3},
			expected: nil,
		},
		{
			name:     "empty",
			data:     []byte{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitAddr(tt.data)
			if !bytes.Equal(got, tt.expected) {
				t.Errorf("SplitAddr() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSerializeAddr(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		ip       net.IP
		port     uint16
		wantType Atyp
		wantLen  int
	}{
		{
			name:     "IPv4",
			ip:       net.ParseIP("192.168.1.1"),
			port:     8080,
			wantType: AtypIPv4,
			wantLen:  1 + net.IPv4len + 2,
		},
		{
			name:     "IPv6",
			ip:       net.ParseIP("::1"),
			port:     443,
			wantType: AtypIPv6,
			wantLen:  1 + net.IPv6len + 2,
		},
		{
			name:     "domain",
			domain:   "example.com",
			port:     80,
			wantType: AtypDomainName,
			wantLen:  1 + 1 + 11 + 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := SerializeAddr(tt.domain, tt.ip, tt.port)

			if len(addr) != tt.wantLen {
				t.Errorf("SerializeAddr() len = %d, want %d", len(addr), tt.wantLen)
			}
			if addr[0] != tt.wantType {
				t.Errorf("SerializeAddr() type = %d, want %d", addr[0], tt.wantType)
			}

			// Verify port is encoded correctly
			gotPort := binary.BigEndian.Uint16(addr[len(addr)-2:])
			if gotPort != tt.port {
				t.Errorf("SerializeAddr() port = %d, want %d", gotPort, tt.port)
			}
		})
	}
}

func TestParseAddr(t *testing.T) {
	tests := []struct {
		name    string
		addr    net.Addr
		wantErr bool
	}{
		{
			name: "TCPAddr",
			addr: &net.TCPAddr{
				IP:   net.ParseIP("10.0.0.1"),
				Port: 1234,
			},
			wantErr: false,
		},
		{
			name: "UDPAddr",
			addr: &net.UDPAddr{
				IP:   net.ParseIP("10.0.0.2"),
				Port: 5678,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := ParseAddr(tt.addr)
			if addr == nil {
				t.Fatal("ParseAddr() returned nil")
			}
			if !addr.Valid() {
				t.Error("ParseAddr() returned invalid address")
			}
		})
	}
}

func TestParseAddrString(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantErr bool
	}{
		{
			name:    "IPv4",
			s:       "192.168.1.1:8080",
			wantErr: false,
		},
		{
			name:    "IPv6",
			s:       "[::1]:443",
			wantErr: false,
		},
		{
			name:    "domain",
			s:       "example.com:80",
			wantErr: false,
		},
		{
			name:    "invalid no port",
			s:       "192.168.1.1",
			wantErr: true,
		},
		{
			name:    "invalid port",
			s:       "192.168.1.1:abc",
			wantErr: true,
		},
		{
			name:    "empty",
			s:       "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := ParseAddrString(tt.s)
			if tt.wantErr {
				if addr != nil {
					t.Error("ParseAddrString() should return nil for invalid input")
				}
				return
			}
			if addr == nil {
				t.Fatal("ParseAddrString() returned nil")
			}
			if !addr.Valid() {
				t.Error("ParseAddrString() returned invalid address")
			}
		})
	}
}

func TestDecodeUDPPacket(t *testing.T) {
	tests := []struct {
		name      string
		packet    []byte
		wantErr   bool
		checkAddr bool
	}{
		{
			name:      "valid packet",
			packet:    []byte{0x00, 0x00, 0x00, AtypIPv4, 8, 8, 8, 8, 0, 53, 1, 2, 3, 4},
			wantErr:   false,
			checkAddr: true,
		},
		{
			name:      "too short",
			packet:    []byte{0x00, 0x00, 0x00},
			wantErr:   true,
			checkAddr: false,
		},
		{
			name:      "non-zero reserved",
			packet:    []byte{0x01, 0x00, 0x00, AtypIPv4, 8, 8, 8, 8, 0, 53},
			wantErr:   true,
			checkAddr: false,
		},
		{
			name:      "fragmented",
			packet:    []byte{0x00, 0x00, 0x01, AtypIPv4, 8, 8, 8, 8, 0, 53},
			wantErr:   true,
			checkAddr: false,
		},
		{
			name:      "invalid address",
			packet:    []byte{0x00, 0x00, 0x00, 0xFF},
			wantErr:   true,
			checkAddr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, payload, err := DecodeUDPPacket(tt.packet)

			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeUDPPacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tt.checkAddr && addr == nil {
					t.Error("DecodeUDPPacket() addr should not be nil")
				}
				if len(payload) == 0 {
					t.Error("DecodeUDPPacket() payload should not be empty")
				}
			}
		})
	}
}

func TestDecodeUDPPacketInPlace(t *testing.T) {
	tests := []struct {
		name        string
		packet      []byte
		wantErr     bool
		wantAddrLen int
		wantPayload int
	}{
		{
			name:        "valid packet",
			packet:      []byte{0x00, 0x00, 0x00, AtypIPv4, 8, 8, 8, 8, 0, 53, 1, 2, 3, 4},
			wantErr:     false,
			wantAddrLen: 1 + net.IPv4len + 2,
			wantPayload: 4,
		},
		{
			name:    "too short",
			packet:  []byte{0x00, 0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "non-zero reserved",
			packet:  []byte{0x01, 0x00, 0x00, AtypIPv4, 8, 8, 8, 8, 0, 53},
			wantErr: true,
		},
		{
			name:    "fragmented",
			packet:  []byte{0x00, 0x00, 0x01, AtypIPv4, 8, 8, 8, 8, 0, 53},
			wantErr: true,
		},
		{
			name:    "invalid address",
			packet:  []byte{0x00, 0x00, 0x00, 0xFF},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, payloadLen, err := DecodeUDPPacketInPlace(tt.packet)

			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeUDPPacketInPlace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if addr == nil {
					t.Error("DecodeUDPPacketInPlace() addr should not be nil")
				}
				if len(addr) != tt.wantAddrLen {
					t.Errorf("DecodeUDPPacketInPlace() addr len = %d, want %d", len(addr), tt.wantAddrLen)
				}
				if payloadLen != tt.wantPayload {
					t.Errorf("DecodeUDPPacketInPlace() payloadLen = %d, want %d", payloadLen, tt.wantPayload)
				}
			}
		})
	}
}

func TestEncodeUDPPacket(t *testing.T) {
	tests := []struct {
		name    string
		addr    Addr
		payload []byte
		wantErr bool
	}{
		{
			name:    "valid IPv4",
			addr:    Addr{AtypIPv4, 8, 8, 8, 8, 0, 53},
			payload: []byte{1, 2, 3, 4},
			wantErr: false,
		},
		{
			name:    "valid IPv6",
			addr:    append(Addr{AtypIPv6}, append(net.ParseIP("::1").To16(), []byte{0, 53}...)...),
			payload: []byte{5, 6, 7, 8},
			wantErr: false,
		},
		{
			name:    "valid domain",
			addr:    Addr{AtypDomainName, 4, 't', 'e', 's', 't', 0, 53},
			payload: []byte{9, 10, 11, 12},
			wantErr: false,
		},
		{
			name:    "nil address",
			addr:    nil,
			payload: []byte{1, 2, 3},
			wantErr: true,
		},
		{
			name:    "empty payload",
			addr:    Addr{AtypIPv4, 8, 8, 8, 8, 0, 53},
			payload: []byte{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := EncodeUDPPacket(tt.addr, tt.payload)

			if (err != nil) != tt.wantErr {
				t.Errorf("EncodeUDPPacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify packet structure
				if len(packet) < 3+len(tt.addr)+len(tt.payload) {
					t.Error("EncodeUDPPacket() packet too short")
				}
				// Check reserved bytes
				if !bytes.Equal(packet[:3], []byte{0x00, 0x00, 0x00}) {
					t.Error("EncodeUDPPacket() reserved bytes should be zero")
				}
			}
		})
	}
}

func TestClientHandshake_AuthRequired(t *testing.T) {
	// Mock server that requires authentication
	readBuffer := &bytes.Buffer{}
	readBuffer.Write([]byte{Version, MethodUserPass}) // Server requires auth
	readBuffer.Write([]byte{0x01, 0x00})              // Auth success
	readBuffer.Write([]byte{Version, 0x00, 0x00})     // Connection success
	readBuffer.Write([]byte{AtypIPv4, 1, 2, 3, 4, 0, 0})

	reader := bufio.NewReader(bytes.NewReader(readBuffer.Bytes()))
	writeBuffer := &bytes.Buffer{}
	writer := bufio.NewWriter(writeBuffer)

	io := bufio.NewReadWriter(reader, writer)

	addr, err := ClientHandshake(io, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, CmdConnect, &User{
		Username: "testuser",
		Password: "testpass",
	})

	if err != nil {
		t.Errorf("ClientHandshake() error = %v", err)
	}
	if addr == nil {
		t.Error("ClientHandshake() addr should not be nil")
	}
}

func TestClientHandshake_NoAuth(t *testing.T) {
	// Mock server that doesn't require authentication
	readBuffer := &bytes.Buffer{}
	readBuffer.Write([]byte{Version, MethodNoAuth}) // No auth required
	readBuffer.Write([]byte{Version, 0x00, 0x00})   // Connection success
	readBuffer.Write([]byte{AtypIPv4, 10, 0, 0, 1, 0, 80})

	reader := bufio.NewReader(bytes.NewReader(readBuffer.Bytes()))
	writeBuffer := &bytes.Buffer{}
	writer := bufio.NewWriter(writeBuffer)

	io := bufio.NewReadWriter(reader, writer)

	addr, err := ClientHandshake(io, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, CmdConnect, nil)

	if err != nil {
		t.Errorf("ClientHandshake() error = %v", err)
	}
	if addr == nil {
		t.Error("ClientHandshake() addr should not be nil")
	}
}

func TestClientHandshake_AuthFailed(t *testing.T) {
	// Mock server that rejects credentials
	readBuffer := &bytes.Buffer{}
	readBuffer.Write([]byte{Version, MethodUserPass})
	readBuffer.Write([]byte{0x01, 0x01}) // Auth failed

	reader := bufio.NewReader(bytes.NewReader(readBuffer.Bytes()))
	writeBuffer := &bytes.Buffer{}
	writer := bufio.NewWriter(writeBuffer)

	io := bufio.NewReadWriter(reader, writer)

	_, err := ClientHandshake(io, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, CmdConnect, &User{
		Username: "baduser",
		Password: "badpass",
	})

	if err == nil {
		t.Error("ClientHandshake() should return error for failed auth")
	}
}

func TestClientHandshake_ConnectionRejected(t *testing.T) {
	// Mock server that rejects connection
	readBuffer := &bytes.Buffer{}
	readBuffer.Write([]byte{Version, MethodNoAuth})
	readBuffer.Write([]byte{Version, 0x05, 0x00}) // Connection refused

	reader := bufio.NewReader(bytes.NewReader(readBuffer.Bytes()))
	writeBuffer := &bytes.Buffer{}
	writer := bufio.NewWriter(writeBuffer)

	io := bufio.NewReadWriter(reader, writer)

	_, err := ClientHandshake(io, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, CmdConnect, nil)

	if err == nil {
		t.Error("ClientHandshake() should return error for rejected connection")
	}
}

func TestClientHandshake_EmptyCredentials(t *testing.T) {
	readBuffer := &bytes.Buffer{}
	readBuffer.Write([]byte{Version, MethodNoAuth})

	reader := bufio.NewReader(bytes.NewReader(readBuffer.Bytes()))
	writeBuffer := &bytes.Buffer{}
	writer := bufio.NewWriter(writeBuffer)

	io := bufio.NewReadWriter(reader, writer)

	_, err := ClientHandshake(io, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, CmdConnect, &User{
		Username: "",
		Password: "",
	})

	if err == nil {
		t.Error("ClientHandshake() should return error for empty credentials")
	}
}

func TestClientHandshake_VersionMismatch(t *testing.T) {
	// Mock server with wrong version
	readBuffer := &bytes.Buffer{}
	readBuffer.Write([]byte{0x04, MethodNoAuth}) // Wrong version

	reader := bufio.NewReader(bytes.NewReader(readBuffer.Bytes()))
	writeBuffer := &bytes.Buffer{}
	writer := bufio.NewWriter(writeBuffer)

	io := bufio.NewReadWriter(reader, writer)

	_, err := ClientHandshake(io, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, CmdConnect, nil)

	if err == nil {
		t.Error("ClientHandshake() should return error for version mismatch")
	}
}

func TestClientHandshake_UnsupportedMethod(t *testing.T) {
	// Mock server with unsupported method
	readBuffer := &bytes.Buffer{}
	readBuffer.Write([]byte{Version, 0xFF}) // Unsupported method

	reader := bufio.NewReader(bytes.NewReader(readBuffer.Bytes()))
	writeBuffer := &bytes.Buffer{}
	writer := bufio.NewWriter(writeBuffer)

	io := bufio.NewReadWriter(reader, writer)

	_, err := ClientHandshake(io, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, CmdConnect, nil)

	if err == nil {
		t.Error("ClientHandshake() should return error for unsupported method")
	}
}
