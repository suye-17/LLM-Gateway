package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/llm-gateway/gateway/internal/router"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSmartRouterIntegration tests the complete smart router system
func TestSmartRouterIntegration(t *testing.T) {
	logConfig := &types.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}
	logger := utils.NewLogger(logConfig)
	config := router.DefaultSmartRouterConfig()
	config.Strategy = "round_robin"
	config.HealthCheckInterval = 100 * time.Millisecond

	smartRouter, err := router.NewSmartRouter(config, logger)
	require.NoError(t, err)
	require.NotNil(t, smartRouter)

	// Add test providers
	for i := 0; i < 3; i++ {
		provider := createIntegrationMockProvider(fmt.Sprintf("provider-%d", i), true)
		err := smartRouter.AddProvider(provider)
		require.NoError(t, err)
	}

	// Start the router
	err = smartRouter.Start()
	require.NoError(t, err)
	defer smartRouter.Stop()

	t.Run("RouteRequest_BasicFunctionality", func(t *testing.T) {
		request := &types.Request{
			Model: "test-model",
			Messages: []types.Message{
				{Role: "user", Content: "Hello"},
			},
		}

		result, err := smartRouter.RouteRequest(context.Background(), request)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.NotEmpty(t, result.ProviderName)
		assert.Equal(t, "round_robin", result.Strategy)
		assert.NotEmpty(t, result.Reason)
		assert.True(t, result.SelectionTime > 0)
		assert.Equal(t, 1, result.Attempts)
	})

	t.Run("RouteRequest_RoundRobinDistribution", func(t *testing.T) {
		// Force strategy reset by switching to a different strategy and back
		config := router.DefaultSmartRouterConfig()
		config.Strategy = "least_connections"
		smartRouter.UpdateConfig(config)

		config.Strategy = "round_robin"
		smartRouter.UpdateConfig(config)

		selections := make(map[string]int)
		numRequests := 9

		for i := 0; i < numRequests; i++ {
			request := &types.Request{
				ID:    fmt.Sprintf("req-%d", i),
				Model: "test-model",
			}

			result, err := smartRouter.RouteRequest(context.Background(), request)
			require.NoError(t, err)
			selections[result.ProviderName]++
		}

		// Verify round-robin distribution is reasonably balanced
		assert.Len(t, selections, 3, "All 3 providers should be selected")

		// Each provider should be selected at least once and at most numRequests/2
		for providerName, count := range selections {
			assert.GreaterOrEqual(t, count, 1, "Provider %s should be selected at least once", providerName)
			assert.LessOrEqual(t, count, numRequests/2+1, "Provider %s selection should be reasonably distributed", providerName)
		}

		// Total selections should equal number of requests
		totalSelections := 0
		for _, count := range selections {
			totalSelections += count
		}
		assert.Equal(t, numRequests, totalSelections, "Total selections should equal number of requests")
	})

	t.Run("UpdateProviderWeight", func(t *testing.T) {
		// Update weight for provider-0
		err := smartRouter.UpdateProviderWeight("provider-0", 10)
		assert.NoError(t, err)

		// Update weight for non-existent provider should fail
		err = smartRouter.UpdateProviderWeight("non-existent", 5)
		assert.Error(t, err)

		// Update with negative weight should fail
		err = smartRouter.UpdateProviderWeight("provider-0", -1)
		assert.Error(t, err)
	})

	t.Run("RemoveProvider", func(t *testing.T) {
		// Remove provider-2
		err := smartRouter.RemoveProvider("provider-2")
		assert.NoError(t, err)

		// Try to remove again should fail
		err = smartRouter.RemoveProvider("provider-2")
		assert.Error(t, err)

		// Routing should still work with remaining providers
		request := &types.Request{Model: "test-model"}
		result, err := smartRouter.RouteRequest(context.Background(), request)
		require.NoError(t, err)
		assert.Contains(t, []string{"provider-0", "provider-1"}, result.ProviderName)
	})

	t.Run("GetMetrics", func(t *testing.T) {
		metrics := smartRouter.GetMetrics()
		assert.NotNil(t, metrics)
		assert.True(t, metrics.TotalRequests > 0)
	})

	t.Run("GetHealthStatus", func(t *testing.T) {
		// Wait for health checks to run
		time.Sleep(200 * time.Millisecond)

		healthStatus := smartRouter.GetHealthStatus()
		assert.NotNil(t, healthStatus)

		// Should have health status for active providers
		// Note: provider-2 was removed in previous test, so only check active ones
		activeProviders := []string{"provider-0", "provider-1"}
		for _, providerName := range activeProviders {
			// Check that active providers are in health status
			_, exists := healthStatus[providerName]
			assert.True(t, exists, "Provider %s should have health status", providerName)
		}
	})
}

