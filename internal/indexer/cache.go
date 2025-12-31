// Package indexer provides file hash caching for incremental indexing.
// The cache stores file content hashes to detect changes between indexing runs.
package indexer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cache manages file hash storage for incremental indexing.
type Cache struct {
	path    string
	entries map[string]CacheEntry
	mu      sync.RWMutex
	dirty   bool
}

// CacheEntry represents a cached file state.
type CacheEntry struct {
	// SHA256 hash of file content
	ContentHash string `json:"content_hash"`

	// Last modification time
	ModTime time.Time `json:"mod_time"`

	// When this file was last indexed
	IndexedAt time.Time `json:"indexed_at"`

	// Chunk IDs generated from this file
	ChunkIDs []string `json:"chunk_ids"`
}

// CacheFile is the JSON structure stored on disk.
type CacheFile struct {
	ProjectID string                `json:"project_id"`
	UpdatedAt time.Time             `json:"updated_at"`
	Files     map[string]CacheEntry `json:"files"`
}

// NewCache creates a new cache for a project.
func NewCache(cacheDir, projectID string) (*Cache, error) {
	path := filepath.Join(cacheDir, projectID+".json")

	cache := &Cache{
		path:    path,
		entries: make(map[string]CacheEntry),
	}

	// Load existing cache if present
	if err := cache.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load cache: %w", err)
	}

	return cache, nil
}

// load reads the cache from disk.
func (c *Cache) load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}

	var cacheFile CacheFile
	if err := json.Unmarshal(data, &cacheFile); err != nil {
		return fmt.Errorf("failed to parse cache: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = cacheFile.Files
	if c.entries == nil {
		c.entries = make(map[string]CacheEntry)
	}

	return nil
}

// Save writes the cache to disk.
func (c *Cache) Save(projectID string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.dirty {
		return nil
	}

	cacheFile := CacheFile{
		ProjectID: projectID,
		UpdatedAt: time.Now().UTC(),
		Files:     c.entries,
	}

	data, err := json.MarshalIndent(cacheFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Write atomically using temp file
	tmpPath := c.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	if err := os.Rename(tmpPath, c.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to save cache: %w", err)
	}

	return nil
}

// Get retrieves a cache entry for a file.
func (c *Cache) Get(filePath string) (CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[filePath]
	return entry, ok
}

// Set updates or creates a cache entry.
func (c *Cache) Set(filePath string, entry CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[filePath] = entry
	c.dirty = true
}

// Delete removes a cache entry.
func (c *Cache) Delete(filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, filePath)
	c.dirty = true
}

// HasChanged checks if a file has changed based on content hash.
func (c *Cache) HasChanged(filePath, contentHash string) bool {
	entry, exists := c.Get(filePath)
	if !exists {
		return true // New file
	}
	return entry.ContentHash != contentHash
}

// GetAllFiles returns all cached file paths.
func (c *Cache) GetAllFiles() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	files := make([]string, 0, len(c.entries))
	for path := range c.entries {
		files = append(files, path)
	}
	return files
}

// GetChunkIDs returns all chunk IDs for a file.
func (c *Cache) GetChunkIDs(filePath string) []string {
	entry, exists := c.Get(filePath)
	if !exists {
		return nil
	}
	return entry.ChunkIDs
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]CacheEntry)
	c.dirty = true
}

// Stats returns cache statistics.
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalChunks int
	for _, entry := range c.entries {
		totalChunks += len(entry.ChunkIDs)
	}

	return CacheStats{
		FileCount:  len(c.entries),
		ChunkCount: totalChunks,
	}
}

// CacheStats contains cache statistics.
type CacheStats struct {
	FileCount  int
	ChunkCount int
}
