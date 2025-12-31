// Package indexer provides the core indexing logic for processing projects.
// It handles file discovery, chunking, embedding, and vector storage.
package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iasik/project-indexer/internal/chunker"
	"github.com/iasik/project-indexer/internal/config"
	"github.com/iasik/project-indexer/internal/embedder"
	"github.com/iasik/project-indexer/internal/vectordb"
)

// Indexer handles project indexing operations.
type Indexer struct {
	cfg             *config.Config
	embedder        embedder.Provider
	vectorDB        vectordb.Provider
	chunkerFactory  *chunker.Factory
	logger          *slog.Logger
	workerCount     int
}

// NewIndexer creates a new indexer instance.
func NewIndexer(
	cfg *config.Config,
	emb embedder.Provider,
	vdb vectordb.Provider,
	logger *slog.Logger,
) *Indexer {
	return &Indexer{
		cfg:            cfg,
		embedder:       emb,
		vectorDB:       vdb,
		chunkerFactory: chunker.NewFactory(cfg.Chunking),
		logger:         logger,
		workerCount:    4, // Parallel file processing
	}
}

// IndexResult contains the results of an indexing operation.
type IndexResult struct {
	ProjectID       string
	FilesScanned    int
	FilesIndexed    int
	FilesSkipped    int
	FilesDeleted    int
	ChunksCreated   int
	ChunksDeleted   int
	OversizedChunks []OversizedChunk
	Duration        time.Duration
	Errors          []error
}

// OversizedChunk represents a chunk that exceeds token limits.
type OversizedChunk struct {
	FilePath    string `json:"file_path"`
	Symbol      string `json:"symbol"`
	TokenCount  int    `json:"token_count"`
	MaxAllowed  int    `json:"max_allowed"`
	ContentSize int    `json:"content_size_bytes"`
}

// IndexProject indexes a single project.
func (idx *Indexer) IndexProject(ctx context.Context, projectCfg *config.ProjectConfig, fullIndex bool) (*IndexResult, error) {
	startTime := time.Now()
	result := &IndexResult{
		ProjectID: projectCfg.ProjectID,
		Errors:    make([]error, 0),
	}

	idx.logger.Info("starting indexing",
		"project", projectCfg.ProjectID,
		"full_index", fullIndex)

	// Load or create cache
	cache, err := NewCache(idx.cfg.Cache.Dir, projectCfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}

	if fullIndex {
		// Clear cache for full reindex
		cache.Clear()
		// Delete all vectors for this project
		if err := idx.vectorDB.DeleteByFilter(ctx, vectordb.Filter{ProjectID: projectCfg.ProjectID}); err != nil {
			return nil, fmt.Errorf("failed to clear vectors: %w", err)
		}
		idx.logger.Info("cleared existing index", "project", projectCfg.ProjectID)
	}

	// Get effective chunking config
	chunkCfg := projectCfg.GetEffectiveChunking(idx.cfg.Chunking)
	idx.chunkerFactory = chunker.NewFactory(chunkCfg)

	// Discover files
	sourcePath := projectCfg.GetFullSourcePath(idx.cfg.Projects.SourceBasePath)
	files, err := idx.discoverFiles(sourcePath, projectCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to discover files: %w", err)
	}
	result.FilesScanned = len(files)
	idx.logger.Info("discovered files", "count", len(files))

	// Find deleted files (in cache but not in filesystem)
	if !fullIndex {
		deletedFiles := idx.findDeletedFiles(cache, files)
		for _, filePath := range deletedFiles {
			chunkIDs := cache.GetChunkIDs(filePath)
			if len(chunkIDs) > 0 {
				if err := idx.vectorDB.Delete(ctx, chunkIDs); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("delete chunks for %s: %w", filePath, err))
				} else {
					result.ChunksDeleted += len(chunkIDs)
				}
			}
			cache.Delete(filePath)
			result.FilesDeleted++
		}
		if len(deletedFiles) > 0 {
			idx.logger.Info("deleted stale files", "count", len(deletedFiles))
		}
	}

	// Process files
	filesToProcess := make([]fileToProcess, 0)
	for _, file := range files {
		contentHash, err := hashFile(file.absPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("hash %s: %w", file.relPath, err))
			continue
		}

		if !fullIndex && !cache.HasChanged(file.relPath, contentHash) {
			result.FilesSkipped++
			continue
		}

		filesToProcess = append(filesToProcess, fileToProcess{
			absPath:     file.absPath,
			relPath:     file.relPath,
			contentHash: contentHash,
		})
	}

	idx.logger.Info("files to process",
		"total", len(files),
		"changed", len(filesToProcess),
		"skipped", result.FilesSkipped)

	// Process changed files in parallel
	processResult := idx.processFiles(ctx, filesToProcess, projectCfg, cache)
	result.FilesIndexed = processResult.filesIndexed
	result.ChunksCreated = processResult.chunksCreated
	result.OversizedChunks = processResult.oversizedChunks
	result.Errors = append(result.Errors, processResult.errors...)

	// Save oversized chunks report if any
	if len(result.OversizedChunks) > 0 {
		idx.saveOversizedReport(projectCfg.ProjectID, result.OversizedChunks)
	}

	// Save cache
	if err := cache.Save(projectCfg.ProjectID); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("save cache: %w", err))
	}

	result.Duration = time.Since(startTime)
	idx.logger.Info("indexing complete",
		"project", projectCfg.ProjectID,
		"files_indexed", result.FilesIndexed,
		"chunks_created", result.ChunksCreated,
		"duration", result.Duration)

	return result, nil
}

