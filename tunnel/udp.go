package tunnel

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	alog "github.com/anacrolix/log"
	"github.com/anacrolix/upnp"
)

const (
	// UdpSessionTimeout is the timeout for UDP sessions
	// Reduced from 5 minutes to 3 minutes for faster resource cleanup
	UdpSessionTimeout = 3 * time.Minute

	// udpRelayBufferSize optimized for DNS and typical UDP (most packets < 512 bytes)
	udpRelayBufferSize = 512

	// upnpCacheDuration is how long to cache discovered UPnP devices
	upnpCacheDuration = 5 * time.Minute
)

// UPnP device cache to avoid repeated discovery
var (
	upnpCacheMu      sync.RWMutex
	upnpCachedDevices []upnp.Device
	upnpCacheExpiry   time.Time
)

type UDPMapping struct {
	device       upnp.Device
	proto        upnp.Protocol
	internalPort int
	externalPort int
}

type UDPSession struct {
	conn     net.PacketConn
	mappings []*UDPMapping
	mutex    sync.Mutex
}

var excludedPorts = map[int]bool{
	53:   true, // DNS
	123:  true, // NTP
	137:  true, // NetBIOS Name Service
	138:  true, // NetBIOS Datagram Service
	161:  true, // SNMP
	162:  true, // SNMP Trap
	1900: true, // SSDP (UPnP discovery)
}

func shouldForwardPort(port int) bool {
	// Don't forward system ports (0-1023) and excluded ports
	if port <= 1023 || excludedPorts[port] {
		return false
	}
	return true
}

func (s *UDPSession) addMapping(mapping *UDPMapping) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.mappings = append(s.mappings, mapping)
}

func (s *UDPSession) cleanup() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, mapping := range s.mappings {
		_ = mapping.device.DeletePortMapping(mapping.proto, mapping.externalPort)
	}
	s.mappings = nil
}

func addPortMapping(session *UDPSession, d upnp.Device, proto upnp.Protocol, internalPort int) {
	externalPort, err := d.AddPortMapping(proto, internalPort, internalPort, "go-pcap2socks", 0)
	if err != nil {
		return
	}

	mapping := &UDPMapping{
		device:       d,
		proto:        proto,
		internalPort: internalPort,
		externalPort: externalPort,
	}
	session.addMapping(mapping)
}

func setupUPnP(session *UDPSession, port int) {
	if !shouldForwardPort(port) {
		return
	}

	devices := getUPnPDevices()

	for _, d := range devices {
		go addPortMapping(session, d, upnp.UDP, port)
	}
}

// getUPnPDevices returns cached UPnP devices or discovers new ones
func getUPnPDevices() []upnp.Device {
	now := time.Now()

	// Try to use cached devices first
	upnpCacheMu.RLock()
	if now.Before(upnpCacheExpiry) && len(upnpCachedDevices) > 0 {
		devices := upnpCachedDevices
		upnpCacheMu.RUnlock()
		return devices
	}
	upnpCacheMu.RUnlock()

	// Cache expired or empty, discover new devices
	upnpCacheMu.Lock()
	defer upnpCacheMu.Unlock()

	// Double-check after acquiring write lock
	if now.Before(upnpCacheExpiry) && len(upnpCachedDevices) > 0 {
		return upnpCachedDevices
	}

	// Perform discovery
	devices := upnp.Discover(0, 2*time.Second, alog.NewLogger("upnp"))

	// Update cache
	upnpCachedDevices = devices
	upnpCacheExpiry = now.Add(upnpCacheDuration)

	return devices
}

func HandleUDPConn(uc adapter.UDPConn) {
	metadata := uc.MD()

	session := &UDPSession{}

	// Setup UPnP port mapping for the source port
	_, srcPort := parseAddr(metadata.Addr())
	setupUPnP(session, int(srcPort))

	pc, err := proxy.DialUDP(metadata)
	if err != nil {
		session.cleanup()
		return
	}
	defer func() {
		pc.Close()
		session.cleanup()
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	go pipeChannel(pc, uc, &wg)
	go pipeChannel(uc, pc, &wg)
	wg.Wait()

	uc.Close()
}

func pipeChannel(from net.PacketConn, to net.PacketConn, wg *sync.WaitGroup) {
	defer wg.Done()

	buf := pool.Get(udpRelayBufferSize)
	defer pool.Put(buf)

	for {
		from.SetReadDeadline(time.Now().Add(UdpSessionTimeout))
		n, dest, err := from.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, io.ErrClosedPipe) || !errors.Is(err, os.ErrDeadlineExceeded) {
				return
			}
			return
		}

		to.SetWriteDeadline(time.Now().Add(UdpSessionTimeout))
		if _, err := to.WriteTo(buf[:n], dest); err != nil {
			return
		}
	}
}
