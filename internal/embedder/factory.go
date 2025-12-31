// Package embedder provides a factory for creating embedding providers.
package embedder

import (
	"fmt"

	"github.com/iasik/project-indexer/internal/config"
)

// NewProvider creates an embedding provider based on configuration.
// This is the main entry point for obtaining an embedder.
func NewProvider(cfg config.EmbeddingConfig) (Provider, error) {
	providerCfg := Config{
		Provider:       cfg.Provider,
		Model:          cfg.Model,
		Endpoint:       cfg.Endpoint,
		Dimensions:     cfg.Dimensions,
		BatchSize:      cfg.BatchSize,
		APIKey:         cfg.GetAPIKey(),
		TimeoutSeconds: int(cfg.GetTimeout().Seconds()),
	}

	switch cfg.Provider {
	case "ollama":
		return NewOllamaEmbedder(providerCfg)

	case "openai":
		return NewOpenAIEmbedder(providerCfg)

	case "huggingface":
		// TODO: Implement HuggingFace embedder
		return nil, fmt.Errorf("huggingface provider not yet implemented")

	default:
		return nil, fmt.Errorf("unknown embedding provider: %s (supported: ollama, openai, huggingface)", cfg.Provider)
	}
}

// MustNewProvider creates a provider or panics on failure.
// Use this only in initialization code where failure is fatal.
func MustNewProvider(cfg config.EmbeddingConfig) Provider {
	provider, err := NewProvider(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to create embedding provider: %v", err))
	}
	return provider
}
