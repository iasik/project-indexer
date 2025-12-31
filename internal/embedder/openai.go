// Package embedder provides OpenAI embedding implementation.
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

// OpenAIEmbedder implements the Provider interface for OpenAI.
type OpenAIEmbedder struct {
	client     *http.Client
	endpoint   string
	model      string
	apiKey     string
	dimensions int
}

// openAIEmbedRequest is the request body for OpenAI embeddings API.
type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// openAIEmbedResponse is the response from OpenAI embeddings API.
type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// NewOpenAIEmbedder creates a new OpenAI embedding provider.
func NewOpenAIEmbedder(cfg Config) (*OpenAIEmbedder, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}

	return &OpenAIEmbedder{
		client: &http.Client{
			Timeout: timeout,
		},
		endpoint:   endpoint,
		model:      cfg.Model,
		apiKey:     cfg.APIKey,
		dimensions: cfg.Dimensions,
	}, nil
}

// Embed generates an embedding vector for a single text.
func (o *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := o.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return results[0], nil
}

// EmbedBatch generates embedding vectors for multiple texts.
func (o *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := openAIEmbedRequest{
		Model: o.model,
		Input: texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", o.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("OpenAI error: %s", result.Error.Message)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	// Sort results by index to maintain order
	vectors := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < len(vectors) {
			vectors[d.Index] = d.Embedding
		}
	}

	return vectors, nil
}

// ModelInfo returns information about the current model.
func (o *OpenAIEmbedder) ModelInfo() ModelInfo {
	return ModelInfo{
		Provider:   "openai",
		Model:      o.model,
		Dimensions: o.dimensions,
	}
}

// Health checks if the OpenAI API is accessible.
func (o *OpenAIEmbedder) Health(ctx context.Context) error {
	// Simple test embedding to verify connectivity and API key
	_, err := o.Embed(ctx, "test")
	if err != nil {
		return fmt.Errorf("OpenAI health check failed: %w", err)
	}
	return nil
}

// Close releases resources (no-op for OpenAI).
func (o *OpenAIEmbedder) Close() error {
	return nil
}
