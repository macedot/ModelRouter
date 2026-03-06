// Package config handles JSON configuration loading
package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Config represents the openmodel configuration
type Config struct {
	Server     ServerConfig               `json:"server"`
	Providers  map[string]ProviderConfig  `json:"providers"`
	Models     map[string][]ModelProvider `json:"models"`
	LogLevel   string                     `json:"log_level"`
	LogFormat  string                     `json:"log_format"`
	Thresholds ThresholdsConfig           `json:"thresholds"`
}

// ServerConfig holds server settings
type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

// ProviderConfig holds provider connection settings
type ProviderConfig struct {
	URL    string `json:"url"`    // Base URL for the provider (e.g., https://api.openai.com/v1)
	APIKey string `json:"apiKey"` // API key (supports ${VAR} expansion)
}

// ModelProvider represents a provider model in the chain
type ModelProvider struct {
	Provider string `json:"provider"` // Provider name from providers config
	Model    string `json:"model"`    // Model name on that provider
}

// ThresholdsConfig holds failure threshold settings
type ThresholdsConfig struct {
	FailuresBeforeSwitch int `json:"failures_before_switch"`
	InitialTimeout       int `json:"initial_timeout_ms"`
	MaxTimeout           int `json:"max_timeout_ms"`
}

// configWithSchema is used to extract the $schema field before full parsing
type configWithSchema struct {
	Schema string `json:"$schema"`
}

// SchemaCache caches compiled JSON schemas
type SchemaCache struct {
	mu        sync.RWMutex
	compilers map[string]*jsonschema.Compiler
}

var (
	// Global schema cache instance
	schemaCache = &SchemaCache{
		compilers: make(map[string]*jsonschema.Compiler),
	}
)

func getSchemaCompiler(schemaURL string) (*jsonschema.Compiler, error) {
	// Check cache first (read lock)
	schemaCache.mu.RLock()
	if compiler, exists := schemaCache.compilers[schemaURL]; exists {
		schemaCache.mu.RUnlock()
		return compiler, nil
	}
	schemaCache.mu.RUnlock()

	// Acquire write lock and double-check
	schemaCache.mu.Lock()
	defer schemaCache.mu.Unlock()

	// Double-check after acquiring write lock
	if compiler, exists := schemaCache.compilers[schemaURL]; exists {
		return compiler, nil
	}

	// Compile new schema
	compiler := jsonschema.NewCompiler()

	var schemaData any

	if strings.HasPrefix(schemaURL, "http://") || strings.HasPrefix(schemaURL, "https://") {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(schemaURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch schema: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("schema fetch returned status %d", resp.StatusCode)
		}

		if err := json.NewDecoder(resp.Body).Decode(&schemaData); err != nil {
			return nil, fmt.Errorf("failed to parse schema: %w", err)
		}
	} else {
		schemaPath := schemaURL
		if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
			schemaPath = filepath.Join(os.Getenv("HOME"), ".config", "openmodel", schemaURL)
		}
		if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
			schemaPath = filepath.Join(filepath.Dir(os.Args[0]), schemaURL)
		}

		schemaBytes, err := os.ReadFile(schemaPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read schema file: %w", err)
		}

		if err := json.Unmarshal(schemaBytes, &schemaData); err != nil {
			return nil, fmt.Errorf("failed to parse schema: %w", err)
		}
	}

	if err := compiler.AddResource(schemaURL, schemaData); err != nil {
		return nil, fmt.Errorf("failed to add schema: %w", err)
	}

	// Store in cache
	schemaCache.compilers[schemaURL] = compiler

	return compiler, nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 12345,
			Host: "localhost",
		},
		Providers: map[string]ProviderConfig{
			"local": {
				URL:    "http://localhost:11434/v1",
				APIKey: "",
			},
		},
		LogLevel:  getLogLevel(),
		LogFormat: getLogFormat(),
		Thresholds: ThresholdsConfig{
			FailuresBeforeSwitch: 3,
			InitialTimeout:       10000,
			MaxTimeout:           300000,
		},
	}
}

// expandEnvVars expands environment variables in ${VAR} format
func expandEnvVars(s string) string {
	for {
		start := strings.Index(s, "${")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			break
		}
		end += start
		varName := s[start+2 : end]
		envValue := os.Getenv(varName)
		s = s[:start] + envValue + s[end+1:]
	}
	return s
}

// expandProviderEnvVars expands environment variables in provider config
func expandProviderEnvVars(pc *ProviderConfig) {
	pc.APIKey = expandEnvVars(pc.APIKey)
	pc.URL = expandEnvVars(pc.URL)
}

// GetConfigPath returns the path to the config file
func GetConfigPath() string {
	// Check for explicit config path in env
	if path := os.Getenv("OPENMODEL_CONFIG"); path != "" {
		return path
	}
	// Default to ~/.config/openmodel/config.json
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "openmodel", "config.json")
}

// getLogLevel returns the log level from environment or default
func getLogLevel() string {
	if level := os.Getenv("OPENMODEL_LOG_LEVEL"); level != "" {
		return level
	}
	return "info"
}

// getLogFormat returns the log format from environment or default
func getLogFormat() string {
	if format := os.Getenv("OPENMODEL_LOG_FORMAT"); format != "" {
		return format
	}
	return "text"
}

// Load loads configuration from file
func Load() (*Config, error) {
	configPath := GetConfigPath()

	if configPath == "" {
		return DefaultConfig(), nil
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return parseConfig(data, true)
}

// LoadFromPath loads configuration from a specific path
func LoadFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Skip schema validation for custom paths
	return parseConfig(data, false)
}

// parseConfig parses configuration data with optional schema validation
func parseConfig(data []byte, validateSchema bool) (*Config, error) {
	// Extract $schema field
	var schemaConfig configWithSchema
	if err := json.Unmarshal(data, &schemaConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate schema is present if validation is enabled
	if validateSchema && schemaConfig.Schema == "" {
		return nil, fmt.Errorf("config file must contain $schema field")
	}

	// Validate schema if enabled
	if validateSchema {
		// Get schema compiler
		compiler, err := getSchemaCompiler(schemaConfig.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to load schema: %w", err)
		}

		compiledSchema, err := compiler.Compile(schemaConfig.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to compile schema: %w", err)
		}

		// Validate config against schema
		var configData any
		if err := json.Unmarshal(data, &configData); err != nil {
			return nil, fmt.Errorf("failed to parse config data: %w", err)
		}
		if err := compiledSchema.Validate(configData); err != nil {
			return nil, fmt.Errorf("config validation failed: %w", err)
		}
	}

	// Parse full config
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Expand environment variables in all provider configs
	for name, provider := range cfg.Providers {
		expandProviderEnvVars(&provider)
		cfg.Providers[name] = provider
	}

	// Allow env vars to override config file values
	if level := os.Getenv("OPENMODEL_LOG_LEVEL"); level != "" {
		cfg.LogLevel = level
	}
	if format := os.Getenv("OPENMODEL_LOG_FORMAT"); format != "" {
		cfg.LogFormat = format
	}

	return cfg, nil
}
