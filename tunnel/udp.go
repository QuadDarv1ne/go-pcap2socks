// Package tunnel provides network tunnel functionality.
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
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	alog "github.com/anacrolix/log"
	"github.com/anacrolix/upnp"
)

// UDP tunnel constants
const (
	// UdpSessionTimeout is the timeout for UDP sessions
	// Reduced to 90 seconds for faster resource cleanup and gaming responsiveness
	UdpSessionTimeout = 90 * time.Second

	// udpMaxSessionLifetime is the maximum lifetime for any UDP session
	udpMaxSessionLifetime = 10 * time.Minute

	// udpMaxIdleResets is the maximum number of idle timeout resets before session ends
	udpMaxIdleResets = 5

	// udpRelayBufferSize increased to support larger UDP packets (PS4, gaming, etc.)
	udpRelayBufferSize = 65535

	// upnpCacheDuration is how long to cache discovered UPnP devices
	upnpCacheDuration = 5 * time.Minute
)

// UPnP device cache to avoid repeated discovery
var (
	upnpCacheMu       sync.RWMutex
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
		goroutine.SafeGo(func() {
			addPortMapping(session, d, upnp.UDP, port)
		})
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
		uc.Close() // Close input connection to prevent leak
		return
	}
	defer func() {
		pc.Close()
		session.cleanup()
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	// Use SafeGo for panic protection (pipeChannel has internal recover but SafeGo adds extra safety)
	goroutine.SafeGo(func() {
		pipeChannel(pc, uc, &wg)
	})
	goroutine.SafeGo(func() {
		pipeChannel(uc, pc, &wg)
	})
	wg.Wait()

	uc.Close()
}

//go:noinline
func pipeChannel(from net.PacketConn, to net.PacketConn, wg *sync.WaitGroup) {
	defer wg.Done()

	// Get buffer from global pool instead of allocating
	buf := buffer.Get(udpRelayBufferSize)
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("UDP pipe panic recovered", "recover", r)
		}
		buffer.Put(buf) // Ensure buffer is returned even on panic
	}()

	// Set deadlines ONCE at session start to avoid syscall overhead
	// Updating deadline on every packet is expensive (2000+ syscalls/sec for gaming traffic)
	deadline := time.Now().Add(UdpSessionTimeout)
	sessionStart := time.Now()
	idleResets := 0
	from.SetReadDeadline(deadline)
	to.SetWriteDeadline(deadline)

	for {
		// Check max session lifetime
		if time.Since(sessionStart) > udpMaxSessionLifetime {
			slog.Debug("UDP session max lifetime exceeded, closing")
			return
		}

		n, dest, err := from.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
				return
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				// Session timeout - extend deadline with limits
				// Prevent infinite session by capping idle resets
				idleResets++
				if idleResets >= udpMaxIdleResets {
					slog.Debug("UDP session idle timeout limit reached, closing")
					return
				}
				deadline := time.Now().Add(UdpSessionTimeout)
				from.SetReadDeadline(deadline)
				to.SetWriteDeadline(deadline)
				continue
			}
			// Check for other network timeout errors
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// Treat same as DeadlineExceeded - extend with limits
				idleResets++
				if idleResets >= udpMaxIdleResets {
					slog.Debug("UDP session idle timeout limit reached, closing")
					return
				}
				deadline := time.Now().Add(UdpSessionTimeout)
				from.SetReadDeadline(deadline)
				to.SetWriteDeadline(deadline)
				continue
			}
			return
		}

		if _, err := to.WriteTo(buf[:n], dest); err != nil {
			// Don't log expected write errors
			if !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
				var netErr net.Error
				if !errors.As(err, &netErr) || !netErr.Timeout() {
					slog.Debug("UDP pipe write error", "err", err)
				}
			}
			return
		}
	}
}
