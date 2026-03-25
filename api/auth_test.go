package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware_NoToken(t *testing.T) {
	store := &Server{authToken: ""}

	handler := store.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	// Теперь если токен не установлен — доступ блокируется (503)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	store := &Server{authToken: "secret123"}

	handler := store.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	store := &Server{authToken: "secret123"}

	handler := store.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer wrongtoken")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	store := &Server{authToken: "secret123"}

	handler := store.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_PlainToken(t *testing.T) {
	store := &Server{authToken: "secret123"}

	handler := store.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "secret123")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestSetAuthToken(t *testing.T) {
	store := &Server{}

	if store.authToken != "" {
		t.Errorf("Expected empty token initially, got %s", store.authToken)
	}

	store.SetAuthToken("newtoken")

	if store.authToken != "newtoken" {
		t.Errorf("Expected token 'newtoken', got %s", store.authToken)
	}
}
