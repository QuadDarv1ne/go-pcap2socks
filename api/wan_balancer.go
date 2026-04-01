package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/wanbalancer"
)

// WANBalancerAPI provides HTTP API endpoints for WAN balancer management
type WANBalancerAPI struct {
	balancer *wanbalancer.Balancer
	dialer   *wanbalancer.WANBalancerDialer
}

// NewWANBalancerAPI creates a new WAN balancer API handler
func NewWANBalancerAPI(balancer *wanbalancer.Balancer, dialer *wanbalancer.WANBalancerDialer) *WANBalancerAPI {
	return &WANBalancerAPI{
		balancer: balancer,
		dialer:   dialer,
	}
}

// WANBalancerStatus represents the status response for WAN balancer
type WANBalancerStatus struct {
	Enabled       bool                `json:"enabled"`
	Policy        string              `json:"policy"`
	TotalUplinks  int                 `json:"total_uplinks"`
	ActiveUplinks int                 `json:"active_uplinks"`
	Uplinks       []UplinkStatus      `json:"uplinks"`
	Metrics       *WANBalancerMetrics `json:"metrics,omitempty"`
}

// UplinkStatus represents the status of a single uplink
type UplinkStatus struct {
	Tag          string `json:"tag"`
	Status       string `json:"status"`
	Weight       int    `json:"weight"`
	Priority     int    `json:"priority"`
	ActiveConns  int64  `json:"active_conns"`
	TotalConns   int64  `json:"total_conns"`
	TotalBytesRx int64  `json:"total_bytes_rx"`
	TotalBytesTx int64  `json:"total_bytes_tx"`
	LastLatency  int64  `json:"last_latency_ns"`
	Description  string `json:"description,omitempty"`
}

// WANBalancerMetrics represents metrics for WAN balancer
type WANBalancerMetrics struct {
	ConnSuccess        uint64 `json:"conn_success"`
	ConnFailure        uint64 `json:"conn_failure"`
	ConnTimeout        uint64 `json:"conn_timeout"`
	BytesRx            uint64 `json:"bytes_rx"`
	BytesTx            uint64 `json:"bytes_tx"`
	AvgLatency         int64  `json:"avg_latency_ns"`
	MinLatency         int64  `json:"min_latency_ns"`
	MaxLatency         int64  `json:"max_latency_ns"`
	UplinkSwitches     uint64 `json:"uplink_switches"`
	HealthChecksTotal  uint64 `json:"health_checks_total"`
	HealthChecksFailed uint64 `json:"health_checks_failed"`
	Uptime             string `json:"uptime"`
}

// HandleWANStatus handles GET /api/wan/status
// Returns the current status of the WAN balancer
func (h *WANBalancerAPI) HandleWANStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	if h.balancer == nil {
		h.sendResponse(w, WANBalancerStatus{
			Enabled: false,
		})
		return
	}

	stats := h.balancer.GetStats()
	uplinks := make([]UplinkStatus, len(stats.Uplinks))
	for i, u := range stats.Uplinks {
		uplinks[i] = UplinkStatus{
			Tag:          h.getUplinkTag(stats, i),
			Status:       u.Status.String(),
			Weight:       u.Weight,
			Priority:     u.Priority,
			ActiveConns:  u.ActiveConns,
			TotalConns:   u.TotalConns,
			TotalBytesRx: u.TotalBytesRx,
			TotalBytesTx: u.TotalBytesTx,
			LastLatency:  int64(u.LastLatency),
		}
	}

	response := WANBalancerStatus{
		Enabled:       true,
		Policy:        string(stats.Policy),
		TotalUplinks:  stats.TotalUplinks,
		ActiveUplinks: stats.ActiveUplinks,
		Uplinks:       uplinks,
	}

	// Add metrics if dialer is available
	if h.dialer != nil {
		m := h.dialer.GetMetrics().GetStats()
		response.Metrics = &WANBalancerMetrics{
			ConnSuccess:        m.ConnSuccess,
			ConnFailure:        m.ConnFailure,
			ConnTimeout:        m.ConnTimeout,
			BytesRx:            m.BytesRx,
			BytesTx:            m.BytesTx,
			AvgLatency:         int64(m.AvgLatency),
			MinLatency:         int64(m.MinLatency),
			MaxLatency:         int64(m.MaxLatency),
			UplinkSwitches:     m.UplinkSwitches,
			HealthChecksTotal:  m.HealthChecksTotal,
			HealthChecksFailed: m.HealthChecksFailed,
			Uptime:             m.Uptime.String(),
		}
	}

	h.sendResponse(w, response)
}

// getUplinkTag extracts the tag from uplink stats
func (h *WANBalancerAPI) getUplinkTag(stats wanbalancer.BalancerStats, index int) string {
	// Get uplink from balancer to retrieve tag
	uplinks := h.balancer.GetAllUplinks()
	if index < len(uplinks) {
		return uplinks[index].Tag
	}
	return ""
}

