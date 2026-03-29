package proxy

import (
	"context"
	"fmt"
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
	tcpPools      map[string]*dnsConnPool // TCP connection pools for plain DNS
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

	// Close all TCP connection pools
	for _, pool := range d.tcpPools {
		pool.Close()
	}
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

	// Initialize TCP connection pools for plain DNS servers
	tcpPools := make(map[string]*dnsConnPool)
	for _, server := range cfg.Servers {
		if server.Type == "" || server.Type == "udp" {
			// Create TCP pool for fallback/large responses
			tcpPools[server.Address] = newDNSConnPool(server.Address)
			slog.Debug("Created TCP connection pool for DNS server", "server", server.Address)
		}
	}

	d := &DNS{
		dnsClient:     dnsClient,
		cfg:           cfg,
		interfaceName: interfaceName,
		dohClients:    dohClients,
		dotClients:    dotClients,
		tcpPools:      tcpPools,
		cache:         newDNSCache(1000), // Cache up to 1k DNS entries (reduced from 10k for memory optimization)
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
		tcpPools:      d.tcpPools,
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
	tcpPools      map[string]*dnsConnPool
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

		// Create context with timeout for async DNS exchange
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Channel for receiving response
		responseCh := make(chan *dns.Msg, 1)
		errCh := make(chan error, 1)

		// Start async exchange with all servers
		go d.asyncExchange(ctx, msg, responseCh, errCh)

		// Wait for response or timeout
		select {
		case response := <-responseCh:
			// Cache successful response
			if cacheKey != "" && response != nil {
				ttl := getTTL(response)
				d.cache.set(cacheKey, response, ttl)
			}
			// Update message ID to match request
			if response != nil {
				response.Id = msg.Id
			}
			d.answerCh <- response
		case err := <-errCh:
			slog.Debug("Async DNS exchange error", "err", err)
			d.answerCh <- &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: dns.RcodeServerFailure}}
		case <-ctx.Done():
			slog.Debug("Async DNS exchange timeout")
			d.answerCh <- &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: dns.RcodeServerFailure}}
		}
	}()

	return len(b), nil
}

// asyncExchange performs DNS exchange with multiple servers asynchronously
func (d *dnsConn) asyncExchange(ctx context.Context, msg *dns.Msg, responseCh chan<- *dns.Msg, errCh chan<- error) {
	var lastErr error

	// Retry configuration with exponential backoff
	const maxRetries = 2
	baseTimeout := 2 * time.Second

	for _, server := range d.cfg.Servers {
		for attempt := 0; attempt <= maxRetries; attempt++ {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
			}

			// Calculate timeout with exponential backoff
			timeout := baseTimeout * time.Duration(1<<uint(attempt))
			if timeout > 8*time.Second {
				timeout = 8 * time.Second
			}

			exchangeCtx, cancel := context.WithTimeout(ctx, timeout)
			
			// Handle local DNS
			if server.Address == "local" {
				localClient := localdns.NewLocalClient(d.interfaceName)
				response, err := localClient.ExchangeWithContext(exchangeCtx, msg)
				cancel()
				if err == nil {
					responseCh <- response
					return
				}
				lastErr = err
				slog.Debug("Local DNS exchange failed", "attempt", attempt+1, "err", err)
				if attempt < maxRetries {
					time.Sleep(time.Duration(attempt*50) * time.Millisecond)
				}
				continue
			}

			// Handle DoH (DNS-over-HTTPS)
			if server.Type == "https" {
				if client, ok := d.dohClients[server.Address]; ok {
					response, err := client.ExchangeWithContext(exchangeCtx, msg)
					cancel()
					if err == nil {
						responseCh <- response
						return
					}
					lastErr = err
					slog.Debug("DoH exchange failed", "server", server.Address, "attempt", attempt+1, "err", err)
					if attempt < maxRetries {
						time.Sleep(time.Duration(attempt*50) * time.Millisecond)
					}
					continue
				}
			}

			// Handle DoT (DNS-over-TLS)
			if server.Type == "tls" {
				if client, ok := d.dotClients[server.Address]; ok {
					response, err := client.ExchangeWithContext(exchangeCtx, msg)
					cancel()
					if err == nil {
						responseCh <- response
						return
					}
					lastErr = err
					slog.Debug("DoT exchange failed", "server", server.Address, "attempt", attempt+1, "err", err)
					if attempt < maxRetries {
						time.Sleep(time.Duration(attempt*50) * time.Millisecond)
					}
					continue
				}
			}

			// Handle plain DNS (default)
			// Try TCP pool first if available
			if pool, ok := d.tcpPools[server.Address]; ok {
				response, err := pool.Exchange(msg)
				cancel()
				if err == nil {
					responseCh <- response
					return
				}
				lastErr = err
				slog.Debug("TCP pool exchange failed, falling back to UDP", "server", server.Address, "err", err)
				if attempt < maxRetries {
					time.Sleep(time.Duration(attempt*50) * time.Millisecond)
				}
				continue
			}

			// Fallback to UDP
			response, _, err := d.dnsClient.ExchangeContext(exchangeCtx, msg, server.Address)
			cancel()
			if err == nil {
				responseCh <- response
				return
			}
			lastErr = err
			slog.Debug("Plain DNS exchange failed", "server", server.Address, "attempt", attempt+1, "err", err)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*50) * time.Millisecond)
			}
		}
	}

	if lastErr != nil {
		errCh <- lastErr
	} else {
		errCh <- fmt.Errorf("no DNS servers available")
	}
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
