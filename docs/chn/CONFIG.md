# 配置指南

本文档描述 go-pcap2socks 的所有配置选项。配置使用 JSON 格式，存储在 `config.json` 文件中。

## 配置文件位置

- **默认**: 与可执行文件相同的目录
- **自定义**: 作为参数传递：`go-pcap2socks /path/to/config.json`

## 打开配置编辑器

```bash
go-pcap2socks config
```

此命令将：
- 如果配置不存在则创建
- 在系统默认编辑器中打开：
  - **Windows**: Notepad
  - **macOS**: 默认文本编辑器
  - **Linux**: `$EDITOR` 或 `$VISUAL` 环境变量，或 nano/vim/vi

## 配置结构

```json
{
  "executeOnStart": [],
  "pcap": { ... },
  "dns": { ... },
  "routing": { ... },
  "outbounds": [ ... ],
  "capture": { ... },
  "language": "zh"
}
```

---

## executeOnStart

应用程序启动时要执行的命令。适用于启动 VPN 连接或其他前置要求。

### 语法

```json
"executeOnStart": [
  "command1",
  "command2 arg1 arg2",
  "command3"
]
```

### 示例

```json
"executeOnStart": [
  "echo '正在启动 go-pcap2socks 和 VPN'",
  "sing-box run -c /etc/sing-box/config.json"
]
```

### 字段

| 字段 | 类型 | 必需 | 默认值 | 描述 |
|------|------|------|--------|------|
| 项目 | 字符串数组 | 否 | `[]` | 每个字符串是一个 shell 命令 |

### 注意

- 命令按顺序执行
- 命令在后台运行
- 错误会被记录但不会阻止启动

---

## pcap

控制数据包捕获接口和虚拟网络配置。

### 语法

```json
"pcap": {
  "interfaceGateway": "en0",
  "mtu": 1500,
  "network": "172.26.0.0/16",
  "localIP": "172.26.0.1",
  "localMAC": "aa:bb:cc:dd:ee:ff"
}
```

### 字段

| 字段 | 类型 | 必需 | 默认值 | 描述 |
|------|------|------|--------|------|
| interfaceGateway | 字符串 | 否 | 自动 | 用作网关的物理网络接口 |
| mtu | 数字 | 否 | 1500 | 最大传输单元大小 |
| network | 字符串 (CIDR) | **是** | - | 虚拟网络地址范围 |
| localIP | 字符串 (IP) | **是** | - | 虚拟网络内的本地 IP 地址 |
| localMAC | 字符串 (MAC) | 否 | 自动 | 虚拟接口的 MAC 地址 |

### 字段详情

#### interfaceGateway

- **类型**: 字符串
- **格式**: 接口名称（如 "en0", "eth0"）或 IP 地址
- **默认**: 自动检测默认网关
- **示例**: `"en0"`, `"192.168.1.1"`

#### mtu

- **类型**: 数字
- **默认**: 1500
- **推荐**: 1486 或更低（考虑以太网开销）
- **注意**: 游戏机可能需要特定的 MTU 值

#### network

- **类型**: 字符串
- **格式**: CIDR 表示法
- **示例**: `"172.26.0.0/16"`, `"192.168.100.0/24"`
- **注意**: 必须包含 localIP

#### localIP

- **类型**: 字符串
- **格式**: IPv4 地址
- **示例**: `"172.26.0.1"`
- **注意**: 必须在网络范围内

#### localMAC

- **类型**: 字符串
- **格式**: `aa:bb:cc:dd:ee:ff`
- **默认**: 物理接口 MAC
- **示例**: `"00:1a:2b:3c:4d:5e"`

### 配置示例

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

配置用于域名解析的 DNS 服务器。

### 语法

```json
"dns": {
  "servers": [
    { "address": "local" },
    { "address": "8.8.8.8" },
    { "address": "1.1.1.1" }
  ]
}
```

### 字段

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| servers | 对象数组 | **是** | DNS 服务器列表 |

### Server 对象字段

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| address | 字符串 | **是** | DNS 服务器地址 |

