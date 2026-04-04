// Package core provides connection tracking for TCP/UDP sessions.
package core

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/buffer"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
)

// ConnMeta хранит метаинформацию о перехваченном соединении
type ConnMeta struct {
	SourceIP   netip.Addr
	SourcePort uint16
	DestIP     netip.Addr
	DestPort   uint16
	Protocol   uint8  // 6 = TCP, 17 = UDP
	Domain     string // Если был перехвачен DNS запрос, тут будет домен
}

func (m ConnMeta) String() string {
	return fmt.Sprintf("%s:%d -> %s:%d (%s)", m.SourceIP, m.SourcePort, m.DestIP, m.DestPort, m.Domain)
}

// TCPConn represents an active TCP proxy connection
type TCPConn struct {
	Meta      ConnMeta
	ProxyConn net.Conn // Реальное соединение с SOCKS5 сервером
	proxyMu   sync.Mutex

	// Управление жизненным циклом горутин
	ctx    context.Context
	cancel context.CancelFunc

	// Каналы для общения между gVisor и proxy-модулем
	ToProxy   chan []byte // gVisor -> proxy
	FromProxy chan []byte // proxy -> gVisor

	closeOnce sync.Once
	closed    atomic.Bool

	// Statistics
	bytesSent     atomic.Uint64
	bytesReceived atomic.Uint64
	lastActivity  atomic.Int64 // Unix timestamp
}

// UDPConn represents an active UDP proxy session
type UDPConn struct {
	Meta      ConnMeta
	ProxyConn net.PacketConn // UDP association с SOCKS5

	// Управление жизненным циклом
	ctx    context.Context
	cancel context.CancelFunc

	// Каналы для обмена пакетами
	ToProxy   chan []byte // gVisor -> proxy
	FromProxy chan []byte // proxy -> gVisor

	closeOnce sync.Once
	closed    atomic.Bool

	// Statistics
	packetsSent     atomic.Uint64
	packetsReceived atomic.Uint64
	bytesSent       atomic.Uint64
	bytesReceived   atomic.Uint64
	lastActivity    atomic.Int64
}

// ConnTracker управляет всеми активными соединениями
type ConnTracker struct {
	mu       sync.RWMutex
	tcpConns map[string]*TCPConn
	udpConns map[string]*UDPConn

	// Proxy dialer
	proxyDialer proxy.Proxy

	// Statistics
	activeTCP  atomic.Int32
	activeUDP  atomic.Int32
	totalTCP   atomic.Uint64
	totalUDP   atomic.Uint64
	droppedTCP atomic.Uint64
	droppedUDP atomic.Uint64

	logger *slog.Logger
}

// ConnTrackerConfig holds configuration for ConnTracker
type ConnTrackerConfig struct {
	ProxyDialer    proxy.Proxy
	Logger         *slog.Logger
	MaxTCPSessions int
	MaxUDPSessions int
}

// NewConnTracker creates a new connection tracker
func NewConnTracker(cfg ConnTrackerConfig) *ConnTracker {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &ConnTracker{
		tcpConns:    make(map[string]*TCPConn),
		udpConns:    make(map[string]*UDPConn),
		proxyDialer: cfg.ProxyDialer,
		logger:      logger,
	}
}

func (ct *ConnTracker) tcpKey(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16) string {
	return fmt.Sprintf("%s:%d-%s:%d", srcIP, srcPort, dstIP, dstPort)
}

func (ct *ConnTracker) udpKey(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16) string {
	return fmt.Sprintf("udp:%s:%d-%s:%d", srcIP, srcPort, dstIP, dstPort)
}

// GetTCPStats returns TCP connection statistics
func (ct *ConnTracker) GetTCPStats() (active int32, total, dropped uint64) {
	return ct.activeTCP.Load(), ct.totalTCP.Load(), ct.droppedTCP.Load()
}

// GetUDPStats returns UDP session statistics
func (ct *ConnTracker) GetUDPStats() (active int32, total, dropped uint64) {
	return ct.activeUDP.Load(), ct.totalUDP.Load(), ct.droppedUDP.Load()
}

