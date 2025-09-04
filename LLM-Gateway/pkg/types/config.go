// Package types defines enhanced configuration structures for production
package types

import (
	"time"
)

// ProductionConfig extends ProviderConfig with production-specific settings
type ProductionConfig struct {
	*ProviderConfig

	// Security Configuration
	APIKeyEnvVar    string `json:"api_key_env" mapstructure:"api_key_env"`
	CredentialsPath string `json:"credentials_path" mapstructure:"credentials_path"`

	// Retry Configuration
	RetryPolicy *RetryPolicy `json:"retry_policy" mapstructure:"retry_policy"`

	// Cost Configuration
	CostTracking bool        `json:"cost_tracking" mapstructure:"cost_tracking"`
	CostLimits   *CostLimits `json:"cost_limits" mapstructure:"cost_limits"`

	// Monitoring Configuration
	MetricsEnabled bool   `json:"metrics_enabled" mapstructure:"metrics_enabled"`
	HealthCheckURL string `json:"health_check_url" mapstructure:"health_check_url"`
}

// RetryPolicy defines retry behavior for provider requests
type RetryPolicy struct {
	MaxRetries      int           `json:"max_retries" mapstructure:"max_retries"`
	BaseDelay       time.Duration `json:"base_delay" mapstructure:"base_delay"`
	MaxDelay        time.Duration `json:"max_delay" mapstructure:"max_delay"`
	BackoffFactor   float64       `json:"backoff_factor" mapstructure:"backoff_factor"`
	RetryableErrors []string      `json:"retryable_errors" mapstructure:"retryable_errors"`
}

// CostLimits defines cost control settings
type CostLimits struct {
	DailyLimit      float64 `json:"daily_limit" mapstructure:"daily_limit"`
	MonthlyLimit    float64 `json:"monthly_limit" mapstructure:"monthly_limit"`
	PerRequestLimit float64 `json:"per_request_limit" mapstructure:"per_request_limit"`
	Currency        string  `json:"currency" mapstructure:"currency"`
}

// ErrorCategory represents different types of provider errors
type ErrorCategory string

const (
	ErrorAuth      ErrorCategory = "auth"       // Authentication errors, not retryable
	ErrorRateLimit ErrorCategory = "rate_limit" // Rate limiting, retryable
	ErrorQuota     ErrorCategory = "quota"      // Quota exceeded, not retryable
	ErrorNetwork   ErrorCategory = "network"    // Network errors, retryable
	ErrorServer    ErrorCategory = "server"     // Server errors, retryable
	ErrorClient    ErrorCategory = "client"     // Client errors, not retryable
	ErrorTimeout   ErrorCategory = "timeout"    // Timeout errors, retryable
)

// TokenEstimate represents token count estimation for cost calculation
type TokenEstimate struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"` // Estimated based on max_tokens
	TotalTokens  int `json:"total_tokens"`
}

// CostBreakdown represents detailed cost calculation result
type CostBreakdown struct {
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	InputCost    float64   `json:"input_cost"`
	OutputCost   float64   `json:"output_cost"`
	TotalCost    float64   `json:"total_cost"`
	Currency     string    `json:"currency"`
	Model        string    `json:"model"`
	Provider     string    `json:"provider"`
	Timestamp    time.Time `json:"timestamp"`
}

// ModelPricing represents pricing information for a specific model
type ModelPricing struct {
	Model       string    `json:"model"`
	Provider    string    `json:"provider"`
	InputPrice  float64   `json:"input_price"`  // Price per 1K input tokens
	OutputPrice float64   `json:"output_price"` // Price per 1K output tokens
	Currency    string    `json:"currency"`
	LastUpdated time.Time `json:"last_updated"`
}

// SecureConfig represents securely loaded configuration
type SecureConfig struct {
	APIKey         string `json:"-"` // Never serialize API keys
	BaseURL        string `json:"base_url"`
	OrganizationID string `json:"organization_id,omitempty"`
	ProjectID      string `json:"project_id,omitempty"`
	Region         string `json:"region,omitempty"`
}

// DefaultRetryPolicy returns a default retry policy
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:    3,
		BaseDelay:     1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		RetryableErrors: []string{
			string(ErrorNetwork),
			string(ErrorServer),
			string(ErrorTimeout),
			string(ErrorRateLimit),
		},
	}
}

// DefaultCostLimits returns default cost limits
func DefaultCostLimits() *CostLimits {
	return &CostLimits{
		DailyLimit:      100.0,  // $100 per day
		MonthlyLimit:    1000.0, // $1000 per month
		PerRequestLimit: 10.0,   // $10 per request
		Currency:        "USD",
	}
}

// NewProductionConfig creates a new ProductionConfig with defaults
func NewProductionConfig(base *ProviderConfig) *ProductionConfig {
	return &ProductionConfig{
		ProviderConfig: base,
		RetryPolicy:    DefaultRetryPolicy(),
		CostTracking:   true,
		CostLimits:     DefaultCostLimits(),
		MetricsEnabled: true,
	}
}

// Validate validates the production configuration
func (pc *ProductionConfig) Validate() error {
	if pc.ProviderConfig == nil {
		return ErrInvalidConfig("base ProviderConfig is required")
	}

	if pc.APIKeyEnvVar == "" && pc.CredentialsPath == "" && pc.APIKey == "" {
		return ErrInvalidConfig("API key source must be specified (env var, credentials path, or direct)")
	}

	if pc.RetryPolicy != nil {
		if pc.RetryPolicy.MaxRetries < 0 {
			return ErrInvalidConfig("max_retries cannot be negative")
		}
		if pc.RetryPolicy.BaseDelay < 0 {
			return ErrInvalidConfig("base_delay cannot be negative")
		}
		if pc.RetryPolicy.BackoffFactor <= 0 {
			return ErrInvalidConfig("backoff_factor must be positive")
		}
	}

	return nil
}

// ErrInvalidConfig represents configuration validation errors
func ErrInvalidConfig(message string) error {
	return &ConfigError{
		Type:    "validation",
		Message: message,
	}
}

// ConfigError represents configuration-related errors
type ConfigError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *ConfigError) Error() string {
	return e.Message
}
