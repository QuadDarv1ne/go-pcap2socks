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

	"github.com/DaniilSokolyuk/go-pcap2socks/cfg"
	"github.com/DaniilSokolyuk/go-pcap2socks/notify"
	"github.com/getlantern/systray"
)

// Run starts the system tray application
func Run() {
	slog.Info("Starting system tray...")

	// Run systray in a goroutine
	go func() {
		systray.Run(onReady, onExit)
	}()

	// Wait for exit signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	systray.Quit()
	slog.Info("Tray application exited")
}

func onReady() {
	systray.SetIcon(getIcon())
	systray.SetTitle("go-pcap2socks")
	systray.SetTooltip("go-pcap2socks - SOCKS5 прокси для устройств")

	// Status menu item
	mStatus := systray.AddMenuItem("Статус: Неизвестно", "")
	mStatus.Disable()

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

	// Handle menu clicks
	for {
		select {
		case <-mConfig.ClickedCh:
			openConfig(cfgFile)

		case <-mAutoConfig.ClickedCh:
			runAutoConfig()

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
	// Simple icon data (placeholder - should be a real .ico file)
	// For now, return empty and systray will use default
	return []byte{}
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
	// Import and run autoConfigure from main
	// For now, just show notification
	notify.Show("Авто-конфигурация", "Запуск авто-конфигурации...", notify.NotifyInfo)
	
	// Run auto-config logic here
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", slog.Any("err", err))
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")
	
	// Check if config exists and load it
	config, err := cfg.Load(cfgFile)
	if err != nil {
		slog.Error("load config error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось загрузить конфиг", notify.NotifyError)
		return
	}
	
	notify.Show(
		"Конфигурация",
		fmt.Sprintf("Сеть: %s\nШлюз: %s", config.PCAP.Network, config.PCAP.LocalIP),
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
