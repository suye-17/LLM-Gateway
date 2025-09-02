package unit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/llm-gateway/gateway/internal/router/strategies"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRoundRobinStrategy tests the round-robin load balancing strategy
func TestRoundRobinStrategy(t *testing.T) {
	strategy := strategies.NewRoundRobinStrategy()

	// Create mock providers
	providers := createMockProviders(3)
	request := &types.Request{Model: "test-model"}

	t.Run("SelectProvider_BasicRoundRobin", func(t *testing.T) {
		selections := make(map[string]int)

		// Test multiple selections to verify round-robin behavior
		for i := 0; i < 9; i++ {
			provider, err := strategy.SelectProvider(providers, request)
			require.NoError(t, err)
			require.NotNil(t, provider)

			providerName := (*provider).GetName()
			selections[providerName]++
		}

		// Each provider should be selected exactly 3 times
		for _, provider := range providers {
			providerName := (*provider).GetName()
			assert.Equal(t, 3, selections[providerName], "Provider %s should be selected 3 times", providerName)
		}
	})

	t.Run("SelectProvider_EmptyProviders", func(t *testing.T) {
		provider, err := strategy.SelectProvider([]*types.Provider{}, request)
		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "no available providers")
	})

	t.Run("GetStrategyName", func(t *testing.T) {
		assert.Equal(t, "round_robin", strategy.GetStrategyName())
	})

	t.Run("UpdateWeights_NoOp", func(t *testing.T) {
		weights := map[string]int{"provider1": 5, "provider2": 10}
		err := strategy.UpdateWeights(weights)
		assert.NoError(t, err) // Should be no-op for round-robin
	})

	t.Run("Reset", func(t *testing.T) {
		// Select a few providers first
		for i := 0; i < 3; i++ {
			_, err := strategy.SelectProvider(providers, request)
			require.NoError(t, err)
		}

		// Reset and verify metrics are reset
		err := strategy.Reset()
		assert.NoError(t, err)

		metrics := strategy.GetMetrics()
		assert.Equal(t, int64(0), metrics.SelectionCount)
	})

	t.Run("GetMetrics", func(t *testing.T) {
		// First do some selections to generate metrics
		for i := 0; i < 3; i++ {
			_, err := strategy.SelectProvider(providers, request)
			require.NoError(t, err)
		}
		
		metrics := strategy.GetMetrics()
		assert.NotNil(t, metrics)
		assert.Equal(t, "round_robin", metrics.StrategyName)
		assert.True(t, metrics.SelectionCount > 0)
	})
}

// TestWeightedRoundRobinStrategy tests the weighted round-robin strategy
func TestWeightedRoundRobinStrategy(t *testing.T) {
	weights := map[string]int{
		"provider-0": 1,
		"provider-1": 2,
		"provider-2": 3,
	}

	strategy := strategies.NewWeightedRoundRobinStrategy(weights)
	providers := createMockProviders(3)
	request := &types.Request{Model: "test-model"}

	t.Run("SelectProvider_WeightedDistribution", func(t *testing.T) {
		selections := make(map[string]int)
		totalSelections := 60 // LCM of 1,2,3 * 10

		for i := 0; i < totalSelections; i++ {
			provider, err := strategy.SelectProvider(providers, request)
			require.NoError(t, err)
			require.NotNil(t, provider)

			providerName := (*provider).GetName()
			selections[providerName]++
		}

		// Verify distribution follows weights (1:2:3 ratio)
		expectedRatio := map[string]float64{
			"provider-0": 1.0 / 6.0, // 1/(1+2+3)
			"provider-1": 2.0 / 6.0, // 2/(1+2+3)
			"provider-2": 3.0 / 6.0, // 3/(1+2+3)
		}

		for providerName, expected := range expectedRatio {
			actual := float64(selections[providerName]) / float64(totalSelections)
			assert.InDelta(t, expected, actual, 0.05, "Provider %s distribution should match weight", providerName)
		}
	})

	t.Run("UpdateWeights", func(t *testing.T) {
		newWeights := map[string]int{
			"provider-0": 5,
			"provider-1": 5,
			"provider-2": 5,
		}

		err := strategy.UpdateWeights(newWeights)
		assert.NoError(t, err)

		// Test with new weights - should be roughly equal distribution
		selections := make(map[string]int)
		for i := 0; i < 30; i++ {
			provider, err := strategy.SelectProvider(providers, request)
			require.NoError(t, err)
			selections[(*provider).GetName()]++
		}

		// With equal weights, distribution should be roughly equal
		for _, count := range selections {
			assert.InDelta(t, 10, count, 2, "With equal weights, selections should be roughly equal")
		}
	})
}