// discoveredFile represents a file found during scanning.
type discoveredFile struct {
	absPath string
	relPath string
}

// discoverFiles finds all indexable files in the project.
func (idx *Indexer) discoverFiles(rootPath string, projectCfg *config.ProjectConfig) ([]discoveredFile, error) {
	var files []discoveredFile

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		// Check exclusions
		if projectCfg.ShouldExcludePath(relPath) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Check file extension
		if !projectCfg.ShouldIncludeFile(path) {
			return nil
		}

		files = append(files, discoveredFile{
			absPath: path,
			relPath: relPath,
		})

		return nil
	})

	return files, err
}

// findDeletedFiles finds files in cache that no longer exist.
func (idx *Indexer) findDeletedFiles(cache *Cache, currentFiles []discoveredFile) []string {
	currentSet := make(map[string]bool)
	for _, f := range currentFiles {
		currentSet[f.relPath] = true
	}

	var deleted []string
	for _, cachedPath := range cache.GetAllFiles() {
		if !currentSet[cachedPath] {
			deleted = append(deleted, cachedPath)
		}
	}
	return deleted
}

// fileToProcess represents a file queued for processing.
type fileToProcess struct {
	absPath     string
	relPath     string
	contentHash string
}

// processResult contains results from parallel file processing.
type processResult struct {
	filesIndexed    int
	chunksCreated   int
	oversizedChunks []OversizedChunk
	errors          []error
}

// ProgressStats tracks processing progress and timing.
type ProgressStats struct {
	mu              sync.Mutex
	totalFiles      int
	processedFiles  int64
	startTime       time.Time
	fileTimes       []time.Duration
}

// Update records a completed file and its processing time.
func (p *ProgressStats) Update(duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	atomic.AddInt64(&p.processedFiles, 1)
	p.fileTimes = append(p.fileTimes, duration)
}

// GetStats returns current progress statistics.
func (p *ProgressStats) GetStats() (processed int, total int, avgDuration time.Duration, eta time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	processed = int(atomic.LoadInt64(&p.processedFiles))
	total = p.totalFiles
	
	if len(p.fileTimes) > 0 {
		var sum time.Duration
		for _, d := range p.fileTimes {
			sum += d
		}
		avgDuration = sum / time.Duration(len(p.fileTimes))
		remaining := total - processed
		eta = avgDuration * time.Duration(remaining)
	}
	return
}

