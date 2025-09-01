// Package config provides configuration management for the LLM Gateway
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"github.com/llm-gateway/gateway/pkg/types"
)

// Manager handles configuration loading and management
type Manager struct {
	config *types.Config
	viper  *viper.Viper
}

// NewManager creates a new configuration manager
func NewManager() *Manager {
	return &Manager{
		viper: viper.New(),
	}
}

// Load loads configuration from various sources
func (m *Manager) Load() error {
	// Set default values
	m.setDefaults()
	
	// Configure viper
	m.viper.SetConfigName("config")
	m.viper.SetConfigType("yaml")
	m.viper.AddConfigPath("./configs")
	m.viper.AddConfigPath(".")
	
	// Enable environment variable support
	m.viper.AutomaticEnv()
	m.viper.SetEnvPrefix("GATEWAY")
	m.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	
	// Try to read config file
	if err := m.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found is OK, we'll use defaults and env vars
	}
	
	// Unmarshal into config struct
	config := &types.Config{}
	if err := m.viper.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	m.config = config
	return nil
}

// setDefaults sets default configuration values
func (m *Manager) setDefaults() {
	// Server defaults
	m.viper.SetDefault("server.host", "0.0.0.0")
	m.viper.SetDefault("server.port", 8080)
	m.viper.SetDefault("server.read_timeout", "30s")
	m.viper.SetDefault("server.write_timeout", "30s")
	m.viper.SetDefault("server.idle_timeout", "120s")
	
	// Database defaults
	m.viper.SetDefault("database.host", "localhost")
	m.viper.SetDefault("database.port", 5432)
	m.viper.SetDefault("database.username", "gateway")
	m.viper.SetDefault("database.password", "password")
	m.viper.SetDefault("database.database", "gateway")
	m.viper.SetDefault("database.max_open_conns", 100)
	m.viper.SetDefault("database.max_idle_conns", 10)
	
	// Redis defaults
	m.viper.SetDefault("redis.host", "localhost")
	m.viper.SetDefault("redis.port", 6379)
	m.viper.SetDefault("redis.password", "")
	m.viper.SetDefault("redis.database", 0)
	
	// Auth defaults
	m.viper.SetDefault("auth.jwt_secret", "your-secret-key")
	m.viper.SetDefault("auth.jwt_expiration", "24h")
	m.viper.SetDefault("auth.enable_api_key", true)
	
	// Logging defaults
	m.viper.SetDefault("logging.level", "info")
	m.viper.SetDefault("logging.format", "json")
	m.viper.SetDefault("logging.output", "stdout")
	
	// Metrics defaults
	m.viper.SetDefault("metrics.enabled", true)
	m.viper.SetDefault("metrics.path", "/metrics")
	m.viper.SetDefault("metrics.port", 9090)
}

// Get returns the current configuration
func (m *Manager) Get() *types.Config {
	return m.config
}

// Watch starts watching for configuration changes
func (m *Manager) Watch(callback func(*types.Config)) error {
	m.viper.WatchConfig()
	m.viper.OnConfigChange(func(e fsnotify.Event) {
		config := &types.Config{}
		if err := m.viper.Unmarshal(config); err != nil {
			// Log error but don't crash
			return
		}
		m.config = config
		if callback != nil {
			callback(config)
		}
	})
	return nil
}

// Validate validates the configuration
func (m *Manager) Validate() error {
	if m.config == nil {
		return fmt.Errorf("configuration not loaded")
	}
	
	// Validate server config
	if m.config.Server.Port <= 0 || m.config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", m.config.Server.Port)
	}
	
	// Validate database config
	if m.config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	
	// Validate auth config
	if m.config.Auth.JWTSecret == "" || m.config.Auth.JWTSecret == "your-secret-key" {
		return fmt.Errorf("jwt secret must be set to a secure value")
	}
	
	return nil
}

// GetString returns a string configuration value
func (m *Manager) GetString(key string) string {
	return m.viper.GetString(key)
}

// GetInt returns an integer configuration value
func (m *Manager) GetInt(key string) int {
	return m.viper.GetInt(key)
}

// GetBool returns a boolean configuration value
func (m *Manager) GetBool(key string) bool {
	return m.viper.GetBool(key)
}

// GetDuration returns a duration configuration value
func (m *Manager) GetDuration(key string) time.Duration {
	return m.viper.GetDuration(key)
}

// Global configuration manager instance
var defaultManager *Manager

// InitDefault initializes the default configuration manager
func InitDefault() error {
	defaultManager = NewManager()
	return defaultManager.Load()
}

// Default returns the default configuration manager
func Default() *Manager {
	return defaultManager
}

// Get returns the current configuration from the default manager
func Get() *types.Config {
	if defaultManager == nil {
		return nil
	}
	return defaultManager.Get()
}