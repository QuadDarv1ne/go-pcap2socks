package notify

import (
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	lastNotification = make(map[string]time.Time)
	notifyMux        sync.Mutex
	minInterval      = 30 * time.Second // Минимальный интервал между одинаковыми уведомлениями
)

// Notification types
const (
	NotifyInfo    = "info"
	NotifyWarning = "warning"
	NotifyError   = "error"
	NotifySuccess = "success"
)

// Show отправляет уведомление пользователю
func Show(title, message, notifyType string) {
	key := title + ":" + message

	// Проверяем, не было ли такого же уведомления недавно
	notifyMux.Lock()
	lastTime, exists := lastNotification[key]
	notifyMux.Unlock()

	if exists && time.Since(lastTime) < minInterval {
		return // Пропускаем дублирующееся уведомление
	}

	// Обновляем время последнего уведомления
	notifyMux.Lock()
	lastNotification[key] = time.Now()
	notifyMux.Unlock()

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
func ClearHistory() {
	notifyMux.Lock()
	defer notifyMux.Unlock()
	lastNotification = make(map[string]time.Time)
}
