package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/ejsadiarin/coredraw/internal/helper"
)

// Routes builds the HTTP router.
func (s *Server) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Use(slogMiddleware)
	r.Use(middleware.Recoverer)

	r.Get("/", s.helloWorldHandler)
	r.Get("/health", s.healthHandler)

	r.Route("/api/coredraw", func(r chi.Router) {
		r.Use(s.Verifier.Middleware)
		r.Route("/boards", s.Boards.Mount)
	})

	return r
}

func (s *Server) helloWorldHandler(w http.ResponseWriter, _ *http.Request) {
	helper.RespondJSON(w, http.StatusOK, map[string]string{"message": "coredraw-api"})
}

func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	stats := s.Pool.Stat()
	helper.RespondJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"pool": map[string]any{
			"total_conns": stats.TotalConns(),
			"idle_conns":  stats.IdleConns(),
		},
	})
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = r.Header.Get("X-Correlation-ID")
		}
		if id == "" {
			id = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func slogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"request_id", w.Header().Get("X-Request-ID"),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
