// Package chunker provides interfaces and types for code/text chunking.
// Chunking is the process of splitting source code and documentation into
// meaningful, self-contained pieces suitable for embedding and retrieval.
package chunker

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// Chunker defines the interface for all chunking strategies.
type Chunker interface {
	// Chunk splits content into chunks with metadata.
	Chunk(content []byte, metadata FileMetadata) ([]Chunk, error)

	// Name returns the chunker strategy name.
	Name() string
}

// FileMetadata contains information about the file being chunked.
type FileMetadata struct {
	// Relative file path within the project
	FilePath string

	// Programming language (e.g., "go", "markdown")
	Language string

	// Module/package name
	Module string

	// Project ID
	ProjectID string
}

// Chunk represents a single chunk of code or documentation.
type Chunk struct {
	// Unique identifier for this chunk (deterministic)
	ID string

	// The actual content
	Content string

	// Symbol name (function, struct, heading, etc.)
	Symbol string

	// Symbol type (function, struct, method, type, heading, etc.)
	SymbolType string

	// Start line in the original file (1-indexed)
	StartLine int

	// End line in the original file (1-indexed)
	EndLine int

	// Estimated token count
	TokenCount int

	// SHA256 hash of content for change detection
	ContentHash string

	// Inherited from FileMetadata
	FilePath  string
	Language  string
	Module    string
	ProjectID string
}

// ChunkingConfig holds chunking parameters.
type ChunkingConfig struct {
	// Minimum tokens per chunk (smaller chunks are merged)
	MinTokens int

	// Ideal chunk size in tokens
	IdealTokens int

	// Maximum tokens per chunk
	MaxTokens int

	// Whether to merge small chunks into parent scope
	MergeSmallChunks bool
}

// DefaultConfig returns default chunking configuration.
func DefaultConfig() ChunkingConfig {
	return ChunkingConfig{
		MinTokens:        200,
		IdealTokens:      500,
		MaxTokens:        800,
		MergeSmallChunks: true,
	}
}

// GenerateChunkID creates a deterministic ID for a chunk.
// Format: {project_id}:{file_path}:{symbol}:{content_hash_prefix}
func GenerateChunkID(projectID, filePath, symbol, contentHash string) string {
	hashPrefix := contentHash
	if len(hashPrefix) > 8 {
		hashPrefix = hashPrefix[:8]
	}

	// Sanitize symbol for ID
	symbolPart := symbol
	if symbolPart == "" {
		symbolPart = "_"
	}
	symbolPart = strings.ReplaceAll(symbolPart, ":", "_")

	return fmt.Sprintf("%s:%s:%s:%s", projectID, filePath, symbolPart, hashPrefix)
}

// HashContent creates a SHA256 hash of content.
func HashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

// EstimateTokens provides a rough token count estimate.
// Uses a simple heuristic: ~4 characters per token for code.
func EstimateTokens(content string) int {
	// Simple heuristic: average 4 chars per token
	// This works reasonably well for code
	return len(content) / 4
}

// SplitIntoLines splits content into lines while preserving line numbers.
type Line struct {
	Number  int
	Content string
}

func SplitIntoLines(content string) []Line {
	rawLines := strings.Split(content, "\n")
	lines := make([]Line, len(rawLines))
	for i, l := range rawLines {
		lines[i] = Line{Number: i + 1, Content: l}
	}
	return lines
}

// JoinLines combines lines back into content.
func JoinLines(lines []Line) string {
	strs := make([]string, len(lines))
	for i, l := range lines {
		strs[i] = l.Content
	}
	return strings.Join(strs, "\n")
}
