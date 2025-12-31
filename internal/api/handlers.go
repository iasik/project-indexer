// Package api provides HTTP handlers for the retrieval tool.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/iasik/project-indexer/internal/vectordb"
)

// RetrieveRequest is the request body for POST /retrieve.
type RetrieveRequest struct {
	// ProjectID is required - specifies which project to search
	ProjectID string `json:"project_id"`

	// Query is the natural language search query
	Query string `json:"query"`

	// TopK is the number of results to return (default: 5, max: 20)
	TopK int `json:"top_k,omitempty"`

	// Filters for narrowing search results
	Filters *RetrieveFilters `json:"filters,omitempty"`
}

// RetrieveFilters contains optional filters for search.
type RetrieveFilters struct {
	// Module filters by package/module name
	Module string `json:"module,omitempty"`

	// Language filters by programming language
	Language string `json:"language,omitempty"`

	// SymbolType filters by symbol type (function, struct, etc.)
	SymbolType string `json:"symbol_type,omitempty"`
}

// RetrieveResponse is the response body for POST /retrieve.
type RetrieveResponse struct {
	// Results contains the retrieved code chunks
	Results []RetrieveResult `json:"results"`

	// QueryTimeMs is the query execution time in milliseconds
	QueryTimeMs int64 `json:"query_time_ms"`
}

// RetrieveResult is a single search result.
type RetrieveResult struct {
	// Content is the actual code/text content
	Content string `json:"content"`

	// Source is the file path
	Source string `json:"source"`

	// Symbol is the function/struct/heading name
	Symbol string `json:"symbol,omitempty"`

	// SymbolType is the type of symbol (function, struct, heading, etc.)
	SymbolType string `json:"symbol_type,omitempty"`

	// ProjectID is the project this result belongs to
	ProjectID string `json:"project_id"`

	// Module is the package/module name
	Module string `json:"module,omitempty"`

	// Language is the programming language
	Language string `json:"language,omitempty"`

	// StartLine is the starting line number in the file
	StartLine int `json:"start_line,omitempty"`

	// EndLine is the ending line number in the file
	EndLine int `json:"end_line,omitempty"`

	// Score is the similarity score (0.0 to 1.0)
	Score float32 `json:"score"`
}

// HealthResponse is the response body for GET /health.
type HealthResponse struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components"`
	Version    string            `json:"version"`
}

// handleRetrieve handles POST /retrieve requests.
func (s *Server) handleRetrieve(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Parse request body
	var req RetrieveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	// Apply defaults
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.TopK > 20 {
		req.TopK = 20
	}

	// Get providers
	emb, vdb := s.getProviders()

	// Generate query embedding
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	queryVector, err := emb.Embed(ctx, req.Query)
	if err != nil {
		s.logger.Error("embedding failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to process query")
		return
	}

	// Build search filter
	filter := vectordb.Filter{
		ProjectID: req.ProjectID,
	}
	if req.Filters != nil {
		filter.Module = req.Filters.Module
		filter.Language = req.Filters.Language
		filter.SymbolType = req.Filters.SymbolType
	}

	// Perform vector search
	searchResults, err := vdb.Search(ctx, vectordb.SearchQuery{
		Vector: queryVector,
		TopK:   req.TopK,
		Filter: filter,
	})
	if err != nil {
		s.logger.Error("search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	// Convert to response format
	results := make([]RetrieveResult, len(searchResults))
	for i, sr := range searchResults {
		results[i] = RetrieveResult{
			Content:    sr.Payload.Content,
			Source:     sr.Payload.FilePath,
			Symbol:     sr.Payload.Symbol,
			SymbolType: sr.Payload.SymbolType,
			ProjectID:  sr.Payload.ProjectID,
			Module:     sr.Payload.Module,
			Language:   sr.Payload.Language,
			StartLine:  sr.Payload.StartLine,
			EndLine:    sr.Payload.EndLine,
			Score:      sr.Score,
		}
	}

	response := RetrieveResponse{
		Results:     results,
		QueryTimeMs: time.Since(startTime).Milliseconds(),
	}

	writeJSON(w, http.StatusOK, response)
}

// handleHealth handles GET /health requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	emb, vdb := s.getProviders()

	components := make(map[string]string)
	status := "healthy"

	// Check embedder
	if err := emb.Health(ctx); err != nil {
		components["embedder"] = "error: " + err.Error()
		status = "degraded"
	} else {
		components["embedder"] = "ok"
	}

	// Check vector DB
	if err := vdb.Health(ctx); err != nil {
		components["vectordb"] = "error: " + err.Error()
		status = "degraded"
	} else {
		components["vectordb"] = "ok"
	}

	response := HealthResponse{
		Status:     status,
		Components: components,
		Version:    s.version,
	}

	statusCode := http.StatusOK
	if status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	writeJSON(w, statusCode, response)
}

// handleRoot handles GET / requests.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":    "project-indexer-retrieval-tool",
		"version": s.version,
		"endpoints": []string{
			"POST /retrieve",
			"GET /health",
		},
	})
}
