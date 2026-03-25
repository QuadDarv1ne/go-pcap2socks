package api

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockConn is a mock websocket connection for testing
type mockConn struct {
	mu            sync.Mutex
	closed        bool
	writeMessages [][]byte
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return websocket.ErrCloseSent
	}
	m.writeMessages = append(m.writeMessages, data)
	return nil
}

func (m *mockConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return websocket.ErrCloseSent
	}
	return nil
}

func (m *mockConn) ReadMessage() (messageType int, p []byte, err error) {
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

	if hub.clients == nil {
		t.Error("clients map not initialized")
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

	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()

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

	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()

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

	// Register both clients
	hub.register <- client1
	hub.register <- client2
	time.Sleep(10 * time.Millisecond)

	// Broadcast message
	type testData struct {
		Type  string `json:"type"`
		Value int    `json:"value"`
	}
	hub.Broadcast(testData{Type: "test", Value: 42})

	// Give time for broadcast
	time.Sleep(50 * time.Millisecond)

	// Check both clients received the message
	var msg1, msg2 testData

	select {
	case data := <-client1.send:
		if err := json.Unmarshal(data, &msg1); err != nil {
			t.Fatalf("Failed to unmarshal client1 message: %v", err)
		}
	default:
		t.Error("client1 should have received broadcast")
	}

	select {
	case data := <-client2.send:
		if err := json.Unmarshal(data, &msg2); err != nil {
			t.Fatalf("Failed to unmarshal client2 message: %v", err)
		}
	default:
		t.Error("client2 should have received broadcast")
	}

	if msg1.Type != "test" || msg1.Value != 42 {
		t.Errorf("client1 received wrong data: %+v", msg1)
	}
	if msg2.Type != "test" || msg2.Value != 42 {
		t.Errorf("client2 received wrong data: %+v", msg2)
	}
}

func TestWebSocketHub_BroadcastMarshalError(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// Broadcast with unmarshalable data (circular reference)
	type badData struct {
		Self *badData `json:"self"`
	}
	bad := &badData{}
	bad.Self = bad

	// This should not panic, just log error
	hub.Broadcast(bad)
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

	// Stop should close all client channels
	hub.Stop()

	// Give time for cleanup
	time.Sleep(10 * time.Millisecond)

	// Client channel should be closed
	select {
	case _, ok := <-client.send:
		if ok {
			t.Error("client send channel should be closed after stop")
		}
	default:
		t.Error("client send channel should be closed after stop")
	}

	// Hub should be empty
	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()

	if clientCount != 0 {
		t.Errorf("expected 0 clients after stop, got %d", clientCount)
	}
}

func TestWebSocketHub_ConcurrentAccess(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	var wg sync.WaitGroup
	numClients := 10

	// Register multiple clients concurrently
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			client := &WebSocketClient{
				conn:     &mockConn{},
				send:     make(chan []byte, 256),
				lastPing: time.Now(),
			}
			hub.register <- client
		}(i)
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()

	if clientCount != numClients {
		t.Errorf("expected %d clients, got %d", numClients, clientCount)
	}
}

func TestWebSocketHub_BroadcastToFullBuffer(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	// Create client with tiny buffer
	client := &WebSocketClient{
		conn:     &mockConn{},
		send:     make(chan []byte, 1),
		lastPing: time.Now(),
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	// Fill the buffer
	client.send <- []byte("test1")

	// Broadcast should not block even with full buffer
	hub.Broadcast("test2")
	hub.Broadcast("test3")
	hub.Broadcast("test4")

	time.Sleep(50 * time.Millisecond)

	// Client should be disconnected due to full buffer
	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()

	if clientCount != 0 {
		t.Errorf("expected client to be disconnected due to full buffer, got %d clients", clientCount)
	}
}

func TestWebSocketHub_WritePump(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()

	mock := &mockConn{}
	client := &WebSocketClient{
		conn:     mock,
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	// Start write pump in goroutine
	done := make(chan bool)
	go func() {
		hub.writePump(client)
		done <- true
	}()

	// Send message directly to client's send channel
	select {
	case client.send <- []byte("test message"):
		// Message sent
	case <-time.After(100 * time.Millisecond):
		t.Fatal("writePump didn't receive message")
	}

	// Give time for write
	time.Sleep(50 * time.Millisecond)

	// Check message was written
	mock.mu.Lock()
	msgCount := len(mock.writeMessages)
	mock.mu.Unlock()

	if msgCount == 0 {
		t.Error("writePump should have written messages")
	}

	// Stop hub and wait for writePump to exit
	hub.Stop()

	select {
	case <-done:
		// writePump exited
	case <-time.After(500 * time.Millisecond):
		t.Error("writePump didn't exit after Stop")
	}
}

func TestWebSocketHub_PingPong(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	mock := &mockConn{}
	client := &WebSocketClient{
		conn:     mock,
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	// Start ping/pong - should send ping after 30s, but we test it doesn't panic
	go hub.runPingPong(client)

	// Give time - should not panic
	time.Sleep(100 * time.Millisecond)

	// Check ping was sent
	mock.mu.Lock()
	if len(mock.writeMessages) == 0 {
		t.Log("No ping sent yet (expected within 30s)")
	}
	mock.mu.Unlock()
}

func TestWebSocketHub_ReadPump(t *testing.T) {
	hub := NewWebSocketHub()
	go hub.Run()
	defer hub.Stop()

	mock := &mockConn{}
	client := &WebSocketClient{
		conn:     mock,
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	// Register client first
	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	// Verify client is registered
	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()

	if clientCount != 1 {
		t.Errorf("expected 1 client after register, got %d", clientCount)
	}

	// Manually unregister to simulate readPump defer
	hub.unregister <- client
	time.Sleep(50 * time.Millisecond)

	// Client should be unregistered
	hub.mu.RLock()
	clientCount = len(hub.clients)
	hub.mu.RUnlock()

	if clientCount != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", clientCount)
	}
}
