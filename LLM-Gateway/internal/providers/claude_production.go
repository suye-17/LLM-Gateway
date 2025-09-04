// Package providers implements the Claude (Anthropic) provider adapter - Production features
package providers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/llm-gateway/gateway/internal/config"
	"github.com/llm-gateway/gateway/pkg/cost"
	"github.com/llm-gateway/gateway/pkg/retry"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// NewProductionClaudeProvider creates a new Anthropic Claude provider for production
func NewProductionClaudeProvider(configPath string, logger *utils.Logger) (*ClaudeProvider, error) {
	// Create configuration manager
	configManager := config.NewSecureConfigManager(configPath, logger)

	// Load production configuration
	prodConfig, err := configManager.LoadProviderConfig("anthropic")
	if err != nil {
		return nil, fmt.Errorf("failed to load Claude configuration: %w", err)
	}

	// Load secure credentials
	secureConfig, err := configManager.GetSecureConfig("anthropic")
	if err != nil {
		return nil, fmt.Errorf("failed to load secure Claude credentials: %w", err)
	}

	// Validate configuration
	if err := prodConfig.Validate(); err != nil {
		return nil, fmt.Errorf("Claude configuration validation failed: %w", err)
	}

	// Validate credentials
	if err := configManager.ValidateAPIKey(context.Background(), "anthropic", secureConfig.APIKey); err != nil {
		return nil, fmt.Errorf("Claude API key validation failed: %w", err)
	}

	// Create components
	retryManager := retry.NewRetryManager(prodConfig.RetryPolicy, logger)
	costCalculator := cost.NewCostCalculator(logger)

	provider := &ClaudeProvider{
		config:         prodConfig,
		secureConfig:   secureConfig,
		logger:         logger,
		configManager:  configManager,
		retryManager:   retryManager,
		costCalculator: costCalculator,
		httpClient: &http.Client{
			Timeout: prodConfig.Timeout,
		},
		rateLimits: &types.RateLimitInfo{
			RequestsPerMinute: prodConfig.RateLimit,
			ResetTime:         time.Now().Add(time.Minute),
		},
	}

	logger.WithField("provider", "anthropic").Info("Production Claude provider initialized successfully")
	return provider, nil
}

// ChatCompletionProduction sends a chat completion request to Claude with production features
func (p *ClaudeProvider) ChatCompletionProduction(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Ensure we have valid API credentials
	if p.secureConfig.APIKey == "" {
		if err := p.configManager.RefreshCredentials("anthropic"); err != nil {
			return nil, fmt.Errorf("no API key available and failed to refresh: %w", err)
		}
		refreshedConfig, err := p.configManager.GetSecureConfig("anthropic")
		if err != nil {
			return nil, fmt.Errorf("failed to get refreshed config: %w", err)
		}
		p.secureConfig = refreshedConfig
	}

	// Estimate cost before making the request
	costEstimate, err := p.costCalculator.EstimateRequestCost(req)
	if err != nil {
		p.logger.WithError(err).Warn("Failed to estimate request cost")
	} else {
		// Validate cost limits
		if p.config.CostLimits != nil && p.config.CostLimits.PerRequestLimit > 0 && costEstimate.EstimatedCost > p.config.CostLimits.PerRequestLimit {
			return nil, fmt.Errorf("estimated cost $%.4f exceeds per-request limit $%.2f",
				costEstimate.EstimatedCost, p.config.CostLimits.PerRequestLimit)
		}

		p.logger.WithFields(map[string]interface{}{
			"estimated_cost": costEstimate.EstimatedCost,
			"input_tokens":   costEstimate.InputTokens,
			"output_tokens":  costEstimate.OutputTokens,
		}).Info("Claude request cost estimated")
	}

	// Execute request with retry logic
	var response *types.ChatCompletionResponse
	err = p.retryManager.ExecuteWithRetry(ctx, func(ctx context.Context, attempt int) error {
		var attemptErr error
		response, attemptErr = p.ChatCompletion(ctx, req) // Use existing implementation

		// Classify errors for retry logic
		if attemptErr != nil {
			retryError := retry.ClassifyError(attemptErr, p.GetName(), "ChatCompletion")
			return retryError
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Calculate actual cost if cost tracking is enabled
	if p.config.CostTracking && response != nil {
		actualCost, err := p.costCalculator.CalculateActualCost(req, response)
		if err != nil {
			p.logger.WithError(err).Warn("Failed to calculate actual cost")
		} else {
			p.logger.WithFields(map[string]interface{}{
				"actual_cost":   actualCost.TotalCost,
				"input_tokens":  actualCost.InputTokens,
				"output_tokens": actualCost.OutputTokens,
			}).Info("Claude request cost calculated")
		}
	}

	return response, nil
}

// GetConfig returns the provider configuration (updated for production config)
func (p *ClaudeProvider) GetConfigProduction() *types.ProviderConfig {
	return p.config.ProviderConfig
}

// EstimateTokens estimates token count for a request (production feature)
func (p *ClaudeProvider) EstimateTokens(req *types.ChatCompletionRequest) (*types.TokenEstimate, error) {
	estimate, err := p.costCalculator.EstimateRequestCost(req)
	if err != nil {
		return nil, err
	}
	return estimate.TokenEstimate, nil
}

// CalculateActualCost calculates the actual cost of a completed request (production feature)
func (p *ClaudeProvider) CalculateActualCost(req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse) (*types.CostBreakdown, error) {
	return p.costCalculator.CalculateActualCost(req, resp)
}
