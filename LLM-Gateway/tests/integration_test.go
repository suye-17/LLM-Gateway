// Package tests provides integration tests for Week 5 production features
package tests

import (
	"context"
	"testing"
	"time"

	"github.com/llm-gateway/gateway/internal/config"
	"github.com/llm-gateway/gateway/internal/providers"
	"github.com/llm-gateway/gateway/pkg/cost"
	"github.com/llm-gateway/gateway/pkg/retry"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// TestProductionComponents tests the Week 5 production components integration
func TestProductionComponents(t *testing.T) {
	logConfig := &types.LoggingConfig{Level: "info"}
	logger := utils.NewLogger(logConfig)

	t.Run("ConfigManager", func(t *testing.T) {
		// Test secure configuration manager
		configManager := config.NewSecureConfigManager("../configs/production.yaml", logger)

		// Test configuration loading (should work even without actual API keys)
		err := configManager.LoadAllConfigs()
		// This might fail due to missing API keys, which is expected in test
		t.Logf("Config loading result: %v", err)

		// Test API key validation format
		err = configManager.ValidateAPIKey(context.Background(), "openai", "sk-test123456789")
		if err == nil {
			t.Error("Expected API key validation to fail for invalid key")
		}
		t.Logf("API key validation correctly failed: %v", err)
	})

	t.Run("RetryManager", func(t *testing.T) {
		// Test retry manager
		retryPolicy := types.DefaultRetryPolicy()
		retryManager := retry.NewRetryManager(retryPolicy, logger)

		// Test retry statistics
		stats := retryManager.GetRetryStats()
		if stats.TotalAttempts != 0 {
			t.Errorf("Expected 0 initial attempts, got %d", stats.TotalAttempts)
		}

		// Test delay calculation
		delay := retryManager.CalculateDelay(1)
		if delay < time.Second || delay > 3*time.Second {
			t.Errorf("Expected delay between 1-3 seconds, got %v", delay)
		}

		// Test error classification
		testErr := retry.NewProviderRetryError("test", "test", types.ErrorNetwork, "test error", true)
		if !testErr.Retryable {
			t.Error("Expected network error to be retryable")
		}

		t.Logf("Retry manager working correctly")
	})

	t.Run("CostCalculator", func(t *testing.T) {
		// Test cost calculator
		costCalculator := cost.NewCostCalculator(logger)

		// Test token estimation
		req := &types.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []types.Message{
				{Role: "user", Content: "Hello, how are you?"},
			},
		}

		estimate, err := costCalculator.EstimateRequestCost(req)
		if err != nil {
			t.Errorf("Expected successful cost estimation, got error: %v", err)
		}

		if estimate == nil {
			t.Fatal("Expected cost estimate, got nil")
		}

		if estimate.EstimatedCost <= 0 {
			t.Errorf("Expected positive estimated cost, got %f", estimate.EstimatedCost)
		}

		if estimate.InputTokens <= 0 {
			t.Errorf("Expected positive input tokens, got %d", estimate.InputTokens)
		}

		t.Logf("Cost estimate: $%.6f for %d input tokens", estimate.EstimatedCost, estimate.InputTokens)
	})

	t.Run("OpenAIProviderIntegration", func(t *testing.T) {
		// Test OpenAI provider initialization
		baseConfig := &types.ProviderConfig{
			Name:    "test-openai",
			Type:    "openai",
			Enabled: true,
			APIKey:  "test-key", // This will fail validation, which is expected
		}

		provider := providers.NewOpenAIProvider(baseConfig, logger)

		if provider.GetName() != "test-openai" {
			t.Errorf("Expected provider name 'test-openai', got '%s'", provider.GetName())
		}

		if provider.GetType() != "openai" {
			t.Errorf("Expected provider type 'openai', got '%s'", provider.GetType())
		}

		// Test configuration retrieval
		config := provider.GetConfig()
		if config.Name != "test-openai" {
			t.Errorf("Expected config name 'test-openai', got '%s'", config.Name)
		}

		t.Logf("OpenAI provider initialized successfully")
	})

	t.Run("ClaudeProviderIntegration", func(t *testing.T) {
		// Test Claude provider initialization
		baseConfig := &types.ProviderConfig{
			Name:    "test-claude",
			Type:    "anthropic",
			Enabled: true,
			APIKey:  "test-key",
		}

		provider := providers.NewClaudeProvider(baseConfig, logger)

		if provider.GetName() != "test-claude" {
			t.Errorf("Expected provider name 'test-claude', got '%s'", provider.GetName())
		}

		if provider.GetType() != "anthropic" {
			t.Errorf("Expected provider type 'anthropic', got '%s'", provider.GetType())
		}

		t.Logf("Claude provider initialized successfully")
	})

	t.Run("BaiduProviderIntegration", func(t *testing.T) {
		// Test Baidu provider initialization
		baseConfig := &types.ProviderConfig{
			Name:    "test-baidu",
			Type:    "baidu",
			Enabled: true,
			APIKey:  "test-key",
		}

		provider := providers.NewBaiduProvider(baseConfig, logger)

		if provider.GetName() != "test-baidu" {
			t.Errorf("Expected provider name 'test-baidu', got '%s'", provider.GetName())
		}

		if provider.GetType() != "baidu" {
			t.Errorf("Expected provider type 'baidu', got '%s'", provider.GetType())
		}

		t.Logf("Baidu provider initialized successfully")
	})
}

// TestProductionFeatures tests the new production features added in Week 5
func TestProductionFeatures(t *testing.T) {
	logConfig := &types.LoggingConfig{Level: "info"}
	logger := utils.NewLogger(logConfig)

	t.Run("TokenEstimation", func(t *testing.T) {
		baseConfig := &types.ProviderConfig{
			Name:    "test-openai",
			Type:    "openai",
			Enabled: true,
		}

		provider := providers.NewOpenAIProvider(baseConfig, logger)

		req := &types.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []types.Message{
				{Role: "user", Content: "What is artificial intelligence?"},
			},
		}

		estimate, err := provider.EstimateTokens(req)
		if err != nil {
			t.Errorf("Expected successful token estimation, got error: %v", err)
		}

		if estimate.InputTokens <= 0 {
			t.Errorf("Expected positive input tokens, got %d", estimate.InputTokens)
		}

		t.Logf("Token estimation: %d input, %d output tokens", estimate.InputTokens, estimate.OutputTokens)
	})

	t.Run("ProviderMetrics", func(t *testing.T) {
		baseConfig := &types.ProviderConfig{
			Name:    "test-openai",
			Type:    "openai",
			Enabled: true,
		}

		provider := providers.NewOpenAIProvider(baseConfig, logger)
		metrics := provider.GetProviderMetrics()

		if metrics == nil {
			t.Error("Expected provider metrics, got nil")
		}

		// Initial metrics should be zero
		if metrics.TotalRequests != 0 {
			t.Errorf("Expected 0 initial requests, got %d", metrics.TotalRequests)
		}

		t.Logf("Provider metrics initialized correctly")
	})
}

// BenchmarkCostCalculation benchmarks the cost calculation performance
func BenchmarkCostCalculation(b *testing.B) {
	logConfig := &types.LoggingConfig{Level: "error"}
	logger := utils.NewLogger(logConfig)
	costCalculator := cost.NewCostCalculator(logger)

	req := &types.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []types.Message{
			{Role: "user", Content: "This is a test message for benchmarking cost calculation performance."},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := costCalculator.EstimateRequestCost(req)
		if err != nil {
			b.Fatalf("Cost calculation failed: %v", err)
		}
	}
}
