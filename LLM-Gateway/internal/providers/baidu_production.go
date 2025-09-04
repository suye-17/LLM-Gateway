// Package providers implements the Baidu (文心一言) provider adapter - Production features
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

// NewProductionBaiduProvider creates a new Baidu provider for production
func NewProductionBaiduProvider(configPath string, logger *utils.Logger) (*BaiduProvider, error) {
	// Create configuration manager
	configManager := config.NewSecureConfigManager(configPath, logger)

	// Load production configuration
	prodConfig, err := configManager.LoadProviderConfig("baidu")
	if err != nil {
		return nil, fmt.Errorf("failed to load Baidu configuration: %w", err)
	}

	// Load secure credentials
	secureConfig, err := configManager.GetSecureConfig("baidu")
	if err != nil {
		return nil, fmt.Errorf("failed to load secure Baidu credentials: %w", err)
	}

	// Validate configuration
	if err := prodConfig.Validate(); err != nil {
		return nil, fmt.Errorf("Baidu configuration validation failed: %w", err)
	}

	// Validate credentials
	if err := configManager.ValidateAPIKey(context.Background(), "baidu", secureConfig.APIKey); err != nil {
		return nil, fmt.Errorf("Baidu API key validation failed: %w", err)
	}

	// Create components
	retryManager := retry.NewRetryManager(prodConfig.RetryPolicy, logger)
	costCalculator := cost.NewCostCalculator(logger)

	// Create Baidu provider with production config
	provider := &BaiduProvider{
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

	logger.WithField("provider", "baidu").Info("Production Baidu provider initialized successfully")
	return provider, nil
}

// BaiduProvider with production features (enhanced structure)
type BaiduProviderProduction struct {
	*BaiduProvider
	config         *types.ProductionConfig
	secureConfig   *types.SecureConfig
	retryManager   retry.RetryManagerInterface
	costCalculator *cost.CostCalculator
	configManager  config.ConfigurationManager
}

// ChatCompletionProduction sends a chat completion request to Baidu with production features
func (p *BaiduProvider) ChatCompletionProduction(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// For Baidu, we need to ensure we have valid access token
	// This would typically involve OAuth2 flow for Baidu API

	// p.config is already *types.ProductionConfig, so use it directly

	// Estimate cost before making the request
	if p.costCalculator != nil {
		costEstimate, err := p.costCalculator.EstimateRequestCost(req)
		if err != nil {
			p.logger.WithError(err).Warn("Failed to estimate request cost")
		} else {
			// Validate cost limits
			if p.config != nil && p.config.CostLimits != nil && p.config.CostLimits.PerRequestLimit > 0 && costEstimate.EstimatedCost > p.config.CostLimits.PerRequestLimit {
				return nil, fmt.Errorf("estimated cost $%.4f exceeds per-request limit $%.2f",
					costEstimate.EstimatedCost, p.config.CostLimits.PerRequestLimit)
			}

			p.logger.WithFields(map[string]interface{}{
				"estimated_cost": costEstimate.EstimatedCost,
				"input_tokens":   costEstimate.InputTokens,
				"output_tokens":  costEstimate.OutputTokens,
			}).Info("Baidu request cost estimated")
		}
	}

	// Execute request with retry logic (if available)
	var response *types.ChatCompletionResponse
	var err error

	if p.retryManager != nil {
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
	} else {
		// Fallback to direct call if no retry manager
		response, err = p.ChatCompletion(ctx, req)
	}

	if err != nil {
		return nil, err
	}

	// Calculate actual cost if cost tracking is enabled
	if p.config != nil && p.config.CostTracking && response != nil && p.costCalculator != nil {
		actualCost, err := p.costCalculator.CalculateActualCost(req, response)
		if err != nil {
			p.logger.WithError(err).Warn("Failed to calculate actual cost")
		} else {
			p.logger.WithFields(map[string]interface{}{
				"actual_cost":   actualCost.TotalCost,
				"input_tokens":  actualCost.InputTokens,
				"output_tokens": actualCost.OutputTokens,
			}).Info("Baidu request cost calculated")
		}
	}

	return response, nil
}

// EstimateTokens estimates token count for a request (production feature)
func (p *BaiduProvider) EstimateTokens(req *types.ChatCompletionRequest) (*types.TokenEstimate, error) {
	if p.costCalculator == nil {
		return nil, fmt.Errorf("cost calculator not available")
	}
	estimate, err := p.costCalculator.EstimateRequestCost(req)
	if err != nil {
		return nil, err
	}
	return estimate.TokenEstimate, nil
}

// CalculateActualCost calculates the actual cost of a completed request (production feature)
func (p *BaiduProvider) CalculateActualCost(req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse) (*types.CostBreakdown, error) {
	if p.costCalculator == nil {
		return nil, fmt.Errorf("cost calculator not available")
	}
	return p.costCalculator.CalculateActualCost(req, resp)
}
