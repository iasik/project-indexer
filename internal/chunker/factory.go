// Package chunker provides a factory for creating appropriate chunkers.
package chunker

import (
	"path/filepath"
	"strings"

	"github.com/iasik/project-indexer/internal/config"
)

// Factory creates chunkers based on file type and configuration.
type Factory struct {
	goChunker       *GoChunker
	markdownChunker *MarkdownChunker
	genericChunker  *GenericChunker
}

// NewFactory creates a new chunker factory.
func NewFactory(cfg config.ChunkingConfig) *Factory {
	chunkCfg := ChunkingConfig{
		MinTokens:        cfg.MinTokens,
		IdealTokens:      cfg.IdealTokens,
		MaxTokens:        cfg.MaxTokens,
		MergeSmallChunks: cfg.MergeSmallChunks,
	}

	return &Factory{
		goChunker:       NewGoChunker(chunkCfg),
		markdownChunker: NewMarkdownChunker(chunkCfg),
		genericChunker:  NewGenericChunker(chunkCfg),
	}
}

// GetChunker returns the appropriate chunker for a file based on extension.
func (f *Factory) GetChunker(filePath string) Chunker {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".go":
		return f.goChunker
	case ".md", ".markdown":
		return f.markdownChunker
	default:
		return f.genericChunker
	}
}

// GetChunkerByStrategy returns a chunker by strategy name.
func (f *Factory) GetChunkerByStrategy(strategy string) Chunker {
	switch strategy {
	case "function":
		return f.goChunker
	case "heading":
		return f.markdownChunker
	case "fixed", "file":
		return f.genericChunker
	default:
		return f.genericChunker
	}
}

// DetectLanguage detects the programming language from file extension.
func DetectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	languages := map[string]string{
		".go":       "go",
		".md":       "markdown",
		".markdown": "markdown",
		".py":       "python",
		".js":       "javascript",
		".ts":       "typescript",
		".jsx":      "javascript",
		".tsx":      "typescript",
		".java":     "java",
		".rs":       "rust",
		".rb":       "ruby",
		".php":      "php",
		".c":        "c",
		".cpp":      "cpp",
		".h":        "c",
		".hpp":      "cpp",
		".cs":       "csharp",
		".swift":    "swift",
		".kt":       "kotlin",
		".scala":    "scala",
		".sql":      "sql",
		".sh":       "shell",
		".bash":     "shell",
		".zsh":      "shell",
		".yaml":     "yaml",
		".yml":      "yaml",
		".json":     "json",
		".xml":      "xml",
		".html":     "html",
		".css":      "css",
		".scss":     "scss",
		".less":     "less",
		".vue":      "vue",
		".svelte":   "svelte",
	}

	if lang, ok := languages[ext]; ok {
		return lang
	}
	return "text"
}

// ExtractModule extracts module/package name from file path.
func ExtractModule(filePath string) string {
	dir := filepath.Dir(filePath)
	if dir == "." {
		return ""
	}

	// Use the parent directory as module name
	parts := strings.Split(dir, string(filepath.Separator))
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}
