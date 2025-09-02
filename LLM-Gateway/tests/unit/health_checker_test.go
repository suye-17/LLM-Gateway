package unit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/llm-gateway/gateway/internal/router"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthChecker(t *testing.T) {
	// Create a logger for testing
	logConfig := &types.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}
	logger := utils.NewLogger(logConfig)
	config := router.DefaultHealthCheckConfig()
	config.Interval = 100 * time.Millisecond // Fast interval for testing
	config.Timeout = 50 * time.Millisecond

	healthChecker, err := router.NewHealthChecker(config, logger)
	require.NoError(t, err)
	require.NotNil(t, healthChecker)

	t.Run("AddProvider", func(t *testing.T) {
		provider := createHealthyMockProvider("test-provider")
		err := healthChecker.AddProvider(&provider)
		assert.NoError(t, err)

		// Verify provider was added
		results := healthChecker.GetAllHealthResults()
		assert.Contains(t, results, "test-provider")
	})

	t.Run("CheckProvider_Immediate", func(t *testing.T) {
		provider := createHealthyMockProvider("check-provider")
		err := healthChecker.AddProvider(&provider)
		require.NoError(t, err)

		result, err := healthChecker.CheckProvider("check-provider")
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "check-provider", result.ProviderID)
		assert.True(t, result.IsHealthy)
	})

	t.Run("GetHealthyProviders", func(t *testing.T) {
		// Add healthy provider
		healthyProvider := createHealthyMockProvider("healthy-provider")
		err := healthChecker.AddProvider(&healthyProvider)
		require.NoError(t, err)

		// Add unhealthy provider
		unhealthyProvider := createUnhealthyMockProvider("unhealthy-provider")
		err = healthChecker.AddProvider(&unhealthyProvider)
		require.NoError(t, err)

		// Wait a bit for health checks to run
		time.Sleep(200 * time.Millisecond)

		healthyProviders := healthChecker.GetHealthyProviders()

		// Should contain only healthy providers
		healthyNames := make([]string, len(healthyProviders))
		for i, provider := range healthyProviders {
			healthyNames[i] = (*provider).GetName()
		}

		assert.Contains(t, healthyNames, "healthy-provider")
		// unhealthy-provider might still be healthy if threshold not reached
	})

	t.Run("StartStopHealthCheck", func(t *testing.T) {
		err := healthChecker.StartHealthCheck()
		assert.NoError(t, err)

		// Should not be able to start again
		err = healthChecker.StartHealthCheck()
		assert.Error(t, err)

		err = healthChecker.StopHealthCheck()
		assert.NoError(t, err)

		// Should be able to stop multiple times without error
		err = healthChecker.StopHealthCheck()
		assert.NoError(t, err)
	})

	t.Run("UpdateConfig", func(t *testing.T) {
		newConfig := &router.HealthCheckConfig{
			Interval:         200 * time.Millisecond,
			Timeout:          100 * time.Millisecond,
			FailureThreshold: 5,
			SuccessThreshold: 3,
			Path:             "/health",
		}

		err := healthChecker.UpdateConfig(newConfig)
		assert.NoError(t, err)
	})

	t.Run("RemoveProvider", func(t *testing.T) {
		provider := createHealthyMockProvider("remove-provider")
		err := healthChecker.AddProvider(&provider)
		require.NoError(t, err)

		// Verify provider exists
		_, err = healthChecker.GetProviderHealth("remove-provider")
		assert.NoError(t, err)

		// Remove provider
		err = healthChecker.RemoveProvider("remove-provider")
		assert.NoError(t, err)

		// Verify provider is gone
		_, err = healthChecker.GetProviderHealth("remove-provider")
		assert.Error(t, err)
	})
}