// processFiles processes files in parallel with progress reporting.
func (idx *Indexer) processFiles(
	ctx context.Context,
	files []fileToProcess,
	projectCfg *config.ProjectConfig,
	cache *Cache,
) processResult {
	result := processResult{
		errors:          make([]error, 0),
		oversizedChunks: make([]OversizedChunk, 0),
	}

	if len(files) == 0 {
		return result
	}

	totalFiles := len(files)
	stats := &ProgressStats{
		totalFiles: totalFiles,
		startTime:  time.Now(),
		fileTimes:  make([]time.Duration, 0, totalFiles),
	}

	// Progress reporter - prints status every 3 seconds
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				processed, total, avgDur, eta := stats.GetStats()
				if processed >= total {
					return
				}
				
				percent := float64(processed) / float64(total) * 100
				elapsed := time.Since(stats.startTime).Round(time.Second)
				
				// Format ETA
				etaStr := "calculating..."
				if avgDur > 0 && processed > 0 {
					etaStr = eta.Round(time.Second).String()
				}
				
				// Print progress (with newline for Docker compatibility)
				fmt.Printf("[Progress] %d/%d files (%.1f%%) | Elapsed: %s | ETA: %s | Avg: %s/file\n",
					processed, total, percent, elapsed, etaStr, avgDur.Round(time.Millisecond))
			}
		}
	}()

	// Create work channel
	workCh := make(chan fileToProcess, len(files))
	for _, f := range files {
		workCh <- f
	}
	close(workCh)

	// Result collection
	type fileResult struct {
		relPath   string
		chunks    []chunker.Chunk
		chunkIDs  []string
		hash      string
		oversized []OversizedChunk
		duration  time.Duration
		err       error
	}
	resultCh := make(chan fileResult, len(files))

	// Token limit for embedding model (nomic-embed-text = 2048)
	// Using ~2.5 chars per token for conservative estimation
	// This catches chunks that might get truncated by the model
	const maxTokens = 2048
	const charsPerToken = 2.5

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < idx.workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range workCh {
				select {
				case <-ctx.Done():
					return
				default:
				}

				fileStart := time.Now()
				chunks, err := idx.processFile(ctx, file, projectCfg)
				fileDuration := time.Since(fileStart)
				
				var chunkIDs []string
				var oversized []OversizedChunk
				
				for _, c := range chunks {
					chunkIDs = append(chunkIDs, c.ID)
					// Estimate token count using ~3.5 chars per token
					estimatedTokens := int(float64(len(c.Content)) / charsPerToken)
					if estimatedTokens > maxTokens {
						oversized = append(oversized, OversizedChunk{
							FilePath:    file.relPath,
							Symbol:      c.Symbol,
							TokenCount:  estimatedTokens,
							MaxAllowed:  maxTokens,
							ContentSize: len(c.Content),
						})
					}
				}

				stats.Update(fileDuration)

				resultCh <- fileResult{
					relPath:   file.relPath,
					chunks:    chunks,
					chunkIDs:  chunkIDs,
					hash:      file.contentHash,
					oversized: oversized,
					duration:  fileDuration,
					err:       err,
				}
			}
		}()
	}

	// Close result channel when workers are done
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results and batch upsert
	var allChunks []chunker.Chunk
	var mu sync.Mutex

	for res := range resultCh {
		if res.err != nil {
			mu.Lock()
			result.errors = append(result.errors, fmt.Errorf("%s: %w", res.relPath, res.err))
			mu.Unlock()
			continue
		}

		mu.Lock()
		result.filesIndexed++
		result.chunksCreated += len(res.chunks)
		result.oversizedChunks = append(result.oversizedChunks, res.oversized...)
		allChunks = append(allChunks, res.chunks...)

		// Update cache
		cache.Set(res.relPath, CacheEntry{
			ContentHash: res.hash,
			ModTime:     time.Now().UTC(),
			IndexedAt:   time.Now().UTC(),
			ChunkIDs:    res.chunkIDs,
		})
		mu.Unlock()
	}

	// Print final progress
	processed, total, avgDur, _ := stats.GetStats()
	totalElapsed := time.Since(stats.startTime).Round(time.Second)
	fmt.Printf("[Complete] %d/%d files processed in %s (avg: %s/file)\n",
		processed, total, totalElapsed, avgDur.Round(time.Millisecond))

	// Batch upsert all chunks
	if len(allChunks) > 0 {
		fmt.Printf("[Upserting] %d chunks to vector database...\n", len(allChunks))
		if err := idx.upsertChunks(ctx, allChunks); err != nil {
			result.errors = append(result.errors, fmt.Errorf("upsert chunks: %w", err))
		}
	}

	return result
}

