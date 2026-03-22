# 安装指南

本指南涵盖 go-pcap2socks 在各平台上的安装。

## 前置要求

- **Go 1.21 或更高版本** - [下载](https://go.dev/dl/)
- **libpcap** - 各平台安装说明如下

## 从源码安装

### 最新稳定版本

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@latest
```

### 最新开发版本

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@main
```

### 特定版本

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@v1.0.0
```

## 各平台依赖

### Linux

#### Debian/Ubuntu

```bash
sudo apt update
sudo apt install libpcap-dev
```

#### Fedora/RHEL/CentOS

```bash
sudo dnf install libpcap-devel
```

#### Arch Linux/Manjaro

```bash
sudo pacman -S libpcap
```

#### Alpine Linux

```bash
sudo apk add libpcap-dev
```

### macOS

使用 Homebrew：

```bash
brew install libpcap
```

### Windows

1. 下载 [Npcap](https://npcap.com/#download)
2. 运行安装程序
3. **重要**: 安装时启用 "WinPcap API-compatible Mode"
4. 如提示，请重启计算机

### FreeBSD

```bash
pkg install libpcap
```

### Termux (Android)

**注意:** 需要已 root 的 Android 设备。

```bash
# 更新软件包列表
pkg update

# 启用 root 仓库
pkg install root-repo

# 安装依赖
pkg install golang libpcap tsu

# 安装 go-pcap2socks
go install github.com/DaniilSokolyuk/go-pcap2socks@latest

# 以 root 权限运行（需要已 root 的设备）
sudo $HOME/go/bin/go-pcap2socks

# 替代方案：使用 tsu 以获得更好的 root 支持
tsu -c "$HOME/go/bin/go-pcap2socks"
```

## 从源码编译

如果您喜欢手动编译：

```bash
# 克隆仓库
git clone https://github.com/DaniilSokolyuk/go-pcap2socks.git
cd go-pcap2socks

# 编译
go build -o go-pcap2socks

# 安装到 GOPATH/bin
go install
```

## 验证

安装后，验证是否正常工作：

```bash
# 检查二进制文件是否存在
go-pcap2socks --help

# 或检查版本（如果已实现）
go-pcap2socks version
```

## 运行

### 首次运行

```bash
# 打开配置编辑器
go-pcap2socks config

# 使用默认配置运行（需要 root/管理员权限）
sudo go-pcap2socks
```

### 使用自定义配置

```bash
sudo go-pcap2socks /path/to/config.json
```

## 二进制文件位置

`go install` 后，二进制文件位于：

- **Linux/macOS**: `$HOME/go/bin/go-pcap2socks`
- **Windows**: `%USERPROFILE%\Go\bin\go-pcap2socks.exe`

为方便使用，请添加到 PATH：

### Linux/macOS

添加到 `~/.bashrc` 或 `~/.zshrc`：

```bash
export PATH=$PATH:$HOME/go/bin
```

### Windows

1. 打开系统属性 → 环境变量
2. 将 `%USERPROFILE%\Go\bin` 添加到 PATH
3. 重启终端

## 卸载

```bash
# 删除二进制文件
rm $HOME/go/bin/go-pcap2socks  # Linux/macOS
del %USERPROFILE%\Go\bin\go-pcap2socks.exe  # Windows

# 删除配置（可选）
rm ~/.config/go-pcap2socks/config.json  # 如果适用
```

## 安装故障排除

### 安装后出现 "command not found"

1. 确保 `$HOME/go/bin` 已添加到 PATH
2. 重启终端
3. 运行 `go env GOPATH` 查找 Go 路径

### 找不到 libpcap

- **Linux**: 安装 `libpcap-dev` 或 `libpcap-devel`
- **macOS**: 通过 Homebrew 安装
- **Windows**: 安装带有 WinPcap 兼容性的 Npcap

### 编译时出现 CGO 错误

确保 CGO 已启用：

```bash
export CGO_ENABLED=1
go build
```

### 运行时出现 Permission denied

始终以管理员/root 权限运行：

- **Windows**: 以管理员身份运行
- **Linux/macOS**: 使用 `sudo`

```bash
sudo go-pcap2socks
```

## 后续步骤

安装后：

1. 阅读 [配置指南](CONFIG.md)
2. 设置配置文件
3. 配置设备的网络设置
4. 开始通过代理路由流量
