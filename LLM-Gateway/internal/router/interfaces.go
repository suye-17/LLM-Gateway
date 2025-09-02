// Package router defines interfaces for intelligent routing system
package router

import (
	"time"

	"github.com/llm-gateway/gateway/internal/router/strategies"
	"github.com/llm-gateway/gateway/pkg/types"
)

// HealthChecker interface defines health checking functionality
type HealthChecker interface {
	// StartHealthCheck starts the health checking routine
	StartHealthCheck() error

	// StopHealthCheck stops the health checking routine
	StopHealthCheck() error

	// CheckProvider performs immediate health check on a provider
	CheckProvider(providerID string) (*HealthResult, error)

	// GetHealthyProviders returns list of healthy providers
	GetHealthyProviders() []*types.Provider

	// GetProviderHealth returns health status of a specific provider
	GetProviderHealth(providerID string) (*HealthResult, error)

	// GetAllHealthResults returns health status of all providers
	GetAllHealthResults() map[string]*HealthResult

	// UpdateConfig updates the health check configuration
	UpdateConfig(config *HealthCheckConfig) error

	// AddProvider adds a provider to health monitoring
	AddProvider(provider *types.Provider) error

	// RemoveProvider removes a provider from health monitoring
	RemoveProvider(providerID string) error
}

// HealthResult represents the result of a health check
type HealthResult struct {
	ProviderID   string        `json:"provider_id"`
	IsHealthy    bool          `json:"is_healthy"`
	ResponseTime time.Duration `json:"response_time"`
	ErrorCount   int64         `json:"error_count"`
	LastCheck    time.Time     `json:"last_check"`
	ErrorRate    float64       `json:"error_rate"`
	Status       string        `json:"status"`
	ErrorMessage string        `json:"error_message,omitempty"`
}

// HealthCheckConfig defines configuration for health checking
type HealthCheckConfig struct {
	Interval         time.Duration `json:"interval"`          // Check interval
	Timeout          time.Duration `json:"timeout"`           // Request timeout
	FailureThreshold int           `json:"failure_threshold"` // Failures before marking unhealthy
	SuccessThreshold int           `json:"success_threshold"` // Successes before marking healthy
	Path             string        `json:"path"`              // Health check endpoint path
}

// MetricsCollector interface defines metrics collection functionality
type MetricsCollector interface {
	// RecordRouting records routing decision metrics
	RecordRouting(providerID string, latency time.Duration, success bool)

	// RecordStrategy records strategy selection metrics
	RecordStrategy(strategyName string, latency time.Duration)

	// RecordProvider records provider-specific metrics
	RecordProvider(providerID string, latency time.Duration, success bool)

	// GetRoutingMetrics returns overall routing metrics
	GetRoutingMetrics() *RoutingMetrics

	// GetStrategyMetrics returns strategy-specific metrics
	GetStrategyMetrics() map[string]*strategies.StrategyMetrics

	// GetProviderMetrics returns provider-specific metrics
	GetProviderMetrics() map[string]*ProviderMetrics
}

// RoutingMetrics represents overall routing metrics
type RoutingMetrics struct {
	TotalRequests    int64         `json:"total_requests"`
	SuccessfulRoutes int64         `json:"successful_routes"`
	FailedRoutes     int64         `json:"failed_routes"`
	AverageLatency   time.Duration `json:"average_latency"`
	RoutingLatency   time.Duration `json:"routing_latency"`
	StartTime        time.Time     `json:"start_time"`
}

// ProviderMetrics represents provider-specific metrics
type ProviderMetrics struct {
	ProviderID        string        `json:"provider_id"`
	RequestCount      int64         `json:"request_count"`
	SuccessCount      int64         `json:"success_count"`
	FailureCount      int64         `json:"failure_count"`
	AverageLatency    time.Duration `json:"average_latency"`
	SuccessRate       float64       `json:"success_rate"`
	ActiveConnections int64         `json:"active_connections"`
	LastUsed          time.Time     `json:"last_used"`
}

// SmartRouterConfig defines configuration for the smart router
type SmartRouterConfig struct {
	Strategy            string               `json:"strategy"` // Load balancing strategy
	HealthCheckInterval time.Duration        `json:"health_check_interval"`
	FailoverEnabled     bool                 `json:"failover_enabled"`
	MaxRetries          int                  `json:"max_retries"`
	Weights             map[string]int       `json:"weights"` // Provider weights
	CircuitBreaker      CircuitBreakerConfig `json:"circuit_breaker"`
	MetricsEnabled      bool                 `json:"metrics_enabled"`
}

// CircuitBreakerConfig defines circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled     bool          `json:"enabled"`
	Threshold   int           `json:"threshold"`    // Failure threshold
	Timeout     time.Duration `json:"timeout"`      // Open state timeout
	MaxRequests int           `json:"max_requests"` // Max requests in half-open
}

// SmartRoutingResult contains the result of provider selection
type SmartRoutingResult struct {
	Provider      types.Provider `json:"-"`
	ProviderName  string         `json:"provider_name"`
	Reason        string         `json:"reason"`
	Attempts      int            `json:"attempts"`
	BackupUsed    bool           `json:"backup_used"`
	Strategy      string         `json:"strategy"`
	LoadFactor    float64        `json:"load_factor"`
	SelectionTime time.Duration  `json:"selection_time"`
}
