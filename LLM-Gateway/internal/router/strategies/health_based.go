// Package strategies implements load balancing strategies
package strategies

import (
	"sort"
	"sync"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
)

// HealthBasedStrategy implements health-based load balancing
// It considers provider health scores, response times, and success rates
type HealthBasedStrategy struct {
	healthScores map[string]*ProviderHealth
	metrics      *StrategyMetrics
	mutex        sync.RWMutex
}

// ProviderHealth represents health information for a provider
type ProviderHealth struct {
	ResponseTime time.Duration `json:"response_time"`
	SuccessRate  float64       `json:"success_rate"`
	ErrorRate    float64       `json:"error_rate"`
	HealthScore  float64       `json:"health_score"`
	LastUpdate   time.Time     `json:"last_update"`
	RequestCount int64         `json:"request_count"`
	SuccessCount int64         `json:"success_count"`
	IsHealthy    bool          `json:"is_healthy"`
}

// NewHealthBasedStrategy creates a new health-based strategy
func NewHealthBasedStrategy() LoadBalanceStrategy {
	return &HealthBasedStrategy{
		healthScores: make(map[string]*ProviderHealth),
		metrics: &StrategyMetrics{
			StrategyName:      "health_based",
			SelectionCount:    0,
			SelectionLatency:  0,
			DistributionStats: make(map[string]float64),
			LastUsed:          time.Now(),
		},
	}
}

// SelectProvider selects the healthiest provider based on composite health score
func (hb *HealthBasedStrategy) SelectProvider(providers []*types.Provider, request *types.Request) (*types.Provider, error) {
	if len(providers) == 0 {
		return nil, ErrNoAvailableProvider
	}

	start := time.Now()

	hb.mutex.Lock()
	defer hb.mutex.Unlock()

	// Initialize health scores for new providers
	for _, provider := range providers {
		providerName := (*provider).GetName()
		if _, exists := hb.healthScores[providerName]; !exists {
			hb.healthScores[providerName] = &ProviderHealth{
				ResponseTime: 100 * time.Millisecond, // Default response time
				SuccessRate:  1.0,                    // Default to healthy
				ErrorRate:    0.0,
				HealthScore:  1.0,
				LastUpdate:   time.Now(),
				RequestCount: 0,
				SuccessCount: 0,
				IsHealthy:    true,
			}
		}
	}

	// Calculate current health scores and find the best provider
	var candidates []providerCandidate

	for _, provider := range providers {
		providerName := (*provider).GetName()
		health := hb.healthScores[providerName]

		// Update health score based on recent metrics
		healthScore := hb.calculateHealthScore(health)
		health.HealthScore = healthScore
		health.LastUpdate = time.Now()

		candidates = append(candidates, providerCandidate{
			provider:    provider,
			healthScore: healthScore,
			name:        providerName,
		})
	}

	// Sort by health score (descending - higher is better)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].healthScore > candidates[j].healthScore
	})

	// Select the healthiest provider
	selected := candidates[0].provider
	selectedName := (*selected).GetName()

	// Update metrics
	hb.updateMetrics(selectedName, time.Since(start))

	return selected, nil
}

// providerCandidate represents a provider candidate with its health score
type providerCandidate struct {
	provider    *types.Provider
	healthScore float64
	name        string
}

// UpdateProviderHealth updates health information for a provider
func (hb *HealthBasedStrategy) UpdateProviderHealth(providerName string, responseTime time.Duration, success bool) {
	hb.mutex.Lock()
	defer hb.mutex.Unlock()

	health, exists := hb.healthScores[providerName]
	if !exists {
		health = &ProviderHealth{
			ResponseTime: responseTime,
			SuccessRate:  0.0,
			ErrorRate:    0.0,
			HealthScore:  0.0,
			LastUpdate:   time.Now(),
			RequestCount: 0,
			SuccessCount: 0,
			IsHealthy:    true,
		}
		hb.healthScores[providerName] = health
	}

	// Update counters
	health.RequestCount++
	if success {
		health.SuccessCount++
	}

	// Update success and error rates
	health.SuccessRate = float64(health.SuccessCount) / float64(health.RequestCount)
	health.ErrorRate = 1.0 - health.SuccessRate

	// Update response time with exponential moving average
	alpha := 0.3 // Smoothing factor
	health.ResponseTime = time.Duration(float64(health.ResponseTime)*(1-alpha) + float64(responseTime)*alpha)

	// Update health status
	health.IsHealthy = health.SuccessRate >= 0.8 && health.ResponseTime < 5*time.Second

	// Recalculate health score
	health.HealthScore = hb.calculateHealthScore(health)
	
	health.LastUpdate = time.Now()
}

