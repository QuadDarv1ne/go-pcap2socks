// Package discord provides Discord webhook notifications for go-pcap2socks.
package discord

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Pre-defined errors for Discord webhook operations
var (
	ErrWebhookDisabled   = errors.New("webhook disabled")
	ErrWebhookSendFailed = errors.New("failed to send webhook")
	ErrInvalidWebhookURL = errors.New("invalid webhook URL")
)

// WebhookClient represents a Discord webhook client
type WebhookClient struct {
	webhookURL string
	httpClient *http.Client
	enabled    bool
	// Status handler for external requests
	statusHandler func() string

	// Rate limiting for device notifications
	deviceNotifyMu    sync.Mutex
	lastDeviceNotify  map[string]time.Time // MAC -> last notify time
	minNotifyInterval time.Duration
}

// Embed represents a Discord embed
type Embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Footer      EmbedFooter  `json:"footer,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
}

// EmbedField represents a field in a Discord embed
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// EmbedFooter represents a footer in a Discord embed
type EmbedFooter struct {
	Text string `json:"text"`
}

// Payload represents a Discord webhook payload
type Payload struct {
	Content   string  `json:"content,omitempty"`
	Embeds    []Embed `json:"embeds,omitempty"`
	Username  string  `json:"username,omitempty"`
	AvatarURL string  `json:"avatar_url,omitempty"`
}

// NewWebhookClient creates a new Discord webhook client
func NewWebhookClient(webhookURL string) *WebhookClient {
	return &WebhookClient{
		webhookURL:        webhookURL,
		httpClient:        &http.Client{Timeout: 30 * time.Second},
		enabled:           webhookURL != "",
		lastDeviceNotify:  make(map[string]time.Time),
		minNotifyInterval: 30 * time.Second, // Min 30s between notifications per device
	}
}

// Send sends a message to Discord webhook.
// Returns ErrWebhookDisabled if webhook is not enabled.
func (w *WebhookClient) Send(content string) error {
	if !w.enabled {
		return ErrWebhookDisabled
	}

	payload := Payload{
		Content:  content,
		Username: "go-pcap2socks",
	}

	return w.sendPayload(payload)
}

// SendEmbed sends an embed message to Discord webhook
func (w *WebhookClient) SendEmbed(embed Embed) error {
	if !w.enabled {
		return fmt.Errorf("webhook disabled")
	}

	payload := Payload{
		Embeds:   []Embed{embed},
		Username: "go-pcap2socks",
	}

	return w.sendPayload(payload)
}

// SendNotification sends a notification embed
func (w *WebhookClient) SendNotification(title, description string, color int) error {
	embed := Embed{
		Title:       title,
		Description: description,
		Color:       color,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: EmbedFooter{
			Text: "go-pcap2socks",
		},
	}

	return w.SendEmbed(embed)
}

// SendInfo sends an info notification (blue)
func (w *WebhookClient) SendInfo(title, description string) error {
	return w.SendNotification(title, description, 3447003) // Blue
}

// SendSuccess sends a success notification (green)
func (w *WebhookClient) SendSuccess(title, description string) error {
	return w.SendNotification(title, description, 5763719) // Green
}

// SendWarning sends a warning notification (orange)
func (w *WebhookClient) SendWarning(title, description string) error {
	return w.SendNotification(title, description, 15158332) // Orange
}

// SendError sends an error notification (red)
func (w *WebhookClient) SendError(title, description string) error {
	return w.SendNotification(title, description, 15548997) // Red
}

// SendStatus sends a status update embed
func (w *WebhookClient) SendStatus(running bool, devices int, traffic string) error {
	color := 5763719 // Green
	status := "🟢 Запущен"
	if !running {
		color = 15548997 // Red
		status = "🔴 Остановлен"
	}

	embed := Embed{
		Title: "📊 Статус go-pcap2socks",
		Color: color,
		Fields: []EmbedField{
			{Name: "Статус", Value: status, Inline: true},
			{Name: "Устройств", Value: fmt.Sprintf("%d", devices), Inline: true},
			{Name: "Трафик", Value: traffic, Inline: true},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: EmbedFooter{
			Text: "go-pcap2socks",
		},
	}

	return w.SendEmbed(embed)
}

// SendDeviceNotification sends a device connection notification with rate limiting
func (w *WebhookClient) SendDeviceNotification(event, ip, mac string) error {
	// Rate limiting: skip if same device notified recently
	w.deviceNotifyMu.Lock()
	lastTime, exists := w.lastDeviceNotify[mac]
	if exists && time.Since(lastTime) < w.minNotifyInterval {
		w.deviceNotifyMu.Unlock()
		return nil // Skip notification (rate limited)
	}
	w.lastDeviceNotify[mac] = time.Now()
	w.deviceNotifyMu.Unlock()

	emoji := "❌"
	color := 15548997 // Red

	if event == "connected" {
		emoji = "✅"
		color = 5763719 // Green
	}

	embed := Embed{
		Title: fmt.Sprintf("%s Устройство %s", emoji, event),
		Color: color,
		Fields: []EmbedField{
			{Name: "IP адрес", Value: ip, Inline: true},
			{Name: "MAC адрес", Value: mac, Inline: true},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: EmbedFooter{
			Text: "go-pcap2socks - ARP Monitor",
		},
	}

	return w.SendEmbed(embed)
}

func (w *WebhookClient) sendPayload(payload Payload) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := w.httpClient.Post(w.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discord webhook error: status %d", resp.StatusCode)
	}

	return nil
}

// IsEnabled returns true if the webhook is enabled
func (w *WebhookClient) IsEnabled() bool {
	return w.enabled
}

// TestConnection tests the webhook connection
func (w *WebhookClient) TestConnection() error {
	return w.SendInfo("✅ Тест подключения", "Discord webhook работает корректно!")
}

// SetStatusHandler sets the status handler callback
func (w *WebhookClient) SetStatusHandler(handler func() string) {
	w.statusHandler = handler
}

// GetStatusMessage returns the current status message
func (w *WebhookClient) GetStatusMessage() string {
	if w.statusHandler != nil {
		return w.statusHandler()
	}
	return "Status handler not configured"
}

// Log sends a log message to Discord
func (w *WebhookClient) Log(level, message string) {
	var color int
	switch level {
	case "INFO":
		color = 3447003 // Blue
	case "WARN":
		color = 15158332 // Orange
	case "ERROR":
		color = 15548997 // Red
	default:
		color = 3447003
	}

	embed := Embed{
		Title:       fmt.Sprintf("📝 Лог: %s", level),
		Description: fmt.Sprintf("```%s```", message),
		Color:       color,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: EmbedFooter{
			Text: "go-pcap2socks - Logger",
		},
	}

	w.SendEmbed(embed)
}
