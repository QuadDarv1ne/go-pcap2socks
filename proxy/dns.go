package proxy

import (
	"context"
	"log/slog"
	"math"
	"net"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	localdns "github.com/QuadDarv1ne/go-pcap2socks/dns"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"github.com/miekg/dns"
)

var _ Proxy = (*DNS)(nil)

// dnsConnPool wraps connection pool for DNS
type dnsConnPool struct {
	pool *localdns.ConnPool
}

func newDNSConnPool(addr string) *dnsConnPool {
	pool := localdns.NewConnPool(addr, "tcp", 4, 30*time.Second, 5*time.Second)
	return &dnsConnPool{pool: pool}
}

func (p *dnsConnPool) Exchange(msg *dns.Msg) (*dns.Msg, error) {
	return p.pool.Exchange(msg)
}

func (p *dnsConnPool) Close() error {
	return p.pool.Close()
}

type DNS struct {
	cfg           cfg.DNS
	dnsClient     *dns.Client
	interfaceName string
	dohClients    map[string]*localdns.DoHClient
	dotClients    map[string]*localdns.DoTClient
	cache         *dnsCache
	stopCleanup   chan struct{}
}

func (d *DNS) Addr() string {
	return "dns-server"
}

func (d *DNS) Mode() Mode {
	return ModeDNS
}

// cleanupLoop periodically removes expired cache entries
func (d *DNS) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.cache.cleanup()

			// Log cache stats
			hits, misses := d.cache.stats()
			if hits+misses > 0 {
				hitRate := float64(hits) / float64(hits+misses) * 100
				slog.Debug("DNS cache stats",
					"hits", hits,
					"misses", misses,
					"hit_rate", hitRate)
			}
		case <-d.stopCleanup:
			return
		}
	}
}

// Close stops the DNS proxy and cleanup goroutine
func (d *DNS) Close() {
	close(d.stopCleanup)
}

func NewDNS(cfg cfg.DNS, interfaceName string) *DNS {
	dnsClient := new(dns.Client)
	dnsClient.UDPSize = math.MaxUint16

	// Initialize DoH clients
	dohClients := make(map[string]*localdns.DoHClient)
	for _, server := range cfg.Servers {
		if server.Type == "https" {
			if client, err := localdns.NewDoHClient(server.Address); err == nil {
				dohClients[server.Address] = client
			} else {
				slog.Warn("failed to create DoH client", "server", server.Address, "err", err)
			}
		}
	}

	// Initialize DoT clients
	dotClients := make(map[string]*localdns.DoTClient)
	for _, server := range cfg.Servers {
		if server.Type == "tls" {
			tlsConfig := &localdns.TLSConfig{
				ServerName: server.ServerName,
				SkipVerify: server.SkipVerify,
			}
			if client, err := localdns.NewDoTClient(server.Address, tlsConfig); err == nil {
				dotClients[server.Address] = client
			} else {
				slog.Warn("failed to create DoT client", "server", server.Address, "err", err)
			}
		}
	}

	d := &DNS{
		dnsClient:     dnsClient,
		cfg:           cfg,
		interfaceName: interfaceName,
		dohClients:    dohClients,
		dotClients:    dotClients,
		cache:         newDNSCache(10000), // Cache up to 10k DNS entries
		stopCleanup:   make(chan struct{}),
	}

	// Start cache cleanup goroutine
	go d.cleanupLoop()

	return d
}

func (d *DNS) DialContext(_ context.Context, _ *M.Metadata) (net.Conn, error) {
	return &nopConn{}, nil
}

func (d *DNS) DialUDP(m *M.Metadata) (net.PacketConn, error) {
	return &dnsConn{
		cfg:           d.cfg,
		m:             m,
		dnsClient:     d.dnsClient,
		answerCh:      make(chan *dns.Msg),
		interfaceName: d.interfaceName,
		dohClients:    d.dohClients,
		dotClients:    d.dotClients,
		cache:         d.cache,
	}, nil
}

type dnsConn struct {
	dnsClient     *dns.Client
	answerCh      chan *dns.Msg
	m             *M.Metadata
	cfg           cfg.DNS
	interfaceName string
	dohClients    map[string]*localdns.DoHClient
	dotClients    map[string]*localdns.DoTClient
	cache         *dnsCache
}

func (d *dnsConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	msg := <-d.answerCh
	_, err = msg.PackBuffer(b)
	if err != nil {
		return 0, nil, err
	}

	return msg.Len(), d.m.UDPAddr(), nil
}

func (d *dnsConn) WriteTo(b []byte, _ net.Addr) (n int, err error) {
	msg := new(dns.Msg)
	err = msg.Unpack(b)
	if err != nil {
		return 0, err
	}

	go func() {
		// Check cache first
		cacheKey := getCacheKey(msg)
		if cacheKey != "" {
			if cached, found := d.cache.get(cacheKey); found {
				// Update message ID to match request
				cached.Id = msg.Id
				d.answerCh <- cached
				return
			}
		}

		var response *dns.Msg
		var lastErr error

		for _, server := range d.cfg.Servers {
			// Handle local DNS
			if server.Address == "local" {
				localClient := localdns.NewLocalClient(d.interfaceName)
				response, lastErr = localClient.Exchange(msg)
				if lastErr == nil {
					// Cache successful response
					if cacheKey != "" {
						ttl := getTTL(response)
						d.cache.set(cacheKey, response, ttl)
					}
					d.answerCh <- response
					return
				}
				slog.Error("local dns exchange failed", slog.Any("err", lastErr))
				continue
			}

			// Handle DoH (DNS-over-HTTPS)
			if server.Type == "https" {
				if client, ok := d.dohClients[server.Address]; ok {
					response, lastErr = client.Exchange(msg)
					if lastErr == nil {
						// Cache successful response
						if cacheKey != "" {
							ttl := getTTL(response)
							d.cache.set(cacheKey, response, ttl)
						}
						d.answerCh <- response
						return
					}
					slog.Error("DoH exchange failed", slog.String("server", server.Address), slog.Any("err", lastErr))
					continue
				}
			}

			// Handle DoT (DNS-over-TLS)
			if server.Type == "tls" {
				if client, ok := d.dotClients[server.Address]; ok {
					response, lastErr = client.Exchange(msg)
					if lastErr == nil {
						// Cache successful response
						if cacheKey != "" {
							ttl := getTTL(response)
							d.cache.set(cacheKey, response, ttl)
						}
						d.answerCh <- response
						return
					}
					slog.Error("DoT exchange failed", slog.String("server", server.Address), slog.Any("err", lastErr))
					continue
				}
			}

			// Handle plain DNS (default)
			response, _, lastErr = d.dnsClient.Exchange(msg, server.Address)
			if lastErr == nil {
				// Cache successful response
				if cacheKey != "" {
					ttl := getTTL(response)
					d.cache.set(cacheKey, response, ttl)
				}
				d.answerCh <- response
				return
			}
			slog.Error("plain dns exchange failed", slog.String("server", server.Address), slog.Any("err", lastErr))
		}

		if lastErr != nil {
			slog.Error("all dns servers failed", slog.Any("err", lastErr))
		}
	}()

	return len(b), nil
}

func (d *dnsConn) Close() error {
	return nil
}

func (d *dnsConn) LocalAddr() net.Addr {
	return nil
}

func (d *dnsConn) SetDeadline(t time.Time) error {
	return nil
}

func (d *dnsConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (d *dnsConn) SetWriteDeadline(t time.Time) error {
	return nil
}

//
//type DNS interface {
//	Exchange(ctx context.Context, message *dns.Msg) (*dns.Msg, error)
//}
