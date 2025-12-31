// Package chunker provides Markdown-specific chunking using heading-based splitting.
// It splits markdown documents at heading boundaries (##, ###, etc.) to create
// semantically meaningful chunks.
package chunker

import (
	"regexp"
	"strings"
)

// MarkdownChunker implements heading-based chunking for Markdown files.
type MarkdownChunker struct {
	config ChunkingConfig
}

// NewMarkdownChunker creates a new Markdown chunker.
func NewMarkdownChunker(cfg ChunkingConfig) *MarkdownChunker {
	return &MarkdownChunker{config: cfg}
}

// Name returns the chunker strategy name.
func (m *MarkdownChunker) Name() string {
	return "heading"
}

// mdSection represents a section of a markdown document.
type mdSection struct {
	heading    string
	level      int
	startLine  int
	endLine    int
	content    string
	tokens     int
}

// headingPattern matches markdown headings.
var headingPattern = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

// Chunk splits Markdown content at heading boundaries.
func (m *MarkdownChunker) Chunk(content []byte, metadata FileMetadata) ([]Chunk, error) {
	lines := strings.Split(string(content), "\n")
	sections := m.extractSections(lines)

	if len(sections) == 0 {
		return m.chunkAsFile(content, metadata), nil
	}

	// Merge small sections if enabled
	if m.config.MergeSmallChunks {
		sections = m.mergeSmallSections(sections)
	}

	// Convert sections to chunks
	chunks := make([]Chunk, 0, len(sections))
	for _, sec := range sections {
		contentHash := HashContent(sec.content)
		chunk := Chunk{
			ID:          GenerateChunkID(metadata.ProjectID, metadata.FilePath, sec.heading, contentHash),
			Content:     sec.content,
			Symbol:      sec.heading,
			SymbolType:  "heading",
			StartLine:   sec.startLine,
			EndLine:     sec.endLine,
			TokenCount:  sec.tokens,
			ContentHash: contentHash,
			FilePath:    metadata.FilePath,
			Language:    "markdown",
			Module:      metadata.Module,
			ProjectID:   metadata.ProjectID,
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// extractSections extracts sections from markdown lines.
func (m *MarkdownChunker) extractSections(lines []string) []mdSection {
	sections := make([]mdSection, 0)
	var currentSection *mdSection

	for i, line := range lines {
		lineNum := i + 1

		matches := headingPattern.FindStringSubmatch(line)
		if matches != nil {
			// Found a heading
			level := len(matches[1])
			heading := matches[2]

			// Close previous section
			if currentSection != nil {
				currentSection.endLine = lineNum - 1
				currentSection.tokens = EstimateTokens(currentSection.content)
				sections = append(sections, *currentSection)
			}

			// Start new section
			currentSection = &mdSection{
				heading:   heading,
				level:     level,
				startLine: lineNum,
				content:   line,
			}
		} else if currentSection != nil {
			// Add line to current section
			currentSection.content += "\n" + line
		} else {
			// Content before first heading - create implicit section
			currentSection = &mdSection{
				heading:   "(intro)",
				level:     0,
				startLine: lineNum,
				content:   line,
			}
		}
	}

	// Close last section
	if currentSection != nil {
		currentSection.endLine = len(lines)
		currentSection.tokens = EstimateTokens(currentSection.content)
		sections = append(sections, *currentSection)
	}

	return sections
}

// mergeSmallSections merges sections below MinTokens into adjacent sections.
func (m *MarkdownChunker) mergeSmallSections(sections []mdSection) []mdSection {
	if len(sections) <= 1 {
		return sections
	}

	result := make([]mdSection, 0, len(sections))

	for i, sec := range sections {
		if sec.tokens < m.config.MinTokens && i > 0 && len(result) > 0 {
			// Merge into previous section
			prev := &result[len(result)-1]
			prev.content += "\n\n" + sec.content
			prev.endLine = sec.endLine
			prev.tokens = EstimateTokens(prev.content)
		} else if sec.tokens < m.config.MinTokens && i < len(sections)-1 {
			// Will merge with next section
			nextSec := &sections[i+1]
			nextSec.content = sec.content + "\n\n" + nextSec.content
			nextSec.startLine = sec.startLine
			nextSec.tokens = EstimateTokens(nextSec.content)
		} else {
			result = append(result, sec)
		}
	}

	return result
}

// chunkAsFile returns the entire file as a single chunk.
func (m *MarkdownChunker) chunkAsFile(content []byte, metadata FileMetadata) []Chunk {
	contentStr := string(content)
	contentHash := HashContent(contentStr)

	// Try to extract title from first heading
	symbol := metadata.FilePath
	lines := strings.Split(contentStr, "\n")
	for _, line := range lines {
		if matches := headingPattern.FindStringSubmatch(line); matches != nil {
			symbol = matches[2]
			break
		}
	}

	return []Chunk{{
		ID:          GenerateChunkID(metadata.ProjectID, metadata.FilePath, symbol, contentHash),
		Content:     contentStr,
		Symbol:      symbol,
		SymbolType:  "document",
		StartLine:   1,
		EndLine:     len(lines),
		TokenCount:  EstimateTokens(contentStr),
		ContentHash: contentHash,
		FilePath:    metadata.FilePath,
		Language:    "markdown",
		Module:      metadata.Module,
		ProjectID:   metadata.ProjectID,
	}}
}
