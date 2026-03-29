package api

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/metrics"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
	"github.com/QuadDarv1ne/go-pcap2socks/wanbalancer"
)

// Pre-defined errors for API operations
var (
	ErrMethodNotAllowed     = fmt.Errorf("method not allowed")
	ErrUnauthorized         = fmt.Errorf("unauthorized")
	ErrRateLimitExceeded    = fmt.Errorf("rate limit exceeded")
	ErrServiceNotRunning    = fmt.Errorf("service not running")
	ErrConfigNotFound       = fmt.Errorf("config not found")
	ErrConfigLoad           = fmt.Errorf("failed to load config")
	ErrConfigSave           = fmt.Errorf("failed to save config")
	ErrInvalidRequest       = fmt.Errorf("invalid request")
	ErrInternalServer       = fmt.Errorf("internal server error")
	ErrMetricsNotInitialized = fmt.Errorf("metrics not initialized")
)

type Server struct {
	mux           *http.ServeMux
	statsStore    *stats.Store
	profileMgr    *profiles.Manager
	upnpMgr       *upnpmanager.Manager
	metrics       *metrics.Collector
	configPath    string
	authToken     string // Optional authentication token
	rateLimiter   *rateLimiter
	wsHub         *WebSocketHub
	hotkeyManager *hotkey.Manager
	macFilterAPI  *MACFilterAPI
	wanBalancerAPI *WANBalancerAPI
	mu            sync.RWMutex
	enabled       bool
	stopChan      chan struct{} // For stopping background goroutines

	// Status cache for frequently accessed /api/status endpoint
	statusCache     Status
	statusCacheMu   sync.RWMutex
	statusCacheTime time.Time
	statusCacheTTL  time.Duration
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type Status struct {
	Running        bool      `json:"running"`
	ProxyMode      string    `json:"proxy_mode"` // "socks5" or "direct"
	Devices        []Device  `json:"devices"`
	Traffic        Traffic   `json:"traffic"`
	Uptime         string    `json:"uptime"`
	StartTime      time.Time `json:"start_time"`
	SocksAvailable bool      `json:"socks_available"`
}

type Device struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	Connected bool   `json:"connected"`
}

type Traffic struct {
	Total    uint64 `json:"total_bytes"`
	Upload   uint64 `json:"upload_bytes"`
	Download uint64 `json:"download_bytes"`
	Packets  uint64 `json:"packets"`
}

// generateSecureToken генерирует криптографически безопасный случайный токен
// для аутентификации API по умолчанию
func generateSecureToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback на детерминированный токен если rand не работает
		// Это может произойти только в крайне необычных обстоятельствах
		slog.Warn("crypto/rand failed, using fallback token", "err", err)
		return "fallback_token_" + time.Now().Format("20060102150405")
	}
	return base64.URLEncoding.EncodeToString(b)
}

