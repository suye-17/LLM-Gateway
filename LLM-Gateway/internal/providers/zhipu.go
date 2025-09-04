// Package providers implements the Zhipu GLM provider adapter
package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/llm-gateway/gateway/pkg/cost"
	"github.com/llm-gateway/gateway/pkg/retry"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// ZhipuProvider implements the Provider interface for Zhipu GLM
type ZhipuProvider struct {
	config         *types.ProductionConfig
	secureConfig   *types.SecureConfig
	retryManager   *retry.RetryManager
	costCalculator *cost.CostCalculator
	logger         *utils.Logger
	httpClient     *http.Client
	rateLimits     *types.RateLimitInfo
}

// Zhipu API structures
type zhipuRequest struct {
	Model    string         `json:"model"`
	Messages []zhipuMessage `json:"messages"`
	Stream   bool           `json:"stream"`
}

type zhipuMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type zhipuResponse struct {
	ID      string        `json:"id"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []zhipuChoice `json:"choices"`
	Usage   zhipuUsage    `json:"usage"`
}

type zhipuChoice struct {
	Index        int          `json:"index"`
	FinishReason string       `json:"finish_reason"`
	Message      zhipuMessage `json:"message"`
}

type zhipuUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type zhipuErrorResponse struct {
	Error zhipuError `json:"error"`
}

type zhipuError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Streaming response structures
type zhipuStreamChunk struct {
	ID      string              `json:"id"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []zhipuStreamChoice `json:"choices"`
}

type zhipuStreamChoice struct {
	Index        int              `json:"index"`
	Delta        zhipuStreamDelta `json:"delta"`
	FinishReason *string          `json:"finish_reason"`
}

type zhipuStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// NewZhipuProvider creates a new Zhipu provider (compatible version)
func NewZhipuProvider(config *types.ProviderConfig, logger *utils.Logger) *ZhipuProvider {
	productionConfig := types.NewProductionConfig(config)
	return NewProductionZhipuProvider(productionConfig, logger)
}

// NewProductionZhipuProvider creates a production-ready Zhipu provider
func NewProductionZhipuProvider(config *types.ProductionConfig, logger *utils.Logger) *ZhipuProvider {
	if config.ProviderConfig.BaseURL == "" {
		config.ProviderConfig.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
	}

	if config.ProviderConfig.Timeout == 0 {
		config.ProviderConfig.Timeout = 60 * time.Second
	}

	// Initialize production components
	retryPolicy := &types.RetryPolicy{
		MaxRetries:    3,
		BaseDelay:     time.Second,
		BackoffFactor: 2.0,
		MaxDelay:      30 * time.Second,
	}
	retryManager := retry.NewRetryManager(retryPolicy, logger)
	costCalculator := cost.NewCostCalculator(logger)

	// Create secure config instance
	secureConfig := &types.SecureConfig{
		APIKey:  config.ProviderConfig.APIKey,
		BaseURL: config.ProviderConfig.BaseURL,
	}

	return &ZhipuProvider{
		config:         config,
		secureConfig:   secureConfig,
		retryManager:   retryManager,
		costCalculator: costCalculator,
		logger:         logger,
		httpClient: &http.Client{
			Timeout: config.ProviderConfig.Timeout,
		},
		rateLimits: &types.RateLimitInfo{
			RequestsPerMinute: config.ProviderConfig.RateLimit,
			ResetTime:         time.Now().Add(time.Minute),
		},
	}
}

// GetName returns the provider name
func (p *ZhipuProvider) GetName() string {
	return p.config.ProviderConfig.Name
}

// GetType returns the provider type
func (p *ZhipuProvider) GetType() string {
	return "zhipu"
}

// Call implements the Provider interface by wrapping ChatCompletion
func (p *ZhipuProvider) Call(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return p.ChatCompletion(ctx, req)
}

// GetConfig returns the provider configuration
func (p *ZhipuProvider) GetConfig() *types.ProviderConfig {
	return p.config.ProviderConfig
}

// ValidateConfig validates the provider configuration
func (p *ZhipuProvider) ValidateConfig(config *types.ProviderConfig) error {
	if config.APIKey == "" {
		return fmt.Errorf("Zhipu API key is required")
	}

	if config.Type != "zhipu" {
		return fmt.Errorf("invalid provider type: expected 'zhipu', got '%s'", config.Type)
	}

	return nil
}

