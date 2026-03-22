package api

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
)

type Server struct {
	mux          *http.ServeMux
	statsStore   *stats.Store
	profileMgr   *profiles.Manager
	upnpMgr      *upnpmanager.Manager
	configPath   string
	mu           sync.RWMutex
	enabled      bool
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

func NewServer(statsStore *stats.Store, profileMgr *profiles.Manager, upnpMgr *upnpmanager.Manager) *Server {
	executable, _ := os.Executable()
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Use provided stats store or get global one
	if statsStore == nil {
		// Will be set from main package
		statsStore = getGlobalStatsStore()
	}

	s := &Server{
		mux:          http.NewServeMux(),
		statsStore:   statsStore,
		profileMgr:   profileMgr,
		upnpMgr:      upnpMgr,
		configPath:   cfgFile,
		enabled:      true,
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

	// Logs endpoints
	s.mux.HandleFunc("/api/logs", s.handleLogs)
	s.mux.HandleFunc("/api/logs/export", s.handleLogsExport)

	// Device endpoints
	s.mux.HandleFunc("/api/devices", s.handleDevices)

	// Config endpoints
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/config/update", s.handleConfigUpdate)
	s.mux.HandleFunc("/api/config/reload", s.handleConfigReload)

	// Profile endpoints
	s.mux.HandleFunc("/api/profiles", s.handleProfiles)
	s.mux.HandleFunc("/api/profiles/switch", s.handleProfileSwitch)

	// UPnP endpoints
	s.mux.HandleFunc("/api/upnp", s.handleUPnP)
	s.mux.HandleFunc("/api/upnp/discover", s.handleUPnPDiscover)
	s.mux.HandleFunc("/api/upnp/add", s.handleUPnPAddPort)
	s.mux.HandleFunc("/api/upnp/remove", s.handleUPnPRemovePort)
	s.mux.HandleFunc("/api/upnp/apply", s.handleUPnPApplyMappings)

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

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get last N lines from logs.txt
	logPath := path.Join(path.Dir(s.configPath), "logs.txt")
	lines, err := readLastLines(logPath, 100)
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.sendSuccess(w, map[string]interface{}{
		"lines": lines,
		"count": len(lines),
	})
}

func (s *Server) handleLogsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logPath := path.Join(path.Dir(s.configPath), "logs.txt")
	data, err := os.ReadFile(logPath)
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", "attachment; filename=logs.txt")
	w.Write(data)
}

func (s *Server) handleConfigReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slog.Info("Config reload requested via API")
	
	// Send success response
	s.sendSuccess(w, map[string]interface{}{
		"message": "Config reload requested. Restart service to apply changes.",
		"status": "pending_restart",
	})
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

	// If profile manager is available, use it
	if s.profileMgr != nil {
		profileList, err := s.profileMgr.ListProfiles()
		if err != nil {
			s.sendError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Get current profile
		currentProfile := s.profileMgr.GetCurrentProfile()

		s.sendSuccess(w, map[string]interface{}{
			"profiles":      profileList,
			"current":       currentProfile,
		})
		return
	}

	// Fallback: list files directly
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

	s.sendSuccess(w, map[string]interface{}{
		"profiles": profileList,
		"current":  "default",
	})
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

	// If profile manager is available, use it
	if s.profileMgr != nil {
		err := s.profileMgr.SwitchProfile(req.Profile)
		if err != nil {
			s.sendError(w, err.Error(), http.StatusNotFound)
			return
		}

		slog.Info("Profile switched via API", "profile", req.Profile)
		s.sendSuccess(w, map[string]interface{}{
			"message":  "Profile switched: " + req.Profile,
			"profile":  req.Profile,
			"restart":  true,
		})
		return
	}

	// Fallback: manual switch
	profilesDir := path.Join(path.Dir(s.configPath), "profiles")
	profileFile := path.Join(profilesDir, req.Profile+".json")

	// Check if profile exists
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		s.sendError(w, "Profile not found: "+req.Profile, http.StatusNotFound)
		return
	}

	// Read profile
	data, err := os.ReadFile(profileFile)
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write to config
	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("Profile switched via API", "profile", req.Profile)
	s.sendSuccess(w, map[string]interface{}{
		"message": "Profile switched: " + req.Profile,
		"profile": req.Profile,
		"restart": true,
	})
}