// calculateHealthScore computes a composite health score (0.0 to 1.0)
func (hb *HealthBasedStrategy) calculateHealthScore(health *ProviderHealth) float64 {
	// Weight factors for different metrics
	const (
		successRateWeight  = 0.5 // 50% weight on success rate
		responseTimeWeight = 0.3 // 30% weight on response time
		freshnessWeight    = 0.2 // 20% weight on data freshness
	)

	// Success rate component (0.0 to 1.0)
	successComponent := health.SuccessRate

	// Response time component (lower is better, normalize to 0.0-1.0)
	// Assume 5 seconds is the worst acceptable response time
	maxAcceptableTime := 5 * time.Second
	responseComponent := 1.0
	if health.ResponseTime > 0 {
		responseComponent = 1.0 - float64(health.ResponseTime)/float64(maxAcceptableTime)
		if responseComponent < 0 {
			responseComponent = 0
		}
	}

	// Freshness component (data age affects reliability)
	freshnessComponent := 1.0
	dataAge := time.Since(health.LastUpdate)
	if dataAge > time.Minute {
		// Decay factor after 1 minute
		freshnessComponent = 1.0 / (1.0 + float64(dataAge)/float64(time.Minute))
	}

	// Combine components
	healthScore := successComponent*successRateWeight +
		responseComponent*responseTimeWeight +
		freshnessComponent*freshnessWeight

	// Ensure score is between 0.0 and 1.0
	if healthScore < 0 {
		healthScore = 0
	}
	if healthScore > 1 {
		healthScore = 1
	}

	return healthScore
}

// UpdateWeights is a no-op for health-based (weights not applicable)
func (hb *HealthBasedStrategy) UpdateWeights(weights map[string]int) error {
	// Health-based strategy doesn't use weights, so this is a no-op
	return nil
}

// GetStrategyName returns the strategy name
func (hb *HealthBasedStrategy) GetStrategyName() string {
	return "health_based"
}

// GetMetrics returns strategy metrics
func (hb *HealthBasedStrategy) GetMetrics() *StrategyMetrics {
	hb.mutex.RLock()
	defer hb.mutex.RUnlock()

	// Add health scores to distribution stats
	for provider, health := range hb.healthScores {
		hb.metrics.DistributionStats[provider+"_health_score"] = health.HealthScore
		hb.metrics.DistributionStats[provider+"_success_rate"] = health.SuccessRate
		hb.metrics.DistributionStats[provider+"_response_time_ms"] = float64(health.ResponseTime / time.Millisecond)
	}

	return hb.metrics
}

// Reset resets the strategy state
func (hb *HealthBasedStrategy) Reset() error {
	hb.mutex.Lock()
	defer hb.mutex.Unlock()

	hb.healthScores = make(map[string]*ProviderHealth)
	hb.metrics.SelectionCount = 0
	hb.metrics.SelectionLatency = 0
	hb.metrics.DistributionStats = make(map[string]float64)
	hb.metrics.LastUsed = time.Now()

	return nil
}

// GetProviderHealth returns health information for a specific provider
func (hb *HealthBasedStrategy) GetProviderHealth(providerName string) *ProviderHealth {
	hb.mutex.RLock()
	defer hb.mutex.RUnlock()

	if health, exists := hb.healthScores[providerName]; exists {
		// Return a copy to avoid race conditions
		return &ProviderHealth{
			ResponseTime: health.ResponseTime,
			SuccessRate:  health.SuccessRate,
			ErrorRate:    health.ErrorRate,
			HealthScore:  health.HealthScore,
			LastUpdate:   health.LastUpdate,
			RequestCount: health.RequestCount,
			SuccessCount: health.SuccessCount,
			IsHealthy:    health.IsHealthy,
		}
	}
	return nil
}

// updateMetrics updates strategy metrics
func (hb *HealthBasedStrategy) updateMetrics(providerName string, latency time.Duration) {
	hb.metrics.SelectionCount++

	// Update average latency
	if hb.metrics.SelectionCount == 1 {
		hb.metrics.SelectionLatency = latency
	} else {
		// Running average calculation
		count := hb.metrics.SelectionCount
		avgNanos := int64(hb.metrics.SelectionLatency)
		newAvgNanos := (avgNanos*(count-1) + int64(latency)) / count
		hb.metrics.SelectionLatency = time.Duration(newAvgNanos)
	}

	// Update distribution stats
	if hb.metrics.DistributionStats == nil {
		hb.metrics.DistributionStats = make(map[string]float64)
	}
	hb.metrics.DistributionStats[providerName]++

	hb.metrics.LastUsed = time.Now()
}