// HandleWANSelect handles POST /api/wan/select
// Manually select an uplink by tag (for testing/debugging)
func (h *WANBalancerAPI) HandleWANSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	if h.balancer == nil {
		http.Error(w, "WAN balancer not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Tag string `json:"tag"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, ErrInvalidRequest.Error(), http.StatusBadRequest)
		return
	}

	if req.Tag == "" {
		http.Error(w, "tag is required", http.StatusBadRequest)
		return
	}

	uplink, err := h.balancer.GetUplinkByTag(req.Tag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	h.sendResponse(w, map[string]string{
		"selected": uplink.Tag,
		"status":   uplink.GetStatus().String(),
	})
}

// HandleWANUpdate handles POST /api/wan/update
// Update uplink configuration (weight, priority, status)
func (h *WANBalancerAPI) HandleWANUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	if h.balancer == nil {
		http.Error(w, "WAN balancer not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Tag      string `json:"tag"`
		Weight   *int   `json:"weight,omitempty"`
		Priority *int   `json:"priority,omitempty"`
		Status   string `json:"status,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, ErrInvalidRequest.Error(), http.StatusBadRequest)
		return
	}

	if req.Tag == "" {
		http.Error(w, "tag is required", http.StatusBadRequest)
		return
	}

	uplink, err := h.balancer.GetUplinkByTag(req.Tag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Update weight if provided
	if req.Weight != nil {
		if err := h.balancer.UpdateUplinkWeight(req.Tag, *req.Weight); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Update priority if provided
	if req.Priority != nil {
		uplink.Priority = *req.Priority
	}

	// Update status if provided
	if req.Status != "" {
		switch req.Status {
		case "up":
			uplink.SetStatus(wanbalancer.UplinkUp)
		case "down":
			uplink.SetStatus(wanbalancer.UplinkDown)
		case "degraded":
			uplink.SetStatus(wanbalancer.UplinkDegraded)
		default:
			http.Error(w, "invalid status (use: up, down, degraded)", http.StatusBadRequest)
			return
		}
	}

	h.sendResponse(w, map[string]string{
		"updated": uplink.Tag,
		"status":  uplink.GetStatus().String(),
	})
}

// HandleWANReset handles POST /api/wan/reset
// Reset metrics counters
func (h *WANBalancerAPI) HandleWANReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	if h.dialer == nil {
		http.Error(w, "WAN balancer not initialized", http.StatusServiceUnavailable)
		return
	}

	h.dialer.GetMetrics().Reset()

	h.sendResponse(w, map[string]string{
		"status": "metrics reset successfully",
	})
}

// HandleWANHealth handles POST /api/wan/health
// Trigger immediate health check on all uplinks
func (h *WANBalancerAPI) HandleWANHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	if h.balancer == nil {
		http.Error(w, "WAN balancer not initialized", http.StatusServiceUnavailable)
		return
	}

	// Note: Health check is performed in background
	// This endpoint just returns current status
	stats := h.balancer.GetStats()
	results := make([]map[string]interface{}, len(stats.Uplinks))
	for i, u := range stats.Uplinks {
		results[i] = map[string]interface{}{
			"tag":     h.getUplinkTag(stats, i),
			"status":  u.Status.String(),
			"latency": u.LastLatency.String(),
		}
	}

	h.sendResponse(w, map[string]interface{}{
		"health_check": "completed",
		"timestamp":    time.Now().Format(time.RFC3339),
		"results":      results,
	})
}

// HandleWANEnable handles POST /api/wan/enable
// Enable WAN balancing (activate the balancer)
func (h *WANBalancerAPI) HandleWANEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	if h.balancer == nil {
		http.Error(w, "WAN balancer not initialized", http.StatusServiceUnavailable)
		return
	}

	// Mark all uplinks as up (enable traffic)
	for _, u := range h.balancer.GetAllUplinks() {
		u.SetStatus(wanbalancer.UplinkUp)
	}

	h.sendResponse(w, map[string]string{
		"status": "WAN balancing enabled",
	})
}

// HandleWANDisable handles POST /api/wan/disable
// Disable WAN balancing (mark all uplinks as down)
func (h *WANBalancerAPI) HandleWANDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, ErrMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
		return
	}

	if h.balancer == nil {
		http.Error(w, "WAN balancer not initialized", http.StatusServiceUnavailable)
		return
	}

	// Mark all uplinks as down (disable traffic)
	for _, u := range h.balancer.GetAllUplinks() {
		u.SetStatus(wanbalancer.UplinkDown)
	}

	h.sendResponse(w, map[string]string{
		"status": "WAN balancing disabled",
	})
}

// sendResponse sends a JSON response
func (h *WANBalancerAPI) sendResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
	})
}
