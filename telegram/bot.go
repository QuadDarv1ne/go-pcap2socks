package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/discord"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

// CommandHandler is a callback function for bot commands
type CommandHandler func(bot *Bot, args []string) string

// HandlerFunc is a callback function for bot commands
type HandlerFunc func() string

// Message represents a Telegram message
type Message struct {
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	Text      string `json:"text"`
	From      User   `json:"from"`
	MessageID int    `json:"message_id"`
}

// User represents a Telegram user
type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// Update represents a Telegram update
type Update struct {
	UpdateID int64   `json:"update_id"`
	Message  Message `json:"message"`
}

// APIResponse represents Telegram API response
type APIResponse struct {
	OK          bool     `json:"ok"`
	Result      []Update `json:"result"`
	Description string   `json:"description"`
}

// Bot represents a Telegram bot
type Bot struct {
	mu           sync.RWMutex
	token        string
	chatID       string
	enabled      bool
	httpClient   *http.Client
	apiURL       string
	lastUpdateID int64
	callbacks    map[string]CommandHandler
	// External handlers
	statusHandler  HandlerFunc
	trafficHandler HandlerFunc
	devicesHandler HandlerFunc
	startHandler   HandlerFunc
	stopHandler    HandlerFunc
	// Periodic reports
	reportInterval time.Duration
	reportStop     chan struct{}
	reportRunning  bool
	// Polling control
	pollStop    context.CancelFunc
	pollRunning bool
}

// SetDiscordWebhook sets the Discord webhook reference for cross-integration
func (b *Bot) SetDiscordWebhook(webhook interface{}) {
	// Store for later use in handlers
	_discordWebhook = webhook
}

// Global discord webhook reference (set from main.go)
var _discordWebhook interface{}

// NewBot creates a new Telegram bot
func NewBot(token, chatID string) *Bot {
	return &Bot{
		token:      token,
		chatID:     chatID,
		enabled:    token != "" && chatID != "",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiURL:     fmt.Sprintf("https://api.telegram.org/bot%s/", token),
		callbacks:  make(map[string]CommandHandler),
		reportStop: make(chan struct{}),
	}
}

// Start starts the bot polling
func (b *Bot) Start() {
	if !b.enabled {
		slog.Info("Telegram bot disabled (no token or chat ID)")
		return
	}

	slog.Info("Starting Telegram bot...")

	// Register default commands
	b.RegisterCommand("/start", b.handleStart)
	b.RegisterCommand("/help", b.handleHelp)
	b.RegisterCommand("/status", b.handleStatus)
	b.RegisterCommand("/start_service", b.handleStartService)
	b.RegisterCommand("/stop_service", b.handleStopService)
	b.RegisterCommand("/traffic", b.handleTraffic)
	b.RegisterCommand("/devices", b.handleDevices)
	b.RegisterCommand("/report", b.handleReport)
	b.RegisterCommand("/discord_status", b.handleDiscordStatus)

	goroutine.SafeGo(func() {
		b.poll()
	})
}

// StartPeriodicReports starts periodic traffic reports
func (b *Bot) StartPeriodicReports(interval time.Duration) {
	if !b.enabled || b.reportRunning {
		return
	}

	b.mu.Lock()
	b.reportInterval = interval
	b.reportRunning = true
	b.mu.Unlock()

	slog.Info("Starting periodic Telegram reports", "interval", interval)

	goroutine.SafeGo(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.sendPeriodicReport()
			case <-b.reportStop:
				slog.Info("Periodic Telegram reports stopped")
				return
			}
		}
	})
}

// StopPeriodicReports stops periodic reports
func (b *Bot) StopPeriodicReports() {
	if !b.reportRunning {
		return
	}

	close(b.reportStop)
	b.mu.Lock()
	b.reportRunning = false
	b.mu.Unlock()
}

func (b *Bot) sendPeriodicReport() {
	if b.trafficHandler == nil {
		return
	}

	report := b.trafficHandler()
	if report != "" {
		// Add timestamp
		text := fmt.Sprintf("📊 *Периодический отчёт*\n\n%s", report)
		b.SendMessage(text)
	}
}

// Stop stops the bot polling
func (b *Bot) Stop() {
	b.mu.Lock()
	b.enabled = false

	// Stop polling
	if b.pollStop != nil && b.pollRunning {
		b.pollStop()
	}
	b.mu.Unlock()

	slog.Info("Telegram bot stopped")
}

// StopPolling stops only the polling loop (for graceful restart)
func (b *Bot) StopPolling() {
	b.mu.Lock()
	if b.pollStop != nil && b.pollRunning {
		b.pollStop()
	}
	b.mu.Unlock()
}

// poll polls Telegram API for new messages
func (b *Bot) poll() {
	ctx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	b.pollStop = cancel
	b.pollRunning = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		b.pollRunning = false
		b.mu.Unlock()
	}()

	errorCount := 0
	const maxErrors = 5

	// Reusable timer for backoff delays
	backoffTimer := time.NewTimer(0)
	if !backoffTimer.Stop() {
		<-backoffTimer.C
	}
	defer backoffTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Telegram bot polling stopped")
			return
		default:
		}

		b.mu.RLock()
		enabled := b.enabled
		b.mu.RUnlock()

		if !enabled {
			return
		}

		updates, err := b.getUpdates(b.lastUpdateID + 1)
		if err != nil {
			slog.Debug("Telegram getUpdates error", "err", err)
			errorCount++

			if errorCount >= maxErrors {
				slog.Warn("Too many Telegram errors, stopping poll")
				return
			}

			// Exponential backoff with reusable timer
			backoff := time.Duration(5*(1<<errorCount)) * time.Second
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
			backoffTimer.Reset(backoff)
			select {
			case <-ctx.Done():
				if !backoffTimer.Stop() {
					<-backoffTimer.C
				}
				return
			case <-backoffTimer.C:
			}
			continue
		}

		errorCount = 0

		for _, update := range updates {
			b.lastUpdateID = update.UpdateID
			b.handleMessage(update.Message)
		}
	}
}

