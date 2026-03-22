# Configuration Guide

This document describes all configuration options for go-pcap2socks. Configuration uses JSON format and is stored in `config.json`.

## Configuration File Location

- **Default**: Same directory as executable
- **Custom**: Pass as argument: `go-pcap2socks /path/to/config.json`

## Opening Configuration Editor

```bash
go-pcap2socks config
```

This command:
- Creates config if it doesn't exist
- Opens in system default editor:
  - **Windows**: Notepad
  - **macOS**: Default text editor
  - **Linux**: `$EDITOR` or `$VISUAL` env var, or nano/vim/vi

## Configuration Structure

```json
{
  "executeOnStart": [],
  "pcap": { ... },
  "dns": { ... },
  "routing": { ... },
  "outbounds": [ ... ],
  "capture": { ... }
}
```

---

## executeOnStart

Commands to execute when the application starts. Useful for starting VPN connections or other prerequisites.

### Syntax

```json
"executeOnStart": [
  "command1",
  "command2 arg1 arg2",
  "command3"
]
```

### Example

```json
"executeOnStart": [
  "echo 'Starting go-pcap2socks with VPN'",
  "sing-box run -c /etc/sing-box/config.json"
]
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| Items | Array of strings | No | `[]` | Each string is a shell command |

### Notes

- Commands run in order
- Commands run in background
- Errors are logged but don't stop startup

---

## pcap

Controls packet capture interface and virtual network configuration.

### Syntax

```json
"pcap": {
  "interfaceGateway": "en0",
  "mtu": 1500,
  "network": "172.26.0.0/16",
  "localIP": "172.26.0.1",
  "localMAC": "aa:bb:cc:dd:ee:ff"
}
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| interfaceGateway | String | No | Auto | Physical network interface to use as gateway |
| mtu | Number | No | 1500 | Maximum Transmission Unit size |
| network | String (CIDR) | **Yes** | - | Virtual network address range |
| localIP | String (IP) | **Yes** | - | Local IP address within virtual network |
| localMAC | String (MAC) | No | Auto | MAC address for virtual interface |

### Field Details

#### interfaceGateway

- **Type**: String
- **Format**: Interface name (e.g., "en0", "eth0") or IP address
- **Default**: Auto-detected default gateway
- **Example**: `"en0"`, `"192.168.1.1"`

#### mtu

- **Type**: Number
- **Default**: 1500
- **Recommended**: 1486 or lower (accounts for Ethernet overhead)
- **Note**: Game consoles may require specific MTU values

#### network

- **Type**: String
- **Format**: CIDR notation
- **Example**: `"172.26.0.0/16"`, `"192.168.100.0/24"`
- **Note**: Must contain localIP

#### localIP

- **Type**: String
- **Format**: IPv4 address
- **Example**: `"172.26.0.1"`
- **Note**: Must be within network range

#### localMAC

- **Type**: String
- **Format**: `aa:bb:cc:dd:ee:ff`
- **Default**: Physical interface MAC
- **Example**: `"00:1a:2b:3c:4d:5e"`

### Example Configuration

```json
"pcap": {
  "interfaceGateway": "192.168.1.1",
  "mtu": 1486,
  "network": "172.26.0.0/16",
  "localIP": "172.26.0.1",
  "localMAC": "02:00:00:00:00:01"
}
```

---

## dns

Configures DNS servers for domain name resolution.

### Syntax

