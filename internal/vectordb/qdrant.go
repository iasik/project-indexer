// Package vectordb provides Qdrant vector database implementation.
package vectordb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// QdrantClient implements the Provider interface for Qdrant.
type QdrantClient struct {
	client         *http.Client
	endpoint       string
	collectionName string
}

// Qdrant API types

type qdrantCreateCollectionRequest struct {
	Vectors struct {
		Size     int    `json:"size"`
		Distance string `json:"distance"`
	} `json:"vectors"`
}

type qdrantUpsertRequest struct {
	Points []qdrantPoint `json:"points"`
}

type qdrantPoint struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

type qdrantSearchRequest struct {
	Vector      []float32              `json:"vector"`
	Limit       int                    `json:"limit"`
	WithPayload bool                   `json:"with_payload"`
	Filter      *qdrantFilter          `json:"filter,omitempty"`
	ScoreThreshold float32             `json:"score_threshold,omitempty"`
}

type qdrantFilter struct {
	Must []qdrantCondition `json:"must,omitempty"`
}

type qdrantCondition struct {
	Key   string           `json:"key"`
	Match qdrantMatchValue `json:"match"`
}

type qdrantMatchValue struct {
	Value string `json:"value"`
}

type qdrantSearchResponse struct {
	Result []struct {
		ID      string                 `json:"id"`
		Score   float32                `json:"score"`
		Payload map[string]interface{} `json:"payload"`
	} `json:"result"`
}

type qdrantDeleteRequest struct {
	Points []string      `json:"points,omitempty"`
	Filter *qdrantFilter `json:"filter,omitempty"`
}

// NewQdrantClient creates a new Qdrant vector database client.
func NewQdrantClient(cfg Config) (*QdrantClient, error) {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &QdrantClient{
		client: &http.Client{
			Timeout: timeout,
		},
		endpoint:       cfg.Endpoint,
		collectionName: cfg.CollectionName,
	}, nil
}

// Upsert inserts or updates vectors with metadata.
func (q *QdrantClient) Upsert(ctx context.Context, points []Point) error {
	if len(points) == 0 {
		return nil
	}

	qdrantPoints := make([]qdrantPoint, len(points))
	for i, p := range points {
		// Convert string ID to UUID format (Qdrant requires UUID or uint64)
		uuid := stringToUUID(p.ID)
		qdrantPoints[i] = qdrantPoint{
			ID:     uuid,
			Vector: p.Vector,
			Payload: map[string]interface{}{
				"original_id":  p.ID, // Store original ID for reference
				"project_id":   p.Payload.ProjectID,
				"file_path":    p.Payload.FilePath,
				"symbol":       p.Payload.Symbol,
				"symbol_type":  p.Payload.SymbolType,
				"language":     p.Payload.Language,
				"module":       p.Payload.Module,
				"start_line":   p.Payload.StartLine,
				"end_line":     p.Payload.EndLine,
				"content":      p.Payload.Content,
				"content_hash": p.Payload.ContentHash,
				"indexed_at":   p.Payload.IndexedAt,
			},
		}
	}

	reqBody := qdrantUpsertRequest{Points: qdrantPoints}
	return q.doRequest(ctx, http.MethodPut,
		fmt.Sprintf("/collections/%s/points", q.collectionName),
		reqBody, nil)
}