// getUpdates gets updates from Telegram API
func (b *Bot) getUpdates(offset int64) ([]Update, error) {
	url := fmt.Sprintf("%sgetUpdates?offset=%d&timeout=30", b.apiURL, offset)

	resp, err := b.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("telegram API error: %s", apiResp.Description)
	}

	return apiResp.Result, nil
}

// handleMessage handles incoming messages
func (b *Bot) handleMessage(msg Message) {
	if msg.Text == "" {
		return
	}

	// Check if message is from authorized chat
	if b.chatID != "" && fmt.Sprintf("%d", msg.Chat.ID) != b.chatID {
		slog.Warn("Telegram message from unauthorized chat", "chat_id", msg.Chat.ID)
		return
	}

	slog.Info("Telegram command received", "command", msg.Text, "from", msg.From.Username)

	// Parse command
	cmd := msg.Text
	args := []string{}

	// Simple command parsing
	for i, ch := range msg.Text {
		if ch == ' ' {
			cmd = msg.Text[:i]
			args = []string{msg.Text[i+1:]}
			break
		}
	}

	// Execute command handler
	b.mu.RLock()
	handler, exists := b.callbacks[cmd]
	b.mu.RUnlock()

	if exists {
		response := handler(b, args)
		if response != "" {
			b.SendMessage(response)
		}
	} else {
		b.SendMessage("Unknown command. Use /help for available commands.")
	}
}

// RegisterCommand registers a command handler
func (b *Bot) RegisterCommand(cmd string, handler CommandHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.callbacks[cmd] = handler
}

// SendMessage sends a message to Telegram
func (b *Bot) SendMessage(text string) error {
	if !b.enabled {
		return fmt.Errorf("bot disabled")
	}

	url := fmt.Sprintf("%ssendMessage", b.apiURL)

	payload := map[string]string{
		"chat_id":    b.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := b.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.OK {
		return fmt.Errorf("telegram API error: %s", result.Description)
	}

	return nil
}

// SendNotification sends a notification message
func (b *Bot) SendNotification(title, message string) {
	text := fmt.Sprintf("*%s*\n\n%s", title, message)
	b.SendMessage(text)
}

// Command handlers

func (b *Bot) handleStart(bot *Bot, args []string) string {
	return "👋 Привет! Я бот для управления go-pcap2socks.\n\nИспользуй /help для списка команд."
}

func (b *Bot) handleHelp(bot *Bot, args []string) string {
	return `📋 *Доступные команды:*

/start - Начать работу
/help - Показать эту справку
/status - Показать статус сервиса
/start_service - Запустить сервис
/stop_service - Остановить сервис
/traffic - Показать статистику трафика
/devices - Показать подключенные устройства
/report - Получить периодический отчёт
/discord_status - Проверить статус Discord webhook`
}

func (b *Bot) handleStatus(bot *Bot, args []string) string {
	if b.statusHandler != nil {
		return b.statusHandler()
	}
	return "Status handler not configured"
}

func (b *Bot) handleStartService(bot *Bot, args []string) string {
	if b.startHandler != nil {
		return b.startHandler()
	}
	return "Start service handler not configured"
}

func (b *Bot) handleStopService(bot *Bot, args []string) string {
	if b.stopHandler != nil {
		return b.stopHandler()
	}
	return "Stop service handler not configured"
}

func (b *Bot) handleTraffic(bot *Bot, args []string) string {
	if b.trafficHandler != nil {
		return b.trafficHandler()
	}
	return "Traffic handler not configured"
}

func (b *Bot) handleDevices(bot *Bot, args []string) string {
	if b.devicesHandler != nil {
		return b.devicesHandler()
	}
	return "Devices handler not configured"
}

func (b *Bot) handleReport(bot *Bot, args []string) string {
	if b.trafficHandler != nil {
		return b.trafficHandler()
	}
	return "Traffic handler not configured"
}

func (b *Bot) handleDiscordStatus(bot *Bot, args []string) string {
	// Get Discord status if configured
	if _discordWebhook != nil {
		if wh, ok := _discordWebhook.(*discord.WebhookClient); ok {
			if wh.IsEnabled() {
				return "🟢 Discord webhook активен\n\n" + wh.GetStatusMessage()
			}
		}
	}
	return "🔴 Discord webhook не настроен"
}

// SetStatusHandler sets the status handler callback
func (b *Bot) SetStatusHandler(handler HandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.statusHandler = handler
}

// SetTrafficHandler sets the traffic handler callback
func (b *Bot) SetTrafficHandler(handler HandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.trafficHandler = handler
}

// SetDevicesHandler sets the devices handler callback
func (b *Bot) SetDevicesHandler(handler HandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.devicesHandler = handler
}

// SetServiceHandlers sets the service control handlers
func (b *Bot) SetServiceHandlers(start, stop HandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.startHandler = start
	b.stopHandler = stop
}

// IsEnabled returns true if the bot is enabled
func (b *Bot) IsEnabled() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.enabled
}

// TestConnection tests the Telegram bot connection
func (b *Bot) TestConnection() error {
	return b.SendMessage("✅ Telegram bot connection test successful!")
}
