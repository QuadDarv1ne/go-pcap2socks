//go:build windows

package tray

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/notify"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/getlantern/systray"
	"github.com/gorilla/websocket"
	"golang.org/x/sys/windows"
)

//go:embed icons/running.ico
var runningIcon embed.FS

//go:embed icons/stopped.ico
var stoppedIcon embed.FS

//go:embed icons/amber.ico
var amberIcon embed.FS

// wsUpgrader creates WebSocket connections
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// WebSocketStatusClient manages WebSocket connection for status updates
type WebSocketStatusClient struct {
	conn       *websocket.Conn
	mu         sync.Mutex
	connected  bool
	reconnect  chan struct{}
	stop       chan struct{}
	statusChan chan *APIStatus
}

// NewWebSocketStatusClient creates a new WebSocket status client
func NewWebSocketStatusClient() *WebSocketStatusClient {
	return &WebSocketStatusClient{
		connected:  false,
		reconnect:  make(chan struct{}, 1),
		stop:       make(chan struct{}),
		statusChan: make(chan *APIStatus, 10),
	}
}

// Connect establishes WebSocket connection
func (c *WebSocketStatusClient) Connect(ctx context.Context, mStatus, mDevices, mStart, mStop *systray.MenuItem) {
	go c.run(ctx, mStatus, mDevices, mStart, mStop)
}

