// Package router implements metrics collection functionality
package router

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/llm-gateway/gateway/internal/router/strategies"
)

// DefaultMetricsCollector implements the MetricsCollector interface
type DefaultMetricsCollector struct {
	routingMetrics  *RoutingMetrics
	strategyMetrics map[string]*strategies.StrategyMetrics
	providerMetrics map[string]*ProviderMetrics
	mutex           sync.RWMutex

	// Internal counters
	totalRequests    int64
	successfulRoutes int64
	failedRoutes     int64
	totalLatency     int64 // in nanoseconds for average calculation
	startTime        time.Time
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() (MetricsCollector, error) {
	collector := &DefaultMetricsCollector{
		routingMetrics: &RoutingMetrics{
			TotalRequests:    0,
			SuccessfulRoutes: 0,
			FailedRoutes:     0,
			AverageLatency:   0,
			RoutingLatency:   0,
			StartTime:        time.Now(),
		},
		strategyMetrics:  make(map[string]*strategies.StrategyMetrics),
		providerMetrics:  make(map[string]*ProviderMetrics),
		totalRequests:    0,
		successfulRoutes: 0,
		failedRoutes:     0,
		totalLatency:     0,
		startTime:        time.Now(),
	}

	return collector, nil
}

// RecordRouting records routing decision metrics
func (mc *DefaultMetricsCollector) RecordRouting(providerID string, latency time.Duration, success bool) {
	// Update atomic counters
	atomic.AddInt64(&mc.totalRequests, 1)
	atomic.AddInt64(&mc.totalLatency, int64(latency))

	if success {
		atomic.AddInt64(&mc.successfulRoutes, 1)
	} else {
		atomic.AddInt64(&mc.failedRoutes, 1)
	}

	// Update routing metrics
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	mc.routingMetrics.TotalRequests = atomic.LoadInt64(&mc.totalRequests)
	mc.routingMetrics.SuccessfulRoutes = atomic.LoadInt64(&mc.successfulRoutes)
	mc.routingMetrics.FailedRoutes = atomic.LoadInt64(&mc.failedRoutes)

	// Calculate average latency
	totalLatencyNanos := atomic.LoadInt64(&mc.totalLatency)
	if mc.routingMetrics.TotalRequests > 0 {
		mc.routingMetrics.AverageLatency = time.Duration(totalLatencyNanos / mc.routingMetrics.TotalRequests)
	}

	// Also record provider-specific metrics if provider ID is provided
	if providerID != "" {
		mc.recordProviderMetricsInternal(providerID, latency, success)
	}
}

// RecordStrategy records strategy selection metrics
func (mc *DefaultMetricsCollector) RecordStrategy(strategyName string, latency time.Duration) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	strategy, exists := mc.strategyMetrics[strategyName]
	if !exists {
		strategy = &strategies.StrategyMetrics{
			StrategyName:      strategyName,
			SelectionCount:    0,
			SelectionLatency:  0,
			DistributionStats: make(map[string]float64),
			LastUsed:          time.Now(),
		}
		mc.strategyMetrics[strategyName] = strategy
	}

	strategy.SelectionCount++

	// Update average latency
	if strategy.SelectionCount == 1 {
		strategy.SelectionLatency = latency
	} else {
		// Running average calculation
		avgNanos := int64(strategy.SelectionLatency)
		newAvgNanos := (avgNanos*(strategy.SelectionCount-1) + int64(latency)) / strategy.SelectionCount
		strategy.SelectionLatency = time.Duration(newAvgNanos)
	}

	strategy.LastUsed = time.Now()
}

// RecordProvider records provider-specific metrics
func (mc *DefaultMetricsCollector) RecordProvider(providerID string, latency time.Duration, success bool) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	mc.recordProviderMetricsInternal(providerID, latency, success)
}

// recordProviderMetricsInternal is the internal implementation for recording provider metrics
func (mc *DefaultMetricsCollector) recordProviderMetricsInternal(providerID string, latency time.Duration, success bool) {
	provider, exists := mc.providerMetrics[providerID]
	if !exists {
		provider = &ProviderMetrics{
			ProviderID:        providerID,
			RequestCount:      0,
			SuccessCount:      0,
			FailureCount:      0,
			AverageLatency:    0,
			SuccessRate:       0.0,
			ActiveConnections: 0,
			LastUsed:          time.Now(),
		}
		mc.providerMetrics[providerID] = provider
	}

	provider.RequestCount++
	if success {
		provider.SuccessCount++
	} else {
		provider.FailureCount++
	}

	// Update average latency
	if provider.RequestCount == 1 {
		provider.AverageLatency = latency
	} else {
		// Running average calculation
		avgNanos := int64(provider.AverageLatency)
		newAvgNanos := (avgNanos*(provider.RequestCount-1) + int64(latency)) / provider.RequestCount
		provider.AverageLatency = time.Duration(newAvgNanos)
	}

	// Update success rate
	provider.SuccessRate = float64(provider.SuccessCount) / float64(provider.RequestCount)

	provider.LastUsed = time.Now()
}