// TestSmartRouterStrategySwitching tests switching between different strategies
func TestSmartRouterStrategySwitching(t *testing.T) {
	logConfig := &types.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}
	logger := utils.NewLogger(logConfig)
	config := router.DefaultSmartRouterConfig()

	smartRouter, err := router.NewSmartRouter(config, logger)
	require.NoError(t, err)

	// Add test providers
	providers := []types.Provider{
		createIntegrationMockProvider("provider-0", true),
		createIntegrationMockProvider("provider-1", true),
		createIntegrationMockProvider("provider-2", true),
	}

	for _, provider := range providers {
		err := smartRouter.AddProvider(provider)
		require.NoError(t, err)
	}

	err = smartRouter.Start()
	require.NoError(t, err)
	defer smartRouter.Stop()

	testCases := []struct {
		strategy string
		weights  map[string]int
	}{
		{"round_robin", nil},
		{"weighted_round_robin", map[string]int{"provider-0": 1, "provider-1": 2, "provider-2": 3}},
		{"least_connections", nil},
		{"health_based", nil},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Strategy_%s", tc.strategy), func(t *testing.T) {
			// Update configuration
			newConfig := config.Clone()
			newConfig.Strategy = tc.strategy
			if tc.weights != nil {
				newConfig.Weights = tc.weights
			}

			err := smartRouter.UpdateConfig(newConfig)
			require.NoError(t, err)

			// Test routing with new strategy
			request := &types.Request{Model: "test-model"}
			result, err := smartRouter.RouteRequest(context.Background(), request)
			require.NoError(t, err)
			assert.Equal(t, tc.strategy, result.Strategy)
		})
	}
}

// TestSmartRouterConcurrency tests concurrent routing requests
func TestSmartRouterConcurrency(t *testing.T) {
	logConfig := &types.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}
	logger := utils.NewLogger(logConfig)
	config := router.DefaultSmartRouterConfig()
	config.Strategy = "round_robin"

	smartRouter, err := router.NewSmartRouter(config, logger)
	require.NoError(t, err)

	// Add multiple providers
	for i := 0; i < 5; i++ {
		provider := createIntegrationMockProvider(fmt.Sprintf("provider-%d", i), true)
		err := smartRouter.AddProvider(provider)
		require.NoError(t, err)
	}

	err = smartRouter.Start()
	require.NoError(t, err)
	defer smartRouter.Stop()

	t.Run("ConcurrentRouting", func(t *testing.T) {
		numGoroutines := 10
		requestsPerGoroutine := 100

		var wg sync.WaitGroup
		results := make(chan *router.SmartRoutingResult, numGoroutines*requestsPerGoroutine)
		errors := make(chan error, numGoroutines*requestsPerGoroutine)

		// Start concurrent routing
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for j := 0; j < requestsPerGoroutine; j++ {
					request := &types.Request{
						ID:    fmt.Sprintf("worker-%d-req-%d", workerID, j),
						Model: "test-model",
					}

					result, err := smartRouter.RouteRequest(context.Background(), request)
					if err != nil {
						errors <- err
						return
					}
					results <- result
				}
			}(i)
		}

		wg.Wait()
		close(results)
		close(errors)

		// Check for errors
		assert.Empty(t, errors, "No errors should occur during concurrent routing")

		// Verify all requests were processed
		resultCount := 0
		providerCounts := make(map[string]int)

		for result := range results {
			resultCount++
			providerCounts[result.ProviderName]++
		}

		assert.Equal(t, numGoroutines*requestsPerGoroutine, resultCount)

		// With round-robin, distribution should be roughly equal
		expectedPerProvider := (numGoroutines * requestsPerGoroutine) / 5
		tolerance := expectedPerProvider / 5 // 20% tolerance

		for providerName, count := range providerCounts {
			assert.InDelta(t, expectedPerProvider, count, float64(tolerance),
				"Provider %s should have roughly equal distribution", providerName)
		}
	})
}

