package notify

import (
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	lastNotification sync.Map // map[string]int64 (key -> timestamp nanoseconds)
	minInterval      int64 = 30 * int64(time.Second) // Минимальный интервал между одинаковыми уведомлениями
	notifyCount      atomic.Int64
)

// Notification types
const (
	NotifyInfo    = "info"
	NotifyWarning = "warning"
	NotifyError   = "error"
	NotifySuccess = "success"
)

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
