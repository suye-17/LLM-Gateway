// Package cost manages pricing information for different LLM providers and models
package cost

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
)

// PricingManager manages pricing information for different providers and models
type PricingManager struct {
	pricing map[string]*types.ModelPricing // key: "provider/model"
	mutex   sync.RWMutex
}

// NewPricingManager creates a new pricing manager with default pricing data
func NewPricingManager() *PricingManager {
	pm := &PricingManager{
		pricing: make(map[string]*types.ModelPricing),
	}

	// Initialize with current pricing data
	pm.initializeDefaultPricing()

	return pm
}

// GetPricing returns pricing information for a specific provider and model
func (pm *PricingManager) GetPricing(provider, model string) (*types.ModelPricing, error) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	key := fmt.Sprintf("%s/%s", provider, model)
	pricing, exists := pm.pricing[key]
	if !exists {
		// Try to find with pattern matching
		if fallbackPricing := pm.findSimilarPricing(provider, model); fallbackPricing != nil {
			return fallbackPricing, nil
		}
		return nil, fmt.Errorf("pricing not found for %s", key)
	}

	return pricing, nil
}

// UpdatePricing updates pricing information for a specific model
func (pm *PricingManager) UpdatePricing(provider, model string, pricing *types.ModelPricing) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	key := fmt.Sprintf("%s/%s", provider, model)
	pm.pricing[key] = pricing
}

// GetDefaultPricing returns default pricing for a provider
func (pm *PricingManager) GetDefaultPricing(provider string) *types.ModelPricing {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// Return default pricing based on provider
	switch provider {
	case "openai":
		return &types.ModelPricing{
			Model:       "default",
			Provider:    "openai",
			InputPrice:  0.0015, // GPT-3.5-turbo pricing as default
			OutputPrice: 0.002,
			Currency:    "USD",
			LastUpdated: time.Now(),
		}
	case "anthropic":
		return &types.ModelPricing{
			Model:       "default",
			Provider:    "anthropic",
			InputPrice:  0.00025, // Claude Haiku pricing as default
			OutputPrice: 0.00125,
			Currency:    "USD",
			LastUpdated: time.Now(),
		}
	case "baidu":
		return &types.ModelPricing{
			Model:       "default",
			Provider:    "baidu",
			InputPrice:  0.0002, // Estimated pricing for Ernie Bot
			OutputPrice: 0.0004,
			Currency:    "USD",
			LastUpdated: time.Now(),
		}
	default:
		return &types.ModelPricing{
			Model:       "default",
			Provider:    provider,
			InputPrice:  0.001,
			OutputPrice: 0.002,
			Currency:    "USD",
			LastUpdated: time.Now(),
		}
	}
}

// ListPricing returns all available pricing information
func (pm *PricingManager) ListPricing() map[string]*types.ModelPricing {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*types.ModelPricing)
	for key, pricing := range pm.pricing {
		result[key] = &types.ModelPricing{
			Model:       pricing.Model,
			Provider:    pricing.Provider,
			InputPrice:  pricing.InputPrice,
			OutputPrice: pricing.OutputPrice,
			Currency:    pricing.Currency,
			LastUpdated: pricing.LastUpdated,
		}
	}

	return result
}

// findSimilarPricing finds pricing for similar models using pattern matching
func (pm *PricingManager) findSimilarPricing(provider, model string) *types.ModelPricing {
	modelLower := strings.ToLower(model)

	// Look for exact matches first, then partial matches
	for key, pricing := range pm.pricing {
		if strings.HasPrefix(key, provider+"/") {
			existingModel := strings.ToLower(strings.TrimPrefix(key, provider+"/"))

			// Exact match
			if existingModel == modelLower {
				return pricing
			}

			// Pattern matching for similar models
			if pm.modelsAreSimilar(modelLower, existingModel) {
				return pricing
			}
		}
	}

	return nil
}

