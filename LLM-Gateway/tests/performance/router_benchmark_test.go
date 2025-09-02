package performance

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/llm-gateway/gateway/internal/router"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
	"github.com/stretchr/testify/require"
)

// Type alias for convenience
type SmartRouterConfig = router.SmartRouterConfig

// MockLogger implements utils.Logger interface for testing
type MockLogger struct{}

func (m *MockLogger) Info(msg string) {}
func (m *MockLogger) Error(msg string) {}
func (m *MockLogger) Warn(msg string) {}
func (m *MockLogger) Debug(msg string) {}

// BenchmarkSmartRouter_RouteRequest_RoundRobin benchmarks round-robin routing
func BenchmarkSmartRouter_RouteRequest_RoundRobin(b *testing.B) {
	router := setupBenchmarkRouter("round_robin", 10)
	defer router.Stop()

	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := router.RouteRequest(context.Background(), request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSmartRouter_RouteRequest_WeightedRoundRobin benchmarks weighted round-robin routing
func BenchmarkSmartRouter_RouteRequest_WeightedRoundRobin(b *testing.B) {
	router := setupBenchmarkRouter("weighted_round_robin", 10)
	defer router.Stop()

	// Set weights
	weights := make(map[string]int)
	for i := 0; i < 10; i++ {
		weights[fmt.Sprintf("provider-%d", i)] = i + 1
	}
	config := &SmartRouterConfig{
		Strategy: "weighted_round_robin",
		Weights:  weights,
	}
	router.UpdateConfig(config)

	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := router.RouteRequest(context.Background(), request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSmartRouter_RouteRequest_LeastConnections benchmarks least connections routing
func BenchmarkSmartRouter_RouteRequest_LeastConnections(b *testing.B) {
	router := setupBenchmarkRouter("least_connections", 10)
	defer router.Stop()

	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := router.RouteRequest(context.Background(), request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSmartRouter_RouteRequest_HealthBased benchmarks health-based routing
func BenchmarkSmartRouter_RouteRequest_HealthBased(b *testing.B) {
	router := setupBenchmarkRouter("health_based", 10)
	defer router.Stop()

	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := router.RouteRequest(context.Background(), request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSmartRouter_ProviderScalability tests performance with different numbers of providers
func BenchmarkSmartRouter_ProviderScalability(b *testing.B) {
	providerCounts := []int{1, 5, 10, 25, 50, 100}

	for _, count := range providerCounts {
		b.Run(fmt.Sprintf("Providers_%d", count), func(b *testing.B) {
			router := setupBenchmarkRouter("round_robin", count)
			defer router.Stop()

			request := &types.Request{Model: "test-model"}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := router.RouteRequest(context.Background(), request)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkSmartRouter_ConcurrencyScalability tests performance with different concurrency levels
func BenchmarkSmartRouter_ConcurrencyScalability(b *testing.B) {
	router := setupBenchmarkRouter("round_robin", 10)
	defer router.Stop()

	request := &types.Request{Model: "test-model"}

	concurrencyLevels := []int{1, 2, 4, 8, 16, 32}

	for _, level := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", level), func(b *testing.B) {
			b.SetParallelism(level)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := router.RouteRequest(context.Background(), request)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkSmartRouter_MemoryAllocation measures memory allocation during routing
func BenchmarkSmartRouter_MemoryAllocation(b *testing.B) {
	router := setupBenchmarkRouter("round_robin", 10)
	defer router.Stop()

	request := &types.Request{Model: "test-model"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := router.RouteRequest(context.Background(), request)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// StressTestSmartRouter performs stress testing with high load
func TestSmartRouter_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	router := setupBenchmarkRouter("round_robin", 20)
	defer router.Stop()

	// Stress test parameters
	duration := 30 * time.Second
	numWorkers := 50
	requestsPerSecond := 1000

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var (
		totalRequests int64
		totalErrors   int64
		wg            sync.WaitGroup
	)

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			ticker := time.NewTicker(time.Duration(1000000000/requestsPerSecond/numWorkers) * time.Nanosecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					request := &types.Request{
						ID:    fmt.Sprintf("stress-worker-%d-%d", workerID, time.Now().UnixNano()),
						Model: "test-model",
					}

					_, err := router.RouteRequest(context.Background(), request)
					if err != nil {
						totalErrors++
					}
					totalRequests++
				}
			}
		}(i)
	}

	// Monitor resources
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&m)

				t.Logf("Memory: Alloc=%d KB, TotalAlloc=%d KB, Sys=%d KB, NumGC=%d",
					m.Alloc/1024, m.TotalAlloc/1024, m.Sys/1024, m.NumGC)
				t.Logf("Goroutines: %d", runtime.NumGoroutine())
			}
		}
	}()

	wg.Wait()

	// Report results
	actualDuration := time.Since(time.Now().Add(-duration))
	requestsPerSecondActual := float64(totalRequests) / actualDuration.Seconds()
	errorRate := float64(totalErrors) / float64(totalRequests) * 100

	t.Logf("Stress test completed:")
	t.Logf("Duration: %v", actualDuration)
	t.Logf("Total requests: %d", totalRequests)
	t.Logf("Total errors: %d", totalErrors)
	t.Logf("Requests/sec: %.2f", requestsPerSecondActual)
	t.Logf("Error rate: %.2f%%", errorRate)

	// Assertions
	require.True(t, totalRequests > 0, "Should have processed requests")
	require.True(t, errorRate < 1.0, "Error rate should be less than 1%")
	require.True(t, requestsPerSecondActual > 500, "Should handle at least 500 requests/sec")
}

// TestSmartRouter_MemoryLeak tests for memory leaks
func TestSmartRouter_MemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	router := setupBenchmarkRouter("round_robin", 10)
	defer router.Stop()

	// Measure initial memory
	runtime.GC()
	var initialMemStats runtime.MemStats
	runtime.ReadMemStats(&initialMemStats)

	// Perform many routing operations
	request := &types.Request{Model: "test-model"}
	for i := 0; i < 100000; i++ {
		_, err := router.RouteRequest(context.Background(), request)
		require.NoError(t, err)

		// Occasionally force GC
		if i%10000 == 0 {
			runtime.GC()
		}
	}

	// Measure final memory
	runtime.GC()
	time.Sleep(100 * time.Millisecond) // Allow finalizers to run
	runtime.GC()

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)

	// Check for memory growth
	memoryGrowth := finalMemStats.Alloc - initialMemStats.Alloc
	memoryGrowthMB := float64(memoryGrowth) / 1024 / 1024

	t.Logf("Initial memory: %d KB", initialMemStats.Alloc/1024)
	t.Logf("Final memory: %d KB", finalMemStats.Alloc/1024)
	t.Logf("Memory growth: %.2f MB", memoryGrowthMB)

	// Assert memory growth is reasonable (less than 10MB)
	require.True(t, memoryGrowthMB < 10.0,
		"Memory growth should be less than 10MB, actual: %.2f MB", memoryGrowthMB)
}

// TestSmartRouter_LatencyUnderLoad tests latency under high load
func TestSmartRouter_LatencyUnderLoad(t *testing.T) {
	router := setupBenchmarkRouter("round_robin", 10)
	defer router.Stop()

	numRequests := 1000
	latencies := make([]time.Duration, numRequests)

	// Measure latencies
	for i := 0; i < numRequests; i++ {
		request := &types.Request{
			ID:    fmt.Sprintf("latency-test-%d", i),
			Model: "test-model",
		}

		start := time.Now()
		_, err := router.RouteRequest(context.Background(), request)
		latency := time.Since(start)

		require.NoError(t, err)
		latencies[i] = latency
	}

	// Calculate percentiles
	latenciesNanos := make([]int64, len(latencies))
	for i, latency := range latencies {
		latenciesNanos[i] = latency.Nanoseconds()
	}

	// Sort for percentile calculation
	for i := 0; i < len(latenciesNanos); i++ {
		for j := i + 1; j < len(latenciesNanos); j++ {
			if latenciesNanos[i] > latenciesNanos[j] {
				latenciesNanos[i], latenciesNanos[j] = latenciesNanos[j], latenciesNanos[i]
			}
		}
	}

	p50 := time.Duration(latenciesNanos[len(latenciesNanos)*50/100])
	p95 := time.Duration(latenciesNanos[len(latenciesNanos)*95/100])
	p99 := time.Duration(latenciesNanos[len(latenciesNanos)*99/100])

	t.Logf("Latency P50: %v", p50)
	t.Logf("Latency P95: %v", p95)
	t.Logf("Latency P99: %v", p99)

	// Assert latency requirements (from ALIGNMENT document)
	require.True(t, p95 < 1*time.Millisecond,
		"P95 latency should be less than 1ms, actual: %v", p95)
	require.True(t, p99 < 5*time.Millisecond,
		"P99 latency should be less than 5ms, actual: %v", p99)
}

// Helper function to setup benchmark router
func setupBenchmarkRouter(strategy string, numProviders int) *router.SmartRouter {
	logConfig := &types.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}
	logger := utils.NewLogger(logConfig)
	config := router.DefaultSmartRouterConfig()
	config.Strategy = strategy
	config.HealthCheckInterval = 1 * time.Second // Less frequent for benchmarks

	smartRouter, _ := router.NewSmartRouter(config, logger)

	// Add providers
	for i := 0; i < numProviders; i++ {
		provider := createBenchmarkProvider(fmt.Sprintf("provider-%d", i))
		smartRouter.AddProvider(provider)
	}

	smartRouter.Start()
	return smartRouter
}

func createBenchmarkProvider(name string) types.Provider {
	return &BenchmarkProvider{name: name}
}

// BenchmarkProvider is optimized for performance testing
type BenchmarkProvider struct {
	name string
}

func (p *BenchmarkProvider) GetName() string { return p.name }
func (p *BenchmarkProvider) GetType() string { return "benchmark" }
func (p *BenchmarkProvider) Call(ctx context.Context, request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Minimal processing for performance testing
	return &types.ChatCompletionResponse{
		ID:       "bench-" + p.name,
		Model:    request.Model,
		Provider: p.name,
	}, nil
}
func (p *BenchmarkProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	return &types.HealthStatus{
		IsHealthy:    true,
		ResponseTime: 1 * time.Millisecond,
		LastChecked:  time.Now(),
	}, nil
}
func (p *BenchmarkProvider) GetRateLimit() *types.RateLimitInfo {
	return &types.RateLimitInfo{
		RequestsPerMinute: 10000,
		RemainingRequests: 9999,
	}
}
func (p *BenchmarkProvider) EstimateCost(request *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	return &types.CostEstimate{TotalCost: 0.001, Currency: "USD"}, nil
}
func (p *BenchmarkProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	return []*types.Model{{Name: "bench-model"}}, nil
}
func (p *BenchmarkProvider) GetConfig() *types.ProviderConfig {
	return &types.ProviderConfig{Name: p.name, Type: "benchmark", Enabled: true}
}
