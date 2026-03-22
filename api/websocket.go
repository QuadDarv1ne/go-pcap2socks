package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	wsUpgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	wsClients   = make(map[*websocket.Conn]bool)
	wsClientsMu sync.RWMutex
)

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type      string      `json:"type"`
	Timestamp string      `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// handleWebSocket upgrades HTTP connection to WebSocket
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade error", "err", err)
		return
	}

	// Register client
	wsClientsMu.Lock()
	wsClients[conn] = true
	wsClientsMu.Unlock()

	// Send initial status
	s.sendWSMessage(conn, "status", s.getStatusData())

	// Handle client messages
	go s.handleWSMessages(conn)

	// Send periodic updates
	s.sendWSUpdates(conn)
}

func (s *Server) handleWSMessages(conn *websocket.Conn) {
	defer func() {
		wsClientsMu.Lock()
		delete(wsClients, conn)
		wsClientsMu.Unlock()
		conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg struct {
			Type   string          `json:"type"`
			Action string          `json:"action"`
			Data   json.RawMessage `json:"data"`
		}

		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		// Handle different message types
		switch msg.Action {
		case "start":
			s.handleStart(nil, nil)
			s.sendWSMessage(conn, "status", s.getStatusData())
		case "stop":
			s.handleStop(nil, nil)
			s.sendWSMessage(conn, "status", s.getStatusData())
		case "refresh":
			s.sendWSMessage(conn, "status", s.getStatusData())
		}
	}
}

func (s *Server) sendWSUpdates(conn *websocket.Conn) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !wsClients[conn] {
			return
		}

		s.sendWSMessage(conn, "status", s.getStatusData())
		s.sendWSMessage(conn, "traffic", s.getTraffic())
	}
}

func (s *Server) sendWSMessage(conn *websocket.Conn, msgType string, data interface{}) {
	msg := WSMessage{
		Type:      msgType,
		Timestamp: time.Now().Format(time.RFC3339),
		Data:      data,
	}

	wsClientsMu.RLock()
	defer wsClientsMu.RUnlock()

	if err := conn.WriteJSON(msg); err != nil {
		slog.Debug("WebSocket write error", "err", err)
	}
}

// BroadcastWSMessage sends a message to all connected clients
func (s *Server) BroadcastWSMessage(msgType string, data interface{}) {
	msg := WSMessage{
		Type:      msgType,
		Timestamp: time.Now().Format(time.RFC3339),
		Data:      data,
	}

	wsClientsMu.RLock()
	defer wsClientsMu.RUnlock()

	for conn := range wsClients {
		if err := conn.WriteJSON(msg); err != nil {
			slog.Debug("WebSocket broadcast error", "err", err)
		}
	}
}

func (s *Server) getStatusData() interface{} {
	return Status{
		Running:        s.enabled,
		ProxyMode:      "socks5",
		Devices:        s.getDevices(),
		Traffic:        s.getTraffic(),
		Uptime:         time.Since(startTime).String(),
		StartTime:      startTime,
		SocksAvailable: true,
	}
}
