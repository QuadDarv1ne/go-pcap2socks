package discord

import (
	"testing"
)

func TestNewWebhookClient(t *testing.T) {
	tests := []struct {
		name      string
		webhookURL string
		wantEnabled bool
	}{
		{
			name:      "valid webhook URL",
			webhookURL: "https://discord.com/api/webhooks/123456789/abcdef",
			wantEnabled: true,
		},
		{
			name:      "empty webhook URL",
			webhookURL: "",
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewWebhookClient(tt.webhookURL)
			
			if client == nil {
				t.Fatal("NewWebhookClient returned nil")
			}
			
			if client.enabled != tt.wantEnabled {
				t.Errorf("enabled = %v, want %v", client.enabled, tt.wantEnabled)
			}
			
			if client.webhookURL != tt.webhookURL {
				t.Errorf("webhookURL = %v, want %v", client.webhookURL, tt.webhookURL)
			}
			
			if client.httpClient == nil {
				t.Error("httpClient is nil")
			}
		})
	}
}

func TestWebhookClient_IsEnabled(t *testing.T) {
	tests := []struct {
		name      string
		webhookURL string
		wantEnabled bool
	}{
		{
			name:      "enabled webhook",
			webhookURL: "https://discord.com/api/webhooks/123456789/abcdef",
			wantEnabled: true,
		},
		{
			name:      "disabled webhook",
			webhookURL: "",
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewWebhookClient(tt.webhookURL)
			got := client.IsEnabled()
			
			if got != tt.wantEnabled {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.wantEnabled)
			}
		})
	}
}

func TestSend_Disabled(t *testing.T) {
	client := NewWebhookClient("")
	
	err := client.Send("test message")
	if err == nil {
		t.Error("Send() should return error when webhook is disabled")
	}
	
	expectedErr := "webhook disabled"
	if err.Error() != expectedErr {
		t.Errorf("Send() error = %v, want %v", err.Error(), expectedErr)
	}
}

func TestSendEmbed_Disabled(t *testing.T) {
	client := NewWebhookClient("")
	
	embed := Embed{
		Title: "Test",
	}
	
	err := client.SendEmbed(embed)
	if err == nil {
		t.Error("SendEmbed() should return error when webhook is disabled")
	}
}

func TestSendNotification_Disabled(t *testing.T) {
	client := NewWebhookClient("")
	
	err := client.SendNotification("title", "description", 0)
	if err == nil {
		t.Error("SendNotification() should return error when webhook is disabled")
	}
}

func TestSendInfo(t *testing.T) {
	client := NewWebhookClient("")
	
	err := client.SendInfo("Test Title", "Test Description")
	if err == nil {
		t.Error("SendInfo() should return error when webhook is disabled")
	}
}

func TestSendSuccess(t *testing.T) {
	client := NewWebhookClient("")
	
	err := client.SendSuccess("Test Title", "Test Description")
	if err == nil {
		t.Error("SendSuccess() should return error when webhook is disabled")
	}
}

func TestSendWarning(t *testing.T) {
	client := NewWebhookClient("")
	
	err := client.SendWarning("Test Title", "Test Description")
	if err == nil {
		t.Error("SendWarning() should return error when webhook is disabled")
	}
}

func TestSendError(t *testing.T) {
	client := NewWebhookClient("")
	
	err := client.SendError("Test Title", "Test Description")
	if err == nil {
		t.Error("SendError() should return error when webhook is disabled")
	}
}

func TestSendStatus_Disabled(t *testing.T) {
	client := NewWebhookClient("")
	
	err := client.SendStatus(true, 5, "100 MB")
	if err == nil {
		t.Error("SendStatus() should return error when webhook is disabled")
	}
}

func TestSendDeviceNotification_Disabled(t *testing.T) {
	client := NewWebhookClient("")
	
	err := client.SendDeviceNotification("connected", "192.168.1.1", "aa:bb:cc:dd:ee:ff")
	if err == nil {
		t.Error("SendDeviceNotification() should return error when webhook is disabled")
	}
}

func TestEmbed_Structure(t *testing.T) {
	embed := Embed{
		Title:       "Test Title",
		Description: "Test Description",
		Color:       3447003,
		Fields: []EmbedField{
			{
				Name:   "Field 1",
				Value:  "Value 1",
				Inline: true,
			},
		},
		Footer: EmbedFooter{
			Text: "Test Footer",
		},
		Timestamp: "2026-03-23T12:00:00Z",
	}
	
	if embed.Title != "Test Title" {
		t.Errorf("Title = %v, want %v", embed.Title, "Test Title")
	}
	
	if embed.Description != "Test Description" {
		t.Errorf("Description = %v, want %v", embed.Description, "Test Description")
	}
	
	if embed.Color != 3447003 {
		t.Errorf("Color = %v, want %v", embed.Color, 3447003)
	}
	
	if len(embed.Fields) != 1 {
		t.Errorf("Fields count = %v, want %v", len(embed.Fields), 1)
	}
	
	if embed.Footer.Text != "Test Footer" {
		t.Errorf("Footer.Text = %v, want %v", embed.Footer.Text, "Test Footer")
	}
}

func TestPayload_Structure(t *testing.T) {
	payload := Payload{
		Content:   "Test content",
		Embeds:    []Embed{{Title: "Test"}},
		Username:  "go-pcap2socks",
		AvatarURL: "https://example.com/avatar.png",
	}
	
	if payload.Content != "Test content" {
		t.Errorf("Content = %v, want %v", payload.Content, "Test content")
	}
	
	if payload.Username != "go-pcap2socks" {
		t.Errorf("Username = %v, want %v", payload.Username, "go-pcap2socks")
	}
	
	if len(payload.Embeds) != 1 {
		t.Errorf("Embeds count = %v, want %v", len(payload.Embeds), 1)
	}
}

func TestSetStatusHandler(t *testing.T) {
	client := NewWebhookClient("https://discord.com/api/webhooks/123456789/abcdef")
	
	testStatus := "Test status message"
	client.SetStatusHandler(func() string {
		return testStatus
	})
	
	got := client.GetStatusMessage()
	if got != testStatus {
		t.Errorf("GetStatusMessage() = %v, want %v", got, testStatus)
	}
}

func TestGetStatusMessage_NoHandler(t *testing.T) {
	client := NewWebhookClient("https://discord.com/api/webhooks/123456789/abcdef")
	
	got := client.GetStatusMessage()
	expected := "Status handler not configured"
	
	if got != expected {
		t.Errorf("GetStatusMessage() = %v, want %v", got, expected)
	}
}

func TestLog_Disabled(t *testing.T) {
	client := NewWebhookClient("")
	
	// Should not panic when disabled
	client.Log("INFO", "Test log message")
	client.Log("WARN", "Test warning")
	client.Log("ERROR", "Test error")
}

func TestLog_Levels(t *testing.T) {
	client := NewWebhookClient("https://discord.com/api/webhooks/123456789/abcdef")
	
	// Test different log levels
	levels := []string{"INFO", "WARN", "ERROR", "DEBUG"}
	
	for _, level := range levels {
		// Should not panic
		client.Log(level, "Test message")
	}
}
