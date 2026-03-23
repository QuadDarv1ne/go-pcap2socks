package device

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/QuadDarv1ne/go-pcap2socks/arpr"
	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/core"
	"github.com/QuadDarv1ne/go-pcap2socks/core/device/iobased"
	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/ethernet"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type PCAP struct {
	stack.LinkEndpoint

	name string

	network    *net.IPNet
	localIP    net.IP
	localMAC   net.HardwareAddr
	handle     *pcap.Handle
	ipMacTable map[string]net.HardwareAddr
	Interface  net.Interface
	mtu        uint32 // Configured MTU (may differ from Interface.MTU)
	rMux       sync.Mutex
	stacker    func() Stacker
	dhcpServer DHCPServer // DHCP server interface
}

const offset = 0

func Open(captureCfg cfg.Capture, ifce net.Interface, netConfig *NetworkConfig, stacker func() Stacker) (_ Device, err error) {
	return OpenWithDHCP(captureCfg, ifce, netConfig, stacker, nil)
}

// OpenWithDHCP opens a PCAP device with optional DHCP server support
func OpenWithDHCP(captureCfg cfg.Capture, ifce net.Interface, netConfig *NetworkConfig, stacker func() Stacker, dhcpServer DHCPServer) (_ Device, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("open tun: %v", r)
		}
	}()

	// Find the pcap device for this interface
	dev, err := findPcapDevice(ifce)
	if err != nil {
		return nil, err
	}

	pcaphInactive, err := createPcapHandle(dev)
	if err != nil {
		return nil, err
	}

	pcaph, err := pcaphInactive.Activate()
	if err != nil {
		return nil, fmt.Errorf("open live error: %w", err)
	}

	// NOTE: BPF filter disabled for DHCP support
	// DHCP broadcast packets from devices without IP are not captured by Npcap with BPF filters
	// We filter packets in Read() instead
	// 
	// Original filter (for reference):
	// "(arp dst host %s and arp src net %s and not arp src host %s) or (src net %s and not dst net %s and not (icmp and src host %s)) or (udp port 67 or udp port 68)"
	
	// No BPF filter - capture all packets and filter in Read()
	// This allows DHCP broadcast packets to be captured

	t := &PCAP{
		name:       "dspcap",
		stacker:    stacker,
		Interface:  ifce,
		network:    netConfig.Network,
		localIP:    netConfig.LocalIP,
		localMAC:   netConfig.LocalMAC,
		mtu:        netConfig.MTU,
		handle:     pcaph,
		ipMacTable: make(map[string]net.HardwareAddr),
		dhcpServer: dhcpServer,
	}

	ep, err := iobased.New(t, netConfig.MTU, offset, t.localMAC)
	if err != nil {
		return nil, fmt.Errorf("create endpoint: %w", err)
	}

	// we are in L2 and using ethernet header
	t.LinkEndpoint = ethernet.New(ep)

	// Setup PCAP capture if enabled
	if captureCfg.Enabled {
		snifferEp, err := NewEthSniffer(t.LinkEndpoint, captureCfg.OutputFile)
		if err != nil {
			slog.Error("Failed to setup PCAP capture", "error", err)
		} else {
			t.LinkEndpoint = snifferEp
		}
	}

	// send gratuitous arp
	{
		arpGratuitous, err := arpr.SendGratuitousArp(netConfig.LocalIP, netConfig.LocalMAC)
		if err != nil {
			return nil, fmt.Errorf("send gratuitous arp error: %w", err)
		}

		err = t.handle.WritePacketData(arpGratuitous)
		if err != nil {
			return nil, fmt.Errorf("write packet error: %w", err)
		}
	}

	return t, nil
}

func createPcapHandle(dev pcap.Interface) (*pcap.InactiveHandle, error) {
	handle, err := pcap.NewInactiveHandle(dev.Name)
	if err != nil {
		return nil, fmt.Errorf("new inactive handle error: %w", err)
	}

	err = handle.SetPromisc(true)
	if err != nil {
		return nil, fmt.Errorf("set promisc error: %w", err)
	}

	err = handle.SetSnapLen(1600)
	if err != nil {
		return nil, fmt.Errorf("set snap len error: %w", err)
	}

	err = handle.SetTimeout(pcap.BlockForever)
	if err != nil {
		return nil, fmt.Errorf("set timeout error: %w", err)
	}

	err = handle.SetImmediateMode(true)
	if err != nil {
		return nil, fmt.Errorf("set immediate mode error: %w", err)
	}

	err = handle.SetBufferSize(4 * 1024 * 1024) // 4MB buffer for better performance
	if err != nil {
		return nil, fmt.Errorf("set buffer size error: %w", err)
	}

	return handle, nil
}

