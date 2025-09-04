// Package config provides secure configuration management for production environments
package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
	"github.com/spf13/viper"
)

// SecureConfigManager manages secure loading and validation of provider configurations
type SecureConfigManager struct {
	logger     *utils.Logger
	configPath string
	configs    map[string]*types.ProductionConfig
	secrets    map[string]types.SecureConfig
	mutex      sync.RWMutex
	lastLoaded time.Time
}

// ConfigurationManager interface defines the contract for configuration management
type ConfigurationManager interface {
	LoadProviderConfig(providerType string) (*types.ProductionConfig, error)
	ValidateAPIKey(ctx context.Context, providerType, apiKey string) error
	GetSecureConfig(providerType string) (*types.SecureConfig, error)
	RefreshCredentials(providerType string) error
	LoadAllConfigs() error
}

// NewSecureConfigManager creates a new secure configuration manager
func NewSecureConfigManager(configPath string, logger *utils.Logger) *SecureConfigManager {
	return &SecureConfigManager{
		logger:     logger,
		configPath: configPath,
		configs:    make(map[string]*types.ProductionConfig),
		secrets:    make(map[string]types.SecureConfig),
	}
}

// LoadProviderConfig loads and returns configuration for a specific provider
func (scm *SecureConfigManager) LoadProviderConfig(providerType string) (*types.ProductionConfig, error) {
	scm.mutex.RLock()
	config, exists := scm.configs[providerType]
	scm.mutex.RUnlock()

	if !exists {
		// Load configuration if not cached
		if err := scm.loadSingleProviderConfig(providerType); err != nil {
			return nil, fmt.Errorf("failed to load config for provider %s: %w", providerType, err)
		}

		scm.mutex.RLock()
		config = scm.configs[providerType]
		scm.mutex.RUnlock()
	}

	if config == nil {
		return nil, fmt.Errorf("provider %s not configured", providerType)
	}

	// Load secure credentials
	secureConfig, err := scm.loadSecureCredentials(providerType, config)
	if err != nil {
		return nil, fmt.Errorf("failed to load secure credentials for %s: %w", providerType, err)
	}

	// Update config with secure credentials
	config.APIKey = secureConfig.APIKey

	return config, nil
}

// ValidateAPIKey validates an API key for a specific provider
func (scm *SecureConfigManager) ValidateAPIKey(ctx context.Context, providerType, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key is empty")
	}

	// Basic format validation based on provider type
	switch providerType {
	case "openai":
		if !strings.HasPrefix(apiKey, "sk-") {
			return fmt.Errorf("invalid OpenAI API key format")
		}
		if len(apiKey) < 20 {
			return fmt.Errorf("OpenAI API key too short")
		}
	case "anthropic":
		if !strings.HasPrefix(apiKey, "sk-ant-") {
			return fmt.Errorf("invalid Anthropic API key format")
		}
		if len(apiKey) < 30 {
			return fmt.Errorf("Anthropic API key too short")
		}
	case "baidu":
		// Baidu uses different key format - just check it's not empty and reasonable length
		if len(apiKey) < 10 {
			return fmt.Errorf("Baidu API key too short")
		}
	default:
		scm.logger.WithField("provider", providerType).Warn("Unknown provider type, skipping key format validation")
	}

	scm.logger.WithField("provider", providerType).Info("API key format validation passed")
	return nil
}

// GetSecureConfig returns secure configuration for a provider
func (scm *SecureConfigManager) GetSecureConfig(providerType string) (*types.SecureConfig, error) {
	scm.mutex.RLock()
	config, exists := scm.secrets[providerType]
	scm.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("secure config not found for provider %s", providerType)
	}

	return &config, nil
}

// RefreshCredentials reloads credentials for a specific provider
func (scm *SecureConfigManager) RefreshCredentials(providerType string) error {
	scm.mutex.Lock()
	defer scm.mutex.Unlock()

	config, exists := scm.configs[providerType]
	if !exists {
		return fmt.Errorf("provider %s not configured", providerType)
	}

	secureConfig, err := scm.loadSecureCredentials(providerType, config)
	if err != nil {
		return fmt.Errorf("failed to refresh credentials: %w", err)
	}

	scm.secrets[providerType] = *secureConfig
	scm.logger.WithField("provider", providerType).Info("Credentials refreshed successfully")

	return nil
}

