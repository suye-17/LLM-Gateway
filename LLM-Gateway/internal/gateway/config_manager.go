// Package gateway implements dynamic configuration management
package gateway

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// ConfigManager manages dynamic configuration updates
type ConfigManager struct {
	config     *types.Config
	logger     *utils.Logger
	mu         sync.RWMutex
	watchers   []ConfigWatcher
	lastUpdate time.Time
}

// ConfigWatcher defines interface for configuration change observers
type ConfigWatcher interface {
	OnConfigChange(oldConfig, newConfig *types.Config) error
}

// ConfigUpdate represents a configuration update request
type ConfigUpdate struct {
	Section string      `json:"section"`
	Key     string      `json:"key"`
	Value   interface{} `json:"value"`
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(config *types.Config, logger *utils.Logger) *ConfigManager {
	return &ConfigManager{
		config:     config,
		logger:     logger,
		watchers:   make([]ConfigWatcher, 0),
		lastUpdate: time.Now(),
	}
}

// GetConfig returns a copy of the current configuration
func (cm *ConfigManager) GetConfig() *types.Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Return a deep copy to prevent modification
	configJSON, _ := json.Marshal(cm.config)
	var configCopy types.Config
	json.Unmarshal(configJSON, &configCopy)

	return &configCopy
}

// UpdateConfig updates configuration with the provided changes
func (cm *ConfigManager) UpdateConfig(updates map[string]interface{}) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	oldConfig := cm.GetConfig()

	// Apply updates
	for key, value := range updates {
		if err := cm.applyUpdate(key, value); err != nil {
			cm.logger.WithField("key", key).
				WithField("error", err.Error()).
				Error("Failed to apply configuration update")
			return fmt.Errorf("failed to update config key %s: %w", key, err)
		}
	}

	// Notify watchers
	for _, watcher := range cm.watchers {
		if err := watcher.OnConfigChange(oldConfig, cm.config); err != nil {
			cm.logger.WithField("error", err.Error()).
				Warn("Configuration watcher failed to handle change")
		}
	}

	cm.lastUpdate = time.Now()
	cm.logger.WithField("updates", len(updates)).
		Info("Configuration updated successfully")

	return nil
}

// applyUpdate applies a single configuration update
func (cm *ConfigManager) applyUpdate(key string, value interface{}) error {
	switch key {
	case "server.port":
		if port, ok := value.(float64); ok {
			cm.config.Server.Port = int(port)
		} else {
			return fmt.Errorf("invalid value type for server.port: expected number")
		}
	case "server.host":
		if host, ok := value.(string); ok {
			cm.config.Server.Host = host
		} else {
			return fmt.Errorf("invalid value type for server.host: expected string")
		}
	case "logging.level":
		if level, ok := value.(string); ok {
			cm.config.Logging.Level = level
		} else {
			return fmt.Errorf("invalid value type for logging.level: expected string")
		}
	case "auth.jwt_expiration":
		if exp, ok := value.(string); ok {
			if duration, err := time.ParseDuration(exp); err == nil {
				cm.config.Auth.JWTExpiration = duration
			} else {
				return fmt.Errorf("invalid duration format for auth.jwt_expiration: %w", err)
			}
		} else {
			return fmt.Errorf("invalid value type for auth.jwt_expiration: expected string")
		}
	case "redis.host":
		if host, ok := value.(string); ok {
			cm.config.Redis.Host = host
		} else {
			return fmt.Errorf("invalid value type for redis.host: expected string")
		}
	case "redis.port":
		if port, ok := value.(float64); ok {
			cm.config.Redis.Port = int(port)
		} else {
			return fmt.Errorf("invalid value type for redis.port: expected number")
		}
	case "database.host":
		if host, ok := value.(string); ok {
			cm.config.Database.Host = host
		} else {
			return fmt.Errorf("invalid value type for database.host: expected string")
		}
	case "database.port":
		if port, ok := value.(float64); ok {
			cm.config.Database.Port = int(port)
		} else {
			return fmt.Errorf("invalid value type for database.port: expected number")
		}
	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	return nil
}

// AddWatcher adds a configuration change watcher
func (cm *ConfigManager) AddWatcher(watcher ConfigWatcher) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.watchers = append(cm.watchers, watcher)
	cm.logger.Info("Configuration watcher added")
}

// RemoveWatcher removes a configuration change watcher
func (cm *ConfigManager) RemoveWatcher(watcher ConfigWatcher) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, w := range cm.watchers {
		if w == watcher {
			cm.watchers = append(cm.watchers[:i], cm.watchers[i+1:]...)
			cm.logger.Info("Configuration watcher removed")
			break
		}
	}
}

// GetLastUpdateTime returns the time of the last configuration update
func (cm *ConfigManager) GetLastUpdateTime() time.Time {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.lastUpdate
}

