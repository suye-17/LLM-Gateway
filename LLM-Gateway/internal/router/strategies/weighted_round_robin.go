// Package strategies implements load balancing strategies
package strategies

import (
	"sync"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
)

// WeightedRoundRobinStrategy implements weighted round-robin load balancing
type WeightedRoundRobinStrategy struct {
	weights       map[string]int
	currentWeight map[string]int64
	metrics       *StrategyMetrics
	mutex         sync.RWMutex
}

// NewWeightedRoundRobinStrategy creates a new weighted round-robin strategy
func NewWeightedRoundRobinStrategy(weights map[string]int) LoadBalanceStrategy {
	wrr := &WeightedRoundRobinStrategy{
		weights:       make(map[string]int),
		currentWeight: make(map[string]int64),
		metrics: &StrategyMetrics{
			StrategyName:      "weighted_round_robin",
			SelectionCount:    0,
			SelectionLatency:  0,
			DistributionStats: make(map[string]float64),
			LastUsed:          time.Now(),
		},
	}

	if weights != nil {
		wrr.UpdateWeights(weights)
	}

	return wrr
}

// SelectProvider selects a provider using weighted round-robin algorithm
func (wrr *WeightedRoundRobinStrategy) SelectProvider(providers []*types.Provider, request *types.Request) (*types.Provider, error) {
	if len(providers) == 0 {
		return nil, ErrNoAvailableProvider
	}

	start := time.Now()

	wrr.mutex.Lock()
	defer wrr.mutex.Unlock()

	var selected *types.Provider
	var maxCurrentWeight int64 = -1

	// Initialize weights for new providers
	for _, provider := range providers {
		providerName := (*provider).GetName()
		if _, exists := wrr.weights[providerName]; !exists {
			wrr.weights[providerName] = 1 // Default weight
		}
		if _, exists := wrr.currentWeight[providerName]; !exists {
			wrr.currentWeight[providerName] = 0
		}
	}

	totalWeight := wrr.getTotalWeight(providers)
	if totalWeight == 0 {
		// If all weights are 0, fall back to round-robin
		return providers[0], nil
	}

	// Weighted round-robin algorithm
	for _, provider := range providers {
		providerName := (*provider).GetName()
		weight := int64(wrr.weights[providerName])

		// Increase current weight
		wrr.currentWeight[providerName] += weight

		// Select provider with maximum current weight
		if wrr.currentWeight[providerName] > maxCurrentWeight {
			maxCurrentWeight = wrr.currentWeight[providerName]
			selected = provider
		}
	}

	// Decrease selected provider's current weight
	if selected != nil {
		selectedName := (*selected).GetName()
		wrr.currentWeight[selectedName] -= int64(totalWeight)

		// Update metrics
		wrr.updateMetrics(selectedName, time.Since(start))
	}

	return selected, nil
}

// UpdateWeights updates provider weights
func (wrr *WeightedRoundRobinStrategy) UpdateWeights(weights map[string]int) error {
	wrr.mutex.Lock()
	defer wrr.mutex.Unlock()

	// Clear existing weights
	wrr.weights = make(map[string]int)
	wrr.currentWeight = make(map[string]int64)

	// Copy new weights
	for provider, weight := range weights {
		if weight < 0 {
			weight = 0
		}
		wrr.weights[provider] = weight
		wrr.currentWeight[provider] = 0
	}

	return nil
}

// GetStrategyName returns the strategy name
func (wrr *WeightedRoundRobinStrategy) GetStrategyName() string {
	return "weighted_round_robin"
}

// GetMetrics returns strategy metrics
func (wrr *WeightedRoundRobinStrategy) GetMetrics() *StrategyMetrics {
	return wrr.metrics
}

// Reset resets the strategy state
func (wrr *WeightedRoundRobinStrategy) Reset() error {
	wrr.mutex.Lock()
	defer wrr.mutex.Unlock()

	for provider := range wrr.currentWeight {
		wrr.currentWeight[provider] = 0
	}

	wrr.metrics.SelectionCount = 0
	wrr.metrics.SelectionLatency = 0
	wrr.metrics.DistributionStats = make(map[string]float64)
	wrr.metrics.LastUsed = time.Now()

	return nil
}

// getTotalWeight calculates total weight for available providers
func (wrr *WeightedRoundRobinStrategy) getTotalWeight(providers []*types.Provider) int {
	total := 0
	for _, provider := range providers {
		providerName := (*provider).GetName()
		if weight, exists := wrr.weights[providerName]; exists {
			total += weight
		}
	}
	return total
}

// updateMetrics updates strategy metrics
func (wrr *WeightedRoundRobinStrategy) updateMetrics(providerName string, latency time.Duration) {
	wrr.metrics.SelectionCount++

	// Update average latency
	if wrr.metrics.SelectionCount == 1 {
		wrr.metrics.SelectionLatency = latency
	} else {
		// Running average calculation
		count := wrr.metrics.SelectionCount
		avgNanos := int64(wrr.metrics.SelectionLatency)
		newAvgNanos := (avgNanos*(count-1) + int64(latency)) / count
		wrr.metrics.SelectionLatency = time.Duration(newAvgNanos)
	}

	// Update distribution stats
	if wrr.metrics.DistributionStats == nil {
		wrr.metrics.DistributionStats = make(map[string]float64)
	}
	wrr.metrics.DistributionStats[providerName]++

	wrr.metrics.LastUsed = time.Now()
}
