package indexer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCache_BasicOperations(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create cache
	cache, err := NewCache(tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	// Test Set and Get
	entry := CacheEntry{
		ContentHash: "abc123",
		ModTime:     time.Now().UTC(),
		IndexedAt:   time.Now().UTC(),
		ChunkIDs:    []string{"chunk1", "chunk2"},
	}

	cache.Set("src/main.go", entry)

	retrieved, exists := cache.Get("src/main.go")
	if !exists {
		t.Error("Expected entry to exist after Set")
	}
	if retrieved.ContentHash != "abc123" {
		t.Errorf("Expected ContentHash 'abc123', got '%s'", retrieved.ContentHash)
	}
	if len(retrieved.ChunkIDs) != 2 {
		t.Errorf("Expected 2 ChunkIDs, got %d", len(retrieved.ChunkIDs))
	}

	// Test non-existent key
	_, exists = cache.Get("nonexistent.go")
	if exists {
		t.Error("Expected entry to not exist")
	}
}

func TestCache_HasChanged(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache, _ := NewCache(tmpDir, "test-project")

	// New file should be marked as changed
	if !cache.HasChanged("new_file.go", "hash123") {
		t.Error("New file should be marked as changed")
	}

	// Set file
	cache.Set("new_file.go", CacheEntry{ContentHash: "hash123"})

	// Same hash should not be changed
	if cache.HasChanged("new_file.go", "hash123") {
		t.Error("File with same hash should not be marked as changed")
	}

	// Different hash should be changed
	if !cache.HasChanged("new_file.go", "hash456") {
		t.Error("File with different hash should be marked as changed")
	}
}

func TestCache_Delete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache, _ := NewCache(tmpDir, "test-project")

	cache.Set("file.go", CacheEntry{ContentHash: "hash1"})
	cache.Delete("file.go")

	_, exists := cache.Get("file.go")
	if exists {
		t.Error("Entry should not exist after Delete")
	}
}

func TestCache_GetAllFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache, _ := NewCache(tmpDir, "test-project")

	cache.Set("file1.go", CacheEntry{ContentHash: "hash1"})
	cache.Set("file2.go", CacheEntry{ContentHash: "hash2"})
	cache.Set("file3.go", CacheEntry{ContentHash: "hash3"})

	files := cache.GetAllFiles()
	if len(files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files))
	}
}

func TestCache_ChunkHashes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache, _ := NewCache(tmpDir, "test-project")

	// Set entry with chunk hashes
	cache.Set("file.go", CacheEntry{
		ContentHash: "filehash",
		ChunkIDs:    []string{"chunk1", "chunk2"},
		ChunkHashes: map[string]string{
			"chunk1": "chunkhash1",
			"chunk2": "chunkhash2",
		},
	})

	// Get chunk hashes
	hashes := cache.GetChunkHashes("file.go")
	if len(hashes) != 2 {
		t.Errorf("Expected 2 chunk hashes, got %d", len(hashes))
	}
	if hashes["chunk1"] != "chunkhash1" {
		t.Errorf("Expected 'chunkhash1', got '%s'", hashes["chunk1"])
	}

	// Non-existent file
	emptyHashes := cache.GetChunkHashes("nonexistent.go")
	if len(emptyHashes) != 0 {
		t.Error("Expected empty map for non-existent file")
	}

	// Update chunk hashes
	cache.SetChunkHashes("file.go", map[string]string{
		"chunk1": "newhash1",
		"chunk3": "chunkhash3",
	})

	updatedHashes := cache.GetChunkHashes("file.go")
	if updatedHashes["chunk1"] != "newhash1" {
		t.Errorf("Expected 'newhash1', got '%s'", updatedHashes["chunk1"])
	}
	if _, exists := updatedHashes["chunk2"]; exists {
		t.Error("chunk2 should not exist after SetChunkHashes")
	}
}

func TestCache_SaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create and populate cache
	cache1, _ := NewCache(tmpDir, "test-project")
	cache1.Set("file1.go", CacheEntry{
		ContentHash: "hash1",
		ChunkIDs:    []string{"chunk1"},
		ChunkHashes: map[string]string{"chunk1": "chunkhash1"},
	})
	cache1.Set("file2.go", CacheEntry{
		ContentHash: "hash2",
		ChunkIDs:    []string{"chunk2", "chunk3"},
	})

	// Save cache
	if err := cache1.Save("test-project"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	cachePath := filepath.Join(tmpDir, "test-project.json")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("Cache file was not created")
	}

	// Load cache in new instance
	cache2, err := NewCache(tmpDir, "test-project")
	if err != nil {
		t.Fatalf("Loading cache failed: %v", err)
	}

	// Verify data was persisted
	entry1, exists := cache2.Get("file1.go")
	if !exists {
		t.Error("file1.go should exist after load")
	}
	if entry1.ContentHash != "hash1" {
		t.Errorf("Expected ContentHash 'hash1', got '%s'", entry1.ContentHash)
	}
	if len(entry1.ChunkHashes) != 1 {
		t.Errorf("Expected 1 chunk hash, got %d", len(entry1.ChunkHashes))
	}

	entry2, exists := cache2.Get("file2.go")
	if !exists {
		t.Error("file2.go should exist after load")
	}
	if len(entry2.ChunkIDs) != 2 {
		t.Errorf("Expected 2 ChunkIDs, got %d", len(entry2.ChunkIDs))
	}
}

func TestCache_Clear(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache, _ := NewCache(tmpDir, "test-project")

	cache.Set("file1.go", CacheEntry{ContentHash: "hash1"})
	cache.Set("file2.go", CacheEntry{ContentHash: "hash2"})

	stats := cache.Stats()
	if stats.FileCount != 2 {
		t.Errorf("Expected 2 files before clear, got %d", stats.FileCount)
	}

	cache.Clear()

	stats = cache.Stats()
	if stats.FileCount != 0 {
		t.Errorf("Expected 0 files after clear, got %d", stats.FileCount)
	}
}

func TestCache_Stats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache, _ := NewCache(tmpDir, "test-project")

	cache.Set("file1.go", CacheEntry{
		ContentHash: "hash1",
		ChunkIDs:    []string{"chunk1", "chunk2", "chunk3"},
	})
	cache.Set("file2.go", CacheEntry{
		ContentHash: "hash2",
		ChunkIDs:    []string{"chunk4", "chunk5"},
	})

	stats := cache.Stats()
	if stats.FileCount != 2 {
		t.Errorf("Expected FileCount 2, got %d", stats.FileCount)
	}
	if stats.ChunkCount != 5 {
		t.Errorf("Expected ChunkCount 5, got %d", stats.ChunkCount)
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cache, _ := NewCache(tmpDir, "test-project")

	// Run concurrent operations
	done := make(chan bool)

	// Writer 1
	go func() {
		for i := 0; i < 100; i++ {
			cache.Set("file1.go", CacheEntry{ContentHash: "hash1"})
		}
		done <- true
	}()

	// Writer 2
	go func() {
		for i := 0; i < 100; i++ {
			cache.Set("file2.go", CacheEntry{ContentHash: "hash2"})
		}
		done <- true
	}()

	// Reader
	go func() {
		for i := 0; i < 100; i++ {
			cache.Get("file1.go")
			cache.GetAllFiles()
			cache.Stats()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// If we get here without panic, concurrent access works
}
