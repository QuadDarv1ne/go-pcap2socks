package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local use
	},
}

// wsConn defines the interface for WebSocket connections (for testability)
// websocket.Conn implements all methods except SetDeadline which we handle separately
type wsConn interface {
	Close() error
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	ReadMessage() (messageType int, p []byte, err error)
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	conn     wsConn
	send     chan []byte
	lastPing time.Time
}

// WebSocketHub manages WebSocket connections
// Optimized with sync.Map for lock-free client management
type WebSocketHub struct {
	clients     sync.Map // map[*WebSocketClient]bool
	broadcast   chan []byte
	register    chan *WebSocketClient
	unregister  chan *WebSocketClient
	stopChan    chan struct{}
	stopOnce    sync.Once
	clientCount atomic.Int32
	wg          sync.WaitGroup // WaitGroup for goroutine cleanup

	// Pool for client slices to reduce allocations
	clientSlicePool sync.Pool
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub() *WebSocketHub {
	h := &WebSocketHub{
		broadcast:  make(chan []byte, 10000),          // Increased buffer for better burst handling
		register:   make(chan *WebSocketClient, 1000), // Increased buffer
		unregister: make(chan *WebSocketClient, 1000), // Increased buffer
		stopChan:   make(chan struct{}),
	}
	h.clientSlicePool.New = func() any {
		return make([]*WebSocketClient, 0, 16)
	}
	return h
}

// Run starts the hub's main loop
func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients.Store(client, true)
			h.clientCount.Add(1)

		case client := <-h.unregister:
			h.clients.Delete(client)
			h.clientCount.Add(-1)
			close(client.send)

		case message := <-h.broadcast:
			// Get slice from pool to reduce allocations
			clientsToClose := h.clientSlicePool.Get().([]*WebSocketClient)
			clientsToClose = clientsToClose[:0]

			h.clients.Range(func(k, v any) bool {
				client := k.(*WebSocketClient)
				select {
				case client.send <- message:
				default:
					// Client buffer full, mark for disconnect
					clientsToClose = append(clientsToClose, client)
				}
				return true
			})

			// Close clients outside lock to prevent deadlock
			if len(clientsToClose) > 0 {
				for _, client := range clientsToClose {
					h.clients.Delete(client)
					h.clientCount.Add(-1)
					close(client.send)
				}
			}
			// Return slice to pool after clearing
			h.clientSlicePool.Put(clientsToClose[:0])

		case <-h.stopChan:
			h.clients.Range(func(k, v any) bool {
				client := k.(*WebSocketClient)
				close(client.send)
				return true
			})
			h.clients = sync.Map{}
			h.clientCount.Store(0)
			return
		}
	}
}

// Stop stops the hub and waits for all goroutines to finish
func (h *WebSocketHub) Stop() {
	h.stopOnce.Do(func() {
		close(h.stopChan)
	})
	// Wait for all goroutines to finish
	h.wg.Wait()
}

// Broadcast sends a message to all connected clients
// Optimized with non-blocking send
func (h *WebSocketHub) Broadcast(data interface{}) {
	message, err := json.Marshal(data)
	if err != nil {
		slog.Error("WebSocket broadcast marshal error", "err", err)
		return
	}

	select {
	case h.broadcast <- message:
	default:
		// Broadcast channel full - skip this update
	}
}

// GetClientCount returns the number of connected clients
func (h *WebSocketHub) GetClientCount() int {
	return int(h.clientCount.Load())
}

// HandleWebSocket handles WebSocket connections
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade error", "err", err)
		return
	}

	client := &WebSocketClient{
		conn:     conn,
		send:     make(chan []byte, 10000), // Increased buffer for better burst handling
		lastPing: time.Now(),
	}

	s.wsHub.register <- client

	// Start ping/pong heartbeat
	s.wsHub.wg.Add(1)
	go s.wsHub.runPingPong(client)

	// Start write pump
	s.wsHub.wg.Add(1)
	go s.wsHub.writePump(client)

	// Start read pump (handle client messages)
	s.wsHub.wg.Add(1)
	go s.wsHub.readPump(client)
}

func (h *WebSocketHub) runPingPong(client *WebSocketClient) {
	defer h.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
			client.lastPing = time.Now()
		case <-h.stopChan:
			return
		}
	}
}

func (h *WebSocketHub) writePump(client *WebSocketClient) {
	defer func() {
		h.wg.Done()
		client.conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			if !ok {
				// Hub closed channel
				client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-h.stopChan:
			return
		}
	}
}

func (h *WebSocketHub) readPump(client *WebSocketClient) {
	defer func() {
		h.wg.Done()
		h.unregister <- client
		client.conn.Close()
	}()

	// Set read deadline so ReadMessage unblocks periodically to check stopChan
	readDeadline := 2 * time.Second

	for {
		select {
		case <-h.stopChan:
			return
		default:
			// Set deadline so ReadMessage doesn't block forever
			client.conn.SetReadDeadline(time.Now().Add(readDeadline))
			_, message, err := client.conn.ReadMessage()
			if err != nil {
				// Check if it's just a deadline exceeded (continue loop)
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) ||
					websocket.IsUnexpectedCloseError(err) {
					return
				}
				// For deadline or other errors, check stopChan
				select {
				case <-h.stopChan:
					return
				default:
					// Not stopped, continue loop
					continue
				}
			}

			// Reset deadline for next read
			client.conn.SetReadDeadline(time.Time{})

			// Handle client messages (e.g., subscribe/unsubscribe)
			var msg struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			// Process message types
			switch msg.Type {
			case "ping":
				client.conn.WriteMessage(websocket.PongMessage, []byte("pong"))
			}
		}
	}
}
