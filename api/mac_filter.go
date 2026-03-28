// Package api provides MAC filtering API endpoints.
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

// MACFilterAPI manages MAC filtering via API
type MACFilterAPI struct {
	mu       sync.RWMutex
	config   *cfg.MACFilter
	configPath string
	onChange func(*cfg.MACFilter) error
}

// MACFilterRequest represents a MAC filter add/remove request
type MACFilterRequest struct {
	MAC string `json:"mac"`
}

// MACFilterResponse represents a MAC filter response
type MACFilterResponse struct {
	Mode   string   `json:"mode"`
	List   []string `json:"list"`
	Count  int      `json:"count"`
	Allowed bool    `json:"allowed,omitempty"`
}

// NewMACFilterAPI creates a new MAC filter API handler
func NewMACFilterAPI(filter *cfg.MACFilter, configPath string, onChange func(*cfg.MACFilter) error) *MACFilterAPI {
	return &MACFilterAPI{
		config:     filter,
		configPath: configPath,
		onChange:   onChange,
	}
}

// GetMode returns the current filter mode
func (h *MACFilterAPI) GetMode() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.config == nil {
		return "disabled"
	}
	return string(h.config.Mode)
}

// GetList returns the current filter list
func (h *MACFilterAPI) GetList() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.config == nil {
		return []string{}
	}
	return h.config.List
}

// HandleGet handles GET /api/macfilter
func (h *MACFilterAPI) HandleGet(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	response := MACFilterResponse{
		Mode:  "disabled",
		List:  []string{},
		Count: 0,
	}

	if h.config != nil {
		response.Mode = string(h.config.Mode)
		response.List = h.config.List
		response.Count = len(h.config.List)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandlePost handles POST /api/macfilter
func (h *MACFilterAPI) HandlePost(w http.ResponseWriter, r *http.Request) {
	var req MACFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if req.MAC == "" {
		http.Error(w, `{"error":"mac is required"}`, http.StatusBadRequest)
		return
	}

	// Normalize MAC address
	mac := normalizeMAC(req.MAC)
	if mac == "" {
		http.Error(w, `{"error":"invalid mac format"}`, http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Initialize config if nil
	if h.config == nil {
		h.config = &cfg.MACFilter{
			Mode: cfg.MACFilterWhitelist,
			List: []string{},
		}
	}

	// Check if MAC already in list
	for _, m := range h.config.List {
		if normalizeMAC(m) == mac {
			http.Error(w, `{"error":"mac already in list"}`, http.StatusConflict)
			return
		}
	}

	// Add MAC to list
	h.config.List = append(h.config.List, mac)

	// Save config
	if err := h.saveConfig(); err != nil {
		http.Error(w, `{"error":"failed to save config"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(MACFilterResponse{
		Mode:  string(h.config.Mode),
		List:  h.config.List,
		Count: len(h.config.List),
	})
}

// HandleDelete handles DELETE /api/macfilter
func (h *MACFilterAPI) HandleDelete(w http.ResponseWriter, r *http.Request) {
	var req MACFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if req.MAC == "" {
		http.Error(w, `{"error":"mac is required"}`, http.StatusBadRequest)
		return
	}

	// Normalize MAC address
	mac := normalizeMAC(req.MAC)
	if mac == "" {
		http.Error(w, `{"error":"invalid mac format"}`, http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.config == nil {
		http.Error(w, `{"error":"filter not configured"}`, http.StatusNotFound)
		return
	}

	// Find and remove MAC
	found := false
	newList := make([]string, 0, len(h.config.List))
	for _, m := range h.config.List {
		if normalizeMAC(m) == mac {
			found = true
			continue
		}
		newList = append(newList, m)
	}

	if !found {
		http.Error(w, `{"error":"mac not found"}`, http.StatusNotFound)
		return
	}

	h.config.List = newList

	// Save config
	if err := h.saveConfig(); err != nil {
		http.Error(w, `{"error":"failed to save config"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MACFilterResponse{
		Mode:  string(h.config.Mode),
		List:  h.config.List,
		Count: len(h.config.List),
	})
}

// HandleCheck handles POST /api/macfilter/check
func (h *MACFilterAPI) HandleCheck(w http.ResponseWriter, r *http.Request) {
	var req MACFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if req.MAC == "" {
		http.Error(w, `{"error":"mac is required"}`, http.StatusBadRequest)
		return
	}

	// Normalize MAC address
	mac := normalizeMAC(req.MAC)
	if mac == "" {
		http.Error(w, `{"error":"invalid mac format"}`, http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	allowed := true
	if h.config != nil {
		allowed = h.config.IsAllowed(mac)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MACFilterResponse{
		Mode:    h.GetMode(),
		Allowed: allowed,
	})
}

// HandleMode handles PUT /api/macfilter/mode
func (h *MACFilterAPI) HandleMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	var mode cfg.MACFilterMode
	switch strings.ToLower(req.Mode) {
	case "whitelist":
		mode = cfg.MACFilterWhitelist
	case "blacklist":
		mode = cfg.MACFilterBlacklist
	case "disabled":
		mode = cfg.MACFilterDisabled
	default:
		http.Error(w, `{"error":"invalid mode (use whitelist/blacklist/disabled)"}`, http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.config == nil {
		h.config = &cfg.MACFilter{
			Mode: mode,
			List: []string{},
		}
	} else {
		h.config.Mode = mode
	}

	// Save config
	if err := h.saveConfig(); err != nil {
		http.Error(w, `{"error":"failed to save config"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MACFilterResponse{
		Mode:  string(h.config.Mode),
		List:  h.config.List,
		Count: len(h.config.List),
	})
}

// HandleClear handles DELETE /api/macfilter/clear
func (h *MACFilterAPI) HandleClear(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.config == nil {
		http.Error(w, `{"error":"filter not configured"}`, http.StatusNotFound)
		return
	}

	h.config.List = []string{}

	// Save config
	if err := h.saveConfig(); err != nil {
		http.Error(w, `{"error":"failed to save config"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MACFilterResponse{
		Mode:  string(h.config.Mode),
		List:  []string{},
		Count: 0,
	})
}

// saveConfig saves the configuration
func (h *MACFilterAPI) saveConfig() error {
	if h.onChange != nil {
		return h.onChange(h.config)
	}
	return nil
}

// normalizeMAC normalizes a MAC address to uppercase with colons
func normalizeMAC(mac string) string {
	// Remove separators
	mac = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(mac, ":", ""), "-", ""))
	
	// Validate length
	if len(mac) != 12 {
		return ""
	}
	
	// Validate hex
	for _, c := range mac {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
			return ""
		}
	}
	
	// Add colons
	result := ""
	for i := 0; i < len(mac); i += 2 {
		if i > 0 {
			result += ":"
		}
		result += mac[i : i+2]
	}
	
	return result
}