func NewServer(statsStore *stats.Store, profileMgr *profiles.Manager, upnpMgr *upnpmanager.Manager, hotkeyMgr *hotkey.Manager, wanBalancerDialer *wanbalancer.WANBalancerDialer) *Server {
	executable, _ := os.Executable()
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector(statsStore)

	// Initialize rate limiter: 100 requests per minute per IP
	rateLimiter := newRateLimiter(100, 1*time.Minute)

	// Initialize WebSocket hub
	wsHub := NewWebSocketHub()
	go wsHub.Run()

	s := &Server{
		mux:           http.NewServeMux(),
		statsStore:    statsStore,
		profileMgr:    profileMgr,
		upnpMgr:       upnpMgr,
		metrics:       metricsCollector,
		configPath:    cfgFile,
		authToken:     generateSecureToken(), // Генерировать безопасный токен по умолчанию
		rateLimiter:   rateLimiter,
		wsHub:         wsHub,
		hotkeyManager: hotkeyMgr,
		enabled:       true,
		stopChan:      make(chan struct{}),
		statusCacheTTL: 500 * time.Millisecond, // Cache status for 500ms
	}

	// Initialize MAC filter API
	s.macFilterAPI = NewMACFilterAPI(nil, cfgFile, s.updateMACFilter)

	// Initialize WAN balancer API
	if wanBalancerDialer != nil {
		s.wanBalancerAPI = NewWANBalancerAPI(wanBalancerDialer.GetBalancer(), wanBalancerDialer)
	}

	slog.Info("API server initialized with auto-generated authentication token", "token_length", len(s.authToken))

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// Public endpoints (no auth required, with rate limiting)
	s.mux.HandleFunc("/api/status", s.rateLimitMiddleware(s.handleStatus))
	s.mux.HandleFunc("/metrics", s.handleMetrics)
	s.mux.HandleFunc("/", s.handleStatic)

	// Protected endpoints (require auth if token is set, with rate limiting)
	s.mux.HandleFunc("/api/start", s.rateLimitMiddleware(s.authMiddleware(s.handleStart)))
	s.mux.HandleFunc("/api/stop", s.rateLimitMiddleware(s.authMiddleware(s.handleStop)))
	s.mux.HandleFunc("/api/traffic", s.rateLimitMiddleware(s.authMiddleware(s.handleTraffic)))
	s.mux.HandleFunc("/api/traffic/export", s.rateLimitMiddleware(s.authMiddleware(s.handleTrafficExport)))
	s.mux.HandleFunc("/api/logs", s.rateLimitMiddleware(s.authMiddleware(s.handleLogs)))
	s.mux.HandleFunc("/api/logs/export", s.rateLimitMiddleware(s.authMiddleware(s.handleLogsExport)))
	s.mux.HandleFunc("/api/devices", s.rateLimitMiddleware(s.authMiddleware(s.handleDevices)))
	s.mux.HandleFunc("/api/config", s.rateLimitMiddleware(s.authMiddleware(s.handleConfig)))
	s.mux.HandleFunc("/api/config/update", s.rateLimitMiddleware(s.authMiddleware(s.handleConfigUpdate)))
	s.mux.HandleFunc("/api/config/reload", s.rateLimitMiddleware(s.authMiddleware(s.handleConfigReload)))
	s.mux.HandleFunc("/api/config/auto", s.rateLimitMiddleware(s.authMiddleware(s.handleAutoConfig)))
	s.mux.HandleFunc("/api/dhcp", s.rateLimitMiddleware(s.authMiddleware(s.handleDHCP)))
	s.mux.HandleFunc("/api/dhcp/leases", s.rateLimitMiddleware(s.authMiddleware(s.handleDHCPLeases)))
	s.mux.HandleFunc("/api/dhcp/metrics", s.rateLimitMiddleware(s.authMiddleware(s.handleDHCPMetrics)))
	s.mux.HandleFunc("/api/profiles", s.rateLimitMiddleware(s.authMiddleware(s.handleProfiles)))
	s.mux.HandleFunc("/api/profiles/switch", s.rateLimitMiddleware(s.authMiddleware(s.handleProfileSwitch)))
	s.mux.HandleFunc("/api/upnp", s.rateLimitMiddleware(s.authMiddleware(s.handleUPnP)))
	s.mux.HandleFunc("/api/upnp/discover", s.rateLimitMiddleware(s.authMiddleware(s.handleUPnPDiscover)))
	s.mux.HandleFunc("/api/upnp/add", s.rateLimitMiddleware(s.authMiddleware(s.handleUPnPAddPort)))
	s.mux.HandleFunc("/api/upnp/remove", s.rateLimitMiddleware(s.authMiddleware(s.handleUPnPRemovePort)))
	s.mux.HandleFunc("/api/upnp/apply", s.rateLimitMiddleware(s.authMiddleware(s.handleUPnPApplyMappings)))
	s.mux.HandleFunc("/api/upnp/preset", s.rateLimitMiddleware(s.authMiddleware(s.handleUPnPApplyPreset)))
	s.mux.HandleFunc("/api/hotkey", s.rateLimitMiddleware(s.authMiddleware(s.handleHotkey)))
	s.mux.HandleFunc("/api/hotkey/toggle", s.rateLimitMiddleware(s.authMiddleware(s.handleHotkeyToggle)))
	// MAC filter endpoints
	s.mux.HandleFunc("/api/macfilter", s.rateLimitMiddleware(s.authMiddleware(s.handleMACFilter)))
	s.mux.HandleFunc("/api/macfilter/check", s.rateLimitMiddleware(s.authMiddleware(s.macFilterAPI.HandleCheck)))
	s.mux.HandleFunc("/api/macfilter/mode", s.rateLimitMiddleware(s.authMiddleware(s.macFilterAPI.HandleMode)))
	s.mux.HandleFunc("/api/macfilter/clear", s.rateLimitMiddleware(s.authMiddleware(s.macFilterAPI.HandleClear)))
	// WAN balancer endpoints
	if s.wanBalancerAPI != nil {
		s.mux.HandleFunc("/api/wan/status", s.rateLimitMiddleware(s.authMiddleware(s.wanBalancerAPI.HandleWANStatus)))
		s.mux.HandleFunc("/api/wan/select", s.rateLimitMiddleware(s.authMiddleware(s.wanBalancerAPI.HandleWANSelect)))
		s.mux.HandleFunc("/api/wan/update", s.rateLimitMiddleware(s.authMiddleware(s.wanBalancerAPI.HandleWANUpdate)))
		s.mux.HandleFunc("/api/wan/reset", s.rateLimitMiddleware(s.authMiddleware(s.wanBalancerAPI.HandleWANReset)))
		s.mux.HandleFunc("/api/wan/health", s.rateLimitMiddleware(s.authMiddleware(s.wanBalancerAPI.HandleWANHealth)))
		s.mux.HandleFunc("/api/wan/enable", s.rateLimitMiddleware(s.authMiddleware(s.wanBalancerAPI.HandleWANEnable)))
		s.mux.HandleFunc("/api/wan/disable", s.rateLimitMiddleware(s.authMiddleware(s.wanBalancerAPI.HandleWANDisable)))
	}
	s.mux.HandleFunc("/api/devices/names", s.rateLimitMiddleware(s.authMiddleware(s.handleDeviceNames)))
	s.mux.HandleFunc("/api/devices/ratelimit", s.rateLimitMiddleware(s.authMiddleware(s.handleDeviceRateLimit)))
	s.mux.HandleFunc("/api/interfaces", s.rateLimitMiddleware(s.authMiddleware(s.handleInterfaces)))
	s.mux.HandleFunc("/ws", s.rateLimitMiddleware(s.authMiddleware(s.handleWebSocket)))
}

