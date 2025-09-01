// Package router implements intelligent routing algorithms for LLM providers
package router

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// RoutingStrategy defines different routing strategies
type RoutingStrategy string

const (
	StrategyRoundRobin         RoutingStrategy = "round_robin"
	StrategyWeightedRoundRobin RoutingStrategy = "weighted_round_robin"
	StrategyLeastLatency       RoutingStrategy = "least_latency"
	StrategyCostOptimized      RoutingStrategy = "cost_optimized"
	StrategyFailover           RoutingStrategy = "failover"
	StrategyLeastConnections   RoutingStrategy = "least_connections"
	StrategyRandom             RoutingStrategy = "random"
)

// RouterConfig defines router configuration
type RouterConfig struct {
	Strategy                RoutingStrategy     `json:"strategy"`
	HealthCheckInterval     time.Duration       `json:"health_check_interval"`
	MaxRetries              int                 `json:"max_retries"`
	RetryDelay              time.Duration       `json:"retry_delay"`
	CircuitBreakerEnabled   bool                `json:"circuit_breaker_enabled"`
	CircuitBreakerThreshold int                 `json:"circuit_breaker_threshold"`
	CircuitBreakerTimeout   time.Duration       `json:"circuit_breaker_timeout"`
	StickySessions          bool                `json:"sticky_sessions"`
	PreferredProviders      []string            `json:"preferred_providers"`
	ModelAffinity           map[string][]string `json:"model_affinity"` // model -> providers
}

// Router implements intelligent routing for LLM requests
type Router struct {
	config          *RouterConfig
	registry        types.ProviderRegistry
	logger          *utils.Logger
	stats           *RoutingStats
	balancer        LoadBalancer
	circuitBreakers map[string]*CircuitBreaker
	mu              sync.RWMutex
}

// RoutingStats tracks routing statistics
type RoutingStats struct {
	RequestCount   map[string]int64         `json:"request_count"`
	SuccessCount   map[string]int64         `json:"success_count"`
	FailureCount   map[string]int64         `json:"failure_count"`
	LatencyStats   map[string]*LatencyStats `json:"latency_stats"`
	LastUsed       map[string]time.Time     `json:"last_used"`
	TotalRequests  int64                    `json:"total_requests"`
	TotalSuccesses int64                    `json:"total_successes"`
	TotalFailures  int64                    `json:"total_failures"`
	mu             sync.RWMutex
}

// LatencyStats tracks latency metrics
type LatencyStats struct {
	Count   int64           `json:"count"`
	Sum     time.Duration   `json:"sum"`
	Min     time.Duration   `json:"min"`
	Max     time.Duration   `json:"max"`
	Average time.Duration   `json:"average"`
	P95     time.Duration   `json:"p95"`
	P99     time.Duration   `json:"p99"`
	Samples []time.Duration `json:"-"` // Keep last 1000 samples for percentiles
}

// RoutingResult contains the result of provider selection
type RoutingResult struct {
	Provider     types.Provider      `json:"-"`
	ProviderName string              `json:"provider_name"`
	Reason       string              `json:"reason"`
	Attempts     int                 `json:"attempts"`
	BackupUsed   bool                `json:"backup_used"`
	Strategy     RoutingStrategy     `json:"strategy"`
	LoadFactor   float64             `json:"load_factor"`
	Cost         *types.CostEstimate `json:"cost,omitempty"`
}

// NewRouter creates a new intelligent router
func NewRouter(config *RouterConfig, registry types.ProviderRegistry, logger *utils.Logger) *Router {
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 100 * time.Millisecond
	}
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}

	router := &Router{
		config:   config,
		registry: registry,
		logger:   logger,
		stats: &RoutingStats{
			RequestCount: make(map[string]int64),
			SuccessCount: make(map[string]int64),
			FailureCount: make(map[string]int64),
			LatencyStats: make(map[string]*LatencyStats),
			LastUsed:     make(map[string]time.Time),
		},
		circuitBreakers: make(map[string]*CircuitBreaker),
	}

	// Initialize load balancer based on strategy
	switch config.Strategy {
	case StrategyRoundRobin:
		router.balancer = NewRoundRobinBalancer()
	case StrategyWeightedRoundRobin:
		router.balancer = NewWeightedRoundRobinBalancer()
	case StrategyLeastLatency:
		router.balancer = NewLeastLatencyBalancer(router.stats)
	case StrategyCostOptimized:
		router.balancer = NewCostOptimizedBalancer()
	case StrategyLeastConnections:
		router.balancer = NewLeastConnectionsBalancer()
	case StrategyRandom:
		router.balancer = NewRandomBalancer()
	default:
		router.balancer = NewRoundRobinBalancer()
	}

	return router
}

