// Package storage defines database models and storage interfaces
package storage

import (
	"time"

	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"unique;not null"`
	Email     string    `json:"email" gorm:"unique;not null"`
	Password  string    `json:"-" gorm:"not null"` // Never serialize password
	FullName  string    `json:"full_name" gorm:"default:''"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	IsAdmin   bool      `json:"is_admin" gorm:"default:false"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships
	APIKeys  []APIKey  `json:"api_keys,omitempty"`
	Quotas   []Quota   `json:"quotas,omitempty"`
	Requests []Request `json:"-"` // Don't serialize to avoid circular references
}

// APIKey represents an API key for authentication
type APIKey struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	UserID     uint       `json:"user_id" gorm:"not null"`
	Name       string     `json:"name" gorm:"not null"`
	Key        string     `json:"key" gorm:"unique;not null;index"`
	KeyHash    string     `json:"-" gorm:"not null"` // Store hashed version
	IsActive   bool       `json:"is_active" gorm:"default:true"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`

	// Relationships
	User     User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Requests []Request `json:"-" gorm:"foreignKey:APIKeyID"`
}

// Quota represents usage quotas for users
type Quota struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	UserID      uint      `json:"user_id" gorm:"not null"`
	QuotaType   string    `json:"quota_type" gorm:"not null"` // requests, tokens, cost
	LimitValue  int64     `json:"limit_value" gorm:"not null"`
	UsedValue   int64     `json:"used_value" gorm:"default:0"`
	ResetPeriod string    `json:"reset_period" gorm:"not null"` // hourly, daily, monthly
	LastResetAt time.Time `json:"last_reset_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Relationships
	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// Provider represents an LLM provider configuration
type Provider struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"unique;not null"`
	Type         string    `json:"type" gorm:"not null"` // openai, anthropic, baidu, etc.
	BaseURL      string    `json:"base_url" gorm:"not null"`
	APIKey       string    `json:"-" gorm:"not null"` // Encrypted storage
	IsEnabled    bool      `json:"is_enabled" gorm:"default:true"`
	Priority     int       `json:"priority" gorm:"default:1"`
	Weight       int       `json:"weight" gorm:"default:100"`
	Timeout      int       `json:"timeout" gorm:"default:30"` // seconds
	RetryCount   int       `json:"retry_count" gorm:"default:3"`
	RateLimit    int       `json:"rate_limit" gorm:"default:1000"` // requests per minute
	CostPerToken float64   `json:"cost_per_token" gorm:"default:0"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relationships
	Models   []Model   `json:"models,omitempty" gorm:"foreignKey:ProviderID"`
	Requests []Request `json:"-" gorm:"foreignKey:ProviderID"`
}

// Model represents a specific model offered by a provider
type Model struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	ProviderID         uint      `json:"provider_id" gorm:"not null"`
	Name               string    `json:"name" gorm:"not null"`
	DisplayName        string    `json:"display_name"`
	MaxTokens          int       `json:"max_tokens" gorm:"default:4096"`
	SupportedModes     string    `json:"supported_modes"` // JSON array: ["chat", "completion"]
	CostPerInputToken  float64   `json:"cost_per_input_token" gorm:"default:0"`
	CostPerOutputToken float64   `json:"cost_per_output_token" gorm:"default:0"`
	IsEnabled          bool      `json:"is_enabled" gorm:"default:true"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Relationships
	Provider Provider  `json:"provider,omitempty" gorm:"foreignKey:ProviderID"`
	Requests []Request `json:"-" gorm:"foreignKey:ModelID"`
}

// Request represents a logged API request
type Request struct {
	ID         uint   `json:"id" gorm:"primaryKey"`
	RequestID  string `json:"request_id" gorm:"unique;not null;index"`
	UserID     *uint  `json:"user_id"`
	APIKeyID   *uint  `json:"api_key_id"`
	ProviderID *uint  `json:"provider_id"`
	ModelID    *uint  `json:"model_id"`

	// Request details
	Method    string `json:"method" gorm:"not null"`
	Path      string `json:"path" gorm:"not null"`
	ClientIP  string `json:"client_ip"`
	UserAgent string `json:"user_agent"`

	// LLM specific
	ModelName        string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	TotalTokens      int    `json:"total_tokens" gorm:"default:0"`

	// Response details
	StatusCode     int   `json:"status_code"`
	ResponseTimeMs int64 `json:"response_time_ms"` // milliseconds
	ResponseSize   int64 `json:"response_size"`

	// Cost calculation
	Cost float64 `json:"cost" gorm:"default:0"`

	// Timestamps
	RequestTime time.Time `json:"request_time"`
	ResponseAt  time.Time `json:"response_at"`
	CreatedAt   time.Time `json:"created_at"`

	// Relationships
	User     *User     `json:"user,omitempty" gorm:"foreignKey:UserID"`
	APIKey   *APIKey   `json:"api_key,omitempty" gorm:"foreignKey:APIKeyID"`
	Provider *Provider `json:"provider,omitempty" gorm:"foreignKey:ProviderID"`
	ModelRef *Model    `json:"model_ref,omitempty" gorm:"foreignKey:ModelID"`
}

// ProviderHealth represents provider health check results
type ProviderHealth struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProviderID   uint      `json:"provider_id" gorm:"not null"`
	IsHealthy    bool      `json:"is_healthy"`
	ResponseTime int64     `json:"response_time_ms"`
	ErrorMessage string    `json:"error_message"`
	CheckedAt    time.Time `json:"checked_at"`
	CreatedAt    time.Time `json:"created_at"`

	// Relationships
	Provider Provider `json:"provider,omitempty" gorm:"foreignKey:ProviderID"`
}

// RateLimitRecord represents rate limiting records
type RateLimitRecord struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Key       string    `json:"key" gorm:"not null;index"` // user:endpoint or ip:endpoint
	Count     int64     `json:"count"`
	Window    string    `json:"window"` // time window identifier
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConfigSetting represents dynamic configuration settings
type ConfigSetting struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Key         string    `json:"key" gorm:"unique;not null"`
	Value       string    `json:"value" gorm:"not null"`
	Type        string    `json:"type" gorm:"not null"` // string, int, bool, json
	Category    string    `json:"category"`
	Description string    `json:"description"`
	IsPublic    bool      `json:"is_public" gorm:"default:false"` // whether visible to non-admin users
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BeforeCreate hooks for model initialization
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	if u.UpdatedAt.IsZero() {
		u.UpdatedAt = time.Now()
	}
	return nil
}

func (a *APIKey) BeforeCreate(tx *gorm.DB) error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = time.Now()
	}
	return nil
}

func (r *Request) BeforeCreate(tx *gorm.DB) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	if r.RequestTime.IsZero() {
		r.RequestTime = time.Now()
	}
	return nil
}
