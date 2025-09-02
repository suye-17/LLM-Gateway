// Package strategies defines interfaces for load balancing strategies
package strategies

import (
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
)

// LoadBalanceStrategy defines the interface for load balancing strategies
type LoadBalanceStrategy interface {
	// SelectProvider selects the best provider from the available list
	SelectProvider(providers []*types.Provider, request *types.Request) (*types.Provider, error)

	// UpdateWeights updates the weights for weighted strategies
	UpdateWeights(weights map[string]int) error

	// GetStrategyName returns the name of the strategy
	GetStrategyName() string

	// GetMetrics returns strategy-specific metrics
	GetMetrics() *StrategyMetrics

	// Reset resets the strategy state
	Reset() error
}

// StrategyMetrics represents metrics for a specific strategy
type StrategyMetrics struct {
	StrategyName      string             `json:"strategy_name"`
	SelectionCount    int64              `json:"selection_count"`
	SelectionLatency  time.Duration      `json:"selection_latency"`
	DistributionStats map[string]float64 `json:"distribution_stats"`
	LastUsed          time.Time          `json:"last_used"`
}
