// Package strategies implements load balancing strategies
package strategies

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
)

var (
	ErrNoAvailableProvider = errors.New("no available providers")
)

// RoundRobinStrategy implements round-robin load balancing
type RoundRobinStrategy struct {
	current int64
	metrics *StrategyMetrics
	mutex   sync.Mutex
}

// NewRoundRobinStrategy creates a new round-robin strategy
func NewRoundRobinStrategy() LoadBalanceStrategy {
	return &RoundRobinStrategy{
		current: -1,
		metrics: &StrategyMetrics{
			StrategyName:      "round_robin",
			SelectionCount:    0,
			SelectionLatency:  0,
			DistributionStats: make(map[string]float64),
			LastUsed:          time.Now(),
		},
	}
}

// SelectProvider selects the next provider using round-robin algorithm
func (rr *RoundRobinStrategy) SelectProvider(providers []*types.Provider, request *types.Request) (*types.Provider, error) {
	if len(providers) == 0 {
		return nil, ErrNoAvailableProvider
	}

	start := time.Now()

	// Atomic increment and get index
	index := atomic.AddInt64(&rr.current, 1) % int64(len(providers))
	selected := providers[index]

	// Update metrics
	rr.updateMetrics((*selected).GetName(), time.Since(start))

	return selected, nil
}

// UpdateWeights is a no-op for round-robin (weights not applicable)
func (rr *RoundRobinStrategy) UpdateWeights(weights map[string]int) error {
	// Round-robin doesn't use weights, so this is a no-op
	return nil
}

// GetStrategyName returns the strategy name
func (rr *RoundRobinStrategy) GetStrategyName() string {
	return "round_robin"
}

// GetMetrics returns strategy metrics
func (rr *RoundRobinStrategy) GetMetrics() *StrategyMetrics {
	return rr.metrics
}

// Reset resets the strategy state
func (rr *RoundRobinStrategy) Reset() error {
	atomic.StoreInt64(&rr.current, -1)
	rr.metrics.SelectionCount = 0
	rr.metrics.SelectionLatency = 0
	rr.metrics.DistributionStats = make(map[string]float64)
	rr.metrics.LastUsed = time.Now()
	return nil
}

// updateMetrics updates strategy metrics
func (rr *RoundRobinStrategy) updateMetrics(providerName string, latency time.Duration) {
	atomic.AddInt64(&rr.metrics.SelectionCount, 1)

	// Update average latency
	count := atomic.LoadInt64(&rr.metrics.SelectionCount)
	if count == 1 {
		rr.metrics.SelectionLatency = latency
	} else {
		// Running average calculation
		avgNanos := int64(rr.metrics.SelectionLatency)
		newAvgNanos := (avgNanos*(count-1) + int64(latency)) / count
		rr.metrics.SelectionLatency = time.Duration(newAvgNanos)
	}

	// Update distribution stats (thread-safe)
	rr.mutex.Lock()
	if rr.metrics.DistributionStats == nil {
		rr.metrics.DistributionStats = make(map[string]float64)
	}
	rr.metrics.DistributionStats[providerName]++
	rr.mutex.Unlock()

	rr.metrics.LastUsed = time.Now()
}
