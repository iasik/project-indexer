// Package chunker provides generic fixed-size chunking for unsupported file types.
package chunker

import (
	"path/filepath"
	"strings"
)

// GenericChunker implements fixed-size chunking for any text file.
type GenericChunker struct {
	config ChunkingConfig
}

// NewGenericChunker creates a new generic chunker.
func NewGenericChunker(cfg ChunkingConfig) *GenericChunker {
	return &GenericChunker{config: cfg}
}

// Name returns the chunker strategy name.
func (g *GenericChunker) Name() string {
	return "fixed"
}

// Chunk splits content into fixed-size chunks based on token limits.
func (g *GenericChunker) Chunk(content []byte, metadata FileMetadata) ([]Chunk, error) {
	contentStr := string(content)
	totalTokens := EstimateTokens(contentStr)

	// If content fits in one chunk, return as-is
	if totalTokens <= g.config.MaxTokens {
		return g.singleChunk(contentStr, metadata), nil
	}

	// Split into multiple chunks
	lines := strings.Split(contentStr, "\n")
	chunks := make([]Chunk, 0)

	var currentLines []string
	var currentTokens int
	startLine := 1

	for i, line := range lines {
		lineTokens := EstimateTokens(line)

		// Check if adding this line would exceed max
		if currentTokens+lineTokens > g.config.MaxTokens && len(currentLines) > 0 {
			// Create chunk from accumulated lines
			chunk := g.createChunk(currentLines, startLine, i, metadata)
			chunks = append(chunks, chunk)

			// Reset for next chunk
			currentLines = []string{line}
			currentTokens = lineTokens
			startLine = i + 1
		} else {
			currentLines = append(currentLines, line)
			currentTokens += lineTokens
		}
	}

	// Add remaining lines as final chunk
	if len(currentLines) > 0 {
		chunk := g.createChunk(currentLines, startLine, len(lines), metadata)
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// singleChunk creates a single chunk from the entire content.
func (g *GenericChunker) singleChunk(content string, metadata FileMetadata) []Chunk {
	contentHash := HashContent(content)
	symbol := filepath.Base(metadata.FilePath)

	return []Chunk{{
		ID:          GenerateChunkID(metadata.ProjectID, metadata.FilePath, symbol, contentHash),
		Content:     content,
		Symbol:      symbol,
		SymbolType:  "file",
		StartLine:   1,
		EndLine:     strings.Count(content, "\n") + 1,
		TokenCount:  EstimateTokens(content),
		ContentHash: contentHash,
		FilePath:    metadata.FilePath,
		Language:    metadata.Language,
		Module:      metadata.Module,
		ProjectID:   metadata.ProjectID,
	}}
}

// createChunk creates a chunk from a slice of lines.
func (g *GenericChunker) createChunk(lines []string, startLine, endLine int, metadata FileMetadata) Chunk {
	content := strings.Join(lines, "\n")
	contentHash := HashContent(content)

	// Create symbol name based on line range
	symbol := filepath.Base(metadata.FilePath)
	if startLine > 1 || endLine < strings.Count(content, "\n")+1 {
		symbol = strings.TrimSuffix(symbol, filepath.Ext(symbol))
		symbol = strings.ReplaceAll(symbol, ".", "_")
	}

	return Chunk{
		ID:          GenerateChunkID(metadata.ProjectID, metadata.FilePath, symbol, contentHash),
		Content:     content,
		Symbol:      symbol,
		SymbolType:  "fragment",
		StartLine:   startLine,
		EndLine:     endLine,
		TokenCount:  EstimateTokens(content),
		ContentHash: contentHash,
		FilePath:    metadata.FilePath,
		Language:    metadata.Language,
		Module:      metadata.Module,
		ProjectID:   metadata.ProjectID,
	}
}
