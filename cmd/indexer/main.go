// Indexer CLI - Batch indexing tool for projects
//
// Usage:
//
//	indexer --project=myproject         # Incremental index
//	indexer --project=myproject --full  # Full reindex
//	indexer --all                       # Index all projects
//	indexer --all --full                # Full reindex all projects
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/iasik/project-indexer/internal/config"
	"github.com/iasik/project-indexer/internal/embedder"
	"github.com/iasik/project-indexer/internal/indexer"
	"github.com/iasik/project-indexer/internal/vectordb"
)

func main() {
	// Parse command line flags
	projectID := flag.String("project", "", "Project ID to index")
	fullIndex := flag.Bool("full", false, "Perform full reindex (clear existing)")
	indexAll := flag.Bool("all", false, "Index all configured projects")
	flag.Parse()

	// Validate flags
	if *projectID == "" && !*indexAll {
		fmt.Fprintln(os.Stderr, "Error: --project or --all is required")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  indexer --project=myproject         # Incremental index")
		fmt.Fprintln(os.Stderr, "  indexer --project=myproject --full  # Full reindex")
		fmt.Fprintln(os.Stderr, "  indexer --all                       # Index all projects")
		os.Exit(1)
	}

	// Setup logger
	logLevel := slog.LevelInfo
	if os.Getenv("DEBUG") != "" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Load configuration
	cfgManager, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	cfg := cfgManager.Get()

	logger.Info("configuration loaded",
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
	defer emb.Close()

	// Check embedder health
	if err := emb.Health(ctx); err != nil {
		logger.Error("embedder health check failed", "error", err)
		logger.Info("hint: ensure embedding model is available",
			"model", cfg.Embedding.Model,
			"endpoint", cfg.Embedding.Endpoint)
		os.Exit(1)
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
	defer vdb.Close()

	// Check vectordb health
	if err := vdb.Health(ctx); err != nil {
		logger.Error("vectordb health check failed", "error", err)
		os.Exit(1)
	}
	logger.Info("vectordb connected",
		"provider", cfg.VectorDB.Provider,
		"collection", cfg.VectorDB.CollectionName)

	// Create indexer
	idx := indexer.NewIndexer(cfg, emb, vdb, logger)

	// Ensure collection exists
	if err := idx.EnsureCollection(ctx); err != nil {
		logger.Error("failed to ensure collection", "error", err)
		os.Exit(1)
	}

	// Run indexing
	if *indexAll {
		results, err := idx.IndexAllProjects(ctx, *fullIndex)
		if err != nil {
			logger.Error("indexing failed", "error", err)
			os.Exit(1)
		}

		// Print summary
		fmt.Println("\n=== Indexing Summary ===")
		totalFiles := 0
		totalChunks := 0
		hasErrors := false

		for projectID, result := range results {
			fmt.Printf("\nProject: %s\n", projectID)
			fmt.Printf("  Files indexed: %d\n", result.FilesIndexed)
			fmt.Printf("  Chunks created: %d\n", result.ChunksCreated)
			fmt.Printf("  Duration: %s\n", result.Duration)

			if len(result.Errors) > 0 {
				hasErrors = true
				fmt.Printf("  Errors: %d\n", len(result.Errors))
				for _, err := range result.Errors {
					fmt.Printf("    - %v\n", err)
				}
			}

			totalFiles += result.FilesIndexed
			totalChunks += result.ChunksCreated
		}

		fmt.Printf("\nTotal: %d files, %d chunks across %d projects\n",
			totalFiles, totalChunks, len(results))

		if hasErrors {
			os.Exit(1)
		}
	} else {
		// Index single project
		projectCfg, err := config.GetProject(cfg.Projects.ConfigDir, *projectID)
		if err != nil {
			logger.Error("failed to load project config", "project", *projectID, "error", err)
			os.Exit(1)
		}

		result, err := idx.IndexProject(ctx, projectCfg, *fullIndex)
		if err != nil {
			logger.Error("indexing failed", "error", err)
			os.Exit(1)
		}

		// Print summary
		fmt.Println("\n=== Indexing Complete ===")
		fmt.Printf("Project: %s\n", result.ProjectID)
		fmt.Printf("Files scanned: %d\n", result.FilesScanned)
		fmt.Printf("Files indexed: %d\n", result.FilesIndexed)
		fmt.Printf("Files skipped: %d\n", result.FilesSkipped)
		fmt.Printf("Files deleted: %d\n", result.FilesDeleted)
		fmt.Printf("Chunks created: %d\n", result.ChunksCreated)
		fmt.Printf("Chunks deleted: %d\n", result.ChunksDeleted)
		fmt.Printf("Duration: %s\n", result.Duration)

		if len(result.OversizedChunks) > 0 {
			fmt.Printf("Oversized chunks: %d (see data/index-cache/reports/%s-oversized.json)\n", 
				len(result.OversizedChunks), result.ProjectID)
		}

		if len(result.Errors) > 0 {
			fmt.Printf("Errors: %d\n", len(result.Errors))
			for _, err := range result.Errors {
				fmt.Printf("  - %v\n", err)
			}
			os.Exit(1)
		}
	}

	logger.Info("indexing completed successfully")
}
