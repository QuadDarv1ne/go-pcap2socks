package notify

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/discord"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/telegram"
)

// Pre-defined errors for notifications
var (
	ErrNotificationFailed    = errors.New("failed to send notification")
	ErrPowerShellUnavailable = errors.New("PowerShell is unavailable")
)

var (
	lastNotification sync.Map                           // map[string]int64 (key -> timestamp nanoseconds)
	minInterval      int64    = 30 * int64(time.Second) // Минимальный интервал между одинаковыми уведомлениями
	notifyCount      atomic.Int64

	// telegramBot holds Telegram bot instance
	telegramBot *telegram.Bot
	// discordWebhook holds Discord webhook client
	discordWebhook *discord.WebhookClient
	// externalEnabled indicates if external notifications are enabled
	externalEnabled atomic.Bool
)

// Notification types
const (
	NotifyInfo    = "info"
	NotifyWarning = "warning"
	NotifyError   = "error"
	NotifySuccess = "success"
)

// InitExternal initializes Telegram and Discord notifications
func InitExternal(telegramCfg *cfg.Telegram, discordCfg *cfg.Discord) {
	if telegramCfg != nil && telegramCfg.Enabled && telegramCfg.Token != "" {
		telegramBot = telegram.NewBot(telegramCfg.Token, telegramCfg.ChatID)
		if telegramBot != nil {
			externalEnabled.Store(true)
			slog.Info("Telegram notifications enabled")
		}
	}

	if discordCfg != nil && discordCfg.WebhookURL != "" {
		discordWebhook = discord.NewWebhookClient(discordCfg.WebhookURL)
		if discordWebhook != nil {
			externalEnabled.Store(true)
			slog.Info("Discord notifications enabled")
		}
	}
}

// Show отправляет уведомление пользователю
// Optimized with sync.Map for lock-free rate limiting
func Show(title, message, notifyType string) {
	key := title + ":" + message
	now := time.Now().UnixNano()

	// Fast path: check if notification was sent recently (lock-free)
	if lastTimeVal, exists := lastNotification.Load(key); exists {
		lastTime := lastTimeVal.(int64)
		if now-lastTime < minInterval {
			return // Пропускаем дублирующееся уведомление
		}
	}

	// Update last notification time (lock-free)
	lastNotification.Store(key, now)
	notifyCount.Add(1)

	// Логируем уведомление
	logNotification(title, message, notifyType)

	// Показываем Windows toast уведомление
	showToastNotification(title, message, notifyType)

	// Отправляем внешние уведомления (Telegram/Discord) с защитой от утечки
	if externalEnabled.Load() {
		goroutine.SafeGo(func() {
			// Add timeout to prevent goroutine leak if external service is slow
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			sendExternalNotification(ctx, title, message, notifyType)
		})
	}
}

// sendExternalNotification sends notification to Telegram and Discord
// Now accepts context for timeout control
func sendExternalNotification(ctx context.Context, title, message, notifyType string) {
	// Send to Telegram with timeout using buffered channel to prevent goroutine leak
	if telegramBot != nil {
		text := formatMessage(title, message, notifyType)
		done := make(chan struct{}, 1)
		goroutine.SafeGo(func() {
			_ = telegramBot.SendMessage(text)
			select {
			case done <- struct{}{}:
			default:
				// Context already timed out, drop result
			}
		})
		select {
		case <-done:
			// Success
		case <-ctx.Done():
			slog.Warn("Telegram notification timeout", "title", title)
		}
	}

	// Send to Discord with timeout using buffered channel to prevent goroutine leak
	if discordWebhook != nil {
		embed := formatDiscordEmbed(title, message, notifyType)
		done := make(chan struct{}, 1)
		goroutine.SafeGo(func() {
			_ = discordWebhook.SendEmbed(embed)
			select {
			case done <- struct{}{}:
			default:
				// Context already timed out, drop result
			}
		})
		select {
		case <-done:
			// Success
		case <-ctx.Done():
			slog.Warn("Discord notification timeout", "title", title)
		}
	}
}

// formatMessage formats message for Telegram
func formatMessage(title, message, notifyType string) string {
	prefix := "📢"
	switch notifyType {
	case NotifyError:
		prefix = "❌"
	case NotifyWarning:
		prefix = "⚠️"
	case NotifySuccess:
		prefix = "✅"
	}
	return prefix + " *" + title + "*\n" + message
}

// formatDiscordEmbed formats embed for Discord
func formatDiscordEmbed(title, message, notifyType string) discord.Embed {
	color := 0x3b82f6 // Blue
	switch notifyType {
	case NotifyError:
		color = 0xef4444 // Red
	case NotifyWarning:
		color = 0xf59e0b // Orange
	case NotifySuccess:
		color = 0x22c55e // Green
	}

	return discord.Embed{
		Title:       title,
		Description: message,
		Color:       color,
		Footer: discord.EmbedFooter{
			Text: "go-pcap2socks",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func logNotification(title, message, notifyType string) {
	switch notifyType {
	case NotifyError:
		slog.Error(title, "message", message)
	case NotifyWarning:
		slog.Warn(title, "message", message)
	case NotifySuccess:
		slog.Info(title, "message", message)
	default:
		slog.Info(title, "message", message)
	}
}

func showToastNotification(title, message, notifyType string) {
	// Используем PowerShell для показа toast уведомления
	// Экранируем специальные XML символы
	titleEsc := escapeXML(title)
	messageEsc := escapeXML(message)

	script := `
param($title, $message)

try {
	[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
	[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

	$template = @"
<toast>
	<visual>
		<binding template="ToastText02">
			<text id="1">$title</text>
			<text id="2">$message</text>
		</binding>
	</visual>
</toast>
"@

	$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
	$xml.LoadXml($template)

	$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
	[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("go-pcap2socks").Show($toast)
} catch {
	# Игнорируем ошибки toast уведомлений
}
	`

	cmd := exec.Command("powershell", "-Command", script, "-", titleEsc, messageEsc)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // Игнорируем ошибки, если PowerShell недоступен
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// ClearHistory очищает историю уведомлений
// Optimized with sync.Map for lock-free clear
func ClearHistory() {
	lastNotification = sync.Map{}
	notifyCount.Store(0)
}

// GetNotificationCount returns the total number of notifications sent
func GetNotificationCount() int64 {
	return notifyCount.Load()
}
