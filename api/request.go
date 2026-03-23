package api

import (
	"encoding/json"
	"net/http"
)

const (
	// MaxRequestBodySize is the maximum size for API request bodies (1MB)
	MaxRequestBodySize = 1 << 20 // 1MB

	// MaxConfigUploadSize is the maximum size for config uploads (10MB)
	MaxConfigUploadSize = 10 << 20 // 10MB
)

// decodeJSONBody decodes JSON from request body with size limit
func (s *Server) decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	// Limit request body size to prevent DoS
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Reject unknown fields for security

	if err := decoder.Decode(dst); err != nil {
		return err
	}

	return nil
}

// decodeJSONBodyWithLimit decodes JSON with custom size limit
func (s *Server) decodeJSONBodyWithLimit(w http.ResponseWriter, r *http.Request, dst interface{}, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}

	return nil
}
