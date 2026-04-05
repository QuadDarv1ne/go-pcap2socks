//go:build windows

package tray

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/notify"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/getlantern/systray"
)

//go:embed icons/app.ico
var appIcon embed.FS

// Global state
var (
	trayRunning    bool
	trayStartTime  time.Time
	trayStatsStore *stats.Store
	trayProfileMgr *profiles.Manager
	apiBaseURL     = "http://127.0.0.1:8080"
	apiToken       = ""
)

// APIStatus represents the status response from the API
type APIStatus struct {
	Running   bool   `json:"running"`
	Uptime    string `json:"uptime"`
	ProxyMode string `json:"proxy_mode"`
	Devices   int    `json:"devices"`
	Total     uint64 `json:"total"`
	Upload    uint64 `json:"upload"`
	Download  uint64 `json:"download"`
	LocalIP   string `json:"local_ip"`
}

// Run starts the system tray application
func Run() {
	slog.Info("Starting system tray...")

	// Initialize stats store
	trayStatsStore = stats.NewStore()

	// Initialize profile manager
	var err error
	trayProfileMgr, err = profiles.NewManager()
	if err != nil {
		slog.Warn("Profile manager init error", "err", err)
	} else {
		trayProfileMgr.CreateDefaultProfiles()
	}

	// Initialize hotkey manager
	hotkeyMgr := hotkey.NewManager()

	// Run systray in a goroutine with panic protection
	goroutine.SafeGo(func() {
		systray.Run(func() {
			onReady(hotkeyMgr)
		}, onExit)
	})

	// Wait for exit signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	hotkeyMgr.Stop()
	systray.Quit()
	slog.Info("Tray application exited")
}

func onReady(hotkeyMgr *hotkey.Manager) {
	// Set icon
	iconData, err := appIcon.ReadFile("icons/app.ico")
	if err == nil {
		systray.SetIcon(iconData)
	}
	systray.SetTitle("go-pcap2socks")
	systray.SetTooltip("go-pcap2socks - SOCKS5 прокси для устройств")

	// Status menu item (disabled, just for info)
	mStatus := systray.AddMenuItem("Статус: Загрузка...", "")
	mStatus.Disable()

	systray.AddSeparator()

	// Quick actions
	mCopyIP := systray.AddMenuItem("📋 Копировать IP шлюза", "")
	mDevices := systray.AddMenuItem("📱 Устройства (0)", "")
	mDevices.Disable()

	systray.AddSeparator()

	// Profile submenu
	mProfileMenu := systray.AddMenuItem("📁 Профили", "")
	mProfileDefault := mProfileMenu.AddSubMenuItem("Default", "")
	mProfileGaming := mProfileMenu.AddSubMenuItem("Gaming", "")
	mProfileStreaming := mProfileMenu.AddSubMenuItem("Streaming", "")

	systray.AddSeparator()

	// Open config
	mConfig := systray.AddMenuItem("⚙️ Открыть конфиг", "")

	// Auto-config
	mAutoConfig := systray.AddMenuItem("🚀 Авто-конфигурация", "")

	systray.AddSeparator()

	// Start/Stop
	mStart := systray.AddMenuItem("▶️ Запустить", "")
	mStop := systray.AddMenuItem("⏹️ Остановить", "")
	mStop.Disable()

	systray.AddSeparator()

	// Show logs
	mLogs := systray.AddMenuItem("📜 Показать логи", "")

	systray.AddSeparator()

	// Quit
	mQuit := systray.AddMenuItem("🚪 Выход", "")

	// Get config path
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", "err", err)
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")
	profilesDir := path.Join(path.Dir(executable), "profiles")

	// Load API token from config
	if config, err := cfg.Load(cfgFile); err == nil && config.API != nil {
		apiToken = config.API.Token
	}

	// Start status polling goroutine with panic protection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	goroutine.SafeGo(func() {
		pollStatus(ctx, mStatus, mDevices, mStart, mStop)
	})

	// Register hotkeys
	proxyToggled := false
	hotkeyMgr.RegisterDefaultHotkeys(
		func() {
			// Toggle proxy
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
			// Restart service
			notify.Show("Горячие клавиши", "Перезапуск сервиса", notify.NotifyInfo)
			mStart.Hide()
			mStop.Show()
			mStatus.SetTitle("Статус: Перезапуск...")
		},
		func() {
			// Stop service
			notify.Show("Горячие клавиши", "Остановка сервиса", notify.NotifyWarning)
			mStop.Hide()
			mStart.Show()
			mStatus.SetTitle("Статус: Остановлено")
		},
		func() {
			// Toggle logs
			showLogs()
		},
	)

	// Handle menu clicks
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
			systray.Quit()
			return
		}
	}
}