func (t *PCAP) Read() []byte {
	t.rMux.Lock()
	defer t.rMux.Unlock()
	data, _, err := t.handle.ZeroCopyReadPacketData()
	if err != nil {
		slog.Error("read packet error: %w", slog.Any("err", err))
		return nil
	}

	ethProtocol := header.Ethernet(data)
	switch ethProtocol.Type() {
	case header.IPv4ProtocolNumber:
		ipProtocol := header.IPv4(data[14:])
		srcAddress := ipProtocol.SourceAddress()
		if !t.network.Contains(srcAddress.AsSlice()) {
			return nil
		}

		// Check if this is a DHCP packet (UDP port 67 or 68)
		if ipProtocol.Protocol() == 17 { // UDP protocol number
			udpHeader := header.UDP(data[14+int(ipProtocol.HeaderLength()):])
			srcPort := udpHeader.SourcePort()
			dstPort := udpHeader.DestinationPort()

			// Log all UDP packets for debugging
			slog.Info("UDP packet captured", "src_port", srcPort, "dst_port", dstPort, "src_mac", net.HardwareAddr(ethProtocol.SourceAddress()).String())
			
			// DHCP uses ports 67 (server) and 68 (client)
			if (srcPort == 68 || dstPort == 67) && t.dhcpServer != nil {
				// This is a DHCP request from a client
				srcMAC := net.HardwareAddr(ethProtocol.SourceAddress())
				dstMAC := net.HardwareAddr(ethProtocol.DestinationAddress())
				slog.Info("DHCP packet captured", 
					"src_port", srcPort, 
					"dst_port", dstPort,
					"src_mac", srcMAC.String(),
					"dst_mac", dstMAC.String())
				
				// Parse the packet and handle DHCP request
				response, err := t.handleDHCP(data)
				if err != nil {
					slog.Error("DHCP handle error", "err", err)
				} else if response != nil {
					slog.Info("DHCP response generated", "response_len", len(response))
					// Send DHCP response
					if err := t.handle.WritePacketData(response); err != nil {
						slog.Error("DHCP write error", "err", err)
					} else {
						slog.Info("DHCP response sent successfully")
					}
				}
				return nil // Don't pass DHCP packets to the stack
			}
		}

		if bytes.Compare(srcAddress.AsSlice(), t.localIP) != 0 {
			t.SetHardwareAddr(srcAddress.AsSlice(), []byte(ethProtocol.SourceAddress()))
		}
	case header.ARPProtocolNumber:
		gPckt := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
		arpLayer, isArp := gPckt.Layer(layers.LayerTypeARP).(*layers.ARP)
		if !isArp {
			return nil
		}

		srcIP := net.IP(arpLayer.SourceProtAddress)
		dstIP := net.IP(arpLayer.DstProtAddress)

		// Same like in BPF filter
		if bytes.Compare(srcIP, t.localIP) != 0 &&
			bytes.Compare(dstIP, t.localIP) == 0 &&
			t.network.Contains(srcIP) {
			t.SetHardwareAddr(srcIP, arpLayer.SourceHwAddress)
		} else {
			return nil
		}

	default:
		return nil
	}

	return data
}

// handleDHCP processes DHCP packets and returns response
func (t *PCAP) handleDHCP(data []byte) ([]byte, error) {
	if t.dhcpServer == nil {
		return nil, nil
	}

	// Parse Ethernet header to get MAC addresses
	eth := header.Ethernet(data)
	srcMAC := net.HardwareAddr(eth.SourceAddress())
	dstMAC := net.HardwareAddr(eth.DestinationAddress())

	// Skip Ethernet header (14 bytes)
	ipStart := 14
	if len(data) <= ipStart {
		return nil, fmt.Errorf("packet too short")
	}

	// Parse IP header
	ip := header.IPv4(data[ipStart:])
	if len(data) <= ipStart+int(ip.HeaderLength()) {
		return nil, fmt.Errorf("packet too short for IP header")
	}

	// Skip IP header
	udpStart := ipStart + int(ip.HeaderLength())
	if len(data) <= udpStart+8 {
		return nil, fmt.Errorf("packet too short for UDP header")
	}

	// Parse UDP header
	udp := header.UDP(data[udpStart:])
	srcPort := udp.SourcePort()
	dstPort := udp.DestinationPort()

	srcAddr := ip.SourceAddress()
	dstAddr := ip.DestinationAddress()
	
	slog.Debug("DHCP packet details", 
		"src_mac", srcMAC.String(),
		"dst_mac", dstMAC.String(),
		"src_ip", net.IP(srcAddr.AsSlice()).String(),
		"dst_ip", net.IP(dstAddr.AsSlice()).String(),
		"src_port", srcPort,
		"dst_port", dstPort)

	// DHCP payload starts after UDP header (8 bytes)
	dhcpStart := udpStart + 8
	if len(data) <= dhcpStart {
		return nil, fmt.Errorf("packet too short for DHCP")
	}

	dhcpData := data[dhcpStart:]

	// Handle DHCP request
	responseData, err := t.dhcpServer.HandleRequest(dhcpData)
	if err != nil || responseData == nil {
		return nil, err
	}

	// Build response packet
	srcIP := t.localIP
	dstIP := net.IPv4(255, 255, 255, 255) // Broadcast for DHCPOFFER/DHCPACK
	
	// If client requested a specific IP or already has one, use unicast
	if len(dhcpData) > 16 {
		clientIP := net.IP(dhcpData[12:16]).To4()
		if !clientIP.Equal(net.IPv4zero) {
			dstIP = clientIP
		}
	}

	responsePacket, err := dhcp.BuildDHCPRequestPacket(
		t.localMAC,           // Source MAC (server)
		dstMAC,               // Destination MAC (client)
		srcIP,                // Source IP (server)
		dstIP,                // Destination IP (client or broadcast)
		67,                   // Source port (DHCP server)
		68,                   // Destination port (DHCP client)
		responseData,
	)
	if err != nil {
		return nil, fmt.Errorf("build DHCP response: %w", err)
	}

	slog.Info("DHCP response sent", 
		"client_mac", dstMAC.String(),
		"dst_ip", dstIP.String())

	return responsePacket, nil
}