// ValidateConfig validates the current configuration
func (cm *ConfigManager) ValidateConfig() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Validate server configuration
	if cm.config.Server.Port <= 0 || cm.config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", cm.config.Server.Port)
	}

	if cm.config.Server.Host == "" {
		return fmt.Errorf("server host cannot be empty")
	}

	// Validate database configuration
	if cm.config.Database.Host == "" {
		return fmt.Errorf("database host cannot be empty")
	}

	if cm.config.Database.Port <= 0 || cm.config.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", cm.config.Database.Port)
	}

	if cm.config.Database.Username == "" {
		return fmt.Errorf("database username cannot be empty")
	}

	if cm.config.Database.Database == "" {
		return fmt.Errorf("database name cannot be empty")
	}

	// Validate Redis configuration
	if cm.config.Redis.Host == "" {
		return fmt.Errorf("redis host cannot be empty")
	}

	if cm.config.Redis.Port <= 0 || cm.config.Redis.Port > 65535 {
		return fmt.Errorf("invalid redis port: %d", cm.config.Redis.Port)
	}

	// Validate auth configuration
	if cm.config.Auth.JWTSecret == "" {
		return fmt.Errorf("JWT secret cannot be empty")
	}

	if cm.config.Auth.JWTExpiration <= 0 {
		return fmt.Errorf("JWT expiration must be positive")
	}

	// Validate logging configuration
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
		"fatal": true,
	}

	if !validLevels[cm.config.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s", cm.config.Logging.Level)
	}

	return nil
}

// GetConfigSummary returns a summary of the current configuration
func (cm *ConfigManager) GetConfigSummary() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return map[string]interface{}{
		"server": map[string]interface{}{
			"host": cm.config.Server.Host,
			"port": cm.config.Server.Port,
		},
		"database": map[string]interface{}{
			"host":     cm.config.Database.Host,
			"port":     cm.config.Database.Port,
			"database": cm.config.Database.Database,
			"username": cm.config.Database.Username,
		},
		"redis": map[string]interface{}{
			"host": cm.config.Redis.Host,
			"port": cm.config.Redis.Port,
		},
		"auth": map[string]interface{}{
			"jwt_expiration": cm.config.Auth.JWTExpiration.String(),
		},
		"logging": map[string]interface{}{
			"level":  cm.config.Logging.Level,
			"format": cm.config.Logging.Format,
		},
		"last_update": cm.lastUpdate.Format(time.RFC3339),
		"watchers":    len(cm.watchers),
	}
}

// ExportConfig exports the current configuration as JSON
func (cm *ConfigManager) ExportConfig() ([]byte, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return json.MarshalIndent(cm.config, "", "  ")
}

// ImportConfig imports configuration from JSON
func (cm *ConfigManager) ImportConfig(configJSON []byte) error {
	var newConfig types.Config
	if err := json.Unmarshal(configJSON, &newConfig); err != nil {
		return fmt.Errorf("failed to parse configuration JSON: %w", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	oldConfig := cm.config
	cm.config = &newConfig
	cm.lastUpdate = time.Now()

	// Notify watchers
	for _, watcher := range cm.watchers {
		if err := watcher.OnConfigChange(oldConfig, cm.config); err != nil {
			cm.logger.WithField("error", err.Error()).
				Warn("Configuration watcher failed to handle import")
		}
	}

	cm.logger.Info("Configuration imported successfully")
	return nil
}

// RoutingConfigWatcher implements ConfigWatcher for routing configuration changes
type RoutingConfigWatcher struct {
	routingService interface{} // Would be *router.Service in real implementation
	logger         *utils.Logger
}

// NewRoutingConfigWatcher creates a new routing configuration watcher
func NewRoutingConfigWatcher(routingService interface{}, logger *utils.Logger) *RoutingConfigWatcher {
	return &RoutingConfigWatcher{
		routingService: routingService,
		logger:         logger,
	}
}

// OnConfigChange handles configuration changes for routing
func (w *RoutingConfigWatcher) OnConfigChange(oldConfig, newConfig *types.Config) error {
	// Check if routing-related configuration has changed
	// This would include provider configurations, routing strategies, etc.

	w.logger.Info("Routing configuration change detected")

	// TODO: Implement actual routing configuration updates
	// This would involve updating provider configurations,
	// routing strategies, health check intervals, etc.

	return nil
}

// AuthConfigWatcher implements ConfigWatcher for authentication configuration changes
type AuthConfigWatcher struct {
	authService interface{} // Would be *auth.AuthService in real implementation
	logger      *utils.Logger
}

// NewAuthConfigWatcher creates a new auth configuration watcher
func NewAuthConfigWatcher(authService interface{}, logger *utils.Logger) *AuthConfigWatcher {
	return &AuthConfigWatcher{
		authService: authService,
		logger:      logger,
	}
}

// OnConfigChange handles configuration changes for authentication
func (w *AuthConfigWatcher) OnConfigChange(oldConfig, newConfig *types.Config) error {
	// Check if auth-related configuration has changed
	if oldConfig.Auth.JWTExpiration != newConfig.Auth.JWTExpiration {
		w.logger.Info("JWT expiration configuration changed")
		// TODO: Update auth service configuration
	}

	if oldConfig.Auth.JWTSecret != newConfig.Auth.JWTSecret {
		w.logger.Warn("JWT secret changed - this may invalidate existing tokens")
		// TODO: Handle JWT secret change
	}

	return nil
}
