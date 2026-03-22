# Installation Guide

This guide covers installation of go-pcap2socks on various platforms.

## Prerequisites

- **Go 1.21 or later** - [Download](https://go.dev/dl/)
- **libpcap** - Platform-specific installation below

## Installing from Source

### Latest Stable Version

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@latest
```

### Latest Development Version

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@main
```

### Specific Version

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@v1.0.0
```

## Platform-Specific Dependencies

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

Using Homebrew:

```bash
brew install libpcap
```

### Windows

1. Download [Npcap](https://npcap.com/#download)
2. Run installer
3. **Important**: Enable "WinPcap API-compatible Mode" during installation
4. Restart your computer if prompted

### FreeBSD

```bash
pkg install libpcap
```

### Termux (Android)

**Note:** Requires a rooted Android device.

```bash
# Update package list
pkg update

# Enable root repository
pkg install root-repo

# Install dependencies
pkg install golang libpcap tsu

# Install go-pcap2socks
go install github.com/DaniilSokolyuk/go-pcap2socks@latest

# Run with root privileges (requires rooted device)
sudo $HOME/go/bin/go-pcap2socks

# Alternative: use tsu for better root handling
tsu -c "$HOME/go/bin/go-pcap2socks"
```

## Building from Source

If you prefer to build manually:

```bash
# Clone repository
git clone https://github.com/DaniilSokolyuk/go-pcap2socks.git
cd go-pcap2socks

# Build
go build -o go-pcap2socks

# Install to GOPATH/bin
go install
```

## Verification

After installation, verify it works:

```bash
# Check if binary exists
go-pcap2socks --help

# Or check version (if implemented)
go-pcap2socks version
```

## Running

### First Run

```bash
# Open configuration editor
go-pcap2socks config

# Run with default config (requires root/admin)
sudo go-pcap2socks
```

### With Custom Config

```bash
sudo go-pcap2socks /path/to/config.json
```

## Binary Location

After `go install`, the binary is located at:

- **Linux/macOS**: `$HOME/go/bin/go-pcap2socks`
- **Windows**: `%USERPROFILE%\Go\bin\go-pcap2socks.exe`

Add to PATH for convenience:

### Linux/macOS

Add to `~/.bashrc` or `~/.zshrc`:

```bash
export PATH=$PATH:$HOME/go/bin
```

### Windows

1. Open System Properties → Environment Variables
2. Add `%USERPROFILE%\Go\bin` to PATH
3. Restart terminal

## Uninstallation

```bash
# Remove binary
rm $HOME/go/bin/go-pcap2socks  # Linux/macOS
del %USERPROFILE%\Go\bin\go-pcap2socks.exe  # Windows

# Remove config (optional)
rm ~/.config/go-pcap2socks/config.json  # If applicable
```

## Troubleshooting Installation

### "command not found" after install

1. Ensure `$HOME/go/bin` is in PATH
2. Restart terminal
3. Run `go env GOPATH` to find Go path

### libpcap not found

- **Linux**: Install `libpcap-dev` or `libpcap-devel`
- **macOS**: Install via Homebrew
- **Windows**: Install Npcap with WinPcap compatibility

### Build fails with CGO errors

Ensure CGO is enabled:

```bash
export CGO_ENABLED=1
go build
```

### Permission denied on run

Always run with administrator/root privileges:

- **Windows**: Run as Administrator
- **Linux/macOS**: Use `sudo`

## Next Steps

After installation:

1. Read [Configuration Guide](CONFIG.md)
2. Set up your config file
3. Configure your device's network settings
4. Start routing traffic through your proxy
