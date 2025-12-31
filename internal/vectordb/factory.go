// Package vectordb provides a factory for creating vector database providers.
package vectordb

import (
	"fmt"

	"github.com/iasik/project-indexer/internal/config"
)

// NewProvider creates a vector database provider based on configuration.
// This is the main entry point for obtaining a vector database client.
func NewProvider(cfg config.VectorDBConfig) (Provider, error) {
	providerCfg := Config{
		Provider:       cfg.Provider,
		Endpoint:       cfg.Endpoint,
		CollectionName: cfg.CollectionName,
		TimeoutSeconds: int(cfg.GetTimeout().Seconds()),
	}

	switch cfg.Provider {
	case "qdrant":
		return NewQdrantClient(providerCfg)

	case "milvus":
		// TODO: Implement Milvus client
		return nil, fmt.Errorf("milvus provider not yet implemented")

	case "weaviate":
		// TODO: Implement Weaviate client
		return nil, fmt.Errorf("weaviate provider not yet implemented")

	default:
		return nil, fmt.Errorf("unknown vectordb provider: %s (supported: qdrant, milvus, weaviate)", cfg.Provider)
	}
}

// MustNewProvider creates a provider or panics on failure.
// Use this only in initialization code where failure is fatal.
func MustNewProvider(cfg config.VectorDBConfig) Provider {
	provider, err := NewProvider(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to create vectordb provider: %v", err))
	}
	return provider
}
