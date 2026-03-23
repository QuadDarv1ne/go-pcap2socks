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
		// Get rate limits from stats store
		uploadLimit, downloadLimit := sp.statsStore.GetRateLimit(metadata.SrcIP.String())
		if uploadLimit > 0 || downloadLimit > 0 {
			sp.rateLimiter.SetLimit(metadata.SrcIP.String(), uploadLimit, downloadLimit)
		}

		return &statsConn{
			Conn:        conn,
			statsStore:  sp.statsStore,
			rateLimiter: sp.rateLimiter,
			metadata:    metadata,
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
		// Get rate limits from stats store
		uploadLimit, downloadLimit := sp.statsStore.GetRateLimit(metadata.SrcIP.String())
		if uploadLimit > 0 || downloadLimit > 0 {
			sp.rateLimiter.SetLimit(metadata.SrcIP.String(), uploadLimit, downloadLimit)
		}

		return &statsPacketConn{
			PacketConn:  conn,
			statsStore:  sp.statsStore,
			rateLimiter: sp.rateLimiter,
			metadata:    metadata,
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
}

func (sc *statsConn) Read(b []byte) (int, error) {
	// Check rate limit for download
	if sc.rateLimiter != nil && !sc.rateLimiter.Allow(sc.metadata.SrcIP.String(), len(b), false) {
		// Rate limited - add small delay
		time.Sleep(10 * time.Millisecond)
	}

	n, err := sc.Conn.Read(b)
	if n > 0 && sc.statsStore != nil && sc.metadata != nil {
		sc.statsStore.RecordTraffic(
			sc.metadata.DstIP.String(),
			sc.metadata.SrcIP.String(),
			uint64(n),
			false, // download
		)
	}
	return n, err
}

func (sc *statsConn) Write(b []byte) (int, error) {
	// Check rate limit for upload
	if sc.rateLimiter != nil && !sc.rateLimiter.Allow(sc.metadata.SrcIP.String(), len(b), true) {
		// Rate limited - add small delay
		time.Sleep(10 * time.Millisecond)
	}

	n, err := sc.Conn.Write(b)
	if n > 0 && sc.statsStore != nil && sc.metadata != nil {
		sc.statsStore.RecordTraffic(
			sc.metadata.DstIP.String(),
			sc.metadata.SrcIP.String(),
			uint64(n),
			true, // upload
		)
	}
	return n, err
}

// statsPacketConn wraps net.PacketConn to track traffic and enforce rate limits
type statsPacketConn struct {
	net.PacketConn
	statsStore  *stats.Store
	rateLimiter *ratelimit.RateLimiter
	metadata    *M.Metadata
}

func (spc *statsPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	// Check rate limit for download
	if spc.rateLimiter != nil && !spc.rateLimiter.Allow(spc.metadata.SrcIP.String(), len(p), false) {
		// Rate limited - add small delay
		time.Sleep(10 * time.Millisecond)
	}

	n, addr, err := spc.PacketConn.ReadFrom(p)
	if n > 0 && spc.statsStore != nil && spc.metadata != nil {
		spc.statsStore.RecordTraffic(
			spc.metadata.DstIP.String(),
			spc.metadata.SrcIP.String(),
			uint64(n),
			false, // download
		)
	}
	return n, addr, err
}

func (spc *statsPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	// Check rate limit for upload
	if spc.rateLimiter != nil && !spc.rateLimiter.Allow(spc.metadata.SrcIP.String(), len(p), true) {
		// Rate limited - add small delay
		time.Sleep(10 * time.Millisecond)
	}

	n, err := spc.PacketConn.WriteTo(p, addr)
	if n > 0 && spc.statsStore != nil && spc.metadata != nil {
		spc.statsStore.RecordTraffic(
			spc.metadata.DstIP.String(),
			spc.metadata.SrcIP.String(),
			uint64(n),
			true, // upload
		)
	}
	return n, err
}
