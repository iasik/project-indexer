// Package config provides configuration loading and management for the project indexer.
// It supports hot reload via SIGHUP signal and provides a unified configuration structure
// for all components (embedding, vectordb, chunking, server).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the global application configuration.
// All fields are loaded from configs/config.yaml.
type Config struct {
	Embedding EmbeddingConfig `yaml:"embedding"`
	VectorDB  VectorDBConfig  `yaml:"vectordb"`
	Projects  ProjectsConfig  `yaml:"projects"`
	Chunking  ChunkingConfig  `yaml:"chunking"`
	Cache     CacheConfig     `yaml:"cache"`
	Server    ServerConfig    `yaml:"server"`
	Logging   LoggingConfig   `yaml:"logging"`
}

// EmbeddingConfig holds embedding provider settings.
// Supports multiple providers: ollama, openai, huggingface.
type EmbeddingConfig struct {
	// Provider name: ollama | openai | huggingface
	Provider string `yaml:"provider"`

	// Model name (varies by provider)
	Model string `yaml:"model"`

	// Provider endpoint URL
	Endpoint string `yaml:"endpoint"`

	// Vector dimensions (must match model output)
	Dimensions int `yaml:"dimensions"`

	// Batch size for bulk embedding requests
	BatchSize int `yaml:"batch_size"`

	// Request timeout
	Timeout string `yaml:"timeout"`

	// Environment variable name for API key (used by OpenAI, etc.)
	APIKeyEnv string `yaml:"api_key_env,omitempty"`
}

// VectorDBConfig holds vector database settings.
// Supports multiple providers: qdrant, milvus, weaviate.
type VectorDBConfig struct {
	// Provider name: qdrant | milvus | weaviate
	Provider string `yaml:"provider"`

	// Provider endpoint URL
	Endpoint string `yaml:"endpoint"`

	// Collection/index name for storing vectors
	CollectionName string `yaml:"collection_name"`

	// Request timeout
	Timeout string `yaml:"timeout"`
}

// ProjectsConfig holds project discovery settings.
type ProjectsConfig struct {
	// Directory containing per-project YAML configs
	ConfigDir string `yaml:"config_dir"`

	// Base path where project source code is mounted
	SourceBasePath string `yaml:"source_base_path"`
}

// ChunkingConfig holds default chunking parameters.
// These can be overridden per-project.
type ChunkingConfig struct {
	// Minimum tokens per chunk (smaller chunks are merged)
	MinTokens int `yaml:"min_tokens"`

	// Ideal chunk size in tokens
	IdealTokens int `yaml:"ideal_tokens"`

	// Maximum tokens per chunk
	MaxTokens int `yaml:"max_tokens"`

	// Whether to merge small chunks into parent scope
	MergeSmallChunks bool `yaml:"merge_small_chunks"`
}

// CacheConfig holds index cache settings.
type CacheConfig struct {
	// Directory for storing cache files
	Dir string `yaml:"dir"`

	// Cache format (currently only "json" is supported)
	Format string `yaml:"format"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Port number to listen on
	Port int `yaml:"port"`

	// Read timeout for incoming requests
	ReadTimeout string `yaml:"read_timeout"`

	// Write timeout for outgoing responses
	WriteTimeout string `yaml:"write_timeout"`

	// Graceful shutdown timeout
	ShutdownTimeout string `yaml:"shutdown_timeout"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	// Log level: debug | info | warn | error
	Level string `yaml:"level"`

	// Output format: json | text
	Format string `yaml:"format"`
}

