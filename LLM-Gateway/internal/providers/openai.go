// Package providers implements the OpenAI provider adapter
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/llm-gateway/gateway/internal/config"
	"github.com/llm-gateway/gateway/pkg/cost"
	"github.com/llm-gateway/gateway/pkg/retry"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// OpenAIProvider implements the Provider interface for OpenAI
type OpenAIProvider struct {
	config         *types.ProductionConfig
	secureConfig   *types.SecureConfig
	logger         *utils.Logger
	httpClient     *http.Client
	rateLimits     *types.RateLimitInfo
	retryManager   retry.RetryManagerInterface
	costCalculator *cost.CostCalculator
	configManager  config.ConfigurationManager
}

// OpenAI API structures
type openAIRequest struct {
	Model            string           `json:"model"`
	Messages         []openAIMessage  `json:"messages"`
	MaxTokens        *int             `json:"max_tokens,omitempty"`
	Temperature      *float64         `json:"temperature,omitempty"`
	TopP             *float64         `json:"top_p,omitempty"`
	N                *int             `json:"n,omitempty"`
	Stream           *bool            `json:"stream,omitempty"`
	Stop             interface{}      `json:"stop,omitempty"`
	PresencePenalty  *float64         `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64         `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]int   `json:"logit_bias,omitempty"`
	User             *string          `json:"user,omitempty"`
	Tools            []openAITool     `json:"tools,omitempty"`
	ToolChoice       interface{}      `json:"tool_choice,omitempty"`
	Functions        []openAIFunction `json:"functions,omitempty"`
	FunctionCall     interface{}      `json:"function_call,omitempty"`
}

type openAIMessage struct {
	Role         string              `json:"role"`
	Content      string              `json:"content"`
	Name         *string             `json:"name,omitempty"`
	ToolCalls    []openAIToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   *string             `json:"tool_call_id,omitempty"`
	FunctionCall *openAIFunctionCall `json:"function_call,omitempty"`
}

type openAIResponse struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	SystemFingerprint *string        `json:"system_fingerprint,omitempty"`
	Choices           []openAIChoice `json:"choices"`
	Usage             openAIUsage    `json:"usage"`
}

