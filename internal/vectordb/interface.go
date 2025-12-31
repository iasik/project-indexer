// Package vectordb provides a pluggable interface for vector database providers.
// It abstracts different vector stores (Qdrant, Milvus, etc.) behind a common interface.
package vectordb

import (
	"context"
)

// Provider defines the interface for vector database providers.
// All vector database implementations must satisfy this interface.
type Provider interface {
	// Upsert inserts or updates vectors with metadata.
	Upsert(ctx context.Context, points []Point) error

	// Search performs similarity search with optional filters.
	Search(ctx context.Context, query SearchQuery) ([]SearchResult, error)

	// Delete removes vectors by their IDs.
	Delete(ctx context.Context, ids []string) error

	// DeleteByFilter removes vectors matching a filter.
	DeleteByFilter(ctx context.Context, filter Filter) error

	// EnsureCollection creates the collection if it doesn't exist.
	EnsureCollection(ctx context.Context, dimensions int) error

	// Health checks if the provider is available.
	Health(ctx context.Context) error

	// Close releases any resources held by the provider.
	Close() error
}

// Point represents a vector with its metadata.
type Point struct {
	// Unique identifier for this vector
	ID string

	// The embedding vector
	Vector []float32

	// Associated metadata
	Payload Payload
}

// Payload contains metadata associated with a vector.
type Payload struct {
	// Project identifier
	ProjectID string `json:"project_id"`

	// Relative file path within the project
	FilePath string `json:"file_path"`

	// Symbol name (function, struct, etc.)
	Symbol string `json:"symbol,omitempty"`

	// Symbol type (function, struct, method, etc.)
	SymbolType string `json:"symbol_type,omitempty"`

	// Programming language
	Language string `json:"language"`

	// Module/package name
	Module string `json:"module,omitempty"`

	// Start line in the file
	StartLine int `json:"start_line"`

	// End line in the file
	EndLine int `json:"end_line"`

	// The actual content/code
	Content string `json:"content"`

	// Hash of the content for change detection
	ContentHash string `json:"content_hash"`

	// When this chunk was indexed
	IndexedAt string `json:"indexed_at"`
}

// SearchQuery defines parameters for a similarity search.
type SearchQuery struct {
	// Query vector
	Vector []float32

	// Number of results to return
	TopK int

	// Optional filters
	Filter Filter

	// Minimum similarity score (0.0 to 1.0)
	ScoreThreshold float32
}

// Filter defines conditions for filtering search results.
type Filter struct {
	// Project ID to filter by (required for multi-project isolation)
	ProjectID string

	// Optional: filter by module
	Module string

	// Optional: filter by language
	Language string

	// Optional: filter by symbol type
	SymbolType string
}

// SearchResult represents a single search result.
type SearchResult struct {
	// Point ID
	ID string

	// Similarity score (0.0 to 1.0 for cosine)
	Score float32

	// The payload/metadata
	Payload Payload
}

// Config holds common configuration for vector database providers.
type Config struct {
	// Provider name
	Provider string

	// API endpoint URL
	Endpoint string

	// Collection/index name
	CollectionName string

	// Request timeout in seconds
	TimeoutSeconds int
}
