package api

import (
	"net/http"
	"strings"
)

// authMiddleware checks for authentication token if configured
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If no token is configured, allow all requests
		if s.authToken == "" {
			next(w, r)
			return
		}

		// Check Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.sendError(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Support both "Bearer <token>" and plain token
		token := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// Validate token
		if token != s.authToken {
			s.sendError(w, "Invalid authentication token", http.StatusUnauthorized)
			return
		}

		// Token is valid, proceed
		next(w, r)
	}
}

// SetAuthToken sets the authentication token for the API server
func (s *Server) SetAuthToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authToken = token
}
