package proxy

import (
	"context"
	"net"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"github.com/QuadDarv1ne/go-pcap2socks/ratelimit"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
)

// StatsProxy wraps a Proxy to track statistics and enforce rate limits
type StatsProxy struct {
	proxy       Proxy
	statsStore  *stats.Store
	rateLimiter *ratelimit.RateLimiter
	enabled     bool
}

// NewStatsProxy creates a new proxy with statistics tracking and rate limiting
func NewStatsProxy(proxy Proxy, store *stats.Store) *StatsProxy {
	return &StatsProxy{
		proxy:       proxy,
		statsStore:  store,
		rateLimiter: ratelimit.NewRateLimiter(),
		enabled:     true,
	}
}

// GetRateLimiter returns the rate limiter for external configuration
func (sp *StatsProxy) GetRateLimiter() *ratelimit.RateLimiter {
	return sp.rateLimiter
}

func (sp *StatsProxy) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	conn, err := sp.proxy.DialContext(ctx, metadata)
	if err != nil {
		return nil, err
	}

	if sp.enabled && sp.statsStore != nil {
		srcIP := metadata.SrcIP.String()
		dstIP := metadata.DstIP.String()

		// Get rate limits from stats store
		uploadLimit, downloadLimit := sp.statsStore.GetRateLimit(srcIP)
		if uploadLimit > 0 || downloadLimit > 0 {
			sp.rateLimiter.SetLimit(srcIP, uploadLimit, downloadLimit)
		}

		return &statsConn{
			Conn:        conn,
			statsStore:  sp.statsStore,
			rateLimiter: sp.rateLimiter,
			metadata:    metadata,
			srcIP:       srcIP,
			dstIP:       dstIP,
		}, nil
	}

	return conn, nil
}

func (sp *StatsProxy) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	conn, err := sp.proxy.DialUDP(metadata)
	if err != nil {
		return nil, err
	}

	if sp.enabled && sp.statsStore != nil {
		srcIP := metadata.SrcIP.String()
		dstIP := metadata.DstIP.String()

		// Get rate limits from stats store
		uploadLimit, downloadLimit := sp.statsStore.GetRateLimit(srcIP)
		if uploadLimit > 0 || downloadLimit > 0 {
			sp.rateLimiter.SetLimit(srcIP, uploadLimit, downloadLimit)
		}

		return &statsPacketConn{
			PacketConn:  conn,
			statsStore:  sp.statsStore,
			rateLimiter: sp.rateLimiter,
			metadata:    metadata,
			srcIP:       srcIP,
			dstIP:       dstIP,
		}, nil
	}

	return conn, nil
}

func (sp *StatsProxy) Mode() Mode {
	return sp.proxy.Mode()
}

func (sp *StatsProxy) Addr() string {
	return sp.proxy.Addr()
}

// statsConn wraps net.Conn to track traffic and enforce rate limits
type statsConn struct {
	net.Conn
	statsStore  *stats.Store
	rateLimiter *ratelimit.RateLimiter
	metadata    *M.Metadata
	srcIP       string // Cached to avoid repeated String() calls
	dstIP       string
}

func (sc *statsConn) Read(b []byte) (int, error) {
	// Check rate limit for download with exponential backoff
	if sc.rateLimiter != nil && !sc.rateLimiter.Allow(sc.srcIP, len(b), false) {
		// Use context-based wait instead of blocking sleep
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = sc.rateLimiter.Wait(ctx, sc.srcIP, len(b), false)
	}

	n, err := sc.Conn.Read(b)
	if n > 0 && sc.statsStore != nil {
		sc.statsStore.RecordTraffic(sc.dstIP, sc.srcIP, uint64(n), false)
	}
	return n, err
}

func (sc *statsConn) Write(b []byte) (int, error) {
	// Check rate limit for upload with exponential backoff
	if sc.rateLimiter != nil && !sc.rateLimiter.Allow(sc.srcIP, len(b), true) {
		// Use context-based wait instead of blocking sleep
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = sc.rateLimiter.Wait(ctx, sc.srcIP, len(b), true)
	}

	n, err := sc.Conn.Write(b)
	if n > 0 && sc.statsStore != nil {
		sc.statsStore.RecordTraffic(sc.dstIP, sc.srcIP, uint64(n), true)
	}
	return n, err
}

// statsPacketConn wraps net.PacketConn to track traffic and enforce rate limits
type statsPacketConn struct {
	net.PacketConn
	statsStore  *stats.Store
	rateLimiter *ratelimit.RateLimiter
	metadata    *M.Metadata
	srcIP       string // Cached to avoid repeated String() calls
	dstIP       string
}

func (spc *statsPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	// Check rate limit for download with context-based wait
	if spc.rateLimiter != nil && !spc.rateLimiter.Allow(spc.srcIP, len(p), false) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = spc.rateLimiter.Wait(ctx, spc.srcIP, len(p), false)
	}

	n, addr, err := spc.PacketConn.ReadFrom(p)
	if n > 0 && spc.statsStore != nil {
		spc.statsStore.RecordTraffic(spc.dstIP, spc.srcIP, uint64(n), false)
	}
	return n, addr, err
}

func (spc *statsPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	// Check rate limit for upload with context-based wait
	if spc.rateLimiter != nil && !spc.rateLimiter.Allow(spc.srcIP, len(p), true) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = spc.rateLimiter.Wait(ctx, spc.srcIP, len(p), true)
	}

	n, err := spc.PacketConn.WriteTo(p, addr)
	if n > 0 && spc.statsStore != nil {
		spc.statsStore.RecordTraffic(spc.dstIP, spc.srcIP, uint64(n), true)
	}
	return n, err
}
