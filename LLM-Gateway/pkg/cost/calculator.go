// Package cost provides cost calculation and estimation for LLM requests
package cost

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// CostCalculator manages cost estimation and calculation for LLM requests
type CostCalculator struct {
	pricingManager *PricingManager
	tokenEstimator *TokenEstimator
	logger         *utils.Logger
}

// CostCalculatorInterface defines the contract for cost calculation
type CostCalculatorInterface interface {
	EstimateRequestCost(req *types.ChatCompletionRequest) (*types.CostEstimate, error)
	CalculateActualCost(req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse) (*types.CostBreakdown, error)
	GetPricingInfo(provider, model string) (*types.ModelPricing, error)
	UpdatePricing(provider, model string, pricing *types.ModelPricing) error
}

// CostEstimate represents estimated cost for a request
type CostEstimate struct {
	*types.TokenEstimate
	EstimatedCost float64   `json:"estimated_cost"`
	Currency      string    `json:"currency"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	PricingDate   time.Time `json:"pricing_date"`
}

// NewCostCalculator creates a new cost calculator
func NewCostCalculator(logger *utils.Logger) *CostCalculator {
	return &CostCalculator{
		pricingManager: NewPricingManager(),
		tokenEstimator: NewTokenEstimator(),
		logger:         logger,
	}
}

// EstimateRequestCost estimates the cost for a chat completion request
func (cc *CostCalculator) EstimateRequestCost(req *types.ChatCompletionRequest) (*CostEstimate, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	// Determine provider from request context or default
	provider := cc.determineProvider(req)

	// Get token estimation
	tokenEstimate, err := cc.tokenEstimator.EstimateTokens(req, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate tokens: %w", err)
	}

	// Get pricing information
	pricing, err := cc.pricingManager.GetPricing(provider, req.Model)
	if err != nil {
		cc.logger.WithError(err).WithField("model", req.Model).Warn("Failed to get pricing, using fallback")
		// Use default pricing for the provider
		pricing = cc.pricingManager.GetDefaultPricing(provider)
	}

	// Calculate estimated cost
	inputCost := float64(tokenEstimate.InputTokens) / 1000.0 * pricing.InputPrice
	outputCost := float64(tokenEstimate.OutputTokens) / 1000.0 * pricing.OutputPrice
	totalCost := inputCost + outputCost

	estimate := &CostEstimate{
		TokenEstimate: tokenEstimate,
		EstimatedCost: math.Round(totalCost*10000) / 10000, // Round to 4 decimal places
		Currency:      pricing.Currency,
		Provider:      provider,
		Model:         req.Model,
		PricingDate:   pricing.LastUpdated,
	}

	cc.logger.WithFields(map[string]interface{}{
		"provider":       provider,
		"model":          req.Model,
		"input_tokens":   tokenEstimate.InputTokens,
		"output_tokens":  tokenEstimate.OutputTokens,
		"estimated_cost": estimate.EstimatedCost,
		"currency":       estimate.Currency,
	}).Debug("Cost estimation completed")

	return estimate, nil
}

// CalculateActualCost calculates the actual cost based on the API response
func (cc *CostCalculator) CalculateActualCost(req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse) (*types.CostBreakdown, error) {
	if req == nil || resp == nil {
		return nil, fmt.Errorf("request and response cannot be nil")
	}

	// Determine provider
	provider := cc.determineProvider(req)

	// Get actual token counts from response
	var inputTokens, outputTokens int
	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		inputTokens = resp.Usage.PromptTokens
		outputTokens = resp.Usage.CompletionTokens
	} else {
		// Fallback to estimation if usage not provided
		cc.logger.WithField("model", req.Model).Warn("No usage data in response, falling back to estimation")
		estimate, err := cc.tokenEstimator.EstimateTokens(req, provider)
		if err != nil {
			return nil, fmt.Errorf("failed to estimate tokens for actual cost: %w", err)
		}
		inputTokens = estimate.InputTokens

		// For output tokens, use the actual response content length
		if len(resp.Choices) > 0 && resp.Choices[0].Message.Content != "" {
			outputTokens = cc.tokenEstimator.EstimateOutputTokens(resp.Choices[0].Message.Content, provider)
		} else {
			outputTokens = estimate.OutputTokens
		}
	}

	// Get pricing information
	pricing, err := cc.pricingManager.GetPricing(provider, req.Model)
	if err != nil {
		pricing = cc.pricingManager.GetDefaultPricing(provider)
	}

	// Calculate actual cost
	inputCost := float64(inputTokens) / 1000.0 * pricing.InputPrice
	outputCost := float64(outputTokens) / 1000.0 * pricing.OutputPrice
	totalCost := inputCost + outputCost

	breakdown := &types.CostBreakdown{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		InputCost:    math.Round(inputCost*10000) / 10000,
		OutputCost:   math.Round(outputCost*10000) / 10000,
		TotalCost:    math.Round(totalCost*10000) / 10000,
		Currency:     pricing.Currency,
		Model:        req.Model,
		Provider:     provider,
		Timestamp:    time.Now(),
	}

	cc.logger.WithFields(map[string]interface{}{
		"provider":      provider,
		"model":         req.Model,
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"input_cost":    breakdown.InputCost,
		"output_cost":   breakdown.OutputCost,
		"total_cost":    breakdown.TotalCost,
		"currency":      breakdown.Currency,
	}).Info("Actual cost calculated")

	return breakdown, nil
}

// GetPricingInfo returns pricing information for a specific provider and model
func (cc *CostCalculator) GetPricingInfo(provider, model string) (*types.ModelPricing, error) {
	pricing, err := cc.pricingManager.GetPricing(provider, model)
	if err != nil {
		return nil, fmt.Errorf("pricing not found for %s/%s: %w", provider, model, err)
	}

	return pricing, nil
}

// UpdatePricing updates pricing information for a model
func (cc *CostCalculator) UpdatePricing(provider, model string, pricing *types.ModelPricing) error {
	if pricing == nil {
		return fmt.Errorf("pricing cannot be nil")
	}

	pricing.LastUpdated = time.Now()
	cc.pricingManager.UpdatePricing(provider, model, pricing)

	cc.logger.WithFields(map[string]interface{}{
		"provider":     provider,
		"model":        model,
		"input_price":  pricing.InputPrice,
		"output_price": pricing.OutputPrice,
	}).Info("Pricing updated")

	return nil
}

// determineProvider determines the provider from the request context
func (cc *CostCalculator) determineProvider(req *types.ChatCompletionRequest) string {
	// Try to determine from model name patterns
	model := strings.ToLower(req.Model)

	switch {
	case strings.Contains(model, "gpt"):
		return "openai"
	case strings.Contains(model, "claude"):
		return "anthropic"
	case strings.Contains(model, "ernie") || strings.Contains(model, "wenxin"):
		return "baidu"
	default:
		// Default fallback
		return "unknown"
	}
}

// ValidateCostLimits checks if a request would exceed cost limits
func (cc *CostCalculator) ValidateCostLimits(estimate *CostEstimate, limits *types.CostLimits) error {
	if limits == nil {
		return nil // No limits configured
	}

	// Check per-request limit
	if limits.PerRequestLimit > 0 && estimate.EstimatedCost > limits.PerRequestLimit {
		return fmt.Errorf("estimated cost $%.4f exceeds per-request limit $%.2f",
			estimate.EstimatedCost, limits.PerRequestLimit)
	}

	// Note: Daily and monthly limits would require additional tracking
	// that could be implemented in a separate cost tracking service

	return nil
}

// GetCostSummary returns a summary of costs for analysis
func (cc *CostCalculator) GetCostSummary(breakdowns []*types.CostBreakdown) *CostSummary {
	if len(breakdowns) == 0 {
		return &CostSummary{}
	}

	summary := &CostSummary{
		TotalRequests: len(breakdowns),
		ByProvider:    make(map[string]*ProviderCostSummary),
		ByModel:       make(map[string]*ModelCostSummary),
	}

	for _, breakdown := range breakdowns {
		summary.TotalCost += breakdown.TotalCost
		summary.TotalInputTokens += breakdown.InputTokens
		summary.TotalOutputTokens += breakdown.OutputTokens

		// Provider summary
		if _, exists := summary.ByProvider[breakdown.Provider]; !exists {
			summary.ByProvider[breakdown.Provider] = &ProviderCostSummary{}
		}
		providerSummary := summary.ByProvider[breakdown.Provider]
		providerSummary.TotalCost += breakdown.TotalCost
		providerSummary.RequestCount++
		providerSummary.InputTokens += breakdown.InputTokens
		providerSummary.OutputTokens += breakdown.OutputTokens

		// Model summary
		modelKey := fmt.Sprintf("%s/%s", breakdown.Provider, breakdown.Model)
		if _, exists := summary.ByModel[modelKey]; !exists {
			summary.ByModel[modelKey] = &ModelCostSummary{
				Provider: breakdown.Provider,
				Model:    breakdown.Model,
			}
		}
		modelSummary := summary.ByModel[modelKey]
		modelSummary.TotalCost += breakdown.TotalCost
		modelSummary.RequestCount++
		modelSummary.InputTokens += breakdown.InputTokens
		modelSummary.OutputTokens += breakdown.OutputTokens
	}

	// Calculate averages
	if summary.TotalRequests > 0 {
		summary.AverageCostPerRequest = summary.TotalCost / float64(summary.TotalRequests)
	}

	return summary
}

// CostSummary represents aggregated cost information
type CostSummary struct {
	TotalRequests         int                             `json:"total_requests"`
	TotalCost             float64                         `json:"total_cost"`
	TotalInputTokens      int                             `json:"total_input_tokens"`
	TotalOutputTokens     int                             `json:"total_output_tokens"`
	AverageCostPerRequest float64                         `json:"average_cost_per_request"`
	ByProvider            map[string]*ProviderCostSummary `json:"by_provider"`
	ByModel               map[string]*ModelCostSummary    `json:"by_model"`
}

// ProviderCostSummary represents cost summary by provider
type ProviderCostSummary struct {
	TotalCost    float64 `json:"total_cost"`
	RequestCount int     `json:"request_count"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
}

// ModelCostSummary represents cost summary by model
type ModelCostSummary struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	TotalCost    float64 `json:"total_cost"`
	RequestCount int     `json:"request_count"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
}
