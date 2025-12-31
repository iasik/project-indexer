// Package config provides project-specific configuration loading.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProjectConfig represents a single project's configuration.
// Each project has its own YAML file in configs/projects/.
type ProjectConfig struct {
	// Unique project identifier (lowercase, hyphenated)
	ProjectID string `yaml:"project_id"`

	// Human-readable display name
	DisplayName string `yaml:"display_name"`

	// Source code path relative to source_base_path
	SourcePath string `yaml:"source_path"`

	// File extensions to include in indexing
	IncludeExtensions []string `yaml:"include_extensions"`

	// Paths/patterns to exclude from indexing
	ExcludePaths []string `yaml:"exclude_paths"`

	// Chunking configuration overrides
	Chunking ProjectChunkingConfig `yaml:"chunking"`

	// Optional metadata for filtering
	Metadata ProjectMetadata `yaml:"metadata"`
}

// ProjectChunkingConfig holds project-specific chunking settings.
type ProjectChunkingConfig struct {
	// Code chunking settings
	Code CodeChunkingConfig `yaml:"code"`

	// Markdown chunking settings
	Markdown MarkdownChunkingConfig `yaml:"markdown"`

	// Override for minimum tokens (optional)
	MinTokens int `yaml:"min_tokens,omitempty"`

	// Override for ideal tokens (optional)
	IdealTokens int `yaml:"ideal_tokens,omitempty"`

	// Override for maximum tokens (optional)
	MaxTokens int `yaml:"max_tokens,omitempty"`
}

// CodeChunkingConfig holds code-specific chunking settings.
type CodeChunkingConfig struct {
	// Strategy: function | file | fixed
	Strategy string `yaml:"strategy"`
}

// MarkdownChunkingConfig holds markdown-specific chunking settings.
type MarkdownChunkingConfig struct {
	// Strategy: heading | paragraph | fixed
	Strategy string `yaml:"strategy"`
}

// ProjectMetadata holds optional project metadata.
type ProjectMetadata struct {
	// Team responsible for the project
	Team string `yaml:"team,omitempty"`

	// Tags for categorization
	Tags []string `yaml:"tags,omitempty"`
}

// GetFullSourcePath returns the absolute path to the project source.
func (p *ProjectConfig) GetFullSourcePath(basePath string) string {
	return filepath.Join(basePath, p.SourcePath)
}

// ShouldIncludeFile checks if a file should be included based on extension.
func (p *ProjectConfig) ShouldIncludeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range p.IncludeExtensions {
		if strings.ToLower(e) == ext {
			return true
		}
	}
	return false
}

// ShouldExcludePath checks if a path matches any exclusion pattern.
func (p *ProjectConfig) ShouldExcludePath(path string) bool {
	for _, pattern := range p.ExcludePaths {
		// Check direct prefix match
		if strings.HasPrefix(path, pattern) {
			return true
		}

		// Check glob pattern match
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}

		// Check if pattern is a directory prefix
		if strings.HasSuffix(pattern, "/") {
			if strings.Contains(path, pattern) {
				return true
			}
		}
	}
	return false
}

// GetChunkingStrategy returns the appropriate chunking strategy for a file.
func (p *ProjectConfig) GetChunkingStrategy(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".go":
		if p.Chunking.Code.Strategy != "" {
			return p.Chunking.Code.Strategy
		}
		return "function"
	case ".md", ".markdown":
		if p.Chunking.Markdown.Strategy != "" {
			return p.Chunking.Markdown.Strategy
		}
		return "heading"
	default:
		return "fixed"
	}
}

// GetEffectiveChunking returns chunking config with global defaults applied.
func (p *ProjectConfig) GetEffectiveChunking(global ChunkingConfig) ChunkingConfig {
	result := global

	if p.Chunking.MinTokens > 0 {
		result.MinTokens = p.Chunking.MinTokens
	}
	if p.Chunking.IdealTokens > 0 {
		result.IdealTokens = p.Chunking.IdealTokens
	}
	if p.Chunking.MaxTokens > 0 {
		result.MaxTokens = p.Chunking.MaxTokens
	}

	return result
}

// Validate checks the project configuration for errors.
func (p *ProjectConfig) Validate() error {
	if p.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}

	// Validate project_id format (lowercase, alphanumeric, hyphens)
	for _, c := range p.ProjectID {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return fmt.Errorf("project_id must contain only lowercase letters, numbers, and hyphens")
		}
	}

	if p.SourcePath == "" {
		return fmt.Errorf("source_path is required")
	}

	if len(p.IncludeExtensions) == 0 {
		return fmt.Errorf("at least one include_extension is required")
	}

	// Validate code chunking strategy
	validCodeStrategies := map[string]bool{
		"function": true,
		"file":     true,
		"fixed":    true,
		"":         true, // Empty uses default
	}
	if !validCodeStrategies[p.Chunking.Code.Strategy] {
		return fmt.Errorf("invalid code chunking strategy: %s", p.Chunking.Code.Strategy)
	}

	// Validate markdown chunking strategy
	validMarkdownStrategies := map[string]bool{
		"heading":   true,
		"paragraph": true,
		"fixed":     true,
		"":          true, // Empty uses default
	}
	if !validMarkdownStrategies[p.Chunking.Markdown.Strategy] {
		return fmt.Errorf("invalid markdown chunking strategy: %s", p.Chunking.Markdown.Strategy)
	}

	return nil
}

// LoadProjectConfig loads a single project configuration from file.
func LoadProjectConfig(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read project config: %w", err)
	}

	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse project config: %w", err)
	}

	// Apply defaults
	applyProjectDefaults(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("project config validation failed: %w", err)
	}

	return &cfg, nil
}

// LoadAllProjects loads all project configurations from the config directory.
func LoadAllProjects(configDir string) (map[string]*ProjectConfig, error) {
	projects := make(map[string]*ProjectConfig)

	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read project config directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process YAML files
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		path := filepath.Join(configDir, name)
		cfg, err := LoadProjectConfig(path)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", name, err)
		}

		projects[cfg.ProjectID] = cfg
	}

	return projects, nil
}

// GetProject loads a specific project configuration by ID.
func GetProject(configDir, projectID string) (*ProjectConfig, error) {
	// Try common file naming patterns
	patterns := []string{
		filepath.Join(configDir, projectID+".yaml"),
		filepath.Join(configDir, projectID+".yml"),
	}

	for _, path := range patterns {
		if _, err := os.Stat(path); err == nil {
			return LoadProjectConfig(path)
		}
	}

	// Fallback: search all files for matching project_id
	projects, err := LoadAllProjects(configDir)
	if err != nil {
		return nil, err
	}

	if cfg, ok := projects[projectID]; ok {
		return cfg, nil
	}

	return nil, fmt.Errorf("project not found: %s", projectID)
}

// applyProjectDefaults sets default values for missing project configuration fields.
func applyProjectDefaults(cfg *ProjectConfig) {
	if cfg.DisplayName == "" {
		cfg.DisplayName = cfg.ProjectID
	}

	if cfg.Chunking.Code.Strategy == "" {
		cfg.Chunking.Code.Strategy = "function"
	}

	if cfg.Chunking.Markdown.Strategy == "" {
		cfg.Chunking.Markdown.Strategy = "heading"
	}

	// Default exclude paths
	if len(cfg.ExcludePaths) == 0 {
		cfg.ExcludePaths = []string{
			".git/",
			"vendor/",
			"node_modules/",
		}
	}
}
