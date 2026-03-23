package cfg

// API holds API server configuration
type API struct {
	Enabled bool   `json:"enabled,omitempty"`
	Port    int    `json:"port,omitempty"`
	Token   string `json:"token,omitempty"` // Optional authentication token
}