// LoadAllConfigs loads all provider configurations from the config file
func (scm *SecureConfigManager) LoadAllConfigs() error {
	// Setup viper to read configuration
	v := viper.New()
	v.SetConfigFile(scm.configPath)
	v.SetConfigType("yaml")

	// Read environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read configuration file
	if err := v.ReadInConfig(); err != nil {
		// If config file doesn't exist, create a basic one
		if os.IsNotExist(err) {
			scm.logger.WithField("path", scm.configPath).Info("Config file not found, creating default configuration")
			if err := scm.createDefaultConfig(); err != nil {
				return fmt.Errorf("failed to create default config: %w", err)
			}
			// Try reading again
			if err := v.ReadInConfig(); err != nil {
				return fmt.Errorf("failed to read created config file: %w", err)
			}
		} else {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Load provider configurations
	providersConfig := v.GetStringMap("providers")
	if len(providersConfig) == 0 {
		return fmt.Errorf("no providers configured")
	}

	scm.mutex.Lock()
	defer scm.mutex.Unlock()

	for providerType := range providersConfig {
		if err := scm.loadSingleProviderConfigFromViper(v, providerType); err != nil {
			scm.logger.WithError(err).WithField("provider", providerType).Error("Failed to load provider config")
			continue
		}
	}

	scm.lastLoaded = time.Now()
	scm.logger.WithField("providers", len(scm.configs)).Info("All provider configurations loaded successfully")

	return nil
}

// loadSingleProviderConfig loads configuration for a single provider
func (scm *SecureConfigManager) loadSingleProviderConfig(providerType string) error {
	v := viper.New()
	v.SetConfigFile(scm.configPath)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	return scm.loadSingleProviderConfigFromViper(v, providerType)
}

// loadSingleProviderConfigFromViper loads a single provider config from viper instance
func (scm *SecureConfigManager) loadSingleProviderConfigFromViper(v *viper.Viper, providerType string) error {
	configKey := fmt.Sprintf("providers.%s", providerType)

	// Load base provider config
	var baseConfig types.ProviderConfig
	if err := v.UnmarshalKey(configKey, &baseConfig); err != nil {
		return fmt.Errorf("failed to unmarshal base config: %w", err)
	}

	// Create production config
	prodConfig := types.NewProductionConfig(&baseConfig)

	// Load production-specific settings
	if err := v.UnmarshalKey(configKey, prodConfig); err != nil {
		return fmt.Errorf("failed to unmarshal production config: %w", err)
	}

	// Set defaults if not specified
	if prodConfig.APIKeyEnvVar == "" {
		prodConfig.APIKeyEnvVar = fmt.Sprintf("%s_API_KEY", strings.ToUpper(providerType))
	}

	// Validate configuration
	if err := prodConfig.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	scm.configs[providerType] = prodConfig
	scm.logger.WithField("provider", providerType).Info("Provider configuration loaded successfully")

	return nil
}

// loadSecureCredentials loads secure credentials for a provider
func (scm *SecureConfigManager) loadSecureCredentials(providerType string, config *types.ProductionConfig) (*types.SecureConfig, error) {
	secureConfig := &types.SecureConfig{
		BaseURL: config.BaseURL,
	}

	// Load API key from various sources (priority order)
	var apiKey string
	var err error

	// 1. Environment variable
	if config.APIKeyEnvVar != "" {
		apiKey = os.Getenv(config.APIKeyEnvVar)
		if apiKey != "" {
			scm.logger.WithField("provider", providerType).Info("API key loaded from environment variable")
		}
	}

	// 2. Credentials file
	if apiKey == "" && config.CredentialsPath != "" {
		apiKey, err = scm.loadFromCredentialsFile(config.CredentialsPath)
		if err != nil {
			scm.logger.WithError(err).WithField("provider", providerType).Warn("Failed to load from credentials file")
		} else if apiKey != "" {
			scm.logger.WithField("provider", providerType).Info("API key loaded from credentials file")
		}
	}

	// 3. Direct configuration (least secure, for development only)
	if apiKey == "" && config.APIKey != "" {
		apiKey = config.APIKey
		scm.logger.WithField("provider", providerType).Warn("API key loaded from direct configuration (not recommended for production)")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("no API key found for provider %s", providerType)
	}

	// Validate the API key format
	if err := scm.ValidateAPIKey(context.Background(), providerType, apiKey); err != nil {
		return nil, fmt.Errorf("API key validation failed: %w", err)
	}

	secureConfig.APIKey = apiKey

	// Load provider-specific configuration
	switch providerType {
	case "openai":
		secureConfig.OrganizationID = os.Getenv("OPENAI_ORGANIZATION_ID")
		secureConfig.ProjectID = os.Getenv("OPENAI_PROJECT_ID")
	case "anthropic":
		// Anthropic doesn't use organization/project IDs currently
	case "baidu":
		secureConfig.Region = os.Getenv("BAIDU_REGION")
		// For Baidu, we also need the secret key
		secretKey := os.Getenv("BAIDU_SECRET_KEY")
		if secretKey == "" {
			return nil, fmt.Errorf("BAIDU_SECRET_KEY environment variable is required")
		}
		// Store secret key in a custom field (extend SecureConfig if needed)
	}

	scm.mutex.Lock()
	scm.secrets[providerType] = *secureConfig
	scm.mutex.Unlock()

	return secureConfig, nil
}

// loadFromCredentialsFile loads API key from a credentials file
func (scm *SecureConfigManager) loadFromCredentialsFile(credentialsPath string) (string, error) {
	// Expand home directory if needed
	if strings.HasPrefix(credentialsPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		credentialsPath = filepath.Join(homeDir, credentialsPath[2:])
	}

	// Read file content
	content, err := os.ReadFile(credentialsPath)
	if err != nil {
		return "", fmt.Errorf("failed to read credentials file: %w", err)
	}

	// Return trimmed content as API key
	return strings.TrimSpace(string(content)), nil
}

// createDefaultConfig creates a default configuration file
func (scm *SecureConfigManager) createDefaultConfig() error {
	// Create directory if it doesn't exist
	configDir := filepath.Dir(scm.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	defaultConfig := `# LLM Gateway Provider Configuration
providers:
  openai:
    name: "OpenAI"
    type: "openai"
    enabled: true
    base_url: "https://api.openai.com/v1"
    api_key_env: "OPENAI_API_KEY"
    timeout: 60s
    retry_count: 3
    rate_limit: 60
    models: ["gpt-3.5-turbo", "gpt-4", "gpt-4-turbo"]
    retry_policy:
      max_retries: 3
      base_delay: 1s
      max_delay: 30s
      backoff_factor: 2.0
    cost_tracking: true
    metrics_enabled: true
    
  anthropic:
    name: "Anthropic"
    type: "anthropic"
    enabled: true
    base_url: "https://api.anthropic.com/v1"
    api_key_env: "ANTHROPIC_API_KEY"
    timeout: 60s
    retry_count: 3
    rate_limit: 60
    models: ["claude-3-sonnet", "claude-3-opus", "claude-3-haiku"]
    retry_policy:
      max_retries: 3
      base_delay: 1s
      max_delay: 30s
      backoff_factor: 2.0
    cost_tracking: true
    metrics_enabled: true
    
  baidu:
    name: "百度文心一言"
    type: "baidu"
    enabled: true
    base_url: "https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop"
    api_key_env: "BAIDU_API_KEY"
    timeout: 60s
    retry_count: 3
    rate_limit: 60
    models: ["ernie-bot", "ernie-bot-turbo"]
    retry_policy:
      max_retries: 3
      base_delay: 1s
      max_delay: 30s
      backoff_factor: 2.0
    cost_tracking: true
    metrics_enabled: true
`

	if err := os.WriteFile(scm.configPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write default config: %w", err)
	}

	scm.logger.WithField("path", scm.configPath).Info("Default configuration file created")
	return nil
}