// GetRoutingMetrics returns overall routing metrics
func (mc *DefaultMetricsCollector) GetRoutingMetrics() *RoutingMetrics {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	// Return a copy to avoid race conditions
	return &RoutingMetrics{
		TotalRequests:    mc.routingMetrics.TotalRequests,
		SuccessfulRoutes: mc.routingMetrics.SuccessfulRoutes,
		FailedRoutes:     mc.routingMetrics.FailedRoutes,
		AverageLatency:   mc.routingMetrics.AverageLatency,
		RoutingLatency:   mc.routingMetrics.RoutingLatency,
		StartTime:        mc.routingMetrics.StartTime,
	}
}

// GetStrategyMetrics returns strategy-specific metrics
func (mc *DefaultMetricsCollector) GetStrategyMetrics() map[string]*strategies.StrategyMetrics {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	metrics := make(map[string]*strategies.StrategyMetrics)

	for name, strategy := range mc.strategyMetrics {
		// Create a copy of distribution stats
		distributionCopy := make(map[string]float64)
		for k, v := range strategy.DistributionStats {
			distributionCopy[k] = v
		}

		metrics[name] = &strategies.StrategyMetrics{
			StrategyName:      strategy.StrategyName,
			SelectionCount:    strategy.SelectionCount,
			SelectionLatency:  strategy.SelectionLatency,
			DistributionStats: distributionCopy,
			LastUsed:          strategy.LastUsed,
		}
	}

	return metrics
}

// GetProviderMetrics returns provider-specific metrics
func (mc *DefaultMetricsCollector) GetProviderMetrics() map[string]*ProviderMetrics {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	metrics := make(map[string]*ProviderMetrics)

	for id, provider := range mc.providerMetrics {
		metrics[id] = &ProviderMetrics{
			ProviderID:        provider.ProviderID,
			RequestCount:      provider.RequestCount,
			SuccessCount:      provider.SuccessCount,
			FailureCount:      provider.FailureCount,
			AverageLatency:    provider.AverageLatency,
			SuccessRate:       provider.SuccessRate,
			ActiveConnections: provider.ActiveConnections,
			LastUsed:          provider.LastUsed,
		}
	}

	return metrics
}

// IncrementActiveConnections increments the active connection count for a provider
func (mc *DefaultMetricsCollector) IncrementActiveConnections(providerID string) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	provider, exists := mc.providerMetrics[providerID]
	if !exists {
		provider = &ProviderMetrics{
			ProviderID:        providerID,
			RequestCount:      0,
			SuccessCount:      0,
			FailureCount:      0,
			AverageLatency:    0,
			SuccessRate:       0.0,
			ActiveConnections: 0,
			LastUsed:          time.Now(),
		}
		mc.providerMetrics[providerID] = provider
	}

	provider.ActiveConnections++
}

// DecrementActiveConnections decrements the active connection count for a provider
func (mc *DefaultMetricsCollector) DecrementActiveConnections(providerID string) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	if provider, exists := mc.providerMetrics[providerID]; exists {
		if provider.ActiveConnections > 0 {
			provider.ActiveConnections--
		}
	}
}

// Reset resets all metrics
func (mc *DefaultMetricsCollector) Reset() error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	// Reset atomic counters
	atomic.StoreInt64(&mc.totalRequests, 0)
	atomic.StoreInt64(&mc.successfulRoutes, 0)
	atomic.StoreInt64(&mc.failedRoutes, 0)
	atomic.StoreInt64(&mc.totalLatency, 0)

	// Reset routing metrics
	mc.routingMetrics = &RoutingMetrics{
		TotalRequests:    0,
		SuccessfulRoutes: 0,
		FailedRoutes:     0,
		AverageLatency:   0,
		RoutingLatency:   0,
		StartTime:        time.Now(),
	}

	// Reset strategy metrics
	mc.strategyMetrics = make(map[string]*strategies.StrategyMetrics)

	// Reset provider metrics
	mc.providerMetrics = make(map[string]*ProviderMetrics)

	mc.startTime = time.Now()

	return nil
}

// GetUptime returns the uptime since the collector was created or last reset
func (mc *DefaultMetricsCollector) GetUptime() time.Duration {
	return time.Since(mc.startTime)
}

// GetMetricsSummary returns a summary of all metrics
func (mc *DefaultMetricsCollector) GetMetricsSummary() map[string]interface{} {
	routing := mc.GetRoutingMetrics()
	strategies := mc.GetStrategyMetrics()
	providers := mc.GetProviderMetrics()

	summary := map[string]interface{}{
		"routing":    routing,
		"strategies": strategies,
		"providers":  providers,
		"uptime":     mc.GetUptime(),
		"timestamp":  time.Now(),
	}

	return summary
}
