# go-pcap2socks Documentation (English)

## Overview

**go-pcap2socks** is a powerful proxy tool that redirects network traffic from any device to a SOCKS5 proxy. It functions like a router, allowing you to connect various devices to a SOCKS5 proxy server.

### Supported Devices

- **Gaming Consoles**: Xbox, PlayStation (PS4, PS5), Nintendo Switch
- **Mobile Devices**: iOS, Android phones and tablets
- **Computers**: Windows, macOS, Linux
- **Other Devices**: Printers, Smart TVs, IoT devices

### Key Features

- 🎮 Route traffic from any device through SOCKS5 proxy
- 🔧 Flexible routing rules based on IP, port, and protocol
- 🌐 DNS resolution support with multiple servers
- 📝 Comprehensive logging and debugging capabilities
- 🚀 High-performance TCP/IP stack (powered by gVisor)
- ⚙️ Easy configuration via JSON file

## Quick Links

- [Installation Guide](INSTALL.md) - Step-by-step installation instructions
- [Configuration Guide](CONFIG.md) - Detailed configuration options
- [Usage Guide](USAGE.md) - How to use go-pcap2socks effectively

## Quick Start

```bash
# Install
go install github.com/DaniilSokolyuk/go-pcap2socks@latest

# Configure (opens config in editor)
go-pcap2socks config

# Run (requires administrator privileges)
sudo go-pcap2socks
```

## Basic Configuration Example

```json
{
  "pcap": {
    "network": "172.26.0.0/16",
    "localIP": "172.26.0.1"
  },
  "dns": {
    "servers": [
      { "address": "local" }
    ]
  },
  "routing": {
    "rules": [
      {
        "dstPort": "53",
        "outboundTag": "dns-out"
      }
    ]
  },
  "outbounds": [
    {
      "tag": "",
      "direct": {}
    },
    {
      "tag": "dns-out",
      "dns": {}
    }
  ]
}
```

## Network Setup

When you start go-pcap2socks, it displays network configuration that you need to apply on your device:

```
Configure your device with these network settings:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  IP Address:     172.26.0.2 - 172.26.255.254
  Subnet Mask:    255.255.0.0
  Gateway:        172.26.0.1
  MTU:            1486 (or lower)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Device Configuration Steps

1. **Open Network Settings** on your device
2. **Set Static IP** in the range shown (excluding gateway IP)
3. **Set Subnet Mask** as displayed
4. **Set Gateway** to the localIP from config
5. **Set MTU** if required (especially for Nintendo Switch and PS5)

## Troubleshooting

### Permission Issues

- **Windows**: Run as Administrator (right-click → Run as Administrator)
- **macOS/Linux**: Use `sudo go-pcap2socks`

### Common Problems

1. **"Permission denied" error**
   - Solution: Run with administrator/root privileges

2. **Device cannot connect**
   - Check firewall settings
   - Verify IP configuration on device
   - Ensure go-pcap2socks is running

3. **Slow connection**
   - Check your SOCKS5 proxy speed
   - Reduce MTU value
   - Check network congestion

### Game Console Setup

For **Nintendo Switch** and **PS5**:
- You must manually set the MTU value in network settings
- The required MTU is displayed when go-pcap2socks starts
- Use the exact value shown or lower

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Device    │────▶│ go-pcap2socks│────▶│  SOCKS5     │
│ (XBOX, PS5, │     │  (Router)    │     │   Proxy     │
│  Phone, etc)│     │              │     │             │
└─────────────┘     └──────────────┘     └─────────────┘
```

## Credits

- **gVisor** - TCP/IP stack (https://github.com/google/gvisor)
- **pcap2socks** - Original idea (https://github.com/zhxie/pcap2socks)
- **tun2socks** - SOCKS5 client (https://github.com/xjasonlyu/tun2socks)
- **sing-box** - Full Cone NAT (https://github.com/SagerNet/sing-box)

## License

See [LICENSE](../../LICENSE)

## Support

For issues and feature requests, please visit the GitHub repository.
