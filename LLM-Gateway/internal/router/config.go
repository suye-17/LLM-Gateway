// Package router provides configuration types for the intelligent routing system
package router

import (
	"fmt"
	"time"
)

// DefaultSmartRouterConfig returns a default router configuration
func DefaultSmartRouterConfig() *SmartRouterConfig {
	return &SmartRouterConfig{
		Strategy:            "round_robin",
		HealthCheckInterval: 30 * time.Second,
		FailoverEnabled:     true,
		MaxRetries:          3,
		Weights:             make(map[string]int),
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:     true,
			Threshold:   5,
			Timeout:     30 * time.Second,
			MaxRequests: 3,
		},
		MetricsEnabled: true,
	}
}

// DefaultHealthCheckConfig returns a default health check configuration
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Interval:         30 * time.Second,
		Timeout:          5 * time.Second,
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Path:             "/health",
	}
}

// ValidateConfig validates the router configuration
func (c *SmartRouterConfig) ValidateConfig() error {
	if c.Strategy == "" {
		return fmt.Errorf("strategy cannot be empty")
	}

	validStrategies := map[string]bool{
		"round_robin":          true,
		"weighted_round_robin": true,
		"least_connections":    true,
		"health_based":         true,
		"least_latency":        true,
		"cost_optimized":       true,
		"random":               true,
	}

	if !validStrategies[c.Strategy] {
		return fmt.Errorf("invalid strategy: %s", c.Strategy)
	}

	if c.HealthCheckInterval <= 0 {
		return fmt.Errorf("health check interval must be positive")
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}

	// Validate circuit breaker config
	if c.CircuitBreaker.Enabled {
		if c.CircuitBreaker.Threshold <= 0 {
			return fmt.Errorf("circuit breaker threshold must be positive")
		}
		if c.CircuitBreaker.Timeout <= 0 {
			return fmt.Errorf("circuit breaker timeout must be positive")
		}
		if c.CircuitBreaker.MaxRequests <= 0 {
			return fmt.Errorf("circuit breaker max requests must be positive")
		}
	}

	// Validate weights
	for provider, weight := range c.Weights {
		if weight < 0 {
			return fmt.Errorf("weight for provider %s cannot be negative", provider)
		}
	}

	return nil
}

// ValidateHealthCheckConfig validates the health check configuration
func (c *HealthCheckConfig) ValidateHealthCheckConfig() error {
	if c.Interval <= 0 {
		return fmt.Errorf("health check interval must be positive")
	}

	if c.Timeout <= 0 {
		return fmt.Errorf("health check timeout must be positive")
	}

	if c.Timeout >= c.Interval {
		return fmt.Errorf("health check timeout must be less than interval")
	}

	if c.FailureThreshold <= 0 {
		return fmt.Errorf("failure threshold must be positive")
	}

	if c.SuccessThreshold <= 0 {
		return fmt.Errorf("success threshold must be positive")
	}

	return nil
}

// Clone creates a deep copy of the router configuration
func (c *SmartRouterConfig) Clone() *SmartRouterConfig {
	clone := &SmartRouterConfig{
		Strategy:            c.Strategy,
		HealthCheckInterval: c.HealthCheckInterval,
		FailoverEnabled:     c.FailoverEnabled,
		MaxRetries:          c.MaxRetries,
		CircuitBreaker:      c.CircuitBreaker,
		MetricsEnabled:      c.MetricsEnabled,
		Weights:             make(map[string]int),
	}

	// Copy weights map
	for k, v := range c.Weights {
		clone.Weights[k] = v
	}

	return clone
}

// Clone creates a deep copy of the health check configuration
func (c *HealthCheckConfig) Clone() *HealthCheckConfig {
	return &HealthCheckConfig{
		Interval:         c.Interval,
		Timeout:          c.Timeout,
		FailureThreshold: c.FailureThreshold,
		SuccessThreshold: c.SuccessThreshold,
		Path:             c.Path,
	}
}