// modelsAreSimilar checks if two model names are similar enough to use same pricing
func (pm *PricingManager) modelsAreSimilar(model1, model2 string) bool {
	// OpenAI model similarity
	if (strings.Contains(model1, "gpt-3.5") && strings.Contains(model2, "gpt-3.5")) ||
		(strings.Contains(model1, "gpt-4") && strings.Contains(model2, "gpt-4")) {
		return true
	}

	// Anthropic model similarity
	if (strings.Contains(model1, "claude-3-haiku") && strings.Contains(model2, "claude-3-haiku")) ||
		(strings.Contains(model1, "claude-3-sonnet") && strings.Contains(model2, "claude-3-sonnet")) ||
		(strings.Contains(model1, "claude-3-opus") && strings.Contains(model2, "claude-3-opus")) {
		return true
	}

	// Baidu model similarity
	if strings.Contains(model1, "ernie") && strings.Contains(model2, "ernie") {
		return true
	}

	return false
}

// initializeDefaultPricing initializes the pricing manager with current pricing data
func (pm *PricingManager) initializeDefaultPricing() {
	now := time.Now()

	// OpenAI pricing (as of 2024)
	openaiModels := map[string]struct{ input, output float64 }{
		"gpt-3.5-turbo":     {0.0015, 0.002},   // $1.50 / $2.00 per 1M tokens
		"gpt-3.5-turbo-16k": {0.003, 0.004},    // $3.00 / $4.00 per 1M tokens
		"gpt-4":             {0.03, 0.06},      // $30.00 / $60.00 per 1M tokens
		"gpt-4-32k":         {0.06, 0.12},      // $60.00 / $120.00 per 1M tokens
		"gpt-4-turbo":       {0.01, 0.03},      // $10.00 / $30.00 per 1M tokens
		"gpt-4o":            {0.005, 0.015},    // $5.00 / $15.00 per 1M tokens
		"gpt-4o-mini":       {0.00015, 0.0006}, // $0.15 / $0.60 per 1M tokens
	}

	for model, prices := range openaiModels {
		key := fmt.Sprintf("openai/%s", model)
		pm.pricing[key] = &types.ModelPricing{
			Model:       model,
			Provider:    "openai",
			InputPrice:  prices.input / 1000, // Convert to per 1K tokens
			OutputPrice: prices.output / 1000,
			Currency:    "USD",
			LastUpdated: now,
		}
	}

	// Anthropic pricing (as of 2024)
	anthropicModels := map[string]struct{ input, output float64 }{
		"claude-3-haiku":    {0.00025, 0.00125}, // $0.25 / $1.25 per 1M tokens
		"claude-3-sonnet":   {0.003, 0.015},     // $3.00 / $15.00 per 1M tokens
		"claude-3-opus":     {0.015, 0.075},     // $15.00 / $75.00 per 1M tokens
		"claude-3.5-sonnet": {0.003, 0.015},     // $3.00 / $15.00 per 1M tokens
	}

	for model, prices := range anthropicModels {
		key := fmt.Sprintf("anthropic/%s", model)
		pm.pricing[key] = &types.ModelPricing{
			Model:       model,
			Provider:    "anthropic",
			InputPrice:  prices.input / 1000, // Convert to per 1K tokens
			OutputPrice: prices.output / 1000,
			Currency:    "USD",
			LastUpdated: now,
		}
	}

	// Baidu pricing (estimated, as official pricing may vary)
	baiduModels := map[string]struct{ input, output float64 }{
		"ernie-bot":       {0.0002, 0.0004}, // Estimated pricing
		"ernie-bot-turbo": {0.0001, 0.0002}, // Estimated pricing
		"ernie-bot-4":     {0.0008, 0.0016}, // Estimated pricing
		"wenxinworkshop":  {0.0002, 0.0004}, // Estimated pricing
	}

	for model, prices := range baiduModels {
		key := fmt.Sprintf("baidu/%s", model)
		pm.pricing[key] = &types.ModelPricing{
			Model:       model,
			Provider:    "baidu",
			InputPrice:  prices.input / 1000, // Convert to per 1K tokens
			OutputPrice: prices.output / 1000,
			Currency:    "USD", // Convert from CNY to USD for consistency
			LastUpdated: now,
		}
	}
}