// Stop gracefully stops the API server and releases resources
func (s *Server) Stop() {
	// Stop rate limiter cleanup goroutine
	if s.rateLimiter != nil {
		s.rateLimiter.stop()
	}

	// Stop WebSocket hub and real-time updates
	s.StopRealTimeUpdates()
}

// handleMetrics exports Prometheus format metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.metrics == nil {
		http.Error(w, ErrMetricsNotInitialized.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	s.metrics.WriteMetrics(w)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	// Check cache first (500ms TTL)
	s.statusCacheMu.RLock()
	if time.Since(s.statusCacheTime) < s.statusCacheTTL {
		// Cache hit - return cached status
		status := s.statusCache
		s.statusCacheMu.RUnlock()
		s.sendSuccess(w, status)
		return
	}
	s.statusCacheMu.RUnlock()

	// Cache miss - build fresh status
	// Get running state from global state if available
	running := s.enabled
	if getIsRunningFn != nil {
		running = getIsRunningFn()
	}

	status := Status{
		Running:        running,
		ProxyMode:      "socks5",
		Devices:        s.getDevices(),
		Traffic:        s.getTraffic(),
		Uptime:         time.Since(startTime).String(),
		StartTime:      startTime,
		SocksAvailable: true,
	}

	// Update cache
	s.statusCacheMu.Lock()
	s.statusCache = status
	s.statusCacheTime = time.Now()
	s.statusCacheMu.Unlock()

	s.sendSuccess(w, status)
}

// getIsRunningFn returns a function to check if service is running
// This is set from main package
var getIsRunningFn func() bool

// SetIsRunningFn sets the function to check if service is running
func SetIsRunningFn(fn func() bool) {
	getIsRunningFn = fn
}

