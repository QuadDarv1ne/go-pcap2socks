//go:build ignore
// +build ignore

// Этот файл содержит внутренние тесты для Telegram бота.
// Они не запускаются автоматически из-за возможного ложного срабатывания антивируса.
// Для запуска вручную: go test -v ./telegram/... -run Internal

package telegram

import (
	"testing"
	"time"
)

// TestInternal запускает все внутренние тесты Telegram бота.
// Этот тест не запускается автоматически из-за возможного ложного срабатывания антивируса.
// Для запуска вручную: go test -v ./telegram/... -run TestInternal
func TestInternal(t *testing.T) {
	t.Run("NewBot", testNewBotInternal)
	t.Run("IsEnabled", testBotIsEnabledInternal)
	t.Run("Stop", testBotStopInternal)
	t.Run("StopPolling", testBotStopPollingInternal)
	t.Run("SendMessage_Disabled", testSendMessageDisabledInternal)
	t.Run("SendNotification_Disabled", testSendNotificationDisabledInternal)
	t.Run("RegisterCommand", testRegisterCommandInternal)
	t.Run("HandleMessage_Empty", testHandleMessageEmptyInternal)
	t.Run("HandleMessage_Unauthorized", testHandleMessageUnauthorizedInternal)
	t.Run("HandleMessage_UnknownCommand", testHandleMessageUnknownCommandInternal)
	t.Run("HandleMessage_KnownCommand", testHandleMessageKnownCommandInternal)
	t.Run("HandleMessage_WithArgs", testHandleMessageWithArgsInternal)
	t.Run("DefaultHandlers", testDefaultHandlersInternal)
	t.Run("SetStatusHandler", testSetStatusHandlerInternal)
	t.Run("SetTrafficHandler", testSetTrafficHandlerInternal)
	t.Run("SetDevicesHandler", testSetDevicesHandlerInternal)
	t.Run("SetServiceHandlers", testSetServiceHandlersInternal)
	t.Run("HandleStatus_NoHandler", testHandleStatusNoHandlerInternal)
	t.Run("HandleTraffic_NoHandler", testHandleTrafficNoHandlerInternal)
	t.Run("HandleDevices_NoHandler", testHandleDevicesNoHandlerInternal)
	t.Run("HandleDiscordStatus_NoDiscord", testHandleDiscordStatusNoDiscordInternal)
	t.Run("StartPeriodicReports_Disabled", testStartPeriodicReportsDisabledInternal)
	t.Run("StartPeriodicReports_AlreadyRunning", testStartPeriodicReportsAlreadyRunningInternal)
	t.Run("StopPeriodicReports_NotRunning", testStopPeriodicReportsNotRunningInternal)
	t.Run("Message_Structure", testMessageStructureInternal)
	t.Run("Update_Structure", testUpdateStructureInternal)
	t.Run("APIResponse_Structure", testAPIResponseStructureInternal)
}

func testNewBotInternal(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		chatID      string
		wantEnabled bool
	}{
		{
			name:        "valid token and chatID",
			token:       "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
			chatID:      "123456789",
			wantEnabled: true,
		},
		{
			name:        "empty token",
			token:       "",
			chatID:      "123456789",
			wantEnabled: false,
		},
		{
			name:        "empty chatID",
			token:       "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
			chatID:      "",
			wantEnabled: false,
		},
		{
			name:        "empty token and chatID",
			token:       "",
			chatID:      "",
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := NewBot(tt.token, tt.chatID)
			
			if bot == nil {
				t.Fatal("NewBot returned nil")
			}
			
			if bot.enabled != tt.wantEnabled {
				t.Errorf("enabled = %v, want %v", bot.enabled, tt.wantEnabled)
			}
			
			if bot.token != tt.token {
				t.Errorf("token = %v, want %v", bot.token, tt.token)
			}
			
			if bot.chatID != tt.chatID {
				t.Errorf("chatID = %v, want %v", bot.chatID, tt.chatID)
			}
			
			if bot.httpClient == nil {
				t.Error("httpClient is nil")
			}
			
			if bot.callbacks == nil {
				t.Error("callbacks map is nil")
			}
			
			if bot.reportStop == nil {
				t.Error("reportStop channel is nil")
			}
		})
	}
}