#### address 值

- `"local"` - 使用系统 DNS
- IP 地址 - 特定 DNS 服务器（如 `"8.8.8.8"`, `"1.1.1.1"`）

### 示例

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

定义将流量路由到不同出站连接的规则。

### 语法

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

### 字段

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| rules | 对象数组 | **是** | 有序的路由规则列表 |

### Rule 对象字段

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| srcPort | 字符串 | 否 | 要匹配的源端口 |
| dstPort | 字符串 | 否 | 要匹配的目标端口 |
| srcIP | 字符串数组 | 否 | 源 IP 地址/CIDR |
| dstIP | 字符串数组 | 否 | 目标 IP 地址/CIDR |
| outboundTag | 字符串 | **是** | 要使用的出站连接标签 |

### 端口格式

- **单个端口**: `"80"`
- **多个端口**: `"80,443,8080"`
- **端口范围**: `"1000-2000"`
- **组合**: `"80,443,1000-2000,3000"`

### IP 格式

- **单个 IP**: `["192.168.1.1"]`（自动视为 /32）
- **CIDR**: `["192.168.0.0/24"]`
- **多个**: `["192.168.0.0/24", "10.0.0.0/8"]`

### 规则匹配

- 按顺序评估规则
- **第一个匹配获胜**
- 如果没有规则匹配，使用默认出站连接（空标签 `""`）

### 规则示例

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

定义传出流量的处理程序。每个出站连接都有一个用于路由的标签。

### 公共字段

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| tag | 字符串 | **是** | 路由标识符 |

**重要**: 空标签 `""` 表示**默认出站连接**，用于不匹配的流量。

### 出站连接类型

#### Direct

直接将流量路由到互联网，不使用代理。

```json
{
  "tag": "direct",
  "direct": {}
}
```

#### SOCKS

通过 SOCKS4/SOCKS5 代理路由流量。

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

**字段:**

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| address | 字符串 | **是** | 代理服务器地址：端口 |
| username | 字符串 | 否 | SOCKS5 认证用户名 |
| password | 字符串 | 否 | SOCKS5 认证密码 |

#### DNS

DNS 查询的特殊处理程序。

```json
{
  "tag": "dns-out",
  "dns": {}
}
```

#### Reject

阻止匹配的流量。

```json
{
  "tag": "block",
  "reject": {}
}
```

### 出站连接示例

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

用于将数据包捕获到 PCAP 文件的调试功能。

### 语法

```json
"capture": {
  "enabled": true,
  "outputFile": "capture-debug.pcap"
}
```

### 字段

| 字段 | 类型 | 必需 | 默认值 | 描述 |
|------|------|------|--------|------|
| enabled | 布尔值 | 否 | `false` | 启用/禁用数据包捕获 |
| outputFile | 字符串 | 否 | 自动 | 保存捕获数据包的目录 |

### 注意

- 捕获所有以太网帧（包括 MAC 地址）
- 如果未指定，时间戳会自动附加到 outputFile
- 适用于排除网络故障

---

## 完整示例

### 最小配置

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

### 全部流量使用 SOCKS 代理

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

### 带 VPN 的复杂路由

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

## 环境变量

| 变量 | 值 | 描述 |
|------|-----|------|
| `SLOG_LEVEL` | `debug`, `info`, `warn`, `error` | 设置日志级别 |
| `PCAP2SOCKS_LANG` | `en`, `ru`, `zh` | 设置语言 |

### 示例

```bash
SLOG_LEVEL=debug PCAP2SOCKS_LANG=zh sudo go-pcap2socks
```

---

## 验证

编辑配置后，验证 JSON 语法：

```bash
# 使用 jq
jq . config.json

# 使用 python
python -m json.tool config.json
```

## 后续步骤

1. 配置您的设置
2. 运行 `go-pcap2socks config` 进行编辑
3. 启动 `sudo go-pcap2socks`
4. 按照显示配置设备网络设置
5. 测试连接

使用示例请参见 [使用指南](USAGE.md)。
