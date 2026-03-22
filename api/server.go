package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"github.com/DaniilSokolyuk/go-pcap2socks/stats"
)

type Server struct {
	mux        *http.ServeMux
	statsStore *stats.Store
	configPath string
	mu         sync.RWMutex
	enabled    bool
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type Status struct {
	Running       bool      `json:"running"`
	ProxyMode     string    `json:"proxy_mode"` // "socks5" or "direct"
	Devices       []Device  `json:"devices"`
	Traffic       Traffic   `json:"traffic"`
	Uptime        string    `json:"uptime"`
	StartTime     time.Time `json:"start_time"`
	SocksAvailable bool     `json:"socks_available"`
}

type Device struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	Connected bool   `json:"connected"`
}

type Traffic struct {
	Total     uint64 `json:"total_bytes"`
	Upload    uint64 `json:"upload_bytes"`
	Download  uint64 `json:"download_bytes"`
	Packets   uint64 `json:"packets"`
}

func NewServer(statsStore *stats.Store) *Server {
	executable, _ := os.Executable()
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Use provided stats store or get global one
	if statsStore == nil {
		// Will be set from main package
		statsStore = getGlobalStatsStore()
	}

	s := &Server{
		mux:        http.NewServeMux(),
		statsStore: statsStore,
		configPath: cfgFile,
		enabled:    true,
	}

	s.setupRoutes()
	return s
}

// getGlobalStatsStore gets the global stats store from main package
func getGlobalStatsStore() *stats.Store {
	// This will be implemented via interface
	return nil
}

func (s *Server) setupRoutes() {
	// Status endpoints
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/start", s.handleStart)
	s.mux.HandleFunc("/api/stop", s.handleStop)
	
	// Traffic endpoints
	s.mux.HandleFunc("/api/traffic", s.handleTraffic)
	s.mux.HandleFunc("/api/traffic/export", s.handleTrafficExport)
	
	// Device endpoints
	s.mux.HandleFunc("/api/devices", s.handleDevices)
	
	// Config endpoints
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/config/update", s.handleConfigUpdate)
	
	// Profile endpoints
	s.mux.HandleFunc("/api/profiles", s.handleProfiles)
	s.mux.HandleFunc("/api/profiles/switch", s.handleProfileSwitch)
	
	// UPnP endpoints
	s.mux.HandleFunc("/api/upnp", s.handleUPnP)
	s.mux.HandleFunc("/api/upnp/discover", s.handleUPnPDiscover)
	
	// Hotkey endpoints
	s.mux.HandleFunc("/api/hotkey", s.handleHotkey)
	s.mux.HandleFunc("/api/hotkey/toggle", s.handleHotkeyToggle)
	
	// WebSocket endpoint
	s.mux.HandleFunc("/ws", s.handleWebSocket)
	
	// Static files (web UI)
	s.mux.HandleFunc("/", s.handleStatic)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := Status{
		Running:        s.enabled,
		ProxyMode:      "socks5",
		Devices:        s.getDevices(),
		Traffic:        s.getTraffic(),
		Uptime:         time.Since(startTime).String(),
		StartTime:      startTime,
		SocksAvailable: true,
	}

	s.sendSuccess(w, status)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	s.enabled = true
	s.mu.Unlock()

	slog.Info("Service started via API")
	s.sendSuccess(w, "Service started")
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	s.enabled = false
	s.mu.Unlock()

	slog.Info("Service stopped via API")
	s.sendSuccess(w, "Service stopped")
}

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	traffic := s.getTraffic()
	s.sendSuccess(w, traffic)
}

func (s *Server) handleTrafficExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	csvData, err := s.statsStore.ExportCSV()
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=traffic.csv")
	w.Write([]byte(csvData))
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := s.getDevices()
	s.sendSuccess(w, devices)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var config interface{}
	json.Unmarshal(data, &config)
	s.sendSuccess(w, config)
}

func (s *Server) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var config map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		s.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("Config updated via API")
	s.sendSuccess(w, "Config updated")
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// List available profiles
	profilesDir := path.Join(path.Dir(s.configPath), "profiles")
	files, err := os.ReadDir(profilesDir)
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	profileList := []string{}
	for _, f := range files {
		if !f.IsDir() && path.Ext(f.Name()) == ".json" {
			profileList = append(profileList, f.Name())
		}
	}

	s.sendSuccess(w, profileList)
}

func (s *Server) handleProfileSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Profile string `json:"profile"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: Implement profile switching
	slog.Info("Profile switch requested", "profile", req.Profile)
	s.sendSuccess(w, "Profile switched: "+req.Profile)
}

func (s *Server) handleUPnP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement UPnP status
	s.sendSuccess(w, map[string]interface{}{
		"enabled": false,
		"message": "UPnP not configured",
	})
}

func (s *Server) handleUPnPDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement UPnP discovery
	s.sendSuccess(w, map[string]interface{}{
		"devices": []string{},
	})
}

func (s *Server) handleHotkey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement hotkey status
	s.sendSuccess(w, map[string]interface{}{
		"enabled": false,
		"toggle": "Ctrl+Alt+P",
	})
}

func (s *Server) handleHotkeyToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement hotkey toggle
	slog.Info("Proxy toggle requested via hotkey")
	s.sendSuccess(w, "Proxy toggled")
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Serve web UI files
	webPath := path.Join(path.Dir(s.configPath), "web")
	filePath := path.Join(webPath, r.URL.Path)
	
	if r.URL.Path == "/" {
		filePath = path.Join(webPath, "index.html")
	}

	http.ServeFile(w, r, filePath)
}

func (s *Server) sendSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
	})
}

func (s *Server) sendError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   message,
	})
}

func (s *Server) getDevices() []Device {
	if s.statsStore == nil {
		return []Device{}
	}

	devicesStats := s.statsStore.GetAllDevices()
	devices := make([]Device, 0, len(devicesStats))

	for _, ds := range devicesStats {
		ds.RLock()
		devices = append(devices, Device{
			IP:        ds.IP,
			MAC:       ds.MAC,
			Hostname:  ds.Hostname,
			Connected: ds.Connected,
		})
		ds.RUnlock()
	}

	return devices
}

func (s *Server) getTraffic() Traffic {
	if s.statsStore == nil {
		return Traffic{}
	}

	total, upload, download, packets := s.statsStore.GetTotalTraffic()
	return Traffic{
		Total:    total,
		Upload:   upload,
		Download: download,
		Packets:  packets,
	}
}

var startTime = time.Now()
