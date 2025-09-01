// Package types defines core types and interfaces for the LLM Gateway
package types

import (
	"context"
	"time"
)

// Request represents a standardized LLM request
type Request struct {
	ID          string                 `json:"id"`
	Model       string                 `json:"model"`
	Messages    []Message              `json:"messages"`
	Temperature float64                `json:"temperature,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	UserID      string                 `json:"user_id,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// Message represents a single message in the conversation
type Message struct {
	Role    string `json:"role"` // system, user, assistant
	Content string `json:"content"`
}

// Response represents a standardized LLM response
type Response struct {
	ID       string    `json:"id"`
	Model    string    `json:"model"`
	Choices  []Choice  `json:"choices"`
	Usage    Usage     `json:"usage"`
	Provider string    `json:"provider"`
	Latency  int64     `json:"latency_ms"`
	Created  time.Time `json:"created"`
}

// Choice represents a single response choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason *string `json:"finish_reason,omitempty"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Provider interface defines the contract for LLM providers
type Provider interface {
	// Basic provider information
	GetName() string
	GetType() string

	// Core functionality
	Call(ctx context.Context, request *ChatCompletionRequest) (*ChatCompletionResponse, error)

	// Health and monitoring
	HealthCheck(ctx context.Context) (*HealthStatus, error)
	GetRateLimit() *RateLimitInfo
	EstimateCost(request *ChatCompletionRequest) (*CostEstimate, error)

	// Configuration
	GetModels(ctx context.Context) ([]*Model, error)
	GetConfig() *ProviderConfig
}

// ProviderMetrics represents provider performance metrics
type ProviderMetrics struct {
	TotalRequests   int64         `json:"total_requests"`
	SuccessRequests int64         `json:"success_requests"`
	FailedRequests  int64         `json:"failed_requests"`
	AvgLatency      time.Duration `json:"avg_latency"`
	LastHealthCheck time.Time     `json:"last_health_check"`
	IsHealthy       bool          `json:"is_healthy"`
}

// Router interface defines the routing strategy
type Router interface {
	Route(request *Request) (Provider, error)
	AddProvider(provider Provider) error
	RemoveProvider(providerID string) error
	GetProviders() []Provider
}

// Middleware interface defines the middleware contract
type Middleware interface {
	Process(ctx *Context, next Handler) error
}

// Handler function type for request handling
type Handler func(ctx *Context) error

// Context represents the request context with additional gateway information
type Context struct {
	Context   context.Context
	Request   *Request
	Response  *Response
	UserID    string
	APIKey    string
	StartTime time.Time
	Metadata  map[string]interface{}
}

// Config represents the gateway configuration
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Username     string `mapstructure:"username"`
	Password     string `mapstructure:"password"`
	Database     string `mapstructure:"database"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

// RedisConfig represents Redis configuration
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	Database int    `mapstructure:"database"`
}

// AuthConfig represents authentication configuration
type AuthConfig struct {
	JWTSecret     string        `mapstructure:"jwt_secret"`
	JWTExpiration time.Duration `mapstructure:"jwt_expiration"`
	EnableAPIKey  bool          `mapstructure:"enable_api_key"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// MetricsConfig represents metrics configuration
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
	Port    int    `mapstructure:"port"`
}

// RegisterRequest represents a user registration request
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"full_name" binding:"required,min=1,max=100"`
}

// RegisterResponse represents a user registration response
type RegisterResponse struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Message  string `json:"message"`
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Model            string         `json:"model" binding:"required"`
	Messages         []Message      `json:"messages" binding:"required"`
	MaxTokens        *int           `json:"max_tokens,omitempty"`
	Temperature      *float64       `json:"temperature,omitempty"`
	TopP             *float64       `json:"top_p,omitempty"`
	N                *int           `json:"n,omitempty"`
	Stream           *bool          `json:"stream,omitempty"`
	Stop             interface{}    `json:"stop,omitempty"`
	PresencePenalty  *float64       `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64       `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]int `json:"logit_bias,omitempty"`
	User             *string        `json:"user,omitempty"`
	RequestID        string         `json:"-"` // Internal field, not serialized
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	ID        string   `json:"id"`
	Object    string   `json:"object"`
	Created   string   `json:"created"`
	Model     string   `json:"model"`
	Choices   []Choice `json:"choices"`
	Usage     Usage    `json:"usage"`
	Provider  string   `json:"provider"`
	LatencyMs int64    `json:"latency_ms"`
	RequestID string   `json:"request_id,omitempty"`
}

// ProviderConfig represents provider configuration
type ProviderConfig struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Enabled      bool              `json:"enabled"`
	BaseURL      string            `json:"base_url"`
	APIKey       string            `json:"api_key"`
	Priority     int               `json:"priority"`
	Weight       int               `json:"weight"`
	Timeout      time.Duration     `json:"timeout"`
	RetryCount   int               `json:"retry_count"`
	RateLimit    int               `json:"rate_limit"`
	Models       []string          `json:"models"`
	CustomConfig map[string]string `json:"custom_config"`
}

// Model represents an AI model
type Model struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	ProviderID         uint      `json:"provider_id" gorm:"not null"`
	Name               string    `json:"name" gorm:"not null"`
	DisplayName        string    `json:"display_name" gorm:"not null"`
	Description        string    `json:"description"`
	ContextLength      int       `json:"context_length" gorm:"default:4096"`
	SupportedModes     string    `json:"supported_modes"` // JSON array: ["chat", "completion"]
	CostPerInputToken  float64   `json:"cost_per_input_token" gorm:"default:0"`
	CostPerOutputToken float64   `json:"cost_per_output_token" gorm:"default:0"`
	IsEnabled          bool      `json:"is_enabled" gorm:"default:true"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// HealthStatus represents the health status of a provider
type HealthStatus struct {
	IsHealthy    bool          `json:"is_healthy"`
	ResponseTime time.Duration `json:"response_time_ms"`
	ErrorMessage string        `json:"error_message,omitempty"`
	LastChecked  time.Time     `json:"last_checked"`
	Endpoint     string        `json:"endpoint"`
}

// CostEstimate represents cost estimation for a request
type CostEstimate struct {
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	TotalTokens   int     `json:"total_tokens"`
	InputCost     float64 `json:"input_cost"`
	OutputCost    float64 `json:"output_cost"`
	TotalCost     float64 `json:"total_cost"`
	Currency      string  `json:"currency"`
	PricePerToken float64 `json:"price_per_token"`
}

// RateLimitInfo represents rate limiting information
type RateLimitInfo struct {
	RequestsPerMinute int       `json:"requests_per_minute"`
	TokensPerMinute   int       `json:"tokens_per_minute"`
	RemainingRequests int       `json:"remaining_requests"`
	RemainingTokens   int       `json:"remaining_tokens"`
	ResetTime         time.Time `json:"reset_time"`
}

// ProviderRegistry manages all registered providers
type ProviderRegistry interface {
	// RegisterProvider registers a new provider
	RegisterProvider(provider Provider) error

	// GetProvider returns a provider by name
	GetProvider(name string) (Provider, error)

	// GetProviders returns all registered providers
	GetProviders() []Provider

	// GetHealthyProviders returns only healthy providers
	GetHealthyProviders() []Provider

	// GetProvidersByType returns providers of a specific type
	GetProvidersByType(providerType string) []Provider
}