// ChatCompletion sends a chat completion request to Zhipu GLM with production features
func (p *ZhipuProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Ensure valid credentials
	if err := p.ensureValidCredentials(); err != nil {
		return nil, fmt.Errorf("credential validation failed: %w", err)
	}

	// Estimate cost before request
	costEstimate, err := p.costCalculator.EstimateRequestCost(req)
	if err != nil {
		return nil, fmt.Errorf("cost estimation failed: %w", err)
	}

	// Validate cost limits (inline validation)
	if costEstimate.EstimatedCost > 1.0 { // $1 limit per request
		return nil, fmt.Errorf("estimated cost $%.4f exceeds limit", costEstimate.EstimatedCost)
	}

	// Execute with retry
	var resp *types.ChatCompletionResponse
	retryOp := func(ctx context.Context, attempt int) error {
		var err error
		resp, err = p.executeAPIRequest(ctx, req)
		return err
	}

	err = p.retryManager.ExecuteWithRetry(ctx, retryOp)
	if err != nil {
		return nil, err
	}

	// Calculate actual cost (for monitoring/logging purposes)
	if actualCost, err := p.costCalculator.CalculateActualCost(req, resp); err == nil {
		p.logger.WithField("actual_cost", actualCost.TotalCost).Info("Cost calculated for Zhipu request")
	}

	return resp, nil
}

