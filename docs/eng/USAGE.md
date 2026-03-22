# Usage Guide

This guide explains how to use go-pcap2socks effectively with various devices and scenarios.

## Table of Contents

1. [Basic Setup](#basic-setup)
2. [Gaming Consoles](#gaming-consoles)
3. [Mobile Devices](#mobile-devices)
4. [Advanced Scenarios](#advanced-scenarios)
5. [Troubleshooting](#troubleshooting)

---

## Basic Setup

### Step 1: Install

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@latest
```

### Step 2: Configure

```bash
go-pcap2socks config
```

Edit the configuration file. Minimal example:

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
      "socks": {
        "address": "127.0.0.1:1080"
      }
    },
    {
      "tag": "dns-out",
      "dns": {}
    }
  ]
}
```

### Step 3: Run

```bash
sudo go-pcap2socks
```

### Step 4: Note Network Settings

When started, you'll see output like:

```
Configure your device with these network settings:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  IP Address:     172.26.0.2 - 172.26.255.254
  Subnet Mask:    255.255.0.0
  Gateway:        172.26.0.1
  MTU:            1486 (or lower)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Step 5: Configure Device

On your device:
1. Open Network Settings
2. Set **Static IP** (any in range, e.g., 172.26.0.100)
3. Set **Subnet Mask** (255.255.0.0)
4. Set **Gateway** (172.26.0.1)
5. Set **DNS** (same as gateway or custom)
6. Set **MTU** if required

### Step 6: Test

Connect to internet from your device and verify traffic routes through proxy.

---

## Gaming Consoles

### PlayStation 5 (PS5)

#### Network Setup

1. **Settings** → **Network** → **Settings**
2. **Set Up Internet Connection**
3. Select your connection type (Wi-Fi or LAN)
4. **Custom Setup**
5. **IP Address Settings**: Manual
   - IP Address: `172.26.0.100` (or any in range)
   - Subnet Mask: `255.255.0.0`
   - Default Gateway: `172.26.0.1`
   - Primary DNS: `172.26.0.1`
   - Secondary DNS: `8.8.8.8`
6. **MTU Settings**: Manual
   - MTU: `1486` (or value shown at startup)
7. **Proxy Server**: Do Not Use
8. **Test Internet Connection**

#### Notes

- MTU must be set manually
- Use the exact MTU value shown when go-pcap2socks starts

### Nintendo Switch

#### Network Setup

1. **System Settings** → **Internet** → **Internet Settings**
2. Select your network
3. **Change Settings**
4. **IPv4 Settings**: Manual
   - IP Address: `172.26.0.100`
   - Subnet Mask: `255.255.0.0`
   - Gateway: `172.26.0.1`
   - Primary DNS: `172.26.0.1`
   - Secondary DNS: `8.8.8.8`
5. **MTU**: Manual
   - MTU: `1486` (or value shown at startup)
6. **Proxy Server**: Do Not Use
7. **Save** → **Test Connection**

#### Notes

- MTU is critical for Switch
- Some games may require port forwarding on your SOCKS proxy

### Xbox Series X/S

#### Network Setup

1. **Settings** → **General** → **Network settings**
2. **Advanced settings**
3. **IP settings**: Manual
   - IP Address: `172.26.0.100`
   - Subnet Mask: `255.255.0.0`
   - Gateway: `172.26.0.1`
   - DNS: `172.26.0.1`
4. **Alternate MAC address**: Use default
5. **Test network connection**

---

## Mobile Devices

### iOS (iPhone/iPad)

#### Wi-Fi Setup

1. **Settings** → **Wi-Fi**
2. Tap ⓘ next to your network
3. **Configure IP**: Manual
   - IP Address: `172.26.0.100`
   - Subnet Mask: `255.255.0.0`
   - Router: `172.26.0.1`
4. **Configure DNS**: Manual
   - DNS Server: `172.26.0.1`
5. **Save**

### Android

#### Wi-Fi Setup (varies by device)

1. **Settings** → **Network & Internet** → **Wi-Fi**
2. Long-press your network → **Modify network**
3. **Advanced options**
4. **IP settings**: Static
   - IP Address: `172.26.0.100`
   - Gateway: `172.26.0.1`
   - Network prefix length: `16` (for /16)
   - DNS 1: `172.26.0.1`
   - DNS 2: `8.8.8.8`
5. **Save**

---

## Advanced Scenarios

### Scenario 1: Route Only Specific Traffic Through Proxy

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
      },
      {
        "dstPort": "80,443",
        "outboundTag": "proxy"
      }
    ]
  },
  "outbounds": [
    {
      "tag": "",
      "direct": {}
    },
    {
      "tag": "proxy",
      "socks": {
        "address": "127.0.0.1:1080"
      }
    },
    {
      "tag": "dns-out",
      "dns": {}
    }
  ]
}
```

This routes only HTTP/HTTPS through proxy, everything else direct.

### Scenario 2: Block Specific Devices

```json
{
  "routing": {
    "rules": [
      {
        "dstPort": "53",
        "outboundTag": "dns-out"
      },
      {
        "srcIP": ["172.26.0.100"],
        "outboundTag": "block"
      }
    ]
  },
  "outbounds": [
    {
      "tag": "",
      "direct": {}
    },
    {
      "tag": "block",
      "reject": {}
    },
    {
      "tag": "dns-out",
      "dns": {}
    }
  ]
}
```

Blocks traffic from device with IP 172.26.0.100.

### Scenario 3: Multiple Proxies with Load Balancing

```json
{
  "routing": {
    "rules": [
      {
        "dstPort": "53",
        "outboundTag": "dns-out"
      },
      {
        "dstPort": "80",
        "outboundTag": "proxy1"
      },
      {
        "dstPort": "443",
        "outboundTag": "proxy2"
      }
    ]
  },
  "outbounds": [
    {
      "tag": "",
      "direct": {}
    },
    {
      "tag": "proxy1",
      "socks": {
        "address": "proxy1.example.com:1080"
      }
    },
    {
      "tag": "proxy2",
      "socks": {
        "address": "proxy2.example.com:1080",
        "username": "user",
        "password": "pass"
      }
    },
    {
      "tag": "dns-out",
      "dns": {}
    }
  ]
}
```

### Scenario 4: VPN Integration

```json
{
  "executeOnStart": [
    "sing-box run -c /etc/sing-box/config.json"
  ],
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
      },
      {
        "dstIP": ["192.168.0.0/16", "10.0.0.0/8"],
        "outboundTag": "direct"
      }
    ]
  },
  "outbounds": [
    {
      "tag": "",
      "socks": {
        "address": "127.0.0.1:1080"
      }
    },
    {
      "tag": "direct",
      "direct": {}
    },
    {
      "tag": "dns-out",
      "dns": {}
    }
  ]
}
```

Starts VPN before go-pcap2socks, routes local network direct.

---

## Troubleshooting

### Device Cannot Connect

**Check:**
1. go-pcap2socks is running
2. IP settings are correct on device
3. Firewall allows connections
4. Gateway IP matches config

**Solution:**
```bash
# Check if running
ps aux | grep go-pcap2socks

# Restart
sudo go-pcap2socks
```

### Slow Connection

**Possible causes:**
1. Slow SOCKS5 proxy
2. MTU too high
3. Network congestion

**Solutions:**
```json
// Reduce MTU
"pcap": {
  "mtu": 1400
}
```

- Try different proxy server
- Check network bandwidth

### DNS Resolution Fails

**Check:**
1. DNS servers are reachable
2. DNS outbound is configured

**Solution:**
```json
"dns": {
  "servers": [
    { "address": "8.8.8.8" },
    { "address": "1.1.1.1" }
  ]
}
```

### Permission Denied

**Solution:**
- **Windows**: Run as Administrator
- **Linux/macOS**: Use `sudo`

```bash
sudo go-pcap2socks
```

### Debug Mode

Enable debug logging:

```bash
SLOG_LEVEL=debug sudo go-pcap2socks
```

Enable packet capture:

```json
"capture": {
  "enabled": true,
  "outputFile": "debug.pcap"
}
```

Analyze with Wireshark:

```bash
wireshark debug.pcap
```

---

## Best Practices

1. **Use static IPs** for devices to simplify routing rules
2. **Test with minimal config** first, then add complexity
3. **Monitor logs** for issues: `SLOG_LEVEL=debug`
4. **Document your setup** for future reference
5. **Backup config** before major changes

## Additional Resources

- [Configuration Guide](CONFIG.md) - All config options
- [Installation Guide](INSTALL.md) - Platform-specific installation
- [GitHub Issues](https://github.com/DaniilSokolyuk/go-pcap2socks/issues) - Report bugs