type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason *string       `json:"finish_reason"`
	Logprobs     interface{}   `json:"logprobs"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIErrorResponse struct {
	Error openAIError `json:"error"`
}

type openAIError struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    *string `json:"code"`
}

// Model pricing (USD per 1K tokens)
var openAIPricing = map[string]struct {
	InputPrice  float64
	OutputPrice float64
}{
	"gpt-4":                  {0.03, 0.06},
	"gpt-4-32k":              {0.06, 0.12},
	"gpt-4-1106-preview":     {0.01, 0.03},
	"gpt-4-0125-preview":     {0.01, 0.03},
	"gpt-4-turbo-preview":    {0.01, 0.03},
	"gpt-3.5-turbo":          {0.001, 0.002},
	"gpt-3.5-turbo-16k":      {0.003, 0.004},
	"gpt-3.5-turbo-1106":     {0.001, 0.002},
	"gpt-3.5-turbo-0125":     {0.0005, 0.0015},
	"text-embedding-ada-002": {0.0001, 0.0001},
	"text-embedding-3-small": {0.00002, 0.00002},
	"text-embedding-3-large": {0.00013, 0.00013},
}

// NewOpenAIProvider creates a new OpenAI provider with production features
func NewOpenAIProvider(baseConfig *types.ProviderConfig, logger *utils.Logger) *OpenAIProvider {
	// Convert to production config
	prodConfig := types.NewProductionConfig(baseConfig)

	// Set defaults
	if prodConfig.BaseURL == "" {
		prodConfig.BaseURL = "https://api.openai.com/v1"
	}
	if prodConfig.Timeout == 0 {
		prodConfig.Timeout = 60 * time.Second
	}
	if prodConfig.APIKeyEnvVar == "" {
		prodConfig.APIKeyEnvVar = "OPENAI_API_KEY"
	}

	// Create configuration manager
	configManager := config.NewSecureConfigManager("configs/production.yaml", logger)

	// Load secure configuration
	secureConfig, err := configManager.GetSecureConfig("openai")
	if err != nil {
		logger.WithError(err).Warn("Failed to load secure config, will attempt to load on first use")
		secureConfig = &types.SecureConfig{
			BaseURL: prodConfig.BaseURL,
		}
	}

	// Create retry manager
	retryManager := retry.NewRetryManager(prodConfig.RetryPolicy, logger)

	// Create cost calculator
	costCalculator := cost.NewCostCalculator(logger)

	return &OpenAIProvider{
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
}

// NewProductionOpenAIProvider creates a new OpenAI provider specifically for production
func NewProductionOpenAIProvider(configPath string, logger *utils.Logger) (*OpenAIProvider, error) {
	// Create configuration manager
	configManager := config.NewSecureConfigManager(configPath, logger)

	// Load production configuration
	prodConfig, err := configManager.LoadProviderConfig("openai")
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAI configuration: %w", err)
	}

	// Load secure credentials
	secureConfig, err := configManager.GetSecureConfig("openai")
	if err != nil {
		return nil, fmt.Errorf("failed to load secure OpenAI credentials: %w", err)
	}

	// Validate configuration
	if err := prodConfig.Validate(); err != nil {
		return nil, fmt.Errorf("OpenAI configuration validation failed: %w", err)
	}

	// Validate credentials
	if err := configManager.ValidateAPIKey(context.Background(), "openai", secureConfig.APIKey); err != nil {
		return nil, fmt.Errorf("OpenAI API key validation failed: %w", err)
	}

	// Create components
	retryManager := retry.NewRetryManager(prodConfig.RetryPolicy, logger)
	costCalculator := cost.NewCostCalculator(logger)

	provider := &OpenAIProvider{
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

	logger.WithField("provider", "openai").Info("Production OpenAI provider initialized successfully")
	return provider, nil
}

// GetName returns the provider name
func (p *OpenAIProvider) GetName() string {
	return p.config.Name
}

// GetType returns the provider type
func (p *OpenAIProvider) GetType() string {
	return "openai"
}

// Call implements the Provider interface by wrapping ChatCompletion
func (p *OpenAIProvider) Call(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return p.ChatCompletion(ctx, req)
}

// GetConfig returns the provider configuration
func (p *OpenAIProvider) GetConfig() *types.ProviderConfig {
	return p.config.ProviderConfig
}

// ValidateConfig validates the provider configuration
func (p *OpenAIProvider) ValidateConfig(config *types.ProviderConfig) error {
	if config.APIKey == "" {
		return fmt.Errorf("OpenAI API key is required")
	}

	if config.Type != "openai" {
		return fmt.Errorf("invalid provider type: expected 'openai', got '%s'", config.Type)
	}

	return nil
}

// ChatCompletion sends a chat completion request to OpenAI with production features
func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Ensure we have valid API credentials
	if err := p.ensureValidCredentials(ctx); err != nil {
		return nil, fmt.Errorf("credential validation failed: %w", err)
	}

	// Estimate cost before making the request
	costEstimate, err := p.costCalculator.EstimateRequestCost(req)
	if err != nil {
		p.logger.WithError(err).Warn("Failed to estimate request cost")
	} else {
		// Validate cost limits (implement basic validation here)
		if p.config.CostLimits != nil && p.config.CostLimits.PerRequestLimit > 0 && costEstimate.EstimatedCost > p.config.CostLimits.PerRequestLimit {
			return nil, fmt.Errorf("estimated cost $%.4f exceeds per-request limit $%.2f",
				costEstimate.EstimatedCost, p.config.CostLimits.PerRequestLimit)
		}

		p.logger.WithFields(map[string]interface{}{
			"estimated_cost": costEstimate.EstimatedCost,
			"input_tokens":   costEstimate.InputTokens,
			"output_tokens":  costEstimate.OutputTokens,
		}).Info("OpenAI request cost estimated")
	}

	// Execute request with retry logic
	var response *types.ChatCompletionResponse
	err = p.retryManager.ExecuteWithRetry(ctx, func(ctx context.Context, attempt int) error {
		var attemptErr error
		response, attemptErr = p.executeAPIRequest(ctx, req, attempt)
		return attemptErr
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
				"input_cost":    actualCost.InputCost,
				"output_cost":   actualCost.OutputCost,
			}).Info("OpenAI request cost calculated")
		}
	}

	return response, nil
}

// executeAPIRequest executes a single API request attempt
func (p *OpenAIProvider) executeAPIRequest(ctx context.Context, req *types.ChatCompletionRequest, attempt int) (*types.ChatCompletionResponse, error) {
	// Convert request to OpenAI format
	openAIReq := p.convertRequest(req)

	// Serialize request
	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, retry.NewProviderRetryError(p.GetName(), "ChatCompletion", types.ErrorClient,
			fmt.Sprintf("failed to marshal request: %v", err), false)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.secureConfig.BaseURL+"/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, retry.NewProviderRetryError(p.GetName(), "ChatCompletion", types.ErrorClient,
			fmt.Sprintf("failed to create request: %v", err), false)
	}

	// Set headers with real API key
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.secureConfig.APIKey)
	httpReq.Header.Set("User-Agent", "LLM-Gateway/2.0")

	// Add organization header if available
	if p.secureConfig.OrganizationID != "" {
		httpReq.Header.Set("OpenAI-Organization", p.secureConfig.OrganizationID)
	}

	// Add project header if available
	if p.secureConfig.ProjectID != "" {
		httpReq.Header.Set("OpenAI-Project", p.secureConfig.ProjectID)
	}

	// Log request attempt
	p.logger.WithFields(map[string]interface{}{
		"model":   req.Model,
		"attempt": attempt,
		"url":     httpReq.URL.String(),
	}).Info("Sending OpenAI API request")

	// Send request
	start := time.Now()
	resp, err := p.httpClient.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		// Classify network error
		retryError := retry.ClassifyError(err, p.GetName(), "ChatCompletion")
		p.logger.WithError(err).WithField("duration", duration).Error("OpenAI API request failed")
		return nil, retryError
	}
	defer resp.Body.Close()

	// Update rate limits from headers
	p.updateRateLimits(resp.Header)

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, retry.NewProviderRetryError(p.GetName(), "ChatCompletion", types.ErrorNetwork,
			fmt.Sprintf("failed to read response: %v", err), true)
	}

	p.logger.WithFields(map[string]interface{}{
		"status_code": resp.StatusCode,
		"duration":    duration,
		"attempt":     attempt,
	}).Info("OpenAI API response received")

	// Handle errors
	if resp.StatusCode != http.StatusOK {
		errorMsg := string(respBody)

		// Try to parse OpenAI error format
		var errorResp openAIErrorResponse
		if err := json.Unmarshal(respBody, &errorResp); err == nil && errorResp.Error.Message != "" {
			errorMsg = errorResp.Error.Message
		}

		// Create detailed retry error
		retryError := retry.ClassifyError(fmt.Errorf("API error: %s", errorMsg), p.GetName(), "ChatCompletion")
		retryError.StatusCode = resp.StatusCode

		// Special handling for specific OpenAI error types
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			retryError.Category = types.ErrorAuth
			retryError.Retryable = false
		case http.StatusTooManyRequests:
			retryError.Category = types.ErrorRateLimit
			retryError.Retryable = true
			// Try to extract retry-after header
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					retryError.RetryAfter = seconds
				}
			}
		case http.StatusPaymentRequired:
			retryError.Category = types.ErrorQuota
			retryError.Retryable = false
		}

		p.logger.WithFields(map[string]interface{}{
			"status_code": resp.StatusCode,
			"error":       errorMsg,
			"retryable":   retryError.Retryable,
		}).Error("OpenAI API returned error")

		return nil, retryError
	}

	// Parse successful response
	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, retry.NewProviderRetryError(p.GetName(), "ChatCompletion", types.ErrorServer,
			fmt.Sprintf("failed to unmarshal response: %v", err), true)
	}

	// Log successful response
	if len(openAIResp.Choices) > 0 {
		p.logger.WithFields(map[string]interface{}{
			"response_length": len(openAIResp.Choices[0].Message.Content),
			"finish_reason":   openAIResp.Choices[0].FinishReason,
			"tokens_used":     openAIResp.Usage.TotalTokens,
		}).Info("OpenAI API request completed successfully")
	}

	// Convert response to standard format
	return p.convertResponse(&openAIResp), nil
}

// ensureValidCredentials ensures we have valid API credentials
func (p *OpenAIProvider) ensureValidCredentials(ctx context.Context) error {
	if p.secureConfig.APIKey == "" {
		// Try to reload credentials
		if err := p.configManager.RefreshCredentials("openai"); err != nil {
			return fmt.Errorf("no API key available and failed to refresh: %w", err)
		}

		// Get refreshed config
		refreshedConfig, err := p.configManager.GetSecureConfig("openai")
		if err != nil {
			return fmt.Errorf("failed to get refreshed config: %w", err)
		}
		p.secureConfig = refreshedConfig
	}

	// Validate API key format
	if err := p.configManager.ValidateAPIKey(ctx, "openai", p.secureConfig.APIKey); err != nil {
		return fmt.Errorf("API key validation failed: %w", err)
	}

	return nil
}

// HealthCheck performs a health check
func (p *OpenAIProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	start := time.Now()

	// Create a simple test request
	req, err := http.NewRequestWithContext(ctx, "GET", p.config.BaseURL+"/models", nil)
	if err != nil {
		return &types.HealthStatus{
			IsHealthy:    false,
			ResponseTime: time.Since(start),
			ErrorMessage: fmt.Sprintf("Failed to create health check request: %v", err),
			LastChecked:  time.Now(),
			Endpoint:     p.config.BaseURL,
		}, nil
	}

	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return &types.HealthStatus{
			IsHealthy:    false,
			ResponseTime: time.Since(start),
			ErrorMessage: fmt.Sprintf("Health check request failed: %v", err),
			LastChecked:  time.Now(),
			Endpoint:     p.config.BaseURL,
		}, nil
	}
	defer resp.Body.Close()

	isHealthy := resp.StatusCode == http.StatusOK
	errorMessage := ""
	if !isHealthy {
		errorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return &types.HealthStatus{
		IsHealthy:    isHealthy,
		ResponseTime: time.Since(start),
		ErrorMessage: errorMessage,
		LastChecked:  time.Now(),
		Endpoint:     p.config.BaseURL,
	}, nil
}

// GetModels returns available models
func (p *OpenAIProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	// This would typically fetch from OpenAI's models endpoint
	// For now, return a static list of common models
	models := []*types.Model{
		{
			Name:               "gpt-4",
			DisplayName:        "GPT-4",
			Description:        "Most capable GPT-4 model and optimized for chat",
			ContextLength:      8192,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.00003,
			CostPerOutputToken: 0.00006,
			IsEnabled:          true,
		},
		{
			Name:               "gpt-3.5-turbo",
			DisplayName:        "GPT-3.5 Turbo",
			Description:        "Most capable GPT-3.5 model and optimized for chat",
			ContextLength:      4096,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.000001,
			CostPerOutputToken: 0.000002,
			IsEnabled:          true,
		},
	}

	return models, nil
}

// EstimateCost estimates the cost for a request
func (p *OpenAIProvider) EstimateCost(req *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	pricing, exists := openAIPricing[req.Model]
	if !exists {
		// Default to GPT-3.5-turbo pricing
		pricing = openAIPricing["gpt-3.5-turbo"]
	}

	// Rough token estimation (1 token ≈ 4 characters)
	inputText := ""
	for _, msg := range req.Messages {
		inputText += msg.Content + " "
	}

	inputTokens := len(inputText) / 4
	maxTokens := 1000
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	inputCost := float64(inputTokens) * pricing.InputPrice / 1000
	outputCost := float64(maxTokens) * pricing.OutputPrice / 1000
	totalCost := inputCost + outputCost

	return &types.CostEstimate{
		InputTokens:   inputTokens,
		OutputTokens:  maxTokens,
		TotalTokens:   inputTokens + maxTokens,
		InputCost:     inputCost,
		OutputCost:    outputCost,
		TotalCost:     totalCost,
		Currency:      "USD",
		PricePerToken: (pricing.InputPrice + pricing.OutputPrice) / 2000,
	}, nil
}

// GetRateLimit returns current rate limit information
func (p *OpenAIProvider) GetRateLimit() *types.RateLimitInfo {
	return p.rateLimits
}

// convertRequest converts standard request to OpenAI format
func (p *OpenAIProvider) convertRequest(req *types.ChatCompletionRequest) *openAIRequest {
	openAIReq := &openAIRequest{
		Model:            req.Model,
		Messages:         make([]openAIMessage, len(req.Messages)),
		MaxTokens:        req.MaxTokens,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		N:                req.N,
		Stream:           req.Stream,
		Stop:             req.Stop,
		PresencePenalty:  req.PresencePenalty,
		FrequencyPenalty: req.FrequencyPenalty,
		LogitBias:        req.LogitBias,
		User:             req.User,
	}

	// Convert messages
	for i, msg := range req.Messages {
		openAIReq.Messages[i] = openAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	return openAIReq
}

// convertResponse converts OpenAI response to standard format
func (p *OpenAIProvider) convertResponse(resp *openAIResponse) *types.ChatCompletionResponse {
	choices := make([]types.Choice, len(resp.Choices))
	for i, choice := range resp.Choices {
		choices[i] = types.Choice{
			Index: choice.Index,
			Message: types.Message{
				Role:    choice.Message.Role,
				Content: choice.Message.Content,
			},
			FinishReason: choice.FinishReason,
		}
	}

	return &types.ChatCompletionResponse{
		ID:      resp.ID,
		Object:  resp.Object,
		Created: time.Unix(resp.Created, 0).Format(time.RFC3339),
		Model:   resp.Model,
		Choices: choices,
		Usage: types.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		Provider:  p.GetName(),
		LatencyMs: 0, // Will be set by caller
	}
}

// updateRateLimits updates rate limit information from response headers
func (p *OpenAIProvider) updateRateLimits(headers http.Header) {
	if remaining := headers.Get("x-ratelimit-remaining-requests"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			p.rateLimits.RemainingRequests = val
		}
	}

	if remaining := headers.Get("x-ratelimit-remaining-tokens"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			p.rateLimits.RemainingTokens = val
		}
	}

	if reset := headers.Get("x-ratelimit-reset-requests"); reset != "" {
		if duration, err := time.ParseDuration(reset); err == nil {
			p.rateLimits.ResetTime = time.Now().Add(duration)
		}
	}
}

// ============================================================================
// Production Features (Week 5)
// ============================================================================

// ProviderMetrics represents metrics for the provider
type ProviderMetrics struct {
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	RetryCount         int64   `json:"retry_count"`
	AverageAttempts    float64 `json:"average_attempts"`
	LastError          string  `json:"last_error"`
}

// ModelSpec represents specifications for a model
type ModelSpec struct {
	Name            string  `json:"name"`
	ContextLength   int     `json:"context_length"`
	CostPer1KTokens float64 `json:"cost_per_1k_tokens"`
}

// EstimateTokens estimates token count for a request (production feature)
func (p *OpenAIProvider) EstimateTokens(req *types.ChatCompletionRequest) (*types.TokenEstimate, error) {
	estimate, err := p.costCalculator.EstimateRequestCost(req)
	if err != nil {
		return nil, err
	}
	// Return the embedded TokenEstimate from CostEstimate
	return estimate.TokenEstimate, nil
}

// CalculateActualCost calculates the actual cost of a completed request (production feature)
func (p *OpenAIProvider) CalculateActualCost(req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse) (*types.CostBreakdown, error) {
	return p.costCalculator.CalculateActualCost(req, resp)
}

// GetProviderMetrics returns provider-specific metrics (production feature)
func (p *OpenAIProvider) GetProviderMetrics() *ProviderMetrics {
	stats := p.retryManager.GetRetryStats()

	return &ProviderMetrics{
		TotalRequests:      stats.TotalAttempts,
		SuccessfulRequests: stats.TotalAttempts - stats.FailedRetries,
		FailedRequests:     stats.FailedRetries,
		RetryCount:         stats.TotalRetries,
		AverageAttempts:    stats.AverageAttempts,
		LastError:          "", // Could be tracked separately if needed
	}
}

// GetLastError returns the last error encountered (production feature)
func (p *OpenAIProvider) GetLastError() *ProviderError {
	// This would require tracking the last error state
	// For now, return nil - could be implemented with a lastError field
	return nil
}

// ValidateCredentials validates the API credentials (production feature)
func (p *OpenAIProvider) ValidateCredentials(ctx context.Context) error {
	return p.ensureValidCredentials(ctx)
}

// GetSupportedModels returns the models supported by this provider (production feature)
func (p *OpenAIProvider) GetSupportedModels() ([]*ModelSpec, error) {
	return []*ModelSpec{
		{Name: "gpt-3.5-turbo", ContextLength: 4096, CostPer1KTokens: 0.002},
		{Name: "gpt-4", ContextLength: 8192, CostPer1KTokens: 0.06},
		{Name: "gpt-4-turbo", ContextLength: 128000, CostPer1KTokens: 0.03},
		{Name: "gpt-4o", ContextLength: 128000, CostPer1KTokens: 0.015},
		{Name: "gpt-4o-mini", ContextLength: 128000, CostPer1KTokens: 0.0006},
	}, nil
}

// GetPricingInfo returns pricing information for a model (production feature)
func (p *OpenAIProvider) GetPricingInfo(model string) (*types.ModelPricing, error) {
	return p.costCalculator.GetPricingInfo("openai", model)
}
