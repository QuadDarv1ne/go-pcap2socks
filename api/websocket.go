package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
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
type WebSocketHub struct {
	mu       sync.RWMutex
	clients  map[*WebSocketClient]bool
	broadcast chan []byte
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	stopChan   chan struct{}
	stopOnce   sync.Once
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[*WebSocketClient]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		stopChan:   make(chan struct{}),
	}
}

// Run starts the hub's main loop
func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			slog.Debug("WebSocket client connected", "total", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			slog.Debug("WebSocket client disconnected", "total", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, disconnect
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()

		case <-h.stopChan:
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
			}
			h.clients = make(map[*WebSocketClient]bool)
			h.mu.Unlock()
			return
		}
	}
}

// Stop stops the hub
func (h *WebSocketHub) Stop() {
	h.stopOnce.Do(func() {
		close(h.stopChan)
	})
}

// Broadcast sends a message to all connected clients
func (h *WebSocketHub) Broadcast(data interface{}) {
	message, err := json.Marshal(data)
	if err != nil {
		slog.Error("WebSocket broadcast marshal error", "err", err)
		return
	}

	select {
	case h.broadcast <- message:
	default:
		// Broadcast channel full
	}
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
		send:     make(chan []byte, 256),
		lastPing: time.Now(),
	}

	s.wsHub.register <- client

	// Start ping/pong heartbeat
	go s.wsHub.runPingPong(client)

	// Start write pump
	go s.wsHub.writePump(client)

	// Start read pump (handle client messages)
	go s.wsHub.readPump(client)
}

func (h *WebSocketHub) runPingPong(client *WebSocketClient) {
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
		h.unregister <- client
		client.conn.Close()
	}()

	for {
		select {
		case <-h.stopChan:
			return
		default:
			_, message, err := client.conn.ReadMessage()
			if err != nil {
				return
			}

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
