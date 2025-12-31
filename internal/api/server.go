// Package api provides the HTTP server and handlers for the retrieval tool.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/iasik/project-indexer/internal/config"
	"github.com/iasik/project-indexer/internal/embedder"
	"github.com/iasik/project-indexer/internal/vectordb"
)

// Server represents the HTTP API server.
type Server struct {
	cfg           *config.Manager
	embedder      embedder.Provider
	vectorDB      vectordb.Provider
	logger        *slog.Logger
	httpServer    *http.Server
	mu            sync.RWMutex
	version       string
}

// NewServer creates a new API server.
func NewServer(
	cfg *config.Manager,
	emb embedder.Provider,
	vdb vectordb.Provider,
	logger *slog.Logger,
) *Server {
	return &Server{
		cfg:      cfg,
		embedder: emb,
		vectorDB: vdb,
		logger:   logger,
		version:  "1.0.0",
	}
}

// Start starts the HTTP server with graceful shutdown.
func (s *Server) Start(ctx context.Context) error {
	cfg := s.cfg.Get()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /retrieve", s.handleRetrieve)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /", s.handleRoot)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      s.loggingMiddleware(mux),
		ReadTimeout:  cfg.Server.GetReadTimeout(),
		WriteTimeout: cfg.Server.GetWriteTimeout(),
	}

	// Setup hot reload
	s.setupHotReload()

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("starting server",
			"port", cfg.Server.Port,
			"version", s.version)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case <-ctx.Done():
		return s.shutdown()
	case err := <-errCh:
		return err
	}
}

// shutdown performs graceful shutdown.
func (s *Server) shutdown() error {
	cfg := s.cfg.Get()
	s.logger.Info("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.GetShutdownTimeout())
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	// Close providers
	if err := s.embedder.Close(); err != nil {
		s.logger.Warn("embedder close error", "error", err)
	}
	if err := s.vectorDB.Close(); err != nil {
		s.logger.Warn("vectordb close error", "error", err)
	}

	s.logger.Info("server stopped")
	return nil
}

// setupHotReload configures SIGHUP handler for config reload.
func (s *Server) setupHotReload() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)

	go func() {
		for range sigCh {
			s.logger.Info("received SIGHUP, reloading config")
			if err := s.cfg.Reload(); err != nil {
				s.logger.Error("config reload failed", "error", err)
			} else {
				s.logger.Info("config reloaded successfully")
			}
		}
	}()
}

// UpdateProviders updates the embedding and vectordb providers (for hot reload).
func (s *Server) UpdateProviders(emb embedder.Provider, vdb vectordb.Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close old providers
	if s.embedder != nil {
		s.embedder.Close()
	}
	if s.vectorDB != nil {
		s.vectorDB.Close()
	}

	s.embedder = emb
	s.vectorDB = vdb
}

// getProviders returns thread-safe access to providers.
func (s *Server) getProviders() (embedder.Provider, vectordb.Provider) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.embedder, s.vectorDB
}

// loggingMiddleware logs all HTTP requests.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status
		wrapped := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

// statusResponseWriter captures the response status code.
type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
