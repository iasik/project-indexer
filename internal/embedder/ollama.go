// Package embedder provides Ollama embedding implementation.
package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaEmbedder implements the Provider interface for Ollama.
type OllamaEmbedder struct {
	client     *http.Client
	endpoint   string
	model      string
	dimensions int
}

// ollamaEmbedRequest is the request body for Ollama embeddings API.
type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// ollamaEmbedResponse is the response from Ollama embeddings API.
type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// ollamaTagsResponse is the response from Ollama tags API (for health check).
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// NewOllamaEmbedder creates a new Ollama embedding provider.
func NewOllamaEmbedder(cfg Config) (*OllamaEmbedder, error) {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &OllamaEmbedder{
		client: &http.Client{
			Timeout: timeout,
		},
		endpoint:   cfg.Endpoint,
		model:      cfg.Model,
		dimensions: cfg.Dimensions,
	}, nil
}

// Embed generates an embedding vector for a single text.
func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model:  o.model,
		Prompt: text,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/embeddings", o.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding returned")
	}

	return result.Embedding, nil
}

// EmbedBatch generates embedding vectors for multiple texts.
// Ollama doesn't have native batch support, so we process sequentially.
func (o *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))

	for i, text := range texts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		embedding, err := o.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		results[i] = embedding
	}

	return results, nil
}

// ModelInfo returns information about the current model.
func (o *OllamaEmbedder) ModelInfo() ModelInfo {
	return ModelInfo{
		Provider:   "ollama",
		Model:      o.model,
		Dimensions: o.dimensions,
	}
}

// Health checks if Ollama is available and the model is loaded.
func (o *OllamaEmbedder) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/tags", o.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("failed to decode tags response: %w", err)
	}

	// Check if our model is available
	modelFound := false
	for _, m := range tags.Models {
		if m.Name == o.model || m.Name == o.model+":latest" {
			modelFound = true
			break
		}
	}

	if !modelFound {
		return fmt.Errorf("model %s not found in ollama, run: ollama pull %s", o.model, o.model)
	}

	return nil
}

// Close releases resources (no-op for Ollama).
func (o *OllamaEmbedder) Close() error {
	return nil
}
