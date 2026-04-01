package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockConn is a mock websocket connection for testing
type mockConn struct {
	closed bool
}

func (m *mockConn) Close() error                                        { m.closed = true; return nil }
func (m *mockConn) WriteMessage(mt int, data []byte) error              { return nil }
func (m *mockConn) WriteControl(mt int, data []byte, d time.Time) error { return nil }
func (m *mockConn) ReadMessage() (int, []byte, error) {
	return websocket.TextMessage, []byte("{}"), nil
}
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }

func TestNewWebSocketHub(t *testing.T) {
	hub := NewWebSocketHub()

	if hub == nil {
		t.Fatal("NewWebSocketHub returned nil")
	}

	if hub.broadcast == nil {
		t.Error("broadcast channel not initialized")
	}

	if hub.register == nil {
		t.Error("register channel not initialized")
	}

	if hub.unregister == nil {
		t.Error("unregister channel not initialized")
	}

	if hub.stopChan == nil {
		t.Error("stopChan not initialized")
	}
}

func TestWebSocketHub_RegisterClient(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	client := &WebSocketClient{
		conn:     &mockConn{},
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	hub.register <- client
	time.Sleep(50 * time.Millisecond)

	clientCount := hub.GetClientCount()

	if clientCount != 1 {
		t.Errorf("expected 1 client, got %d", clientCount)
	}
}

func TestWebSocketHub_UnregisterClient(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	client := &WebSocketClient{
		conn:     &mockConn{},
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	// Register
	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	// Unregister
	hub.unregister <- client
	time.Sleep(10 * time.Millisecond)

	clientCount := hub.GetClientCount()

	if clientCount != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", clientCount)
	}

	// Channel should be closed
	select {
	case _, ok := <-client.send:
		if ok {
			t.Error("client send channel should be closed")
		}
	default:
		t.Error("client send channel should be closed, but didn't block")
	}
}

func TestWebSocketHub_Broadcast(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// Create clients with buffered channels
	client1 := &WebSocketClient{
		conn:     &mockConn{},
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	client2 := &WebSocketClient{
		conn:     &mockConn{},
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	hub.register <- client1
	hub.register <- client2
	time.Sleep(10 * time.Millisecond)

	// Broadcast a message
	type testData struct {
		Type  string `json:"type"`
		Value int    `json:"value"`
	}
	hub.Broadcast(testData{Type: "test", Value: 42})

	// Give time for message to be sent
	time.Sleep(50 * time.Millisecond)

	// Client 1 should have received message
	select {
	case msg := <-client1.send:
		var data testData
		if err := json.Unmarshal(msg, &data); err != nil {
			t.Errorf("failed to unmarshal message: %v", err)
		}
		if data.Type != "test" || data.Value != 42 {
			t.Errorf("unexpected message: %+v", data)
		}
	default:
		t.Error("client1 should have received message")
	}

	// Client 2 should have received message
	select {
	case msg := <-client2.send:
		var data testData
		if err := json.Unmarshal(msg, &data); err != nil {
			t.Errorf("failed to unmarshal message: %v", err)
		}
		if data.Type != "test" || data.Value != 42 {
			t.Errorf("unexpected message: %+v", data)
		}
	default:
		t.Error("client2 should have received message")
	}
}

func TestWebSocketHub_BroadcastMarshalError(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// Broadcast something that can't be marshaled
	hub.Broadcast(make(chan int))

	// Should not panic
	time.Sleep(10 * time.Millisecond)
}

func TestWebSocketHub_Stop(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()

	client := &WebSocketClient{
		conn:     &mockConn{},
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	hub.Stop()

	// Give time for stop to process
	time.Sleep(50 * time.Millisecond)

	// Client channel should be closed
	select {
	case _, ok := <-client.send:
		if ok {
			t.Error("client send channel should be closed after Stop")
		}
	default:
		t.Error("client send channel should be closed after Stop")
	}
}

func TestWebSocketHub_GetClientCount(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	if hub.GetClientCount() != 0 {
		t.Errorf("expected 0 clients initially, got %d", hub.GetClientCount())
	}

	client := &WebSocketClient{
		conn:     &mockConn{},
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	if hub.GetClientCount() != 1 {
		t.Errorf("expected 1 client after register, got %d", hub.GetClientCount())
	}

	hub.unregister <- client
	time.Sleep(10 * time.Millisecond)

	if hub.GetClientCount() != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", hub.GetClientCount())
	}
}

func TestWebSocketHub_ConcurrentAccess(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// Register multiple clients concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			client := &WebSocketClient{
				conn:     &mockConn{},
				send:     make(chan []byte, 256),
				lastPing: time.Now(),
			}
			hub.register <- client
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	time.Sleep(50 * time.Millisecond)

	if hub.GetClientCount() != 10 {
		t.Errorf("expected 10 clients, got %d", hub.GetClientCount())
	}
}