// Search performs similarity search with optional filters.
func (q *QdrantClient) Search(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	reqBody := qdrantSearchRequest{
		Vector:         query.Vector,
		Limit:          query.TopK,
		WithPayload:    true,
		ScoreThreshold: query.ScoreThreshold,
	}

	// Build filter
	if query.Filter.ProjectID != "" || query.Filter.Module != "" ||
		query.Filter.Language != "" || query.Filter.SymbolType != "" {
		reqBody.Filter = &qdrantFilter{
			Must: make([]qdrantCondition, 0),
		}

		if query.Filter.ProjectID != "" {
			reqBody.Filter.Must = append(reqBody.Filter.Must, qdrantCondition{
				Key:   "project_id",
				Match: qdrantMatchValue{Value: query.Filter.ProjectID},
			})
		}
		if query.Filter.Module != "" {
			reqBody.Filter.Must = append(reqBody.Filter.Must, qdrantCondition{
				Key:   "module",
				Match: qdrantMatchValue{Value: query.Filter.Module},
			})
		}
		if query.Filter.Language != "" {
			reqBody.Filter.Must = append(reqBody.Filter.Must, qdrantCondition{
				Key:   "language",
				Match: qdrantMatchValue{Value: query.Filter.Language},
			})
		}
		if query.Filter.SymbolType != "" {
			reqBody.Filter.Must = append(reqBody.Filter.Must, qdrantCondition{
				Key:   "symbol_type",
				Match: qdrantMatchValue{Value: query.Filter.SymbolType},
			})
		}
	}

	var resp qdrantSearchResponse
	err := q.doRequest(ctx, http.MethodPost,
		fmt.Sprintf("/collections/%s/points/search", q.collectionName),
		reqBody, &resp)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, len(resp.Result))
	for i, r := range resp.Result {
		results[i] = SearchResult{
			ID:    r.ID,
			Score: r.Score,
			Payload: Payload{
				ProjectID:   getString(r.Payload, "project_id"),
				FilePath:    getString(r.Payload, "file_path"),
				Symbol:      getString(r.Payload, "symbol"),
				SymbolType:  getString(r.Payload, "symbol_type"),
				Language:    getString(r.Payload, "language"),
				Module:      getString(r.Payload, "module"),
				StartLine:   getInt(r.Payload, "start_line"),
				EndLine:     getInt(r.Payload, "end_line"),
				Content:     getString(r.Payload, "content"),
				ContentHash: getString(r.Payload, "content_hash"),
				IndexedAt:   getString(r.Payload, "indexed_at"),
			},
		}
	}

	return results, nil
}

// Delete removes vectors by their IDs.
func (q *QdrantClient) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	// Convert string IDs to UUIDs
	uuids := make([]string, len(ids))
	for i, id := range ids {
		uuids[i] = stringToUUID(id)
	}

	reqBody := qdrantDeleteRequest{Points: uuids}
	return q.doRequest(ctx, http.MethodPost,
		fmt.Sprintf("/collections/%s/points/delete", q.collectionName),
		reqBody, nil)
}

// DeleteByFilter removes vectors matching a filter.
func (q *QdrantClient) DeleteByFilter(ctx context.Context, filter Filter) error {
	qdrantFilter := &qdrantFilter{
		Must: make([]qdrantCondition, 0),
	}

	if filter.ProjectID != "" {
		qdrantFilter.Must = append(qdrantFilter.Must, qdrantCondition{
			Key:   "project_id",
			Match: qdrantMatchValue{Value: filter.ProjectID},
		})
	}

	reqBody := qdrantDeleteRequest{Filter: qdrantFilter}
	return q.doRequest(ctx, http.MethodPost,
		fmt.Sprintf("/collections/%s/points/delete", q.collectionName),
		reqBody, nil)
}

// EnsureCollection creates the collection if it doesn't exist.
func (q *QdrantClient) EnsureCollection(ctx context.Context, dimensions int) error {
	// Check if collection exists
	resp, err := q.client.Get(fmt.Sprintf("%s/collections/%s", q.endpoint, q.collectionName))
	if err != nil {
		return fmt.Errorf("failed to check collection: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // Collection exists
	}

	// Create collection
	reqBody := qdrantCreateCollectionRequest{}
	reqBody.Vectors.Size = dimensions
	reqBody.Vectors.Distance = "Cosine"

	return q.doRequest(ctx, http.MethodPut,
		fmt.Sprintf("/collections/%s", q.collectionName),
		reqBody, nil)
}

// Health checks if Qdrant is available.
func (q *QdrantClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/readyz", q.endpoint), nil)
	if err != nil {
		return err
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qdrant returned status %d", resp.StatusCode)
	}

	return nil
}

// Close releases resources (no-op for Qdrant HTTP client).
func (q *QdrantClient) Close() error {
	return nil
}

// doRequest performs an HTTP request to Qdrant.
func (q *QdrantClient) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, q.endpoint+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// Helper functions

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

// stringToUUID converts a string ID to a deterministic UUID v5 format.
// This is needed because Qdrant requires point IDs to be UUIDs or uint64.
func stringToUUID(s string) string {
	// Generate SHA-256 hash of the string
	hash := sha256.Sum256([]byte(s))
	
	// Format as UUID v4-like string (using first 16 bytes of hash)
	// Format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4],
		hash[4:6],
		hash[6:8],
		hash[8:10],
		hash[10:16])
}
