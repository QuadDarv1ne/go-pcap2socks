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

type DNS struct {
	cfg           cfg.DNS
	dnsClient     *dns.Client
	interfaceName string
	dohClients    map[string]*localdns.DoHClient
	dotClients    map[string]*localdns.DoTClient
}

func (d *DNS) Addr() string {
	panic("implement me")
}

func (d *DNS) Mode() Mode {
	panic("implement me")
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

	return &DNS{
		dnsClient:     dnsClient,
		cfg:           cfg,
		interfaceName: interfaceName,
		dohClients:    dohClients,
		dotClients:    dotClients,
	}
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
		var response *dns.Msg
		var lastErr error

		for _, server := range d.cfg.Servers {
			// Handle local DNS
			if server.Address == "local" {
				localClient := localdns.NewLocalClient(d.interfaceName)
				response, lastErr = localClient.Exchange(msg)
				if lastErr == nil {
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