// CreateTCP создает новую TCP запись, устанавливает SOCKS5 соединение и запускает реле
func (ct *ConnTracker) CreateTCP(parentCtx context.Context, meta ConnMeta) (*TCPConn, error) {
	k := ct.tcpKey(meta.SourceIP, meta.SourcePort, meta.DestIP, meta.DestPort)

	ct.mu.Lock()
	// Check if connection already exists
	if _, exists := ct.tcpConns[k]; exists {
		ct.mu.Unlock()
		ct.droppedTCP.Add(1)
		return nil, fmt.Errorf("connection already tracked")
	}

	ctx, cancel := context.WithCancel(parentCtx)

	tc := &TCPConn{
		Meta:         meta,
		ctx:          ctx,
		cancel:       cancel,
		ToProxy:      make(chan []byte, 128), // Optimized buffer: 128 packets
		FromProxy:    make(chan []byte, 128), // Optimized buffer: 128 packets
		lastActivity: atomic.Int64{},
	}
	tc.lastActivity.Store(time.Now().Unix())

	ct.tcpConns[k] = tc
	ct.activeTCP.Add(1)
	ct.totalTCP.Add(1)
	ct.mu.Unlock()

	ct.logger.Info("TCP connection created", "conn", meta.String())

	// Запускаем worker'ов в фоне
	go ct.relayToProxy(tc)
	go ct.relayFromProxy(tc)

	return tc, nil
}

// GetTCP возвращает существующее TCP соединение
func (ct *ConnTracker) GetTCP(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16) (*TCPConn, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	tc, ok := ct.tcpConns[ct.tcpKey(srcIP, srcPort, dstIP, dstPort)]
	return tc, ok
}

// RemoveTCP безопасно закрывает TCP соединение и удаляет из мапы
func (ct *ConnTracker) RemoveTCP(tc *TCPConn) {
	// Quick check to avoid deadlock if already closed
	if tc.closed.Load() {
		return
	}

	k := ct.tcpKey(tc.Meta.SourceIP, tc.Meta.SourcePort, tc.Meta.DestIP, tc.Meta.DestPort)

	tc.closeOnce.Do(func() {
		tc.closed.Store(true)
		tc.cancel() // Останавливаем все горутины этого соединения

		// Close proxy connection first (with mutex protection)
		tc.proxyMu.Lock()
		if tc.ProxyConn != nil {
			tc.ProxyConn.Close()
			tc.ProxyConn = nil
		}
		tc.proxyMu.Unlock()

		// Drain buffered packets and return to pool
		drainChannel(tc.ToProxy)
		drainChannel(tc.FromProxy)

		// Close channels
		close(tc.ToProxy)
		close(tc.FromProxy)

		// Remove from map with minimal lock hold time
		ct.mu.Lock()
		delete(ct.tcpConns, k)
		ct.mu.Unlock()

		ct.activeTCP.Add(-1)
		ct.logger.Info("TCP connection closed", "conn", tc.Meta.String(),
			"bytes_sent", tc.bytesSent.Load(),
			"bytes_received", tc.bytesReceived.Load())
	})
}

// drainChannel drains a channel and returns buffers to pool
func drainChannel(ch <-chan []byte) {
	for {
		select {
		case data := <-ch:
			buffer.Put(data)
		default:
			return
		}
	}
}

