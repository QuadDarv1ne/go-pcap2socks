//go:build ignore

package telegram

import (
	"testing"
	"time"
)

func TestNewBot(t *testing.T) {
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

func TestBot_IsEnabled(t *testing.T) {
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

func TestBot_Stop(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	// Should not panic
	bot.Stop()
	
	if bot.enabled {
		t.Error("Bot should be disabled after Stop()")
	}
}

func TestBot_StopPolling(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	// Should not panic when not running
	bot.StopPolling()
}

func TestSendMessage_Disabled(t *testing.T) {
	bot := NewBot("", "")
	
	err := bot.SendMessage("test message")
	if err == nil {
		t.Error("SendMessage() should return error when bot is disabled")
	}
}

func TestSendNotification_Disabled(t *testing.T) {
	bot := NewBot("", "")
	
	// Should not panic
	bot.SendNotification("title", "message")
}

func TestRegisterCommand(t *testing.T) {
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

func TestHandleMessage_Empty(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	msg := Message{
		Text: "",
	}
	
	// Should not panic with empty message
	bot.handleMessage(msg)
}

func TestHandleMessage_Unauthorized(t *testing.T) {
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

func TestHandleMessage_UnknownCommand(t *testing.T) {
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

func TestHandleMessage_KnownCommand(t *testing.T) {
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

func TestHandleMessage_WithArgs(t *testing.T) {
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

func TestDefaultHandlers(t *testing.T) {
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

func TestSetStatusHandler(t *testing.T) {
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

func TestSetTrafficHandler(t *testing.T) {
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

func TestSetDevicesHandler(t *testing.T) {
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

func TestSetServiceHandlers(t *testing.T) {
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

func TestHandleStatus_NoHandler(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	result := bot.handleStatus(bot, []string{})
	expected := "Status handler not configured"
	
	if result != expected {
		t.Errorf("handleStatus() = %v, want %v", result, expected)
	}
}

func TestHandleTraffic_NoHandler(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	result := bot.handleTraffic(bot, []string{})
	expected := "Traffic handler not configured"
	
	if result != expected {
		t.Errorf("handleTraffic() = %v, want %v", result, expected)
	}
}

func TestHandleDevices_NoHandler(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	result := bot.handleDevices(bot, []string{})
	expected := "Devices handler not configured"
	
	if result != expected {
		t.Errorf("handleDevices() = %v, want %v", result, expected)
	}
}

func TestHandleDiscordStatus_NoDiscord(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	result := bot.handleDiscordStatus(bot, []string{})
	expected := "🔴 Discord webhook не настроен"
	
	if result != expected {
		t.Errorf("handleDiscordStatus() = %v, want %v", result, expected)
	}
}

func TestStartPeriodicReports_Disabled(t *testing.T) {
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

func TestStartPeriodicReports_AlreadyRunning(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	// Set reportRunning to true manually for testing
	bot.mu.Lock()
	bot.reportRunning = true
	bot.mu.Unlock()
	
	// Should not start again
	bot.StartPeriodicReports(time.Hour)
}

func TestStopPeriodicReports_NotRunning(t *testing.T) {
	bot := NewBot("123456789:ABCdefGHIjklMNOpqrsTUVwxyz", "123456789")
	
	// Should not panic when not running
	bot.StopPeriodicReports()
}

func TestMessage_Structure(t *testing.T) {
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

func TestUpdate_Structure(t *testing.T) {
	update := Update{
		UpdateID: 12345,
		Message:  Message{Text: "test"},
	}
	
	if update.UpdateID != 12345 {
		t.Errorf("UpdateID = %v, want %v", update.UpdateID, 12345)
	}
}

func TestAPIResponse_Structure(t *testing.T) {
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
