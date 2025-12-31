// Package embedder provides a pluggable interface for text embedding providers.
// It abstracts different embedding services (Ollama, OpenAI, etc.) behind a common interface.
package embedder

import (
	"context"
)

// Provider defines the interface for embedding providers.
// All embedding implementations must satisfy this interface.
type Provider interface {
	// Embed generates an embedding vector for a single text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embedding vectors for multiple texts.
	// This is more efficient than calling Embed multiple times.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// ModelInfo returns information about the current model.
	ModelInfo() ModelInfo

	// Health checks if the provider is available.
	Health(ctx context.Context) error

	// Close releases any resources held by the provider.
	Close() error
}

// ModelInfo contains metadata about an embedding model.
type ModelInfo struct {
	// Provider name (e.g., "ollama", "openai")
	Provider string

	// Model name (e.g., "nomic-embed-text")
	Model string

	// Vector dimensions
	Dimensions int
}

// Config holds common configuration for embedding providers.
type Config struct {
	// Provider name
	Provider string

	// Model name
	Model string

	// API endpoint URL
	Endpoint string

	// Vector dimensions
	Dimensions int

	// Batch size for bulk operations
	BatchSize int

	// API key (for providers that require it)
	APIKey string

	// Request timeout in seconds
	TimeoutSeconds int
}

// EmbedResult represents the result of an embedding operation.
type EmbedResult struct {
	// The embedding vector
	Vector []float32

	// Token count (if available)
	TokenCount int
}

// BatchEmbedResult represents the result of a batch embedding operation.
type BatchEmbedResult struct {
	// Embedding vectors (one per input text)
	Vectors [][]float32

	// Total token count (if available)
	TotalTokens int
}
