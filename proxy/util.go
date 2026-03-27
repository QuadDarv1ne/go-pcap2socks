package proxy

import (
	"net"
	"time"
)

const (
	tcpKeepAlivePeriod = 30 * time.Second
)

// setKeepAlive sets tcp keepalive option for tcp connection.
//go:inline
func setKeepAlive(c net.Conn) {
	if tcp, ok := c.(*net.TCPConn); ok {
		tcp.SetKeepAlive(true)
		tcp.SetKeepAlivePeriod(tcpKeepAlivePeriod)
	}
}

// setNoDelay disables TCP Nagle algorithm for lower latency (gaming optimization).
//go:inline
func setNoDelay(c net.Conn) {
	if tcp, ok := c.(*net.TCPConn); ok {
		tcp.SetNoDelay(true)
	}
}

// safeConnClose closes tcp connection safely.
//go:inline
func safeConnClose(c net.Conn, err error) {
	if c != nil && err != nil {
		c.Close()
	}
}