// handleStart handles the /api/start endpoint to start the service
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	// Call start callback if available
	if startServiceFn != nil {
		if err := startServiceFn(); err != nil {
			s.sendError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Invalidate status cache
		s.statusCacheMu.Lock()
		s.statusCacheTime = time.Time{}
		s.statusCacheMu.Unlock()
		s.sendSuccess(w, "Service started")
		return
	}

	// Fallback: just set flag
	s.mu.Lock()
	s.enabled = true
	s.mu.Unlock()
	// Invalidate status cache
	s.statusCacheMu.Lock()
	s.statusCacheTime = time.Time{}
	s.statusCacheMu.Unlock()

	s.sendSuccess(w, "Service started")
}

// handleStop handles the /api/stop endpoint to stop the service
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	// Call stop callback if available
	if stopServiceFn != nil {
		if err := stopServiceFn(); err != nil {
			s.sendError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Invalidate status cache
		s.statusCacheMu.Lock()
		s.statusCacheTime = time.Time{}
		s.statusCacheMu.Unlock()
		s.sendSuccess(w, "Service stopped")
		return
	}

	// Fallback: just set flag
	s.mu.Lock()
	s.enabled = false
	s.mu.Unlock()
	// Invalidate status cache
	s.statusCacheMu.Lock()
	s.statusCacheTime = time.Time{}
	s.statusCacheMu.Unlock()

	s.sendSuccess(w, "Service stopped")
}

// startServiceFn and stopServiceFn are callbacks for real service control
var startServiceFn func() error
var stopServiceFn func() error