```json
"dns": {
  "servers": [
    { "address": "local" },
    { "address": "8.8.8.8" },
    { "address": "1.1.1.1" }
  ]
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| servers | Array of objects | **Yes** | List of DNS servers |

### Server Object Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| address | String | **Yes** | DNS server address |

#### address Values

- `"local"` - Use system DNS
- IP address - Specific DNS server (e.g., `"8.8.8.8"`, `"1.1.1.1"`)

### Example

```json
"dns": {
  "servers": [
    { "address": "local" },
    { "address": "8.8.8.8" },
    { "address": "208.67.222.222" }
  ]
}
```

---

## routing

Defines rules for routing traffic to different outbounds.

### Syntax

```json
"routing": {
  "rules": [
    {
      "srcPort": "1024-65535",
      "dstPort": "80,443",
      "srcIP": ["192.168.1.0/24"],
      "dstIP": ["10.0.0.0/8"],
      "outboundTag": "proxy"
    }
  ]
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| rules | Array of objects | **Yes** | Ordered list of routing rules |

### Rule Object Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| srcPort | String | No | Source port(s) to match |
| dstPort | String | No | Destination port(s) to match |
| srcIP | Array of strings | No | Source IP addresses/CIDRs |
| dstIP | Array of strings | No | Destination IP addresses/CIDRs |
| outboundTag | String | **Yes** | Tag of outbound to use |

### Port Format

- **Single port**: `"80"`
- **Multiple ports**: `"80,443,8080"`
- **Port range**: `"1000-2000"`
- **Combined**: `"80,443,1000-2000,3000"`

### IP Format

- **Single IP**: `["192.168.1.1"]` (automatically /32)
- **CIDR**: `["192.168.0.0/24"]`
- **Multiple**: `["192.168.0.0/24", "10.0.0.0/8"]`

### Rule Matching

- Rules are evaluated in order
- **First match wins**
- If no rules match, uses default outbound (empty tag `""`)

### Example Rules

```json
"routing": {
  "rules": [
    {
      "dstPort": "53",
      "outboundTag": "dns-out"
    },
    {
      "dstIP": ["192.168.0.0/16", "10.0.0.0/8"],
      "outboundTag": "direct"
    },
    {
      "dstPort": "80,443",
      "outboundTag": "proxy"
    },
    {
      "srcIP": ["172.26.0.100"],
      "outboundTag": "block"
    }
  ]
}
```

---

## outbounds

Defines handlers for outgoing traffic. Each outbound has a tag for routing.

### Common Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| tag | String | **Yes** | Identifier for routing |

**Important**: Empty tag `""` denotes the **default outbound** for unmatched traffic.

### Outbound Types

#### Direct

Routes traffic directly to internet without proxy.

```json
{
  "tag": "direct",
  "direct": {}
}
```

#### SOCKS

Routes traffic through SOCKS4/SOCKS5 proxy.

```json
{
  "tag": "proxy",
  "socks": {
    "address": "proxy.example.com:1080",
    "username": "optional-user",
    "password": "optional-pass"
  }
}
```

**Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| address | String | **Yes** | Proxy server address:port |
| username | String | No | SOCKS5 authentication username |
| password | String | No | SOCKS5 authentication password |

#### DNS

Special handler for DNS queries.

```json
{
  "tag": "dns-out",
  "dns": {}
}
```

#### Reject

Blocks matching traffic.

```json
{
  "tag": "block",
  "reject": {}
}
```

### Example Outbounds

```json
"outbounds": [
  {
    "tag": "",
    "direct": {}
  },
  {
    "tag": "proxy",
    "socks": {
      "address": "127.0.0.1:1080",
      "username": "user",
      "password": "pass"
    }
  },
  {
    "tag": "dns-out",
    "dns": {}
  },
  {
    "tag": "block",
    "reject": {}
  }
]
```

---

## capture

Debug feature for capturing packets to PCAP file.

### Syntax

```json
"capture": {
  "enabled": true,
  "outputFile": "capture-debug.pcap"
}
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| enabled | Boolean | No | `false` | Enable/disable packet capture |
| outputFile | String | No | Auto | Path to save captured packets |

### Notes

- Captures all Ethernet frames (including MAC addresses)
- Timestamp automatically appended to outputFile if not specified
- Useful for troubleshooting network issues

---

## Complete Examples

### Minimal Configuration

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

### SOCKS Proxy for All Traffic

```json
{
  "pcap": {
    "network": "172.26.0.0/16",
    "localIP": "172.26.0.1"
  },
  "dns": {
    "servers": [
      { "address": "8.8.8.8" }
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

### Complex Routing with VPN

```json
{
  "executeOnStart": [
    "sing-box run -c /etc/sing-box/config.json"
  ],
  "pcap": {
    "interfaceGateway": "en0",
    "mtu": 1486,
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
      "tag": "direct",
      "direct": {}
    },
    {
      "tag": "proxy",
      "socks": {
        "address": "127.0.0.1:1080",
        "username": "user",
        "password": "pass"
      }
    },
    {
      "tag": "dns-out",
      "dns": {}
    }
  ],
  "capture": {
    "enabled": true,
    "outputFile": "debug.pcap"
  }
}
```

---

## Environment Variables

| Variable | Values | Description |
|----------|--------|-------------|
| `SLOG_LEVEL` | `debug`, `info`, `warn`, `error` | Set logging level |

### Example

```bash
SLOG_LEVEL=debug sudo go-pcap2socks
```

---

## Validation

After editing config, validate JSON syntax:

```bash
# Using jq
jq . config.json

# Using python
python -m json.tool config.json
```

## Next Steps

1. Configure your settings
2. Run `go-pcap2socks config` to edit
3. Start with `sudo go-pcap2socks`
4. Configure device network settings as displayed
5. Test connectivity

For usage examples, see [Usage Guide](USAGE.md).
