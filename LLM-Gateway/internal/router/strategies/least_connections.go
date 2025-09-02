// Package strategies implements load balancing strategies
package strategies

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
)

// LeastConnectionsStrategy implements least connections load balancing
type LeastConnectionsStrategy struct {
	connections map[string]*int64 // Provider name -> active connections
	metrics     *StrategyMetrics
	mutex       sync.RWMutex
}

// NewLeastConnectionsStrategy creates a new least connections strategy
func NewLeastConnectionsStrategy() LoadBalanceStrategy {
	return &LeastConnectionsStrategy{
		connections: make(map[string]*int64),
		metrics: &StrategyMetrics{
			StrategyName:      "least_connections",
			SelectionCount:    0,
			SelectionLatency:  0,
			DistributionStats: make(map[string]float64),
			LastUsed:          time.Now(),
		},
	}
}

// SelectProvider selects the provider with the least active connections
func (lc *LeastConnectionsStrategy) SelectProvider(providers []*types.Provider, request *types.Request) (*types.Provider, error) {
	if len(providers) == 0 {
		return nil, ErrNoAvailableProvider
	}

	start := time.Now()

	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	var selected *types.Provider
	var minConnections int64 = math.MaxInt64

	// Initialize connections for new providers
	for _, provider := range providers {
		providerName := (*provider).GetName()
		if _, exists := lc.connections[providerName]; !exists {
			var zero int64 = 0
			lc.connections[providerName] = &zero
		}
	}

	// Find provider with minimum connections
	for _, provider := range providers {
		providerName := (*provider).GetName()
		connections := atomic.LoadInt64(lc.connections[providerName])

		if connections < minConnections {
			minConnections = connections
			selected = provider
		}
	}

	// Increment connection count for selected provider
	if selected != nil {
		selectedName := (*selected).GetName()
		atomic.AddInt64(lc.connections[selectedName], 1)

		// Update metrics
		lc.updateMetrics(selectedName, time.Since(start))
	}

	return selected, nil
}

// DecrementConnections decrements the connection count for a provider
// This should be called when a request completes
func (lc *LeastConnectionsStrategy) DecrementConnections(providerName string) {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	if connPtr, exists := lc.connections[providerName]; exists {
		current := atomic.LoadInt64(connPtr)
		if current > 0 {
			atomic.AddInt64(connPtr, -1)
		}
	}
}

// GetConnections returns the current connection count for a provider
func (lc *LeastConnectionsStrategy) GetConnections(providerName string) int64 {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	if connPtr, exists := lc.connections[providerName]; exists {
		return atomic.LoadInt64(connPtr)
	}
	return 0
}

// UpdateWeights is a no-op for least connections (weights not applicable)
func (lc *LeastConnectionsStrategy) UpdateWeights(weights map[string]int) error {
	// Least connections doesn't use weights, so this is a no-op
	return nil
}

// GetStrategyName returns the strategy name
func (lc *LeastConnectionsStrategy) GetStrategyName() string {
	return "least_connections"
}

// GetMetrics returns strategy metrics
func (lc *LeastConnectionsStrategy) GetMetrics() *StrategyMetrics {
	// Add current connection distribution to metrics
	lc.mutex.RLock()
	connectionStats := make(map[string]float64)
	for provider, connPtr := range lc.connections {
		connectionStats[provider+"_connections"] = float64(atomic.LoadInt64(connPtr))
	}
	lc.mutex.RUnlock()

	// Merge with existing distribution stats
	for k, v := range connectionStats {
		lc.metrics.DistributionStats[k] = v
	}

	return lc.metrics
}

// Reset resets the strategy state
func (lc *LeastConnectionsStrategy) Reset() error {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()

	// Reset all connection counts
	for provider, connPtr := range lc.connections {
		atomic.StoreInt64(connPtr, 0)
		delete(lc.connections, provider)
	}
	lc.connections = make(map[string]*int64)

	lc.metrics.SelectionCount = 0
	lc.metrics.SelectionLatency = 0
	lc.metrics.DistributionStats = make(map[string]float64)
	lc.metrics.LastUsed = time.Now()

	return nil
}

// updateMetrics updates strategy metrics
func (lc *LeastConnectionsStrategy) updateMetrics(providerName string, latency time.Duration) {
	lc.metrics.SelectionCount++

	// Update average latency
	if lc.metrics.SelectionCount == 1 {
		lc.metrics.SelectionLatency = latency
	} else {
		// Running average calculation
		count := lc.metrics.SelectionCount
		avgNanos := int64(lc.metrics.SelectionLatency)
		newAvgNanos := (avgNanos*(count-1) + int64(latency)) / count
		lc.metrics.SelectionLatency = time.Duration(newAvgNanos)
	}

	// Update distribution stats
	if lc.metrics.DistributionStats == nil {
		lc.metrics.DistributionStats = make(map[string]float64)
	}
	lc.metrics.DistributionStats[providerName]++

	lc.metrics.LastUsed = time.Now()
}
