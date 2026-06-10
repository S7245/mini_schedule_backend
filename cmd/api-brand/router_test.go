package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	brandHandler "github.com/zkw/mini-schedule/backend/internal/interfaces/brand"
)

func TestBrandRouterAllowsConfiguredPreflightOrigin(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{Debug: false},
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"http://localhost:3002"},
		},
	}
	router := newBrandRouter(
		brandHandler.NewHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		slog.Default(),
	)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/brand/login", nil)
	req.Header.Set("Origin", "http://localhost:3002")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3002" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "http://localhost:3002")
	}
}
