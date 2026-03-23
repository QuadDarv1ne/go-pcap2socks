package tunnel

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/buffer"
	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	"github.com/QuadDarv1ne/go-pcap2socks/ratelimit"
	alog "github.com/anacrolix/log"
	"github.com/anacrolix/upnp"
)

const (
	// UdpSessionTimeout is the timeout for UDP sessions
	// Reduced from 5 minutes to 3 minutes for faster resource cleanup
	UdpSessionTimeout = 3 * time.Minute

	// udpRelayBufferSize uses adaptive buffer sizing
	udpRelayBufferSize = buffer.SmallBufferSize // DNS and typical UDP fits in 512 bytes

	// upnpCacheDuration is how long to cache discovered UPnP devices
	upnpCacheDuration = 5 * time.Minute
)

// Rate limiters for frequent UDP log messages
var (
	udpDialErrorLimiter = ratelimit.NewLimiter(1, 5)   // 1/sec, burst 5
	udpConnLimiter      = ratelimit.NewLimiter(10, 20) // 10/sec, burst 20
	udpReadErrorLimiter = ratelimit.NewLimiter(1, 3)   // 1/sec, burst 3
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
		err := mapping.device.DeletePortMapping(mapping.proto, mapping.externalPort)
		if err != nil {
			slog.Warn("Failed to remove UPnP port mapping",
				"device", mapping.device.GetLocalIPAddress(),
				"internalPort", mapping.internalPort,
				"externalPort", mapping.externalPort,
				"error", err)
		} else {
			slog.Info("Successfully removed UPnP port mapping",
				"device", mapping.device.GetLocalIPAddress(),
				"internalPort", mapping.internalPort,
				"externalPort", mapping.externalPort)
		}
	}
	s.mappings = nil
}

func addPortMapping(session *UDPSession, d upnp.Device, proto upnp.Protocol, internalPort int) {
	logger := slog.With(
		"device", d.GetLocalIPAddress(),
		"proto", proto,
		"internalPort", internalPort,
	)

	externalPort, err := d.AddPortMapping(proto, internalPort, internalPort, "go-pcap2socks", 0)
	if err != nil {
		logger.Warn("Failed to add UPnP port mapping", "error", err)
		return
	}

	mapping := &UDPMapping{
		device:       d,
		proto:        proto,
		internalPort: internalPort,
		externalPort: externalPort,
	}
	session.addMapping(mapping)

	logger.Info("Successfully added UPnP port mapping", "externalPort", externalPort)
}

func setupUPnP(session *UDPSession, port int) {
	if !shouldForwardPort(port) {
		slog.Debug("Skipping UPnP setup for excluded port", "port", port)
		return
	}

	devices := getUPnPDevices()
	slog.Info("Discovered UPnP devices", "count", len(devices))

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
		slog.Debug("Using cached UPnP devices", "count", len(devices))
		return devices
	}
	upnpCacheMu.RUnlock()

	// Cache expired or empty, discover new devices
	upnpCacheMu.Lock()
	defer upnpCacheMu.Unlock()

	// Double-check after acquiring write lock
	if now.Before(upnpCacheExpiry) && len(upnpCachedDevices) > 0 {
		slog.Debug("Using cached UPnP devices (double-check)", "count", len(upnpCachedDevices))
		return upnpCachedDevices
	}

	// Perform discovery
	slog.Debug("Discovering UPnP devices (cache miss or expired)")
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
		if udpDialErrorLimiter.Allow() {
			slog.Debug("[UDP] dial error", "error", err)
		}
		session.cleanup()
		return
	}
	defer func() {
		pc.Close()
		session.cleanup()
	}()

	if udpConnLimiter.Allow() {
		slog.Debug("[UDP] Connection", "source", metadata.SourceAddress(), "dest", metadata.DestinationAddress())
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	go pipeChannel(pc, uc, &wg)
	go pipeChannel(uc, pc, &wg)
	wg.Wait()

	uc.Close()
	if udpConnLimiter.Allow() {
		slog.Debug("[UDP] Connection closed", "source", metadata.SourceAddress(), "dest", metadata.DestinationAddress())
	}
}

func pipeChannel(from net.PacketConn, to net.PacketConn, wg *sync.WaitGroup) {
	defer wg.Done()

	buf := buffer.Get(udpRelayBufferSize)
	defer buffer.Put(buf)

	for {
		from.SetReadDeadline(time.Now().Add(UdpSessionTimeout))
		n, dest, err := from.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				slog.Debug("[UDP] pipe closed", "source", from.LocalAddr(), "dest", to.LocalAddr(), "error", err)
				return
			}
			if !errors.Is(err, os.ErrDeadlineExceeded) {
				if udpReadErrorLimiter.Allow() {
					slog.Debug("[UDP] read error", "source", from.LocalAddr(), "dest", to.LocalAddr(), "error", err)
				}
			}

			return
		}

		to.SetWriteDeadline(time.Now().Add(UdpSessionTimeout))
		if _, err := to.WriteTo(buf[:n], dest); err != nil {
			if udpReadErrorLimiter.Allow() {
				slog.Debug("[UDP] write error", "source", from.LocalAddr(), "dest", dest, "error", err)
			}
			return
		}
	}
}