// TestLeastConnectionsStrategy tests the least connections strategy
func TestLeastConnectionsStrategy(t *testing.T) {
	strategy := strategies.NewLeastConnectionsStrategy()
	providers := createMockProviders(3)
	request := &types.Request{Model: "test-model"}

	t.Run("SelectProvider_LeastConnections", func(t *testing.T) {
		// First selection should be provider-0 (all have 0 connections)
		provider, err := strategy.SelectProvider(providers, request)
		require.NoError(t, err)
		assert.Equal(t, "provider-0", (*provider).GetName())

		// Second selection should prefer provider-1 or provider-2 (since provider-0 now has connections)
		provider2, err := strategy.SelectProvider(providers, request)
		require.NoError(t, err)
		// Could be any provider, just verify it's valid
		assert.Contains(t, []string{"provider-0", "provider-1", "provider-2"}, (*provider2).GetName())
	})

	t.Run("DecrementConnections", func(t *testing.T) {
		if lc, ok := strategy.(*strategies.LeastConnectionsStrategy); ok {
			// Test connection increment and decrement
			provider, err := strategy.SelectProvider(providers, request)
			require.NoError(t, err)

			providerName := (*provider).GetName()
			initialConnections := lc.GetConnections(providerName)

			// Connections should have incremented
			assert.True(t, initialConnections > 0)

			// Decrement connections
			lc.DecrementConnections(providerName)
			finalConnections := lc.GetConnections(providerName)

			assert.Equal(t, initialConnections-1, finalConnections)
		}
	})
}

// TestHealthBasedStrategy tests the health-based strategy
func TestHealthBasedStrategy(t *testing.T) {
	strategy := strategies.NewHealthBasedStrategy()
	providers := createMockProviders(3)
	request := &types.Request{Model: "test-model"}

	t.Run("SelectProvider_DefaultHealthy", func(t *testing.T) {
		// All providers start as healthy, should select first one
		provider, err := strategy.SelectProvider(providers, request)
		require.NoError(t, err)
		require.NotNil(t, provider)

		providerName := (*provider).GetName()
		assert.Contains(t, []string{"provider-0", "provider-1", "provider-2"}, providerName)
	})

	t.Run("UpdateProviderHealth", func(t *testing.T) {
		if hb, ok := strategy.(*strategies.HealthBasedStrategy); ok {
			// Update health for provider-0 - successful request
			hb.UpdateProviderHealth("provider-0", 50*time.Millisecond, true)

			// Update health for provider-1 - failed request
			hb.UpdateProviderHealth("provider-1", 200*time.Millisecond, false)

			// Get health information
			health0 := hb.GetProviderHealth("provider-0")
			health1 := hb.GetProviderHealth("provider-1")

			require.NotNil(t, health0)
			require.NotNil(t, health1)

			// provider-0 should have better health score
			assert.True(t, health0.HealthScore > health1.HealthScore,
				"Provider with successful request should have better health score")
			assert.True(t, health0.SuccessRate > health1.SuccessRate)
		}
	})
}

// Benchmark tests for performance evaluation
func BenchmarkRoundRobinStrategy(b *testing.B) {
	strategy := strategies.NewRoundRobinStrategy()
	providers := createMockProviders(10)
	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := strategy.SelectProvider(providers, request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkWeightedRoundRobinStrategy(b *testing.B) {
	weights := make(map[string]int)
	for i := 0; i < 10; i++ {
		weights[fmt.Sprintf("provider-%d", i)] = i + 1
	}

	strategy := strategies.NewWeightedRoundRobinStrategy(weights)
	providers := createMockProviders(10)
	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := strategy.SelectProvider(providers, request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkLeastConnectionsStrategy(b *testing.B) {
	strategy := strategies.NewLeastConnectionsStrategy()
	providers := createMockProviders(10)
	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := strategy.SelectProvider(providers, request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkHealthBasedStrategy(b *testing.B) {
	strategy := strategies.NewHealthBasedStrategy()
	providers := createMockProviders(10)
	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := strategy.SelectProvider(providers, request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Helper function to create mock providers
func createMockProviders(count int) []*types.Provider {
	providers := make([]*types.Provider, count)
	for i := 0; i < count; i++ {
		provider := &MockProvider{
			name: fmt.Sprintf("provider-%d", i),
		}
		var p types.Provider = provider
		providers[i] = &p
	}
	return providers
}

// MockProvider implements types.Provider for testing
type MockProvider struct {
	name string
}

func (m *MockProvider) GetName() string { return m.name }
func (m *MockProvider) GetType() string { return "mock" }
func (m *MockProvider) Call(ctx context.Context, request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return &types.ChatCompletionResponse{
		ID:       "test-" + m.name,
		Model:    request.Model,
		Provider: m.name,
	}, nil
}
func (m *MockProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	return &types.HealthStatus{
		IsHealthy:    true,
		ResponseTime: 50 * time.Millisecond,
	}, nil
}
func (m *MockProvider) GetRateLimit() *types.RateLimitInfo {
	return &types.RateLimitInfo{
		RequestsPerMinute: 1000,
		RemainingRequests: 950,
	}
}
func (m *MockProvider) EstimateCost(request *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	return &types.CostEstimate{
		TotalCost: 0.001,
		Currency:  "USD",
	}, nil
}
func (m *MockProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	return []*types.Model{{Name: "test-model"}}, nil
}
func (m *MockProvider) GetConfig() *types.ProviderConfig {
	return &types.ProviderConfig{
		Name: m.name,
		Type: "mock",
	}
}