func (s *Server) handleProfileCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Config      interface{} `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		s.sendError(w, "Profile name is required", http.StatusBadRequest)
		return
	}

	// If profile manager is available, use it
	if s.profileMgr != nil {
		profile := profiles.Profile{
			Name:        req.Name,
			Description: req.Description,
			Config:      req.Config,
		}

		err := s.profileMgr.SaveProfile(req.Name, profile)
		if err != nil {
			s.sendError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		slog.Info("Profile created via API", "profile", req.Name)
		s.sendSuccess(w, map[string]interface{}{
			"message": "Profile created: " + req.Name,
			"profile": req.Name,
		})
		return
	}

	s.sendError(w, "Profile manager not available", http.StatusInternalServerError)
}

func (s *Server) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
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

	// If profile manager is available, use it
	if s.profileMgr != nil {
		err := s.profileMgr.DeleteProfile(req.Profile)
		if err != nil {
			s.sendError(w, err.Error(), http.StatusBadRequest)
			return
		}

		slog.Info("Profile deleted via API", "profile", req.Profile)
		s.sendSuccess(w, map[string]interface{}{
			"message": "Profile deleted: " + req.Profile,
			"profile": req.Profile,
		})
		return
	}

	s.sendError(w, "Profile manager not available", http.StatusInternalServerError)
}

func (s *Server) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	profileName := r.URL.Query().Get("name")
	if profileName == "" {
		s.sendError(w, "Profile name is required", http.StatusBadRequest)
		return
	}

	// If profile manager is available, use it
	if s.profileMgr != nil {
		config, err := s.profileMgr.LoadProfile(profileName)
		if err != nil {
			s.sendError(w, err.Error(), http.StatusNotFound)
			return
		}

		s.sendSuccess(w, map[string]interface{}{
			"name":   profileName,
			"config": config,
		})
		return
	}

	s.sendError(w, "Profile manager not available", http.StatusInternalServerError)
}

func (s *Server) handleUPnP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.upnpMgr == nil {
		s.sendSuccess(w, map[string]interface{}{
			"enabled":         false,
			"message":         "UPnP not configured or not available",
			"active_mappings": 0,
		})
		return
	}

	externalIP, _ := s.upnpMgr.GetExternalIP()
	activeMappings := s.upnpMgr.GetActiveMappings()

	s.sendSuccess(w, map[string]interface{}{
		"enabled":         true,
		"external_ip":     externalIP,
		"active_mappings": activeMappings,
		"internal_ip":     "", // Could be added to manager
	})
}

func (s *Server) handleUPnPDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Refresh port mappings
	if s.upnpMgr != nil {
		if err := s.upnpMgr.RefreshMappings(); err != nil {
			s.sendError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.sendSuccess(w, map[string]interface{}{
			"message":         "UPnP mappings refreshed",
			"active_mappings": s.upnpMgr.GetActiveMappings(),
		})
		return
	}

	s.sendError(w, "UPnP not available", http.StatusInternalServerError)
}

func (s *Server) handleUPnPAddPort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Protocol     string `json:"protocol"` // TCP, UDP, both
		ExternalPort int    `json:"externalPort"`
		InternalPort int    `json:"internalPort"`
		Description  string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.upnpMgr == nil {
		s.sendError(w, "UPnP not available", http.StatusInternalServerError)
		return
	}

	protocol := req.Protocol
	if protocol == "" {
		protocol = "both"
	}

	description := req.Description
	if description == "" {
		description = "go-pcap2socks"
	}

	err := s.upnpMgr.AddDynamicMapping(protocol, req.ExternalPort, req.InternalPort, description)
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.sendSuccess(w, map[string]interface{}{
		"message":          "Port mapping added",
		"protocol":         protocol,
		"external_port":    req.ExternalPort,
		"internal_port":    req.InternalPort,
		"active_mappings":  s.upnpMgr.GetActiveMappings(),
	})
}

func (s *Server) handleUPnPRemovePort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Protocol string `json:"protocol"` // TCP, UDP
		Port     int    `json:"port"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.upnpMgr == nil {
		s.sendError(w, "UPnP not available", http.StatusInternalServerError)
		return
	}

	err := s.upnpMgr.RemoveDynamicMapping(req.Protocol, req.Port)
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.sendSuccess(w, map[string]interface{}{
		"message":          "Port mapping removed",
		"protocol":         req.Protocol,
		"port":             req.Port,
		"active_mappings":  s.upnpMgr.GetActiveMappings(),
	})
}

func (s *Server) handleUPnPApplyMappings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.upnpMgr == nil {
		s.sendError(w, "UPnP not available", http.StatusInternalServerError)
		return
	}

	err := s.upnpMgr.ApplyPortMappings()
	if err != nil {
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.sendSuccess(w, map[string]interface{}{
		"message":          "Port mappings applied",
		"active_mappings":  s.upnpMgr.GetActiveMappings(),
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

// readLastLines reads last N lines from a file
func readLastLines(filePath string, n int) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) <= n {
		return lines, scanner.Err()
	}

	return lines[len(lines)-n:], scanner.Err()
}