// RouteRequest routes a request to the best available provider
func (r *Router) RouteRequest(ctx context.Context, req *types.ChatCompletionRequest) (*RoutingResult, error) {
	r.mu.RLock()
	strategy := r.config.Strategy
	r.mu.RUnlock()

	r.stats.mu.Lock()
	r.stats.TotalRequests++
	r.stats.mu.Unlock()

	// Get available providers
	providers := r.getAvailableProviders(req.Model)
	if len(providers) == 0 {
		return nil, fmt.Errorf("no available providers for model: %s", req.Model)
	}

	// Apply strategy-specific filtering and sorting
	providers = r.filterProviders(providers, req)

	var lastErr error
	attempts := 0

	for attempts < r.config.MaxRetries && len(providers) > 0 {
		attempts++

		// Select provider using the configured strategy
		provider, err := r.balancer.SelectProvider(providers, req)
		if err != nil {
			lastErr = err
			continue
		}

		// Check circuit breaker
		if r.config.CircuitBreakerEnabled {
			cb := r.getCircuitBreaker(provider.GetName())
			if cb.IsOpen() {
				r.logger.WithField("provider", provider.GetName()).Warn("Circuit breaker is open, skipping provider")
				providers = r.removeProvider(providers, provider)
				continue
			}
		}

		// Calculate cost estimate if needed
		var cost *types.CostEstimate
		if strategy == StrategyCostOptimized {
			cost, _ = provider.EstimateCost(req)
		}

		result := &RoutingResult{
			Provider:     provider,
			ProviderName: provider.GetName(),
			Reason:       r.getSelectionReason(strategy, provider),
			Attempts:     attempts,
			BackupUsed:   attempts > 1,
			Strategy:     strategy,
			Cost:         cost,
		}

		// Update stats
		r.updateRequestStats(provider.GetName())

		return result, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to route request after %d attempts: %w", attempts, lastErr)
	}

	return nil, fmt.Errorf("no suitable provider found after %d attempts", attempts)
}

// RecordSuccess records a successful request
func (r *Router) RecordSuccess(providerName string, latency time.Duration) {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()

	r.stats.SuccessCount[providerName]++
	r.stats.TotalSuccesses++
	r.stats.LastUsed[providerName] = time.Now()

	// Update latency stats
	if r.stats.LatencyStats[providerName] == nil {
		r.stats.LatencyStats[providerName] = &LatencyStats{
			Min:     latency,
			Max:     latency,
			Samples: make([]time.Duration, 0, 1000),
		}
	}

	stats := r.stats.LatencyStats[providerName]
	stats.Count++
	stats.Sum += latency

	if latency < stats.Min {
		stats.Min = latency
	}
	if latency > stats.Max {
		stats.Max = latency
	}

	stats.Average = stats.Sum / time.Duration(stats.Count)

	// Keep last 1000 samples for percentile calculations
	stats.Samples = append(stats.Samples, latency)
	if len(stats.Samples) > 1000 {
		stats.Samples = stats.Samples[1:]
	}

	// Calculate percentiles
	r.calculatePercentiles(stats)

	// Record success in circuit breaker
	if r.config.CircuitBreakerEnabled {
		cb := r.getCircuitBreaker(providerName)
		cb.RecordSuccess()
	}
}

// RecordFailure records a failed request
func (r *Router) RecordFailure(providerName string, err error) {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()

	r.stats.FailureCount[providerName]++
	r.stats.TotalFailures++

	// Record failure in circuit breaker
	if r.config.CircuitBreakerEnabled {
		cb := r.getCircuitBreaker(providerName)
		cb.RecordFailure()
	}

	r.logger.WithField("provider", providerName).
		WithField("error", err.Error()).
		Warn("Provider request failed")
}

// GetStats returns current routing statistics
func (r *Router) GetStats() *RoutingStats {
	r.stats.mu.RLock()
	defer r.stats.mu.RUnlock()

	// Deep copy stats
	stats := &RoutingStats{
		RequestCount:   make(map[string]int64),
		SuccessCount:   make(map[string]int64),
		FailureCount:   make(map[string]int64),
		LatencyStats:   make(map[string]*LatencyStats),
		LastUsed:       make(map[string]time.Time),
		TotalRequests:  r.stats.TotalRequests,
		TotalSuccesses: r.stats.TotalSuccesses,
		TotalFailures:  r.stats.TotalFailures,
	}

	for k, v := range r.stats.RequestCount {
		stats.RequestCount[k] = v
	}
	for k, v := range r.stats.SuccessCount {
		stats.SuccessCount[k] = v
	}
	for k, v := range r.stats.FailureCount {
		stats.FailureCount[k] = v
	}
	for k, v := range r.stats.LatencyStats {
		stats.LatencyStats[k] = &LatencyStats{
			Count:   v.Count,
			Sum:     v.Sum,
			Min:     v.Min,
			Max:     v.Max,
			Average: v.Average,
			P95:     v.P95,
			P99:     v.P99,
		}
	}
	for k, v := range r.stats.LastUsed {
		stats.LastUsed[k] = v
	}

	return stats
}

