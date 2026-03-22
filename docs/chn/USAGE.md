# 使用指南

本指南解释如何有效地在各种设备和场景下使用 go-pcap2socks。

## 目录

1. [基本设置](#基本设置)
2. [游戏机](#游戏机)
3. [移动设备](#移动设备)
4. [高级场景](#高级场景)
5. [故障排除](#故障排除)

---

## 基本设置

### 步骤 1: 安装

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@latest
```

### 步骤 2: 配置

```bash
go-pcap2socks config
```

编辑配置文件。最小示例：

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

### 步骤 3: 运行

```bash
sudo go-pcap2socks
```

### 步骤 4: 记录网络设置

启动时，您会看到如下输出：

```
请使用以下网络设置配置您的设备：
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  IP 地址：172.26.0.2 - 172.26.255.254
  子网掩码：255.255.0.0
  网关：172.26.0.1
  MTU:   1486（或更低）
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### 步骤 5: 配置设备

在您的设备上：
1. 打开**网络设置**
2. 设置**静态 IP**（范围内的任意值，如 172.26.0.100）
3. 设置**子网掩码**（255.255.0.0）
4. 设置**网关**（172.26.0.1）
5. 设置**DNS**（与网关相同或自定义）
6. 设置**MTU**（如果需要）

### 步骤 6: 测试

从您的设备连接到互联网，验证流量是否通过代理路由。

---

## 游戏机

### PlayStation 5 (PS5)

#### 网络设置

1. **设置** → **网络** → **设置**
2. **设置互联网连接**
3. 选择您的连接类型（Wi-Fi 或 LAN）
4. **自定义设置**
5. **IP 地址设置**: 手动
   - IP 地址：`172.26.0.100`（或范围内的任意值）
   - 子网掩码：`255.255.0.0`
   - 默认网关：`172.26.0.1`
   - 主要 DNS: `172.26.0.1`
   - 备用 DNS: `8.8.8.8`
6. **MTU 设置**: 手动
   - MTU: `1486`（或启动时显示的值）
7. **代理服务器**: 不使用
8. **测试互联网连接**

#### 注意

- 必须手动设置 MTU
- 使用 go-pcap2socks 启动时显示的确切 MTU 值

### Nintendo Switch

#### 网络设置

1. **主机设置** → **互联网** → **互联网设置**
2. 选择您的网络
3. **更改设置**
4. **IPv4 设置**: 手动
   - IP 地址：`172.26.0.100`
   - 子网掩码：`255.255.0.0`
   - 网关：`172.26.0.1`
   - 主要 DNS: `172.26.0.1`
   - 备用 DNS: `8.8.8.8`
5. **MTU**: 手动
   - MTU: `1486`（或启动时显示的值）
6. **代理服务器**: 不使用
7. **保存** → **测试连接**

#### 注意

- MTU 对 Switch 至关重要
- 某些游戏可能需要在 SOCKS 代理上进行端口转发

### Xbox Series X/S

#### 网络设置

1. **设置** → **常规** → **网络设置**
2. **高级设置**
3. **IP 设置**: 手动
   - IP 地址：`172.26.0.100`
   - 子网掩码：`255.255.0.0`
   - 网关：`172.26.0.1`
   - DNS: `172.26.0.1`
4. **备用 MAC 地址**: 使用默认
5. **测试网络连接**

---

## 移动设备

### iOS (iPhone/iPad)

#### Wi-Fi 设置

1. **设置** → **Wi-Fi**
2. 点击网络旁边的 ⓘ
3. **配置 IP**: 手动
   - IP 地址：`172.26.0.100`
   - 子网掩码：`255.255.0.0`
   - 路由器：`172.26.0.1`
4. **配置 DNS**: 手动
   - DNS 服务器：`172.26.0.1`
5. **保存**

### Android

#### Wi-Fi 设置（因设备而异）

1. **设置** → **网络和互联网** → **Wi-Fi**
2. 长按您的网络 → **修改网络**
3. **高级选项**
4. **IP 设置**: 静态
   - IP 地址：`172.26.0.100`
   - 网关：`172.26.0.1`
   - 网络前缀长度：`16`（对应 /16）
   - DNS 1: `172.26.0.1`
   - DNS 2: `8.8.8.8`
5. **保存**

---

## 高级场景

### 场景 1: 仅通过代理路由特定流量

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

这仅将 HTTP/HTTPS 通过代理路由，其他所有流量直接连接。

### 场景 2: 阻止特定设备

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

阻止来自 IP 为 172.26.0.100 的设备的流量。

### 场景 3: 多代理负载均衡

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

### 场景 4: VPN 集成

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

在 go-pcap2socks 之前启动 VPN，将本地网络直接路由。

---

## 故障排除

### 设备无法连接

**检查:**
1. go-pcap2socks 正在运行
2. 设备上的 IP 设置正确
3. 防火墙允许连接
4. 网关 IP 与配置匹配

**解决方案:**
```bash
# 检查是否运行
ps aux | grep go-pcap2socks

# 重启
sudo go-pcap2socks
```

### 连接缓慢

**可能原因:**
1. SOCKS5 代理缓慢
2. MTU 太高
3. 网络拥塞

**解决方案:**
```json
// 降低 MTU
"pcap": {
  "mtu": 1400
}
```

- 尝试不同的代理服务器
- 检查网络带宽

### DNS 解析失败

**检查:**
1. DNS 服务器可访问
2. DNS outbound 已配置

**解决方案:**
```json
"dns": {
  "servers": [
    { "address": "8.8.8.8" },
    { "address": "1.1.1.1" }
  ]
}
```

### Permission denied

**解决方案:**
- **Windows**: 以管理员身份运行
- **Linux/macOS**: 使用 `sudo`

```bash
sudo go-pcap2socks
```

### 调试模式

启用 debug 日志：

```bash
SLOG_LEVEL=debug sudo go-pcap2socks
```

启用数据包捕获：

```json
"capture": {
  "enabled": true,
  "outputFile": "debug.pcap"
}
```

使用 Wireshark 分析：

```bash
wireshark debug.pcap
```

---

## 最佳实践

1. **为设备使用静态 IP** 以简化路由规则
2. **首先使用最小配置测试**，然后增加复杂性
3. **监控日志**查找问题：`SLOG_LEVEL=debug`
4. **记录您的设置**以供将来参考
5. **在重大更改前备份配置**

## 其他资源

- [配置指南](CONFIG.md) - 所有配置选项
- [安装指南](INSTALL.md) - 各平台安装
- [GitHub Issues](https://github.com/DaniilSokolyuk/go-pcap2socks/issues) - 报告错误