func (t *PCAP) Write(p []byte) (n int, err error) {
	err = t.handle.WritePacketData(p)
	if err != nil {
		// Check if it's a network adapter disconnected error
		if isAdapterDisconnected(err) {
			// Don't log every packet, just log once
			slog.Warn("Network adapter disconnected, waiting for reconnection...")
			return 0, nil // Silently drop packets, don't error
		}
		slog.Debug("write packet error: %w", slog.Any("err", err))
		return 0, nil
	}

	return len(p), nil
}

// isAdapterDisconnected checks if the error is due to network adapter being disconnected
func isAdapterDisconnected(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for common Windows Npcap errors related to adapter disconnection
	disconnectErrors := []string{
		"PacketSendPacket failed",
		"сетевой носитель отключен",
		"network medium disconnected",
		"adapter disconnected",
		"no such device",
	}

	for _, disconnectErr := range disconnectErrors {
		if contains(errStr, disconnectErr) {
			return true
		}
	}

	return false
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (t *PCAP) Name() string {
	return t.name
}

func (t *PCAP) Close() {
	t.handle.Close()
	t.LinkEndpoint.Close() // Cascade close: sniffer → ethernet → iobased
}

func (t *PCAP) Type() string {
	return "pcap"
}

func (t *PCAP) SetHardwareAddr(srcIP net.IP, srcMAC net.HardwareAddr) {
	if _, ok := t.ipMacTable[string(srcIP)]; !ok {
		slog.Info(fmt.Sprintf("Device %s (%s) joined the network", srcIP, srcMAC))
		t.ipMacTable[string(srcIP)] = srcMAC
		// after restart app some devices doesnt react to GratuitousArp, so we need to add them manually
		t.stacker().AddStaticNeighbor(core.NicID, header.IPv4ProtocolNumber, tcpip.AddrFrom4Slice(srcIP), tcpip.LinkAddress(srcMAC))
	}
}

type Stacker interface {
	AddStaticNeighbor(nicID tcpip.NICID, protocol tcpip.NetworkProtocolNumber, addr tcpip.Address, linkAddr tcpip.LinkAddress) tcpip.Error
	AddProtocolAddress(id tcpip.NICID, protocolAddress tcpip.ProtocolAddress, properties stack.AddressProperties) tcpip.Error
}

func findPcapDevice(ifce net.Interface) (pcap.Interface, error) {
	// Get all pcap devices
	devices, err := pcap.FindAllDevs()
	if err != nil {
		return pcap.Interface{}, fmt.Errorf("find all devices error: %w", err)
	}

	// Get interface addresses
	addrs, err := ifce.Addrs()
	if err != nil {
		return pcap.Interface{}, fmt.Errorf("get interface addresses error: %w", err)
	}

	// Find matching device
	for _, dev := range devices {
		for _, devAddr := range dev.Addresses {
			for _, ifaceAddr := range addrs {
				if ipnet, ok := ifaceAddr.(*net.IPNet); ok {
					if devAddr.IP.Equal(ipnet.IP) {
						return dev, nil
					}
				}
			}
		}
	}

	return pcap.Interface{}, fmt.Errorf("pcap device not found for interface %s", ifce.Name)
}