// CreateUDP создает новую UDP сессию
func (ct *ConnTracker) CreateUDP(parentCtx context.Context, meta ConnMeta) (*UDPConn, error) {
	k := ct.udpKey(meta.SourceIP, meta.SourcePort, meta.DestIP, meta.DestPort)

	ct.mu.Lock()
	// Check if session already exists
	if _, exists := ct.udpConns[k]; exists {
		ct.mu.Unlock()
		ct.droppedUDP.Add(1)
		return nil, fmt.Errorf("UDP session already tracked")
	}

	ctx, cancel := context.WithCancel(parentCtx)

	uc := &UDPConn{
		Meta:         meta,
		ctx:          ctx,
		cancel:       cancel,
		ToProxy:      make(chan []byte, 256), // gVisor -> proxy
		FromProxy:    make(chan []byte, 256), // proxy -> gVisor
		lastActivity: atomic.Int64{},
	}
	uc.lastActivity.Store(time.Now().Unix())

	ct.udpConns[k] = uc
	ct.activeUDP.Add(1)
	ct.totalUDP.Add(1)
	ct.mu.Unlock()

	ct.logger.Info("UDP session created", "conn", meta.String())

	// Запускаем worker для отправки пакетов в proxy
	go ct.relayUDPPackets(uc)

	return uc, nil
}

// GetUDP возвращает существующую UDP сессию
func (ct *ConnTracker) GetUDP(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16) (*UDPConn, bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	uc, ok := ct.udpConns[ct.udpKey(srcIP, srcPort, dstIP, dstPort)]
	return uc, ok
}

// RemoveUDP безопасно закрывает UDP сессию и удаляет из мапы
func (ct *ConnTracker) RemoveUDP(uc *UDPConn) {
	k := ct.udpKey(uc.Meta.SourceIP, uc.Meta.SourcePort, uc.Meta.DestIP, uc.Meta.DestPort)

	uc.closeOnce.Do(func() {
		uc.closed.Store(true)
		uc.cancel()

		// Drain buffered packets and return to pool
		drainChannel(uc.ToProxy)
		drainChannel(uc.FromProxy)

		// Close channels
		close(uc.ToProxy)
		close(uc.FromProxy)

		// Close proxy connection
		if uc.ProxyConn != nil {
			uc.ProxyConn.Close()
		}

		ct.mu.Lock()
		delete(ct.udpConns, k)
		ct.mu.Unlock()

		ct.activeUDP.Add(-1)
		ct.logger.Info("UDP session closed", "conn", uc.Meta.String(),
			"packets_sent", uc.packetsSent.Load(),
			"packets_received", uc.packetsReceived.Load(),
			"bytes_sent", uc.bytesSent.Load(),
			"bytes_received", uc.bytesReceived.Load())
	})
}

// CloseAll вызывается при выключении программы (Ctrl+C)
func (ct *ConnTracker) CloseAll() {
	// Copy all connections under lock to avoid deadlock
	ct.mu.Lock()
	tcpConns := make([]*TCPConn, 0, len(ct.tcpConns))
	for _, tc := range ct.tcpConns {
		tcpConns = append(tcpConns, tc)
	}
	udpConns := make([]*UDPConn, 0, len(ct.udpConns))
	for _, uc := range ct.udpConns {
		udpConns = append(udpConns, uc)
	}
	ct.mu.Unlock()

	// Close all connections outside the lock
	for _, tc := range tcpConns {
		ct.RemoveTCP(tc)
	}
	for _, uc := range udpConns {
		ct.RemoveUDP(uc)
	}

	ct.logger.Info("All connections closed",
		"tcp_total", ct.totalTCP.Load(),
		"udp_total", ct.totalUDP.Load())
}

// Stop gracefully stops all connections with context-based timeout
// This ensures all active connections are properly closed before exit
func (ct *ConnTracker) Stop(ctx context.Context) error {
	ct.logger.Info("Stopping connection tracker...", "active_tcp", ct.activeTCP.Load(), "active_udp", ct.activeUDP.Load())

	ct.mu.Lock()
	defer ct.mu.Unlock()

	// Close all TCP connections gracefully
	for _, tc := range ct.tcpConns {
		select {
		case <-ctx.Done():
			ct.logger.Warn("Connection tracker stop timeout, forcing close")
			goto forceClose
		default:
		}
		ct.RemoveTCP(tc)
	}

	// Close all UDP connections gracefully
	for _, uc := range ct.udpConns {
		select {
		case <-ctx.Done():
			ct.logger.Warn("Connection tracker stop timeout, forcing close")
			goto forceClose
		default:
		}
		ct.RemoveUDP(uc)
	}

forceClose:
	ct.logger.Info("Connection tracker stopped",
		"tcp_total", ct.totalTCP.Load(),
		"udp_total", ct.totalUDP.Load())

	return ctx.Err()
}