// run manages WebSocket connection lifecycle
func (c *WebSocketStatusClient) run(ctx context.Context, mStatus, mDevices, mStart, mStop *systray.MenuItem) {
	for {
		select {
		case <-c.stop:
			return
		case <-ctx.Done():
			return
		default:
			c.connectAndListen(ctx, mStatus, mDevices, mStart, mStop)

			// Wait before reconnect
			select {
			case <-time.After(3 * time.Second):
			case <-c.stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

// connectAndListen establishes connection and listens for messages
func (c *WebSocketStatusClient) connectAndListen(ctx context.Context, mStatus, mDevices, mStart, mStop *systray.MenuItem) {
	url := fmt.Sprintf("ws://127.0.0.1:8080/ws?token=%s", apiToken)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		slog.Debug("WebSocket connect failed", "error", err)
		return
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.connected = false
		c.conn.Close()
		c.mu.Unlock()
	}()

	slog.Info("WebSocket status connected")

	// Read messages
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	for {
		select {
		case <-c.stop:
			return
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				slog.Debug("WebSocket read error", "error", err)
				return
			}

			var status APIStatus
			if err := json.Unmarshal(message, &status); err != nil {
				continue
			}

			// Update UI
			c.updateUI(&status, mStatus, mDevices, mStart, mStop)
		}
	}
}

// updateUI updates tray UI with status
func (c *WebSocketStatusClient) updateUI(status *APIStatus, mStatus, mDevices, mStart, mStop *systray.MenuItem) {
	// Update icon based on status with animation
	if status.Running {
		c.animateIconChange(runningIcon, "icons/running.ico")
		mStatus.SetTitle(fmt.Sprintf("Статус: Запущено (%s)", status.Uptime))
		mStart.Hide()
		mStop.Show()
	} else {
		c.animateIconChange(stoppedIcon, "icons/stopped.ico")
		mStatus.SetTitle("Статус: Остановлено")
		mStart.Show()
		mStop.Hide()
	}

	mDevices.SetTitle(fmt.Sprintf("📱 Устройства (%d)", status.Devices))

	tooltip := fmt.Sprintf("go-pcap2socks\n↑ Загрузка: %s\n↓ Выгрузка: %s\nУстройств: %d\nUptime: %s",
		formatBytes(status.Upload),
		formatBytes(status.Download),
		status.Devices,
		status.Uptime)
	systray.SetTooltip(tooltip)
}

// animateIconChange animates icon change with fade effect
func (c *WebSocketStatusClient) animateIconChange(iconFS embed.FS, iconPath string) {
	// Show transition icon first
	transitionData, err := amberIcon.ReadFile("icons/amber.ico")
	if err == nil {
		systray.SetIcon(transitionData)
	}

	// Small delay for transition effect
	time.Sleep(100 * time.Millisecond)

	// Set final icon
	iconData, err := iconFS.ReadFile(iconPath)
	if err == nil {
		systray.SetIcon(iconData)
	}
}

// Stop closes WebSocket connection
func (c *WebSocketStatusClient) Stop() {
	close(c.stop)
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()
}

// RunWithWebSocket runs the tray application with WebSocket real-time updates
func RunWithWebSocket() {
	slog.Info("Starting system tray with WebSocket...")

	trayStatsStore = stats.NewStore()

	var err error
	trayProfileMgr, err = profiles.NewManager()
	if err != nil {
		slog.Warn("Profile manager init error", "err", err)
	} else {
		trayProfileMgr.CreateDefaultProfiles()
	}

	hotkeyMgr := hotkey.NewManager()

	// Run systray in a goroutine with panic protection
	goroutine.SafeGo(func() {
		systray.Run(func() {
			onReadyWithWebSocket(hotkeyMgr)
		}, onExit)
	})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, windows.SIGINT, windows.SIGTERM)
	<-sigChan

	hotkeyMgr.Stop()
	systray.Quit()
	slog.Info("Tray application exited")
}

func onReadyWithWebSocket(hotkeyMgr *hotkey.Manager) {
	iconData, err := appIcon.ReadFile("icons/app.ico")
	if err == nil {
		systray.SetIcon(iconData)
	}
	systray.SetTitle("go-pcap2socks")
	systray.SetTooltip("go-pcap2socks - SOCKS5 прокси для устройств")

	mStatus := systray.AddMenuItem("Статус: Загрузка...", "")
	mStatus.Disable()

	systray.AddSeparator()

	mCopyIP := systray.AddMenuItem("📋 Копировать IP шлюза", "")

	systray.AddSeparator()

	mDevices := systray.AddMenuItem("📱 Устройства (0)", "")
	mDevices.Disable()

	systray.AddSeparator()

	mConfig := systray.AddMenuItem("⚙️ Открыть конфиг", "")
	mAutoConfig := systray.AddMenuItem("🚀 Авто-конфигурация", "")

	systray.AddSeparator()

	mProfileDefault := systray.AddMenuItem("📁 Default", "")
	mProfileGaming := systray.AddMenuItem("🎮 Gaming", "")
	mProfileStreaming := systray.AddMenuItem("📺 Streaming", "")

	systray.AddSeparator()

	mStart := systray.AddMenuItem("▶️ Запустить", "")
	mStop := systray.AddMenuItem("⏹️ Остановить", "")
	mStop.Hide()

	systray.AddSeparator()

	mLogs := systray.AddMenuItem("📜 Показать логи", "")

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("🚪 Выход", "")

	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", "err", err)
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")
	profilesDir := path.Join(path.Dir(executable), "profiles")

	if config, err := cfg.Load(cfgFile); err == nil && config.API != nil {
		apiToken = config.API.Token
	}

	// Start WebSocket client for real-time updates
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wsClient := NewWebSocketStatusClient()
	wsClient.Connect(ctx, mStatus, mDevices, mStart, mStop)

	// Fallback to polling if WebSocket not available
	goroutine.SafeGo(func() {
		time.Sleep(2 * time.Second)
		wsClient.mu.Lock()
		connected := wsClient.connected
		wsClient.mu.Unlock()

		if !connected {
			slog.Info("WebSocket not available, falling back to polling")
			go pollStatus(ctx, mStatus, mDevices, mStart, mStop)
		}
	})

	proxyToggled := false
	hotkeyMgr.RegisterDefaultHotkeys(
		func() {
			proxyToggled = !proxyToggled
			if proxyToggled {
				mStatus.SetTitle("Статус: Запущено (Hotkey)")
				mStart.Hide()
				mStop.Show()
				notify.Show("Горячие клавиши", "Прокси включён", notify.NotifySuccess)
			} else {
				mStatus.SetTitle("Статус: Остановлено (Hotkey)")
				mStop.Hide()
				mStart.Show()
				notify.Show("Горячие клавиши", "Прокси выключен", notify.NotifyWarning)
			}
		},
		func() {
			notify.Show("Горячие клавиши", "Перезапуск сервиса", notify.NotifyInfo)
			mStart.Hide()
			mStop.Show()
			mStatus.SetTitle("Статус: Перезапуск...")
		},
		func() {
			notify.Show("Горячие клавиши", "Остановка сервиса", notify.NotifyWarning)
			mStop.Hide()
			mStart.Show()
			mStatus.SetTitle("Статус: Остановлено")
		},
		func() {
			showLogs()
		},
	)

	for {
		select {
		case <-mConfig.ClickedCh:
			openConfig(cfgFile)

		case <-mAutoConfig.ClickedCh:
			runAutoConfig()

		case <-mProfileDefault.ClickedCh:
			switchProfile(profilesDir, "default")

		case <-mProfileGaming.ClickedCh:
			switchProfile(profilesDir, "gaming")

		case <-mProfileStreaming.ClickedCh:
			switchProfile(profilesDir, "streaming")

		case <-mStart.ClickedCh:
			mStart.Hide()
			mStop.Show()
			mStatus.SetTitle("Статус: Запущено")
			notify.Show("go-pcap2socks", "Сервис запущен", notify.NotifySuccess)

		case <-mStop.ClickedCh:
			mStart.Show()
			mStop.Hide()
			mStatus.SetTitle("Статус: Остановлено")
			notify.Show("go-pcap2socks", "Сервис остановлен", notify.NotifyWarning)

		case <-mCopyIP.ClickedCh:
			copyLocalIPToClipboard()

		case <-mLogs.ClickedCh:
			showLogs()

		case <-mQuit.ClickedCh:
			cancel()
			wsClient.Stop()
			return
		}
	}
}
