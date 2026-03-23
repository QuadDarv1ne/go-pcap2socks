//go:build windows

package tray

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/notify"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/getlantern/systray"
)

// Global state
var (
	trayRunning    bool
	trayStartTime  time.Time
	trayStatsStore *stats.Store
	trayProfileMgr *profiles.Manager
)

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

	// Run systray in a goroutine
	go func() {
		systray.Run(func() {
			onReady(hotkeyMgr)
		}, onExit)
	}()

	// Wait for exit signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	hotkeyMgr.Stop()
	systray.Quit()
	slog.Info("Tray application exited")
}

func onReady(hotkeyMgr *hotkey.Manager) {
	icon := getIcon()
	if icon != nil {
		systray.SetIcon(icon)
	}
	systray.SetTitle("go-pcap2socks")
	systray.SetTooltip("go-pcap2socks - SOCKS5 прокси для устройств")

	// Status menu item
	mStatus := systray.AddMenuItem("Статус: Неизвестно", "")
	mStatus.Disable()

	systray.AddSeparator()

	// Profile submenu
	mProfileMenu := systray.AddMenuItem("Профили", "")
	mProfileDefault := mProfileMenu.AddSubMenuItem("Default", "")
	mProfileGaming := mProfileMenu.AddSubMenuItem("Gaming", "")
	mProfileStreaming := mProfileMenu.AddSubMenuItem("Streaming", "")

	systray.AddSeparator()

	// Open config
	mConfig := systray.AddMenuItem("Открыть конфиг", "")

	// Auto-config
	mAutoConfig := systray.AddMenuItem("Авто-конфигурация", "")

	systray.AddSeparator()

	// Start/Stop
	mStart := systray.AddMenuItem("Запустить", "")
	mStop := systray.AddMenuItem("Остановить", "")
	mStop.Disable()

	systray.AddSeparator()

	// Show logs
	mLogs := systray.AddMenuItem("Показать логи", "")

	systray.AddSeparator()

	// Quit
	mQuit := systray.AddMenuItem("Выход", "")

	// Get config path
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", slog.Any("err", err))
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")
	profilesDir := path.Join(path.Dir(executable), "profiles")

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

		case <-mLogs.ClickedCh:
			showLogs()

		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func onExit() {
	slog.Info("Tray exit")
}

func getIcon() []byte {
	// Use systray default icon (empty = system default)
	// For custom icon, load from file or embed .ico data
	return nil // Use system default icon
}

func openConfig(cfgFile string) {
	cmd := exec.Command("notepad", cfgFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		slog.Error("open config error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось открыть конфиг", notify.NotifyError)
	}
}

func runAutoConfig() {
	notify.Show("Авто-конфигурация", "Запуск авто-конфигурации...", notify.NotifyInfo)

	// Run auto-config command
	cmd := exec.Command(os.Args[0], "auto-config")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		slog.Error("auto-config error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось выполнить авто-конфигурацию: "+err.Error(), notify.NotifyError)
		return
	}

	// Load and display config
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", slog.Any("err", err))
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")

	config, err := cfg.Load(cfgFile)
	if err != nil {
		slog.Error("load config error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось загрузить конфиг", notify.NotifyError)
		return
	}

	notify.Show(
		"Конфигурация создана",
		fmt.Sprintf("Сеть: %s\nШлюз: %s\nDHCP: %s",
			config.PCAP.Network,
			config.PCAP.LocalIP,
			map[bool]string{true: "включен", false: "выключен"}[config.DHCP != nil && config.DHCP.Enabled]),
		notify.NotifySuccess,
	)
}

func showLogs() {
	// Open logs in notepad or create a simple log viewer
	logFile := path.Join(path.Dir(os.Args[0]), "logs.txt")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		notify.Show("Логи", "Файл логов не найден", notify.NotifyWarning)
		return
	}

	cmd := exec.Command("notepad", logFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		slog.Error("open logs error", slog.Any("err", err))
	}
}

// switchProfile switches to the specified profile
func switchProfile(profilesDir, profile string) {
	profileFile := path.Join(profilesDir, profile+".json")
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось переключить профиль", notify.NotifyError)
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Check if profile exists
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		notify.Show("Профиль", fmt.Sprintf("Профиль '%s' не найден", profile), notify.NotifyWarning)
		return
	}

	// Read profile
	data, err := os.ReadFile(profileFile)
	if err != nil {
		slog.Error("read profile error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось прочитать профиль", notify.NotifyError)
		return
	}

	// Write to config
	if err := os.WriteFile(cfgFile, data, 0644); err != nil {
		slog.Error("write config error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось применить профиль", notify.NotifyError)
		return
	}

	notify.Show("Профиль", fmt.Sprintf("Переключено на '%s'. Перезапустите сервис.", profile), notify.NotifySuccess)
}