// --- TCP Relay Workers ---

// relayToProxy читает из канала ToProxy (от gVisor) и пишет в SOCKS5
func (ct *ConnTracker) relayToProxy(tc *TCPConn) {
	defer func() {
		if r := recover(); r != nil {
			ct.logger.Error("relayToProxy panic recovered", "recover", r, "conn", tc.Meta.String())
		}
		ct.RemoveTCP(tc)
	}()

	for {
		select {
		case <-tc.ctx.Done():
			return
		case payload, ok := <-tc.ToProxy:
			if !ok {
				return
			}

			// Update activity timestamp
			tc.lastActivity.Store(time.Now().Unix())

			tc.proxyMu.Lock()
			if tc.ProxyConn == nil {
				// Lazy dial on first packet
				tc.proxyMu.Unlock()
				if err := ct.dialProxy(tc); err != nil {
					buffer.Put(payload) // Return buffer to pool on error
					ct.logger.Warn("Dial proxy failed", "err", err, "conn", tc.Meta.String())
					return
				}
				tc.proxyMu.Lock()
			}
			proxyConn := tc.ProxyConn
			tc.proxyMu.Unlock()

			proxyConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			n, err := proxyConn.Write(payload)
			if err != nil {
				buffer.Put(payload) // Return buffer to pool on error
				ct.logger.Debug("Write to proxy failed", "err", err)
				return
			}
			tc.bytesSent.Add(uint64(n))

			// Return buffer to pool after successful use
			buffer.Put(payload)
		}
	}
}

// relayFromProxy читает из SOCKS5 и отправляет в канал FromProxy (в gVisor)
func (ct *ConnTracker) relayFromProxy(tc *TCPConn) {
	defer func() {
		if r := recover(); r != nil {
			ct.logger.Error("relayFromProxy panic recovered", "recover", r, "conn", tc.Meta.String())
		}
	}()

	// Use buffer pool for efficient memory management
	buf := buffer.Get(buffer.LargeBufferSize)
	defer buffer.Put(buf)

	for {
		select {
		case <-tc.ctx.Done():
			return
		default:
			tc.proxyMu.Lock()
			proxyConn := tc.ProxyConn
			tc.proxyMu.Unlock()

			if proxyConn == nil {
				// Wait for proxy connection with context-aware sleep
				select {
				case <-tc.ctx.Done():
					return
				case <-time.After(100 * time.Millisecond):
					continue
				}
			}

			proxyConn.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err := proxyConn.Read(buf)
			if err != nil {
				ct.logger.Debug("Read from proxy failed", "err", err)
				return
			}

			tc.lastActivity.Store(time.Now().Unix())
			tc.bytesReceived.Add(uint64(n))

			// Use buffer.Clone for efficient memory management
			data := buffer.Clone(buf[:n])

			select {
			case tc.FromProxy <- data:
			case <-tc.ctx.Done():
				buffer.Put(data) // Return to pool if send failed
				return
			}
		}
	}
}