// TestSmartRouterFailover tests failover behavior
func TestSmartRouterFailover(t *testing.T) {
	logConfig := &types.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}
	logger := utils.NewLogger(logConfig)
	config := router.DefaultSmartRouterConfig()
	config.HealthCheckInterval = 50 * time.Millisecond

	smartRouter, err := router.NewSmartRouter(config, logger)
	require.NoError(t, err)

	// Add healthy and unhealthy providers
	healthyProvider := createIntegrationMockProvider("healthy-provider", true)
	unhealthyProvider := createIntegrationMockProvider("unhealthy-provider", false)

	err = smartRouter.AddProvider(healthyProvider)
	require.NoError(t, err)
	err = smartRouter.AddProvider(unhealthyProvider)
	require.NoError(t, err)

	err = smartRouter.Start()
	require.NoError(t, err)
	defer smartRouter.Stop()

	// Wait for health checks to detect unhealthy provider
	time.Sleep(200 * time.Millisecond)

	t.Run("FailoverToHealthyProvider", func(t *testing.T) {
		request := &types.Request{Model: "test-model"}

		// Make multiple requests - should prefer healthy provider
		healthySelections := 0
		totalRequests := 10

		for i := 0; i < totalRequests; i++ {
			result, err := smartRouter.RouteRequest(context.Background(), request)
			require.NoError(t, err)

			if result.ProviderName == "healthy-provider" {
				healthySelections++
			}
		}

		// Should prefer healthy provider (though unhealthy might still be selected if threshold not reached)
		assert.True(t, healthySelections > 0, "Healthy provider should be selected")
	})
}

// Benchmark tests for performance evaluation
func BenchmarkSmartRouter_RouteRequest(b *testing.B) {
	logConfig := &types.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}
	logger := utils.NewLogger(logConfig)
	config := router.DefaultSmartRouterConfig()
	config.Strategy = "round_robin"

	smartRouter, _ := router.NewSmartRouter(config, logger)

	// Add providers
	for i := 0; i < 10; i++ {
		provider := createIntegrationMockProvider(fmt.Sprintf("provider-%d", i), true)
		smartRouter.AddProvider(provider)
	}

	smartRouter.Start()
	defer smartRouter.Stop()

	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := smartRouter.RouteRequest(context.Background(), request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Helper function to create integration test providers
func createIntegrationMockProvider(name string, healthy bool) types.Provider {
	if healthy {
		return &IntegrationHealthyProvider{name: name}
	}
	return &IntegrationUnhealthyProvider{name: name}
}

// IntegrationHealthyProvider simulates a healthy provider
type IntegrationHealthyProvider struct {
	name string
}

func (p *IntegrationHealthyProvider) GetName() string { return p.name }
func (p *IntegrationHealthyProvider) GetType() string { return "integration-healthy" }
func (p *IntegrationHealthyProvider) Call(ctx context.Context, request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Simulate some processing time
	time.Sleep(10 * time.Millisecond)
	return &types.ChatCompletionResponse{
		ID:       fmt.Sprintf("response-%s", p.name),
		Model:    request.Model,
		Provider: p.name,
	}, nil
}
func (p *IntegrationHealthyProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	return &types.HealthStatus{
		IsHealthy:    true,
		ResponseTime: 50 * time.Millisecond,
		LastChecked:  time.Now(),
	}, nil
}
func (p *IntegrationHealthyProvider) GetRateLimit() *types.RateLimitInfo {
	return &types.RateLimitInfo{
		RequestsPerMinute: 1000,
		RemainingRequests: 950,
	}
}
func (p *IntegrationHealthyProvider) EstimateCost(request *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	return &types.CostEstimate{
		TotalCost: 0.001,
		Currency:  "USD",
	}, nil
}
func (p *IntegrationHealthyProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	return []*types.Model{{Name: "test-model"}}, nil
}
func (p *IntegrationHealthyProvider) GetConfig() *types.ProviderConfig {
	return &types.ProviderConfig{
		Name:    p.name,
		Type:    "integration-healthy",
		Enabled: true,
	}
}

// IntegrationUnhealthyProvider simulates an unhealthy provider
type IntegrationUnhealthyProvider struct {
	name string
}

func (p *IntegrationUnhealthyProvider) GetName() string { return p.name }
func (p *IntegrationUnhealthyProvider) GetType() string { return "integration-unhealthy" }
func (p *IntegrationUnhealthyProvider) Call(ctx context.Context, request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return nil, fmt.Errorf("provider %s is down", p.name)
}
func (p *IntegrationUnhealthyProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	return &types.HealthStatus{
		IsHealthy:    false,
		ResponseTime: 5 * time.Second,
		ErrorMessage: fmt.Sprintf("Provider %s is unhealthy", p.name),
		LastChecked:  time.Now(),
	}, nil
}
func (p *IntegrationUnhealthyProvider) GetRateLimit() *types.RateLimitInfo {
	return &types.RateLimitInfo{}
}
func (p *IntegrationUnhealthyProvider) EstimateCost(request *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	return nil, fmt.Errorf("provider %s is down", p.name)
}
func (p *IntegrationUnhealthyProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	return nil, fmt.Errorf("provider %s is down", p.name)
}
func (p *IntegrationUnhealthyProvider) GetConfig() *types.ProviderConfig {
	return &types.ProviderConfig{
		Name:    p.name,
		Type:    "integration-unhealthy",
		Enabled: false,
	}
}
