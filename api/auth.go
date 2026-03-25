package api

import (
	"net/http"
	"strings"
)

// authMiddleware проверяет токен аутентификации
// Если токен установлен, все запросы должны быть аутентифицированы
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Получаем токен из сервера
		s.mu.RLock()
		authToken := s.authToken
		s.mu.RUnlock()

		// Если токен не установлен (пустой), блокируем все запросы
		// Это более безопасное поведение по умолчанию
		if authToken == "" {
			s.sendError(w, "Authentication not configured. Please set a token in config or restart the server.", http.StatusServiceUnavailable)
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
		if token != authToken {
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

// GetAuthToken returns the current authentication token (for debugging)
func (s *Server) GetAuthToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authToken
}