// dialProxy establishes connection to proxy server with retry and exponential backoff
// Caller must hold tc.proxyMu lock
func (ct *ConnTracker) dialProxy(tc *TCPConn) error {
	if tc.ProxyConn != nil {
		return nil
	}

	metadata := &M.Metadata{
		Network: M.TCP,
		SrcIP:   tc.Meta.SourceIP.AsSlice(),
		SrcPort: tc.Meta.SourcePort,
		DstIP:   tc.Meta.DestIP.AsSlice(),
		DstPort: tc.Meta.DestPort,
	}

	// Retry with exponential backoff
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(tc.ctx, 10*time.Second)

		conn, err := ct.proxyDialer.DialContext(ctx, metadata)

		if err == nil {
			cancel() // Success - cancel context
			tc.ProxyConn = conn
			ct.logger.Debug("Proxy connection established", "conn", tc.Meta.String(), "attempt", attempt+1)
			return nil
		}

		cancel() // Error - cancel context
		lastErr = err

		// Don't retry on context cancellation
		if tc.ctx.Err() != nil {
			return fmt.Errorf("proxy dial cancelled: %w", tc.ctx.Err())
		}

		// Exponential backoff: 100ms, 200ms, 400ms
		if attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<uint(attempt))
			ct.logger.Debug("Proxy dial failed, retrying",
				"conn", tc.Meta.String(),
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"delay", delay,
				"err", err)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("proxy dial failed after %d attempts: %w", maxRetries, lastErr)
}

// --- UDP Relay Workers ---

// relayUDPPackets отправляет UDP пакеты в proxy
func (ct *ConnTracker) relayUDPPackets(uc *UDPConn) {
	defer func() {
		if r := recover(); r != nil {
			ct.logger.Error("relayUDPPackets panic recovered", "recover", r, "conn", uc.Meta.String())
		}
		ct.RemoveUDP(uc)
	}()

	for {
		select {
		case <-uc.ctx.Done():
			return
		case payload, ok := <-uc.ToProxy:
			if !ok {
				return
			}

			uc.lastActivity.Store(time.Now().Unix())
			uc.packetsSent.Add(1)
			uc.bytesSent.Add(uint64(len(payload)))

			if uc.ProxyConn == nil {
				// Lazy dial UDP association
				if err := ct.dialUDPProxy(uc); err != nil {
					buffer.Put(payload) // Return buffer to pool on error
					ct.logger.Warn("Dial UDP proxy failed", "err", err, "conn", uc.Meta.String())
					return
				}
			}

			// Send packet to proxy
			addr := &net.UDPAddr{
				IP:   uc.Meta.DestIP.AsSlice(),
				Port: int(uc.Meta.DestPort),
			}
			_, err := uc.ProxyConn.WriteTo(payload, addr)
			if err != nil {
				buffer.Put(payload) // Return buffer to pool on error
				ct.logger.Warn("Write UDP to proxy failed", "err", err)
				return
			}

			// Return buffer to pool after successful use
			buffer.Put(payload)
		}
	}
}

// dialUDPProxy establishes UDP association with proxy
func (ct *ConnTracker) dialUDPProxy(uc *UDPConn) error {
	if uc.ProxyConn != nil {
		return nil
	}

	metadata := &M.Metadata{
		Network: M.UDP,
		SrcIP:   uc.Meta.SourceIP.AsSlice(),
		SrcPort: uc.Meta.SourcePort,
		DstIP:   uc.Meta.DestIP.AsSlice(),
		DstPort: uc.Meta.DestPort,
	}

	pc, err := ct.proxyDialer.DialUDP(metadata)
	if err != nil {
		return fmt.Errorf("proxy UDP dial: %w", err)
	}

	uc.ProxyConn = pc

	// Start reading from proxy
	go ct.readUDPFromProxy(uc)

	ct.logger.Debug("UDP proxy association established", "conn", uc.Meta.String())
	return nil
}