// SetServiceCallbacks sets the start and stop callbacks
func SetServiceCallbacks(startFn func() error, stopFn func() error) {
	startServiceFn = startFn
	stopServiceFn = stopFn
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

	// Send success response
	s.sendSuccess(w, map[string]interface{}{
		"message": "Config reload requested. Restart service to apply changes.",
		"status":  "pending_restart",
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
			"profiles": profileList,
			"current":  currentProfile,
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

		s.sendSuccess(w, map[string]interface{}{
			"message": "Profile switched: " + req.Profile,
			"profile": req.Profile,
			"restart": true,
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

	s.sendSuccess(w, map[string]interface{}{
		"message": "Profile switched: " + req.Profile,
		"profile": req.Profile,
		"restart": true,
	})
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
		"message":         "Port mapping added",
		"protocol":        protocol,
		"external_port":   req.ExternalPort,
		"internal_port":   req.InternalPort,
		"active_mappings": s.upnpMgr.GetActiveMappings(),
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
		"message":         "Port mapping removed",
		"protocol":        req.Protocol,
		"port":            req.Port,
		"active_mappings": s.upnpMgr.GetActiveMappings(),
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
		"message":         "Port mappings applied",
		"active_mappings": s.upnpMgr.GetActiveMappings(),
	})
}

func (s *Server) handleUPnPApplyPreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Game string `json:"game"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Game == "" {
		s.sendError(w, "game parameter is required", http.StatusBadRequest)
		return
	}

	if s.upnpMgr == nil {
		s.sendError(w, "UPnP not available", http.StatusInternalServerError)
		return
	}

	// Get available presets from config
	availablePresets := []string{"ps4", "ps5", "xbox", "switch"}
	isValidPreset := false
	for _, preset := range availablePresets {
		if strings.ToLower(req.Game) == preset {
			isValidPreset = true
			break
		}
	}

	if !isValidPreset {
		s.sendError(w, "Invalid game preset. Available: "+strings.Join(availablePresets, ", "), http.StatusBadRequest)
		return
	}

	// Apply preset by adding port mappings
	leaseDuration := 3600
	if s.upnpMgr != nil && s.upnpMgr.GetConfig() != nil {
		leaseDuration = s.upnpMgr.GetConfig().LeaseDuration
		if leaseDuration == 0 {
			leaseDuration = 3600
		}
	}

	// Get ports for preset
	ports := upnpmanager.GetGamePresetPorts(req.Game)
	mappingsApplied := 0

	for _, port := range ports {
		// Add TCP mapping
		if err := s.upnpMgr.AddDynamicMapping("TCP", port, port, fmt.Sprintf("go-pcap2socks %s", req.Game)); err != nil {
			slog.Debug("TCP port mapping failed", "port", port, "game", req.Game, "err", err)
		} else {
			mappingsApplied++
		}

		// Add UDP mapping
		if err := s.upnpMgr.AddDynamicMapping("UDP", port, port, fmt.Sprintf("go-pcap2socks %s", req.Game)); err != nil {
			slog.Debug("UDP port mapping failed", "port", port, "game", req.Game, "err", err)
		} else {
			mappingsApplied++
		}
	}

	s.sendSuccess(w, map[string]interface{}{
		"message":         fmt.Sprintf("Game preset '%s' applied: %d port mappings (TCP+UDP)", req.Game, mappingsApplied),
		"game":            req.Game,
		"ports":           ports,
		"active_mappings": s.upnpMgr.GetActiveMappings(),
	})
}

func (s *Server) handleHotkey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get registered hotkeys (Windows only)
	hotkeyList := s.getHotkeysList()

	s.sendSuccess(w, map[string]interface{}{
		"enabled": s.hotkeyManager != nil && len(hotkeyList) > 0,
		"hotkeys": hotkeyList,
	})
}

func (s *Server) handleHotkeyToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		Action string `json:"action"` // "toggle", "enable", "disable"
		Hotkey string `json:"hotkey"` // hotkey name
	}

	if err := s.decodeJSONBodyWithLimit(w, r, &req, 1024); err != nil {
		s.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// For now, just acknowledge the request
	// Full implementation would require callback integration
	s.sendSuccess(w, map[string]interface{}{
		"action": req.Action,
		"status": "acknowledged",
	})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Serve web UI files
	webPath := filepath.Join(filepath.Dir(s.configPath), "web")

	// Handle root path
	if r.URL.Path == "/" {
		filePath := filepath.Join(webPath, "index.html")
		http.ServeFile(w, r, filePath)
		return
	}

	// Clean and validate path to prevent directory traversal
	// Convert URL path to filepath and clean it
	requestPath := filepath.FromSlash(path.Clean("/" + r.URL.Path))
	filePath := filepath.Join(webPath, requestPath)

	// Security: use filepath.Rel for robust path traversal prevention
	// This is more secure than prefix checking as it handles symlinks and edge cases
	rel, err := filepath.Rel(webPath, filePath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		slog.Warn("Path traversal attempt blocked", "path", r.URL.Path, "resolved", filePath)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Additional check: verify file exists and is within webPath
	absWebPath, err := filepath.Abs(webPath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !strings.HasPrefix(absFilePath, absWebPath) {
		slog.Warn("Path traversal attempt blocked (abs check)", "path", r.URL.Path, "resolved", absFilePath)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File not found, serve index.html for SPA routing
		filePath = filepath.Join(webPath, "index.html")
	}

	// Set content type based on file extension
	ext := path.Ext(filePath)
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	}

	// Add security headers to prevent XSS and other attacks
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:")

	http.ServeFile(w, r, filePath)
}

// sendSuccess sends a JSON success response
func (s *Server) sendSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
	}); err != nil {
		slog.Debug("Failed to encode success response", "err", err)
	}
}

// sendError sends a JSON error response with HTTP status code
func (s *Server) sendError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   message,
	}); err != nil {
		slog.Debug("Failed to encode error response", "err", err)
	}
}

// SuccessResponse creates a success API response
func SuccessResponse(data interface{}) APIResponse {
	return APIResponse{
		Success: true,
		Data:    data,
	}
}

// ErrorResponse creates an error API response
func ErrorResponse(message string) APIResponse {
	return APIResponse{
		Success: false,
		Error:   message,
	}
}

// deviceSlicePool provides pooling for Device slices to reduce allocations
// /api/status is called frequently, so we pool the slice allocation
var deviceSlicePool = sync.Pool{
	New: func() any {
		return make([]Device, 0, 256)
	},
}

func (s *Server) getDevices() []Device {
	if s.statsStore == nil {
		return []Device{}
	}

	devicesStats := s.statsStore.GetAllDevices()

	// Get slice from pool
	devices := deviceSlicePool.Get().([]Device)
	devices = devices[:0] // Reset length, keep capacity

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

	// Note: Caller should return slice to pool after use
	// For now, let GC handle it as this is a read-only snapshot
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

// SetStartTime sets the service start time
func SetStartTime(t time.Time) {
	startTime = t
}

// GetStartTime returns the service start time
func GetStartTime() time.Time {
	return startTime
}

// handleMACFilter dispatches MAC filter requests based on HTTP method
func (s *Server) handleMACFilter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.macFilterAPI.HandleGet(w, r)
	case http.MethodPost:
		s.macFilterAPI.HandlePost(w, r)
	case http.MethodDelete:
		s.macFilterAPI.HandleDelete(w, r)
	default:
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// updateMACFilter is the callback for MACFilterAPI to save config
func (s *Server) updateMACFilter(filter *cfg.MACFilter) error {
	// Load current config
	config, err := cfg.Load(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Update MAC filter
	config.MACFilter = filter

	// Save config
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	slog.Info("MAC filter configuration updated", "mode", filter.Mode, "count", len(filter.List))
	return nil
}

// handleDeviceNames returns all device names
func (s *Server) handleDeviceNames(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleDeviceNamesGet(w, r)
	case http.MethodPost:
		s.handleDeviceNamesUpdate(w, r)
	default:
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeviceNamesGet returns all device names
func (s *Server) handleDeviceNamesGet(w http.ResponseWriter, r *http.Request) {
	names := s.statsStore.GetAllDeviceNames()
	s.sendSuccess(w, map[string]interface{}{
		"names": names,
	})
}

// handleDeviceNamesUpdate updates a device name
func (s *Server) handleDeviceNamesUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MAC  string `json:"mac"`
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MAC == "" {
		s.sendError(w, "MAC address required", http.StatusBadRequest)
		return
	}

	s.statsStore.SetCustomName(req.MAC, req.Name)
	s.sendSuccess(w, "Device name updated")
}

// handleDeviceRateLimit updates rate limits for a device
func (s *Server) handleDeviceRateLimit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		MAC      string `json:"mac"`
		Upload   uint64 `json:"upload"`   // bytes/sec
		Download uint64 `json:"download"` // bytes/sec
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MAC == "" {
		s.sendError(w, "MAC address required", http.StatusBadRequest)
		return
	}

	s.statsStore.SetRateLimit(req.MAC, req.Upload, req.Download)
	s.sendSuccess(w, "Rate limit updated")
}

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

// handleAutoConfig runs automatic configuration
func (s *Server) handleAutoConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Run auto-config command
	cmd := exec.Command(os.Args[0], "auto-config")
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("auto-config error", slog.Any("err", err), slog.String("output", string(output)))
		s.sendError(w, "Auto-config failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.sendSuccess(w, map[string]string{
		"message": "Auto-config completed successfully",
		"output":  string(output),
	})
}

// handleDHCP returns DHCP server status
func (s *Server) handleDHCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleDHCPGet(w, r)
	case http.MethodPost:
		s.handleDHCPUpdate(w, r)
	default:
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDHCPGet returns DHCP configuration
func (s *Server) handleDHCPGet(w http.ResponseWriter, r *http.Request) {
	config, err := cfg.Load(s.configPath)
	if err != nil {
		s.sendError(w, "Failed to load config", http.StatusInternalServerError)
		return
	}

	dhcpEnabled := false
	poolStart := ""
	poolEnd := ""
	leaseDuration := 0

	if config.DHCP != nil {
		dhcpEnabled = config.DHCP.Enabled
		poolStart = config.DHCP.PoolStart
		poolEnd = config.DHCP.PoolEnd
		leaseDuration = config.DHCP.LeaseDuration
	}

	s.sendSuccess(w, map[string]interface{}{
		"enabled":        dhcpEnabled,
		"pool_start":     poolStart,
		"pool_end":       poolEnd,
		"lease_duration": leaseDuration,
	})
}

// handleDHCPUpdate updates DHCP configuration
func (s *Server) handleDHCPUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled       bool   `json:"enabled"`
		PoolStart     string `json:"pool_start"`
		PoolEnd       string `json:"pool_end"`
		LeaseDuration int    `json:"lease_duration"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	config, err := cfg.Load(s.configPath)
	if err != nil {
		s.sendError(w, "Failed to load config", http.StatusInternalServerError)
		return
	}

	if config.DHCP == nil {
		config.DHCP = &cfg.DHCP{}
	}

	config.DHCP.Enabled = req.Enabled
	if req.PoolStart != "" {
		config.DHCP.PoolStart = req.PoolStart
	}
	if req.PoolEnd != "" {
		config.DHCP.PoolEnd = req.PoolEnd
	}
	if req.LeaseDuration > 0 {
		config.DHCP.LeaseDuration = req.LeaseDuration
	}

	// Save config
	if err := saveConfig(s.configPath, config); err != nil {
		s.sendError(w, "Failed to save config", http.StatusInternalServerError)
		return
	}

	s.sendSuccess(w, "DHCP configuration updated")
}

// saveConfig saves the configuration to a file
func saveConfig(filePath string, config *cfg.Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// handleDHCPLeases returns current DHCP leases
func (s *Server) handleDHCPLeases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get leases from global DHCP server if available
	leases := getDHCPLeases()
	s.sendSuccess(w, map[string]interface{}{
		"leases": leases,
	})
}

// getDHCPLeases returns current DHCP leases from global server
// This is set from main package
var getDHCPLeases func() []map[string]interface{}

// SetGetDHCPLeasesFn sets the function to get DHCP leases
func SetGetDHCPLeasesFn(fn func() []map[string]interface{}) {
	getDHCPLeases = fn
}

// getDHCPMetrics returns current DHCP metrics from global server
var getDHCPMetrics func() map[string]interface{}

// SetGetDHCPMetricsFn sets the function to get DHCP metrics
func SetGetDHCPMetricsFn(fn func() map[string]interface{}) {
	getDHCPMetrics = fn
}

// handleDHCPMetrics returns DHCP server metrics
func (s *Server) handleDHCPMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := getDHCPMetrics()
	if metrics == nil {
		metrics = map[string]interface{}{
			"available": false,
			"message":   "DHCP metrics not available",
		}
	}

	s.sendSuccess(w, metrics)
}

