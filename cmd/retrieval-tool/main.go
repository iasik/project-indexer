// Retrieval Tool - HTTP API server for semantic code search
//
// This server provides the retrieval endpoint for LLM/Copilot integration.
// It embeds queries and searches the vector database for relevant code chunks.
//
// Endpoints:
//
//	POST /retrieve - Semantic search for code
//	GET  /health   - Health check
//
// Hot reload:
//
//	Send SIGHUP to reload configuration without restart.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/iasik/project-indexer/internal/api"
	"github.com/iasik/project-indexer/internal/config"
	"github.com/iasik/project-indexer/internal/embedder"
	"github.com/iasik/project-indexer/internal/vectordb"
)

func main() {
	// Setup logger
	logLevel := slog.LevelInfo
	if os.Getenv("DEBUG") != "" {
		logLevel = slog.LevelDebug
	}

	logFormat := os.Getenv("LOG_FORMAT")
	var handler slog.Handler
	if logFormat == "text" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	logger := slog.New(handler)

	logger.Info("starting retrieval tool")

	// Load configuration
	cfgManager, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	cfg := cfgManager.Get()

	logger.Info("configuration loaded",
		"port", cfg.Server.Port,
		"embedding_provider", cfg.Embedding.Provider,
		"vectordb_provider", cfg.VectorDB.Provider)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Initialize embedding provider
	emb, err := embedder.NewProvider(cfg.Embedding)
	if err != nil {
		logger.Error("failed to create embedder", "error", err)
		os.Exit(1)
	}

	// Check embedder health (with retries for startup)
	logger.Info("waiting for embedder...", "endpoint", cfg.Embedding.Endpoint)
	for i := 0; i < 30; i++ {
		if err := emb.Health(ctx); err == nil {
			break
		}
		if i == 29 {
			logger.Error("embedder health check failed after retries", "error", err)
			os.Exit(1)
		}
		select {
		case <-ctx.Done():
			os.Exit(0)
		case <-make(chan struct{}):
		default:
			// Sleep 1 second between retries
			sleepCtx, sleepCancel := context.WithTimeout(ctx, 1e9)
			<-sleepCtx.Done()
			sleepCancel()
		}
	}
	logger.Info("embedder connected",
		"provider", cfg.Embedding.Provider,
		"model", cfg.Embedding.Model)

	// Initialize vector database
	vdb, err := vectordb.NewProvider(cfg.VectorDB)
	if err != nil {
		logger.Error("failed to create vectordb", "error", err)
		os.Exit(1)
	}

	// Check vectordb health (with retries for startup)
	logger.Info("waiting for vectordb...", "endpoint", cfg.VectorDB.Endpoint)
	for i := 0; i < 30; i++ {
		if err := vdb.Health(ctx); err == nil {
			break
		}
		if i == 29 {
			logger.Error("vectordb health check failed after retries", "error", err)
			os.Exit(1)
		}
		select {
		case <-ctx.Done():
			os.Exit(0)
		default:
			sleepCtx, sleepCancel := context.WithTimeout(ctx, 1e9)
			<-sleepCtx.Done()
			sleepCancel()
		}
	}
	logger.Info("vectordb connected",
		"provider", cfg.VectorDB.Provider,
		"collection", cfg.VectorDB.CollectionName)

	// Ensure collection exists
	if err := vdb.EnsureCollection(ctx, cfg.Embedding.Dimensions); err != nil {
		logger.Warn("failed to ensure collection (may already exist)", "error", err)
	}

	// Create and start server
	server := api.NewServer(cfgManager, emb, vdb, logger)

	if err := server.Start(ctx); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