// readUDPFromProxy читает пакеты от proxy и отправляет в gVisor
func (ct *ConnTracker) readUDPFromProxy(uc *UDPConn) {
	defer func() {
		if r := recover(); r != nil {
			ct.logger.Error("readUDPFromProxy panic recovered", "recover", r, "conn", uc.Meta.String())
		}
	}()

	// Use buffer pool for efficient memory management
	buf := buffer.Get(buffer.MediumBufferSize)
	defer buffer.Put(buf)

	for {
		select {
		case <-uc.ctx.Done():
			return
		default:
			if uc.ProxyConn == nil {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			uc.ProxyConn.SetReadDeadline(time.Now().Add(120 * time.Second))
			n, _, err := uc.ProxyConn.ReadFrom(buf)
			if err != nil {
				ct.logger.Debug("Read UDP from proxy failed", "err", err)
				return
			}

			uc.lastActivity.Store(time.Now().Unix())
			uc.packetsReceived.Add(1)
			uc.bytesReceived.Add(uint64(n))

			// Use buffer.Clone for efficient memory management
			data := buffer.Clone(buf[:n])

			// Send packet to gVisor via FromProxy channel
			select {
			case uc.FromProxy <- data:
			case <-uc.ctx.Done():
				buffer.Put(data) // Return to pool if send failed
				return
			}
		}
	}
}

// GetActiveConnections returns a snapshot of active connections for monitoring
func (ct *ConnTracker) GetActiveConnections() []ConnMeta {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	metas := make([]ConnMeta, 0, len(ct.tcpConns)+len(ct.udpConns))
	for _, tc := range ct.tcpConns {
		metas = append(metas, tc.Meta)
	}
	for _, uc := range ct.udpConns {
		metas = append(metas, uc.Meta)
	}
	return metas
}

// TCPSend sends data to a TCP connection (called from gVisor TCP handler)
func (ct *ConnTracker) TCPSend(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16, data []byte) error {
	tc, ok := ct.GetTCP(srcIP, srcPort, dstIP, dstPort)
	if !ok {
		return fmt.Errorf("connection not found")
	}

	select {
	case tc.ToProxy <- data:
		return nil
	case <-tc.ctx.Done():
		return fmt.Errorf("connection closed")
	default:
		// Channel full - connection is slow
		ct.droppedTCP.Add(1)
		return fmt.Errorf("send buffer full")
	}
}

// UDPSend sends a UDP packet (called from gVisor UDP handler)
func (ct *ConnTracker) UDPSend(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16, data []byte) error {
	uc, ok := ct.GetUDP(srcIP, srcPort, dstIP, dstPort)
	if !ok {
		return fmt.Errorf("UDP session not found")
	}

	select {
	case uc.ToProxy <- data:
		return nil
	case <-uc.ctx.Done():
		return fmt.Errorf("session closed")
	default:
		ct.droppedUDP.Add(1)
		return fmt.Errorf("send buffer full")
	}
}

// ExportMetrics returns conntrack metrics for Prometheus
func (ct *ConnTracker) ExportMetrics() map[string]interface{} {
	tcpActive, tcpTotal, tcpDropped := ct.GetTCPStats()
	udpActive, udpTotal, udpDropped := ct.GetUDPStats()

	return map[string]interface{}{
		"tcp_active_sessions": tcpActive,
		"tcp_total_sessions":  tcpTotal,
		"tcp_dropped_packets": tcpDropped,
		"udp_active_sessions": udpActive,
		"udp_total_sessions":  udpTotal,
		"udp_dropped_packets": udpDropped,
		"total_active":        int(tcpActive) + int(udpActive),
		// Extended metrics for better observability
		"tcp_dropped_rate": float64(tcpDropped) / float64(max64(tcpTotal, 1)),
		"udp_dropped_rate": float64(udpDropped) / float64(max64(udpTotal, 1)),
		"health_score":     calculateHealthScore(int(tcpActive), int(tcpTotal), int(tcpDropped), int(udpActive), int(udpTotal), int(udpDropped)),
	}
}

// calculateHealthScore calculates a health score from 0.0 to 1.0
func calculateHealthScore(tcpActive, tcpTotal, tcpDropped, udpActive, udpTotal, udpDropped int) float64 {
	totalSessions := tcpTotal + udpTotal
	totalDropped := tcpDropped + udpDropped

	if totalSessions == 0 {
		return 1.0 // No sessions, considered healthy
	}

	dropRate := float64(totalDropped) / float64(totalSessions)
	// Health score: 1.0 = perfect, 0.0 = all dropped
	health := 1.0 - dropRate
	if health < 0 {
		health = 0
	}
	return health
}

// max64 returns the maximum of two uint64 values
func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
