# go-pcap2socks 文档

## 概述

**go-pcap2socks** 是一个强大的代理工具，可将任何设备的网络流量重定向到 SOCKS5 代理。它像路由器一样运行，允许您将各种设备连接到 SOCKS5 代理服务器。

### 支持的设备

- **游戏机**: Xbox、PlayStation (PS4, PS5)、Nintendo Switch
- **移动设备**: iOS、Android 手机和平板电脑
- **计算机**: Windows、macOS、Linux
- **其他设备**: 打印机、智能电视、IoT 设备

### 主要功能

- 🎮 通过 SOCKS5 代理路由任何设备的流量
- 🔧 基于 IP、端口和协议的灵活路由规则
- 🌐 支持多个 DNS 服务器
- 📝 全面的日志记录和调试功能
- 🚀 高性能 TCP/IP 协议栈（基于 gVisor）
- ⚙️ 通过 JSON 文件轻松配置

## 快速链接

- [安装指南](INSTALL.md) - 分步安装说明
- [配置指南](CONFIG.md) - 详细的配置选项
- [使用指南](USAGE.md) - 如何有效使用 go-pcap2socks

## 快速开始

```bash
# 安装
go install github.com/DaniilSokolyuk/go-pcap2socks@latest

# 配置（在编辑器中打开配置）
go-pcap2socks config

# 运行（需要管理员权限）
sudo go-pcap2socks
```

## 基本配置示例

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

## 网络设置

启动 go-pcap2socks 时，它会显示您需要在设备上应用的网络配置：

```
请使用以下网络设置配置您的设备：
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  IP 地址：172.26.0.2 - 172.26.255.254
  子网掩码：255.255.0.0
  网关：172.26.0.1
  MTU:   1486（或更低）
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### 设备配置步骤

1. **打开网络设置** 在您的设备上
2. **设置静态 IP** 从显示的范围内（不包括网关 IP）
3. **设置子网掩码** 如显示所示
4. **设置网关** 为配置中的 localIP
5. **设置 MTU** 如果需要（特别是 Nintendo Switch 和 PS5）

## 故障排除

### 权限问题

- **Windows**: 以管理员身份运行（右键点击 → 以管理员身份运行）
- **macOS/Linux**: 使用 `sudo go-pcap2socks`

### 常见问题

1. **"Permission denied" 错误**
   - 解决方案：以管理员/root 权限运行

2. **设备无法连接**
   - 检查防火墙设置
   - 验证设备上的 IP 配置
   - 确保 go-pcap2socks 正在运行

3. **连接缓慢**
   - 检查您的 SOCKS5 代理速度
   - 降低 MTU 值
   - 检查网络拥塞情况

### 游戏机设置

对于 **Nintendo Switch** 和 **PS5**：
- 您必须在网络设置中手动设置 MTU 值
- go-pcap2socks 启动时会显示所需的 MTU 值
- 使用显示的确切值或更低的值

## 架构

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   设备      │────▶│ go-pcap2socks│────▶│  SOCKS5     │
│ (XBOX, PS5, │     │   (路由器)   │     │   代理      │
│  手机等)    │     │              │     │             │
└─────────────┘     └──────────────┘     └─────────────┘
```

## 致谢

- **gVisor** - TCP/IP 协议栈 (https://github.com/google/gvisor)
- **pcap2socks** - 原始创意 (https://github.com/zhxie/pcap2socks)
- **tun2socks** - SOCKS5 客户端 (https://github.com/xjasonlyu/tun2socks)
- **sing-box** - Full Cone NAT (https://github.com/SagerNet/sing-box)

## 许可证

参见 [LICENSE](../../LICENSE)

## 支持

如有问题和功能请求，请访问 GitHub 仓库。