// UpdateConfig updates router configuration
func (r *Router) UpdateConfig(config *RouterConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = config

	// Reinitialize balancer if strategy changed
	switch config.Strategy {
	case StrategyRoundRobin:
		r.balancer = NewRoundRobinBalancer()
	case StrategyWeightedRoundRobin:
		r.balancer = NewWeightedRoundRobinBalancer()
	case StrategyLeastLatency:
		r.balancer = NewLeastLatencyBalancer(r.stats)
	case StrategyCostOptimized:
		r.balancer = NewCostOptimizedBalancer()
	case StrategyLeastConnections:
		r.balancer = NewLeastConnectionsBalancer()
	case StrategyRandom:
		r.balancer = NewRandomBalancer()
	}
}

// getAvailableProviders returns providers that support the requested model
func (r *Router) getAvailableProviders(model string) []types.Provider {
	allProviders := r.registry.GetHealthyProviders()
	var availableProviders []types.Provider

	// Check model affinity first
	if preferredProviders, exists := r.config.ModelAffinity[model]; exists {
		for _, providerName := range preferredProviders {
			for _, provider := range allProviders {
				if provider.GetName() == providerName {
					availableProviders = append(availableProviders, provider)
					break
				}
			}
		}
		if len(availableProviders) > 0 {
			return availableProviders
		}
	}

	// Fall back to checking if providers support the model
	for _, provider := range allProviders {
		if r.providerSupportsModel(provider, model) {
			availableProviders = append(availableProviders, provider)
		}
	}

	return availableProviders
}

// filterProviders applies additional filtering based on strategy
func (r *Router) filterProviders(providers []types.Provider, req *types.ChatCompletionRequest) []types.Provider {
	// Apply preferred providers filter
	if len(r.config.PreferredProviders) > 0 {
		var filtered []types.Provider
		for _, providerName := range r.config.PreferredProviders {
			for _, provider := range providers {
				if provider.GetName() == providerName {
					filtered = append(filtered, provider)
				}
			}
		}
		if len(filtered) > 0 {
			providers = filtered
		}
	}

	return providers
}

// providerSupportsModel checks if a provider supports a specific model
func (r *Router) providerSupportsModel(provider types.Provider, model string) bool {
	// This is a simplified implementation
	// In reality, you might query the provider's available models
	switch provider.GetType() {
	case "openai":
		return model == "gpt-3.5-turbo" || model == "gpt-4" || model == "gpt-4-turbo-preview"
	case "anthropic":
		return model == "claude-3-opus-20240229" || model == "claude-3-sonnet-20240229" || model == "claude-3-haiku-20240307"
	case "baidu":
		return model == "ernie-bot-4" || model == "ernie-3.5-8k" || model == "ernie-bot-turbo"
	default:
		return true // Assume all providers support all models by default
	}
}

// removeProvider removes a provider from the list
func (r *Router) removeProvider(providers []types.Provider, toRemove types.Provider) []types.Provider {
	for i, provider := range providers {
		if provider.GetName() == toRemove.GetName() {
			return append(providers[:i], providers[i+1:]...)
		}
	}
	return providers
}

// updateRequestStats updates request statistics
func (r *Router) updateRequestStats(providerName string) {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()

	r.stats.RequestCount[providerName]++
}

// getCircuitBreaker gets or creates a circuit breaker for a provider
func (r *Router) getCircuitBreaker(providerName string) *CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cb, exists := r.circuitBreakers[providerName]; exists {
		return cb
	}

	cb := NewCircuitBreaker(r.config.CircuitBreakerThreshold, r.config.CircuitBreakerTimeout)
	r.circuitBreakers[providerName] = cb
	return cb
}

// getSelectionReason returns a human-readable reason for provider selection
func (r *Router) getSelectionReason(strategy RoutingStrategy, provider types.Provider) string {
	switch strategy {
	case StrategyRoundRobin:
		return "Round robin selection"
	case StrategyWeightedRoundRobin:
		return "Weighted round robin selection"
	case StrategyLeastLatency:
		return "Lowest latency provider"
	case StrategyCostOptimized:
		return "Most cost-effective provider"
	case StrategyLeastConnections:
		return "Least active connections"
	case StrategyRandom:
		return "Random selection"
	default:
		return fmt.Sprintf("Selected by %s strategy", strategy)
	}
}

// calculatePercentiles calculates P95 and P99 latencies
func (r *Router) calculatePercentiles(stats *LatencyStats) {
	if len(stats.Samples) == 0 {
		return
	}

	// Sort samples
	samples := make([]time.Duration, len(stats.Samples))
	copy(samples, stats.Samples)
	sort.Slice(samples, func(i, j int) bool {
		return samples[i] < samples[j]
	})

	// Calculate percentiles
	p95Index := int(math.Ceil(0.95*float64(len(samples)))) - 1
	p99Index := int(math.Ceil(0.99*float64(len(samples)))) - 1

	if p95Index >= 0 && p95Index < len(samples) {
		stats.P95 = samples[p95Index]
	}
	if p99Index >= 0 && p99Index < len(samples) {
		stats.P99 = samples[p99Index]
	}
}