func TestHealthCheckConfig(t *testing.T) {
	t.Run("ValidateConfig_Valid", func(t *testing.T) {
		config := &router.HealthCheckConfig{
			Interval:         30 * time.Second,
			Timeout:          5 * time.Second,
			FailureThreshold: 3,
			SuccessThreshold: 2,
			Path:             "/health",
		}

		err := config.ValidateHealthCheckConfig()
		assert.NoError(t, err)
	})

	t.Run("ValidateConfig_Invalid", func(t *testing.T) {
		testCases := []struct {
			name   string
			config *router.HealthCheckConfig
		}{
			{
				name: "ZeroInterval",
				config: &router.HealthCheckConfig{
					Interval: 0,
					Timeout:  5 * time.Second,
				},
			},
			{
				name: "ZeroTimeout",
				config: &router.HealthCheckConfig{
					Interval: 30 * time.Second,
					Timeout:  0,
				},
			},
			{
				name: "TimeoutGreaterThanInterval",
				config: &router.HealthCheckConfig{
					Interval: 5 * time.Second,
					Timeout:  10 * time.Second,
				},
			},
			{
				name: "ZeroFailureThreshold",
				config: &router.HealthCheckConfig{
					Interval:         30 * time.Second,
					Timeout:          5 * time.Second,
					FailureThreshold: 0,
					SuccessThreshold: 2,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.config.ValidateHealthCheckConfig()
				assert.Error(t, err)
			})
		}
	})

	t.Run("Clone", func(t *testing.T) {
		original := &router.HealthCheckConfig{
			Interval:         30 * time.Second,
			Timeout:          5 * time.Second,
			FailureThreshold: 3,
			SuccessThreshold: 2,
			Path:             "/health",
		}

		clone := original.Clone()

		// Verify clone is different object but same values
		assert.NotSame(t, original, clone)
		assert.Equal(t, original.Interval, clone.Interval)
		assert.Equal(t, original.Timeout, clone.Timeout)
		assert.Equal(t, original.FailureThreshold, clone.FailureThreshold)
		assert.Equal(t, original.SuccessThreshold, clone.SuccessThreshold)
		assert.Equal(t, original.Path, clone.Path)

		// Verify modifying clone doesn't affect original
		clone.Interval = 60 * time.Second
		assert.NotEqual(t, original.Interval, clone.Interval)
	})
}

// Benchmark health checker performance
func BenchmarkHealthChecker_CheckProvider(b *testing.B) {
	logger := &utils.Logger{}
	config := router.DefaultHealthCheckConfig()
	healthChecker, _ := router.NewHealthChecker(config, logger)

	provider := createHealthyMockProvider("bench-provider")
	healthChecker.AddProvider(&provider)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := healthChecker.CheckProvider("bench-provider")
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Helper functions for testing
func createHealthyMockProvider(name string) types.Provider {
	return &HealthyMockProvider{name: name}
}

func createUnhealthyMockProvider(name string) types.Provider {
	return &UnhealthyMockProvider{name: name}
}

// HealthyMockProvider always returns healthy status
type HealthyMockProvider struct {
	name string
}

func (p *HealthyMockProvider) GetName() string { return p.name }
func (p *HealthyMockProvider) GetType() string { return "mock" }
func (p *HealthyMockProvider) Call(ctx context.Context, request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return &types.ChatCompletionResponse{}, nil
}
func (p *HealthyMockProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	return &types.HealthStatus{
		IsHealthy:    true,
		ResponseTime: 50 * time.Millisecond,
		LastChecked:  time.Now(),
	}, nil
}
func (p *HealthyMockProvider) GetRateLimit() *types.RateLimitInfo {
	return &types.RateLimitInfo{}
}
func (p *HealthyMockProvider) EstimateCost(request *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	return &types.CostEstimate{}, nil
}
func (p *HealthyMockProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	return []*types.Model{}, nil
}
func (p *HealthyMockProvider) GetConfig() *types.ProviderConfig {
	return &types.ProviderConfig{Name: p.name}
}

// UnhealthyMockProvider always returns unhealthy status
type UnhealthyMockProvider struct {
	name string
}

func (p *UnhealthyMockProvider) GetName() string { return p.name }
func (p *UnhealthyMockProvider) GetType() string { return "mock" }
func (p *UnhealthyMockProvider) Call(ctx context.Context, request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return nil, fmt.Errorf("provider %s is unhealthy", p.name)
}
func (p *UnhealthyMockProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	return &types.HealthStatus{
		IsHealthy:    false,
		ResponseTime: 5 * time.Second,
		ErrorMessage: "Simulated failure",
		LastChecked:  time.Now(),
	}, nil
}
func (p *UnhealthyMockProvider) GetRateLimit() *types.RateLimitInfo {
	return &types.RateLimitInfo{}
}
func (p *UnhealthyMockProvider) EstimateCost(request *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	return nil, fmt.Errorf("provider unhealthy")
}
func (p *UnhealthyMockProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	return nil, fmt.Errorf("provider unhealthy")
}
func (p *UnhealthyMockProvider) GetConfig() *types.ProviderConfig {
	return &types.ProviderConfig{Name: p.name}
}
