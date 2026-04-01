package cfg

// API holds API server configuration
type API struct {
	Enabled bool   `json:"enabled,omitempty"`
	Port    int    `json:"port,omitempty"`
	Token   string `json:"token,omitempty"` // Optional authentication token

	// HTTPS configuration (optional)
	HTTPS *HTTPSConfig `json:"https,omitempty"`
}

// HTTPSConfig holds HTTPS/TLS configuration for the API server
type HTTPSConfig struct {
	Enabled    bool   `json:"enabled"`               // Enable HTTPS
	CertFile   string `json:"cert_file"`             // Path to TLS certificate file
	KeyFile    string `json:"key_file"`              // Path to TLS private key file
	AutoTLS    bool   `json:"autotls,omitempty"`     // Auto-generate self-signed certificate
	Domain     string `json:"domain,omitempty"`      // Domain for Let's Encrypt (future)
	ForceHTTPS bool   `json:"force_https,omitempty"` // Redirect HTTP to HTTPS
}