// ChatCompletionStream handles streaming chat completion requests
func (p *ZhipuProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest, callback func(string, bool)) error {
	// Ensure valid credentials
	if err := p.ensureValidCredentials(); err != nil {
		return fmt.Errorf("credential validation failed: %w", err)
	}

	// Convert request to Zhipu format with streaming enabled
	zhipuReq := p.convertRequest(req)
	zhipuReq.Stream = true

	// Serialize request
	reqBody, err := json.Marshal(zhipuReq)
	if err != nil {
		return fmt.Errorf("failed to marshal stream request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.ProviderConfig.BaseURL+"/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create stream request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.secureConfig.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	p.logger.WithField("model", req.Model).Info("Starting streaming request to Zhipu GLM")

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("streaming HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("streaming request failed with status %d: %s", resp.StatusCode, string(body))
	}

		// Read streaming response
	scanner := bufio.NewScanner(resp.Body)
	p.logger.Info("Starting to read streaming response")
	
	for scanner.Scan() {
		line := scanner.Text()
		p.logger.WithField("raw_line", line).Debug("Received stream line")
		
		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Handle SSE format - 智谱AI使用 "data:" 而不是 "data: "
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			p.logger.WithField("data", data).Debug("Processing SSE data")
			
			// Check for end signal
			if data == "[DONE]" {
				p.logger.Info("Received stream completion signal")
				callback("", true) // Signal completion
				break
			}

			// Parse JSON chunk
			var chunk zhipuStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				p.logger.WithError(err).WithField("data", data).Warn("Failed to parse stream chunk")
				continue
			}

			p.logger.WithField("chunk", chunk).Debug("Parsed stream chunk")

			// Check if this is the final chunk (done=true or finish_reason present)
			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				
				// Send content if available
				if choice.Delta.Content != "" {
					content := choice.Delta.Content
					p.logger.WithField("content", content).Debug("Sending content to callback")
					callback(content, false)
				}
				
				// Check for completion
				if choice.FinishReason != nil && *choice.FinishReason != "" {
					p.logger.WithField("finish_reason", *choice.FinishReason).Info("Stream completed with finish reason")
					callback("", true)
					break
				}
			} else {
				p.logger.Debug("No choices in chunk")
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	p.logger.Info("Streaming request completed")
	return nil
}

// executeAPIRequest performs a single API request attempt
func (p *ZhipuProvider) executeAPIRequest(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Convert request to Zhipu format
	zhipuReq := p.convertRequest(req)

	// Serialize request
	reqBody, err := json.Marshal(zhipuReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.ProviderConfig.BaseURL+"/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers with secure API key
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.secureConfig.APIKey)

	p.logger.WithField("model", req.Model).Info("Sending request to Zhipu GLM")

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, retry.ClassifyError(err, "zhipu", "network_error")
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Update rate limits from headers
	p.updateRateLimits(resp.Header)

	// Handle errors with classification
	if resp.StatusCode != http.StatusOK {
		var errorResp zhipuErrorResponse
		if err := json.Unmarshal(respBody, &errorResp); err == nil {
			return nil, retry.ClassifyZhipuError(resp.StatusCode, errorResp.Error.Code, errorResp.Error.Message)
		}
		return nil, retry.ClassifyHTTPError(resp.StatusCode, string(respBody))
	}

	// Parse response
	var zhipuResp zhipuResponse
	if err := json.Unmarshal(respBody, &zhipuResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	p.logger.WithField("tokens", zhipuResp.Usage.TotalTokens).Info("Zhipu GLM request completed")

	// Convert response to standard format
	return p.convertResponse(&zhipuResp), nil
}

// ensureValidCredentials ensures API key is valid and loaded
func (p *ZhipuProvider) ensureValidCredentials() error {
	if p.secureConfig.APIKey == "" {
		// Try to load from environment
		apiKey := os.Getenv("ZHIPU_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("Zhipu API key not found in environment variable ZHIPU_API_KEY")
		}
		p.secureConfig.APIKey = apiKey
	}
	return nil
}

// HealthCheck performs a health check
func (p *ZhipuProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	start := time.Now()

	// Simple health check request
	req := &types.ChatCompletionRequest{
		Model: "glm-4.5",
		Messages: []types.Message{
			{Role: "user", Content: "ping"},
		},
	}

	_, err := p.ChatCompletion(ctx, req)

	status := &types.HealthStatus{
		IsHealthy:    err == nil,
		LastChecked:  time.Now(),
		ResponseTime: time.Since(start),
		Endpoint:     p.config.ProviderConfig.BaseURL,
	}

	if err != nil {
		status.ErrorMessage = err.Error()
	}

	return status, nil
}

// GetModels returns supported models
func (p *ZhipuProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	// Return static model list for Zhipu
	models := []*types.Model{
		{
			Name:        "glm-4-flash",
			DisplayName: "GLM-4-Flash",
			Description: "智谱最新快速模型",
		},
		{
			Name:        "glm-4",
			DisplayName: "GLM-4",
			Description: "智谱GLM-4标准模型",
		},
	}
	return models, nil
}

// EstimateCost estimates the cost for a request
func (p *ZhipuProvider) EstimateCost(req *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	// Simple token estimation
	inputTokens := len(req.Messages) * 10 // Rough estimate
	return &types.CostEstimate{
		InputTokens:  inputTokens,
		OutputTokens: 100, // Default estimate
		TotalTokens:  inputTokens + 100,
		InputCost:    float64(inputTokens) * 0.0001 / 1000,
		OutputCost:   100 * 0.0002 / 1000,
		TotalCost:    (float64(inputTokens)*0.0001 + 100*0.0002) / 1000,
		Currency:     "CNY",
	}, nil
}

// GetRateLimit returns current rate limit info
func (p *ZhipuProvider) GetRateLimit() *types.RateLimitInfo {
	return p.rateLimits
}

// convertRequest converts standard request to Zhipu format
func (p *ZhipuProvider) convertRequest(req *types.ChatCompletionRequest) *zhipuRequest {
	stream := false
	if req.Stream != nil {
		stream = *req.Stream
	}

	zhipuReq := &zhipuRequest{
		Model:    req.Model,
		Messages: make([]zhipuMessage, len(req.Messages)),
		Stream:   stream,
	}

	// Use default model if not specified
	if zhipuReq.Model == "" {
		zhipuReq.Model = "glm-4.5"
	}

	// Convert messages
	for i, msg := range req.Messages {
		zhipuReq.Messages[i] = zhipuMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	return zhipuReq
}

// convertResponse converts Zhipu response to standard format
func (p *ZhipuProvider) convertResponse(resp *zhipuResponse) *types.ChatCompletionResponse {
	choices := make([]types.Choice, len(resp.Choices))
	for i, choice := range resp.Choices {
		finishReason := choice.FinishReason
		choices[i] = types.Choice{
			Index: choice.Index,
			Message: types.Message{
				Role:    choice.Message.Role,
				Content: choice.Message.Content,
			},
			FinishReason: &finishReason,
		}
	}

	return &types.ChatCompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: fmt.Sprintf("%d", resp.Created),
		Model:   resp.Model,
		Choices: choices,
		Usage: types.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		Provider: p.GetName(),
	}
}

// updateRateLimits updates rate limit information from response headers
func (p *ZhipuProvider) updateRateLimits(headers http.Header) {
	// Zhipu GLM specific rate limit headers
	if remaining := headers.Get("X-RateLimit-Remaining"); remaining != "" {
		p.rateLimits.ResetTime = time.Now().Add(time.Minute)
	}
}

// EstimateTokens estimates token count for a request
func (p *ZhipuProvider) EstimateTokens(req *types.ChatCompletionRequest) (*types.TokenEstimate, error) {
	// Simple token estimation for Zhipu
	inputTokens := 0
	for _, msg := range req.Messages {
		inputTokens += len(msg.Content) / 4 // Rough estimation
	}

	return &types.TokenEstimate{
		InputTokens:  inputTokens,
		OutputTokens: 100, // Default estimate
	}, nil
}

// CalculateActualCost calculates actual cost based on response
func (p *ZhipuProvider) CalculateActualCost(req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse) (*types.CostBreakdown, error) {
	// Calculate based on actual usage
	inputCost := float64(resp.Usage.PromptTokens) * 0.0001 / 1000 // CNY per 1k tokens
	outputCost := float64(resp.Usage.CompletionTokens) * 0.0002 / 1000

	return &types.CostBreakdown{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		InputCost:    inputCost,
		OutputCost:   outputCost,
		TotalCost:    inputCost + outputCost,
		Currency:     "CNY",
		Model:        resp.Model,
		Provider:     p.GetName(),
		Timestamp:    time.Now(),
	}, nil
}

// GetProviderMetrics returns provider-specific metrics
func (p *ZhipuProvider) GetProviderMetrics() map[string]interface{} {
	return map[string]interface{}{
		"provider":        p.GetName(),
		"type":            p.GetType(),
		"base_url":        p.config.ProviderConfig.BaseURL,
		"timeout":         p.config.ProviderConfig.Timeout.String(),
		"rate_limit":      p.rateLimits.RequestsPerMinute,
		"reset_time":      p.rateLimits.ResetTime,
		"retry_available": true,
		"cost_tracking":   true,
		"secure_config":   true,
	}
}

// GetSupportedModels returns detailed supported models
func (p *ZhipuProvider) GetSupportedModels() []*types.Model {
	return []*types.Model{
		{
			Name:               "glm-4.5",
			DisplayName:        "GLM-4.5",
			Description:        "智谱GLM-4.5标准模型，性能优异",
			ContextLength:      8192,
			CostPerInputToken:  0.0001,
			CostPerOutputToken: 0.0002,
			IsEnabled:          true,
		},
		{
			Name:               "glm-4.5v",
			DisplayName:        "GLM-4.5V",
			Description:        "智谱GLM-4.5V多模态模型，支持视觉理解",
			ContextLength:      8192,
			CostPerInputToken:  0.0003,
			CostPerOutputToken: 0.0006,
			IsEnabled:          true,
		},
		{
			Name:               "glm-4.5-air",
			DisplayName:        "GLM-4.5-Air",
			Description:        "智谱GLM-4.5-Air轻量级模型，响应快速",
			ContextLength:      8192,
			CostPerInputToken:  0.0001,
			CostPerOutputToken: 0.0002,
			IsEnabled:          true,
		},
	}
}

// GetPricingInfo returns pricing information for the provider
func (p *ZhipuProvider) GetPricingInfo() map[string]interface{} {
	return map[string]interface{}{
		"provider":     p.GetName(),
		"currency":     "CNY",
		"pricing_unit": "per_1k_tokens",
		"models":       p.GetSupportedModels(),
		"free_tier":    "新用户有免费额度",
		"billing_info": "按使用量计费",
	}
}