// handleWebSocket handles WebSocket connections for real-time updates
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	s.HandleWebSocket(w, r)
}

// StartRealTimeUpdates starts broadcasting real-time stats to WebSocket clients
func (s *Server) StartRealTimeUpdates(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.broadcastStats()
			case <-s.stopChan:
				return
			}
		}
	}()
}

// StopRealTimeUpdates stops the real-time updates
func (s *Server) StopRealTimeUpdates() {
	close(s.stopChan)
	if s.wsHub != nil {
		s.wsHub.Stop()
	}
}

// broadcastStats broadcasts current stats to all WebSocket clients
func (s *Server) broadcastStats() {
	if s.wsHub == nil {
		return
	}

	// Get current stats
	traffic := s.getTraffic()
	devices := s.getDevices()

	// Build stats message
	stats := map[string]interface{}{
		"type":      "stats",
		"timestamp": time.Now().Unix(),
		"traffic": map[string]uint64{
			"total":    traffic.Total,
			"upload":   traffic.Upload,
			"download": traffic.Download,
		},
		"devices": map[string]int{
			"total":     len(devices),
			"connected": countConnected(devices),
		},
	}

	s.wsHub.Broadcast(stats)
}

// countConnected counts connected devices
func countConnected(devices []Device) int {
	count := 0
	for _, d := range devices {
		if d.Connected {
			count++
		}
	}
	return count
}

// handleInterfaces returns list of available network interfaces
func (s *Server) handleInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	// Import auto package locally to avoid circular dependency
	interfaces := getInterfaceList()
	s.sendSuccess(w, map[string]interface{}{
		"interfaces": interfaces,
		"count":      len(interfaces),
	})
}

// getInterfaceList returns list of network interfaces (delegated to auto package)
var getInterfaceList func() []interface{}

// SetInterfaceListFn sets the function to get interface list
func SetInterfaceListFn(fn func() []interface{}) {
	getInterfaceList = fn
}
