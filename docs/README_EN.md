# 🚀 go-pcap2socks

**Transparent Network Proxy with DHCP Server for Windows**

[![Release](https://img.shields.io/github/v/release/QuadDarv1ne/go-pcap2socks)](https://github.com/QuadDarv1ne/go-pcap2socks/releases)
[![License](https://img.shields.io/github/license/QuadDarv1ne/go-pcap2socks)](LICENSE)
[![Go Report](https://goreportcard.com/badge/github.com/QuadDarv1ne/go-pcap2socks)](https://goreportcard.com/report/github.com/QuadDarv1ne/go-pcap2socks)

go-pcap2socks is a transparent network proxy for Windows that intercepts traffic from devices on your local network and routes it through SOCKS5/HTTP/HTTP3 proxies or directly.

## ✨ Features

| Feature | Description |
|---------|-------------|
| 🖧 **DHCP Server** | Automatic IP address assignment |
| 🔀 **Multi-Protocol** | SOCKS5, HTTP, HTTP/3 (QUIC) |
| ⚖️ **Load Balancing** | Failover, round-robin, least-load |
| 📋 **Flexible Routing** | Rules by IP, ports, protocols |
| 🌐 **Web Interface** | Monitoring and management |
| 🤖 **API Access** | REST API integration |
| 📱 **Telegram Bot** | Remote control |
| 💬 **Discord Alerts** | Notifications |
| 🎮 **UPnP** | Auto port forwarding |
| 🔒 **MAC Filtering** | Access control |

## 📋 Requirements

- Windows 10/11
- Administrator rights
- [Npcap](https://npcap.com) (with WinPcap API-compatible mode)

## 🚀 Quick Start

```powershell
# Automatic configuration
.\go-pcap2socks.exe auto-start

# Or manual
.\go-pcap2socks.exe auto-config
.\go-pcap2socks.exe
```

Open http://localhost:8080 for the web interface.

## ⚙️ Configuration

```json
{
  "pcap": {
    "interfaceGateway": "192.168.137.1",
    "network": "192.168.137.0/24",
    "mtu": 1486
  },
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.137.100",
    "poolEnd": "192.168.137.200",
    "leaseDuration": 86400
  },
  "outbounds": [
    {"tag": "", "direct": {}},
    {"tag": "proxy", "socks": {"address": "proxy.example.com:1080"}}
  ],
  "routing": {
    "rules": [
      {"dstPort": "443", "outboundTag": "proxy"}
    ]
  }
}
```

## 📊 Performance

```
Router Match:         ~8 ns/op     0 B/op    0 allocs/op
Router DialContext:   ~170 ns/op   40 B/op   2 allocs/op
Router Cache Hit:     ~245 ns/op   40 B/op   2 allocs/op
```

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## 📄 License

MIT License - see [LICENSE](LICENSE) file.
