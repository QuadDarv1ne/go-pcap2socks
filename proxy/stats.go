package proxy

import (
	"context"
	"net"

	M "github.com/DaniilSokolyuk/go-pcap2socks/md"
	"github.com/DaniilSokolyuk/go-pcap2socks/stats"
)

// StatsProxy wraps a Proxy to record traffic statistics
type StatsProxy struct {
	proxy       Proxy
	statsStore  *stats.Store
	enabled     bool
}

// NewStatsProxy creates a new proxy with statistics tracking
func NewStatsProxy(proxy Proxy, store *stats.Store) *StatsProxy {
	return &StatsProxy{
		proxy:      proxy,
		statsStore: store,
		enabled:    true,
	}
}

func (sp *StatsProxy) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	conn, err := sp.proxy.DialContext(ctx, metadata)
	if err != nil {
		return nil, err
	}

	if sp.enabled && sp.statsStore != nil {
		return &statsConn{
			Conn:       conn,
			statsStore: sp.statsStore,
			metadata:   metadata,
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
		return &statsPacketConn{
			PacketConn: conn,
			statsStore: sp.statsStore,
			metadata:   metadata,
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

// statsConn wraps net.Conn to track traffic
type statsConn struct {
	net.Conn
	statsStore *stats.Store
	metadata   *M.Metadata
}

func (sc *statsConn) Read(b []byte) (int, error) {
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

// statsPacketConn wraps net.PacketConn to track traffic
type statsPacketConn struct {
	net.PacketConn
	statsStore *stats.Store
	metadata   *M.Metadata
}

func (spc *statsPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
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