// GetTimeout parses and returns the embedding timeout duration.
func (e *EmbeddingConfig) GetTimeout() time.Duration {
	d, err := time.ParseDuration(e.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GetAPIKey returns the API key from environment variable.
func (e *EmbeddingConfig) GetAPIKey() string {
	if e.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(e.APIKeyEnv)
}

// GetTimeout parses and returns the vectordb timeout duration.
func (v *VectorDBConfig) GetTimeout() time.Duration {
	d, err := time.ParseDuration(v.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GetReadTimeout parses and returns the server read timeout.
func (s *ServerConfig) GetReadTimeout() time.Duration {
	d, err := time.ParseDuration(s.ReadTimeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GetWriteTimeout parses and returns the server write timeout.
func (s *ServerConfig) GetWriteTimeout() time.Duration {
	d, err := time.ParseDuration(s.WriteTimeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GetShutdownTimeout parses and returns the graceful shutdown timeout.
func (s *ServerConfig) GetShutdownTimeout() time.Duration {
	d, err := time.ParseDuration(s.ShutdownTimeout)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// Manager handles configuration loading and hot reload.
type Manager struct {
	configPath string
	config     *Config
	mu         sync.RWMutex
	onChange   []func(*Config)
}

// NewManager creates a new configuration manager.
func NewManager(configPath string) *Manager {
	return &Manager{
		configPath: configPath,
		onChange:   make([]func(*Config), 0),
	}
}

// Load reads and parses the configuration file.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults
	applyDefaults(&cfg)

	// Validate configuration
	if err := validate(&cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	m.config = &cfg
	return nil
}

// Reload reloads the configuration and notifies listeners.
func (m *Manager) Reload() error {
	if err := m.Load(); err != nil {
		return err
	}

	// Notify change listeners
	cfg := m.Get()
	for _, fn := range m.onChange {
		fn(cfg)
	}

	return nil
}

// Get returns the current configuration.
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// OnChange registers a callback to be called when configuration changes.
func (m *Manager) OnChange(fn func(*Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = append(m.onChange, fn)
}

// applyDefaults sets default values for missing configuration fields.
func applyDefaults(cfg *Config) {
	// Embedding defaults
	if cfg.Embedding.Provider == "" {
		cfg.Embedding.Provider = "ollama"
	}
	if cfg.Embedding.Model == "" {
		cfg.Embedding.Model = "nomic-embed-text"
	}
	if cfg.Embedding.Endpoint == "" {
		cfg.Embedding.Endpoint = "http://ollama:11434"
	}
	if cfg.Embedding.Dimensions == 0 {
		cfg.Embedding.Dimensions = 768
	}
	if cfg.Embedding.BatchSize == 0 {
		cfg.Embedding.BatchSize = 32
	}
	if cfg.Embedding.Timeout == "" {
		cfg.Embedding.Timeout = "30s"
	}

	// VectorDB defaults
	if cfg.VectorDB.Provider == "" {
		cfg.VectorDB.Provider = "qdrant"
	}
	if cfg.VectorDB.Endpoint == "" {
		cfg.VectorDB.Endpoint = "http://qdrant:6333"
	}
	if cfg.VectorDB.CollectionName == "" {
		cfg.VectorDB.CollectionName = "code_chunks"
	}
	if cfg.VectorDB.Timeout == "" {
		cfg.VectorDB.Timeout = "30s"
	}

	// Projects defaults
	if cfg.Projects.ConfigDir == "" {
		cfg.Projects.ConfigDir = "/app/configs/projects"
	}
	if cfg.Projects.SourceBasePath == "" {
		cfg.Projects.SourceBasePath = "/sources"
	}

	// Chunking defaults
	if cfg.Chunking.MinTokens == 0 {
		cfg.Chunking.MinTokens = 200
	}
	if cfg.Chunking.IdealTokens == 0 {
		cfg.Chunking.IdealTokens = 500
	}
	if cfg.Chunking.MaxTokens == 0 {
		cfg.Chunking.MaxTokens = 800
	}

	// Cache defaults
	if cfg.Cache.Dir == "" {
		cfg.Cache.Dir = "/app/data/index-cache"
	}
	if cfg.Cache.Format == "" {
		cfg.Cache.Format = "json"
	}

	// Server defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout == "" {
		cfg.Server.ReadTimeout = "30s"
	}
	if cfg.Server.WriteTimeout == "" {
		cfg.Server.WriteTimeout = "30s"
	}
	if cfg.Server.ShutdownTimeout == "" {
		cfg.Server.ShutdownTimeout = "10s"
	}

	// Logging defaults
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
}

// validate checks the configuration for errors.
func validate(cfg *Config) error {
	// Validate embedding config
	validEmbeddingProviders := map[string]bool{
		"ollama":      true,
		"openai":      true,
		"huggingface": true,
	}
	if !validEmbeddingProviders[cfg.Embedding.Provider] {
		return fmt.Errorf("invalid embedding provider: %s", cfg.Embedding.Provider)
	}
	if cfg.Embedding.Dimensions <= 0 {
		return fmt.Errorf("embedding dimensions must be positive")
	}

	// Validate vectordb config
	validVectorDBProviders := map[string]bool{
		"qdrant":   true,
		"milvus":   true,
		"weaviate": true,
	}
	if !validVectorDBProviders[cfg.VectorDB.Provider] {
		return fmt.Errorf("invalid vectordb provider: %s", cfg.VectorDB.Provider)
	}

	// Validate chunking config
	if cfg.Chunking.MinTokens >= cfg.Chunking.MaxTokens {
		return fmt.Errorf("min_tokens must be less than max_tokens")
	}

	// Validate server port
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
	}

	return nil
}

// LoadFromEnv loads configuration from the path specified in CONFIG_PATH env var.
func LoadFromEnv() (*Manager, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	// Make path absolute
	if !filepath.IsAbs(configPath) {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		configPath = filepath.Join(wd, configPath)
	}

	manager := NewManager(configPath)
	if err := manager.Load(); err != nil {
		return nil, err
	}

	return manager, nil
}