func onExit() {
	slog.Info("Tray exit")
}

// pollStatus polls the API status every 5 seconds
func pollStatus(ctx context.Context, mStatus, mDevices, mStart, mStop *systray.MenuItem) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status := fetchStatus()
			if status != nil {
				// Update status text
				if status.Running {
					mStatus.SetTitle(fmt.Sprintf("Статус: Запущено (%s)", status.Uptime))
					mStart.Hide()
					mStop.Show()
				} else {
					mStatus.SetTitle("Статус: Остановлено")
					mStart.Show()
					mStop.Hide()
				}

				// Update devices count
				mDevices.SetTitle(fmt.Sprintf("📱 Устройства (%d)", status.Devices))

				// Update tooltip with traffic info
				tooltip := fmt.Sprintf("go-pcap2socks\n↑ Загрузка: %s\n↓ Выгрузка: %s\nУстройств: %d",
					formatBytes(status.Upload),
					formatBytes(status.Download),
					status.Devices)
				systray.SetTooltip(tooltip)
			}
		}
	}
}

// fetchStatus fetches the status from the API
func fetchStatus() *APIStatus {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest("GET", apiBaseURL+"/api/status", nil)
	if err != nil {
		return nil
	}

	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var status APIStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil
	}

	return &status
}

// formatBytes formats bytes to human readable string
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// copyLocalIPToClipboard copies the local IP to clipboard
func copyLocalIPToClipboard() {
	// Try to get from API first
	status := fetchStatus()
	localIP := ""
	if status != nil && status.LocalIP != "" {
		localIP = status.LocalIP
	} else {
		// Fallback to config
		executable, err := os.Executable()
		if err == nil {
			cfgFile := path.Join(path.Dir(executable), "config.json")
			if config, err := cfg.Load(cfgFile); err == nil {
				localIP = config.PCAP.LocalIP
			}
		}
	}

	if localIP != "" {
		// Use PowerShell to copy to clipboard
		cmd := exec.Command("powershell", "-command", fmt.Sprintf("'%s' | Set-Clipboard", localIP))
		if err := cmd.Run(); err == nil {
			notify.Show("go-pcap2socks", fmt.Sprintf("IP скопирован: %s", localIP), notify.NotifySuccess)
		}
	}
}

// showLogs shows the logs in a simple window
func showLogs() {
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", "err", err)
		return
	}

	logFile := path.Join(path.Dir(executable), "go-pcap2socks.log")

	// Check if log file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		notify.Show("go-pcap2socks", "Файл логов не найден", notify.NotifyWarning)
		return
	}

	// Open with notepad for now (can be improved with custom viewer)
	cmd := exec.Command("notepad.exe", logFile)
	cmd.Start()
}

func openConfig(cfgFile string) {
	cmd := exec.Command("notepad.exe", cfgFile)
	cmd.Start()
}

func runAutoConfig() {
	notify.Show("go-pcap2socks", "Авто-конфигурация запущена...", notify.NotifyInfo)
	// Implementation would call the auto-config logic
}

func switchProfile(profilesDir, profile string) {
	profileFile := path.Join(profilesDir, profile+".json")
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		notify.Show("go-pcap2socks", fmt.Sprintf("Профиль не найден: %s", profile), notify.NotifyWarning)
		return
	}

	// Copy profile to config.json
	configData, err := os.ReadFile(profileFile)
	if err != nil {
		notify.Show("go-pcap2socks", fmt.Sprintf("Ошибка чтения профиля: %v", err), notify.NotifyError)
		return
	}

	executable, err := os.Executable()
	if err != nil {
		notify.Show("go-pcap2socks", fmt.Sprintf("Ошибка: %v", err), notify.NotifyError)
		return
	}

	cfgFile := path.Join(path.Dir(executable), "config.json")
	if err := os.WriteFile(cfgFile, configData, 0644); err != nil {
		notify.Show("go-pcap2socks", fmt.Sprintf("Ошибка записи: %v", err), notify.NotifyError)
		return
	}

	notify.Show("go-pcap2socks", fmt.Sprintf("Профиль применён: %s", profile), notify.NotifySuccess)
}

// getIcon returns the embedded icon (legacy function)
func getIcon() []byte {
	iconData, err := appIcon.ReadFile("icons/app.ico")
	if err == nil {
		return iconData
	}
	return nil
}