func testBotIsEnabledInternal(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		chatID      string
		wantEnabled bool
	}{
		{
			name:        "enabled bot",
			token:       "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
			chatID:      "123456789",
			wantEnabled: true,
		},
		{
			name:        "disabled bot - empty token",
			token:       "",
			chatID:      "123456789",
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := NewBot(tt.token, tt.chatID)
			got := bot.IsEnabled()
			
			if got != tt.wantEnabled {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.wantEnabled)
			}
		})
	}
}

func testBotStopInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	// Should not panic
	bot.Stop()
	
	if bot.enabled {
		t.Error("Bot should be disabled after Stop()")
	}
}

func testBotStopPollingInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	// Should not panic when not running
	bot.StopPolling()
}

func testSendMessageDisabledInternal(t *testing.T) {
	bot := NewBot("", "")
	
	err := bot.SendMessage("test message")
	if err == nil {
		t.Error("SendMessage() should return error when bot is disabled")
	}
}

func testSendNotificationDisabledInternal(t *testing.T) {
	bot := NewBot("", "")
	
	// Should not panic
	bot.SendNotification("title", "message")
}

func testRegisterCommandInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")

	testHandler := func(b *Bot, args []string) string {
		return "response"
	}

	bot.RegisterCommand("/test", testHandler)
	
	bot.mu.RLock()
	_, exists := bot.callbacks["/test"]
	bot.mu.RUnlock()
	
	if !exists {
		t.Error("Command /test should be registered")
	}
}

func testHandleMessageEmptyInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	msg := Message{
		Text: "",
	}
	
	// Should not panic with empty message
	bot.handleMessage(msg)
}

func testHandleMessageUnauthorizedInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	msg := Message{
		Text: "/start",
		Chat: struct {
			ID int64 `json:"id"`
		}{ID: 999999}, // Different chat ID
	}
	
	// Should not panic, just log warning
	bot.handleMessage(msg)
}

func testHandleMessageUnknownCommandInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	msg := Message{
		Text: "/unknown_command",
		Chat: struct {
			ID int64 `json:"id"`
		}{ID: 123456789},
	}
	
	// Should not panic
	bot.handleMessage(msg)
}

func testHandleMessageKnownCommandInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	handlerCalled := false
	testHandler := func(b *Bot, args []string) string {
		handlerCalled = true
		return "test response"
	}
	
	bot.RegisterCommand("/test", testHandler)
	
	msg := Message{
		Text: "/test",
		Chat: struct {
			ID int64 `json:"id"`
		}{ID: 123456789},
	}
	
	// Should not panic
	bot.handleMessage(msg)
	
	if !handlerCalled {
		t.Error("Handler should be called")
	}
}

func testHandleMessageWithArgsInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	var receivedArgs []string
	testHandler := func(b *Bot, args []string) string {
		receivedArgs = args
		return "response"
	}
	
	bot.RegisterCommand("/cmd", testHandler)
	
	msg := Message{
		Text: "/cmd arg1 arg2",
		Chat: struct {
			ID int64 `json:"id"`
		}{ID: 123456789},
	}
	
	bot.handleMessage(msg)
	
	if len(receivedArgs) != 1 {
		t.Errorf("Expected 1 argument, got %d", len(receivedArgs))
	}
}

func testDefaultHandlersInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	// Register default handlers
	bot.RegisterCommand("/start", bot.handleStart)
	bot.RegisterCommand("/help", bot.handleHelp)
	
	tests := []struct {
		cmd      string
		expected string
	}{
		{"/start", "👋 Привет! Я бот для управления go-pcap2socks."},
		{"/help", "📋 *Доступные команды:*"},
	}
	
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			msg := Message{
				Text: tt.cmd,
				Chat: struct {
					ID int64 `json:"id"`
				}{ID: 123456789},
			}
			
			// Should not panic
			bot.handleMessage(msg)
		})
	}
}

func testSetStatusHandlerInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	testStatus := "Test status"
	bot.SetStatusHandler(func() string {
		return testStatus
	})
	
	bot.mu.RLock()
	handler := bot.statusHandler
	bot.mu.RUnlock()
	
	if handler == nil {
		t.Error("statusHandler should be set")
	}
}

func testSetTrafficHandlerInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	testTraffic := "Test traffic"
	bot.SetTrafficHandler(func() string {
		return testTraffic
	})
	
	bot.mu.RLock()
	handler := bot.trafficHandler
	bot.mu.RUnlock()
	
	if handler == nil {
		t.Error("trafficHandler should be set")
	}
}

func testSetDevicesHandlerInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	testDevices := "Test devices"
	bot.SetDevicesHandler(func() string {
		return testDevices
	})
	
	bot.mu.RLock()
	handler := bot.devicesHandler
	bot.mu.RUnlock()
	
	if handler == nil {
		t.Error("devicesHandler should be set")
	}
}

func testSetServiceHandlersInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	bot.SetServiceHandlers(
		func() string { return "started" },
		func() string { return "stopped" },
	)
	
	bot.mu.RLock()
	startHandler := bot.startHandler
	stopHandler := bot.stopHandler
	bot.mu.RUnlock()
	
	if startHandler == nil {
		t.Error("startHandler should be set")
	}
	
	if stopHandler == nil {
		t.Error("stopHandler should be set")
	}
}

func testHandleStatusNoHandlerInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	result := bot.handleStatus(bot, []string{})
	expected := "Status handler not configured"
	
	if result != expected {
		t.Errorf("handleStatus() = %v, want %v", result, expected)
	}
}

func testHandleTrafficNoHandlerInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	result := bot.handleTraffic(bot, []string{})
	expected := "Traffic handler not configured"
	
	if result != expected {
		t.Errorf("handleTraffic() = %v, want %v", result, expected)
	}
}

func testHandleDevicesNoHandlerInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	result := bot.handleDevices(bot, []string{})
	expected := "Devices handler not configured"
	
	if result != expected {
		t.Errorf("handleDevices() = %v, want %v", result, expected)
	}
}

func testHandleDiscordStatusNoDiscordInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	result := bot.handleDiscordStatus(bot, []string{})
	expected := "🔴 Discord webhook не настроен"
	
	if result != expected {
		t.Errorf("handleDiscordStatus() = %v, want %v", result, expected)
	}
}

func testStartPeriodicReportsDisabledInternal(t *testing.T) {
	bot := NewBot("", "")
	
	// Should not panic when disabled
	bot.StartPeriodicReports(time.Hour)
	
	bot.mu.RLock()
	running := bot.reportRunning
	bot.mu.RUnlock()
	
	if running {
		t.Error("Periodic reports should not start when bot is disabled")
	}
}

func testStartPeriodicReportsAlreadyRunningInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	// Set reportRunning to true manually for testing
	bot.mu.Lock()
	bot.reportRunning = true
	bot.mu.Unlock()
	
	// Should not start again
	bot.StartPeriodicReports(time.Hour)
}

func testStopPeriodicReportsNotRunningInternal(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	// Should not panic when not running
	bot.StopPeriodicReports()
}

func testMessageStructureInternal(t *testing.T) {
	msg := Message{
		Chat: struct {
			ID int64 `json:"id"`
		}{ID: 123456789},
		Text:      "/start",
		From:      User{ID: 987654, FirstName: "Test", Username: "testuser"},
		MessageID: 42,
	}
	
	if msg.Chat.ID != 123456789 {
		t.Errorf("Chat.ID = %v, want %v", msg.Chat.ID, 123456789)
	}
	
	if msg.Text != "/start" {
		t.Errorf("Text = %v, want %v", msg.Text, "/start")
	}
	
	if msg.From.ID != 987654 {
		t.Errorf("From.ID = %v, want %v", msg.From.ID, 987654)
	}
	
	if msg.MessageID != 42 {
		t.Errorf("MessageID = %v, want %v", msg.MessageID, 42)
	}
}

func testUpdateStructureInternal(t *testing.T) {
	update := Update{
		UpdateID: 12345,
		Message:  Message{Text: "test"},
	}
	
	if update.UpdateID != 12345 {
		t.Errorf("UpdateID = %v, want %v", update.UpdateID, 12345)
	}
}

func testAPIResponseStructureInternal(t *testing.T) {
	resp := APIResponse{
		OK:          true,
		Result:      []Update{{UpdateID: 1}},
		Description: "success",
	}
	
	if !resp.OK {
		t.Error("OK should be true")
	}
	
	if len(resp.Result) != 1 {
		t.Errorf("Result count = %v, want %v", len(resp.Result), 1)
	}
}