// processFile processes a single file: read, chunk, embed.
func (idx *Indexer) processFile(
	ctx context.Context,
	file fileToProcess,
	projectCfg *config.ProjectConfig,
) ([]chunker.Chunk, error) {
	// Read file content
	content, err := os.ReadFile(file.absPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Create file metadata
	metadata := chunker.FileMetadata{
		FilePath:  file.relPath,
		Language:  chunker.DetectLanguage(file.relPath),
		Module:    chunker.ExtractModule(file.relPath),
		ProjectID: projectCfg.ProjectID,
	}

	// Get appropriate chunker
	chunkr := idx.chunkerFactory.GetChunker(file.relPath)

	// Chunk the file
	chunks, err := chunkr.Chunk(content, metadata)
	if err != nil {
		return nil, fmt.Errorf("chunk file: %w", err)
	}

	return chunks, nil
}

// upsertChunks embeds and upserts chunks to vector DB.
func (idx *Indexer) upsertChunks(ctx context.Context, chunks []chunker.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	// Extract content for embedding
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	// Get embeddings in batches with progress
	batchSize := idx.cfg.Embedding.BatchSize
	var allVectors [][]float32
	totalBatches := (len(texts) + batchSize - 1) / batchSize
	
	embedStart := time.Now()
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batchNum := (i / batchSize) + 1
		batch := texts[i:end]
		
		batchStart := time.Now()
		vectors, err := idx.embedder.EmbedBatch(ctx, batch)
		batchDuration := time.Since(batchStart)
		
		if err != nil {
			return fmt.Errorf("embed batch %d-%d: %w", i, end, err)
		}
		allVectors = append(allVectors, vectors...)
		
		// Calculate ETA
		elapsed := time.Since(embedStart)
		avgPerBatch := elapsed / time.Duration(batchNum)
		remainingBatches := totalBatches - batchNum
		eta := avgPerBatch * time.Duration(remainingBatches)
		
		fmt.Printf("[Embedding] Batch %d/%d (%d chunks) | took: %s | ETA: %s\n",
			batchNum, totalBatches, len(batch), 
			batchDuration.Round(time.Millisecond), 
			eta.Round(time.Second))
	}
	
	fmt.Printf("[Embedding] Complete: %d chunks in %s\n", len(texts), time.Since(embedStart).Round(time.Second))

	// Create points for vector DB
	points := make([]vectordb.Point, len(chunks))
	indexedAt := time.Now().UTC().Format(time.RFC3339)

	for i, c := range chunks {
		points[i] = vectordb.Point{
			ID:     c.ID,
			Vector: allVectors[i],
			Payload: vectordb.Payload{
				ProjectID:   c.ProjectID,
				FilePath:    c.FilePath,
				Symbol:      c.Symbol,
				SymbolType:  c.SymbolType,
				Language:    c.Language,
				Module:      c.Module,
				StartLine:   c.StartLine,
				EndLine:     c.EndLine,
				Content:     c.Content,
				ContentHash: c.ContentHash,
				IndexedAt:   indexedAt,
			},
		}
	}

	// Upsert to vector DB
	return idx.vectorDB.Upsert(ctx, points)
}

// hashFile computes SHA256 hash of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// EnsureCollection ensures the vector DB collection exists.
func (idx *Indexer) EnsureCollection(ctx context.Context) error {
	return idx.vectorDB.EnsureCollection(ctx, idx.cfg.Embedding.Dimensions)
}

// saveOversizedReport saves oversized chunks to a JSON file for review.
func (idx *Indexer) saveOversizedReport(projectID string, chunks []OversizedChunk) {
	reportDir := filepath.Join(idx.cfg.Cache.Dir, "reports")
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		idx.logger.Error("failed to create reports directory", "error", err)
		return
	}

	reportFile := filepath.Join(reportDir, fmt.Sprintf("%s-oversized.json", projectID))
	
	report := struct {
		ProjectID  string           `json:"project_id"`
		GeneratedAt string          `json:"generated_at"`
		TotalCount int              `json:"total_count"`
		MaxTokens  int              `json:"max_tokens_allowed"`
		Chunks     []OversizedChunk `json:"chunks"`
	}{
		ProjectID:   projectID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		TotalCount:  len(chunks),
		MaxTokens:   2048,
		Chunks:      chunks,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		idx.logger.Error("failed to marshal oversized report", "error", err)
		return
	}

	if err := os.WriteFile(reportFile, data, 0644); err != nil {
		idx.logger.Error("failed to write oversized report", "error", err)
		return
	}

	idx.logger.Warn("oversized chunks detected",
		"count", len(chunks),
		"report", reportFile)
}

// IndexAllProjects indexes all configured projects.
func (idx *Indexer) IndexAllProjects(ctx context.Context, fullIndex bool) (map[string]*IndexResult, error) {
	projects, err := config.LoadAllProjects(idx.cfg.Projects.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load projects: %w", err)
	}

	results := make(map[string]*IndexResult)

	for projectID, projectCfg := range projects {
		result, err := idx.IndexProject(ctx, projectCfg, fullIndex)
		if err != nil {
			idx.logger.Error("failed to index project",
				"project", projectID,
				"error", err)
			results[projectID] = &IndexResult{
				ProjectID: projectID,
				Errors:    []error{err},
			}
			continue
		}
		results[projectID] = result
	}

	return results, nil
}
