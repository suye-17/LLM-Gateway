// Package providers implements the Claude (Anthropic) provider adapter
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

// ClaudeProvider implements the Provider interface for Anthropic Claude
type ClaudeProvider struct {
	config         *types.ProductionConfig
	secureConfig   *types.SecureConfig
	logger         *utils.Logger
	httpClient     *http.Client
	rateLimits     *types.RateLimitInfo
	retryManager   retry.RetryManagerInterface
	costCalculator *cost.CostCalculator
	configManager  config.ConfigurationManager
}

// Claude API structures
type claudeRequest struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	Messages      []claudeMessage `json:"messages"`
	System        string          `json:"system,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	TopK          *int            `json:"top_k,omitempty"`
	Stream        *bool           `json:"stream,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	ID           string       `json:"id"`
	Type         string       `json:"type"`
	Role         string       `json:"role"`
	Content      []claudeText `json:"content"`
	Model        string       `json:"model"`
	StopReason   *string      `json:"stop_reason"`
	StopSequence *string      `json:"stop_sequence"`
	Usage        claudeUsage  `json:"usage"`
}

type claudeText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeErrorResponse struct {
	Type  string      `json:"type"`
	Error claudeError `json:"error"`
}

type claudeError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Claude model pricing (USD per 1M tokens)
var claudePricing = map[string]struct {
	InputPrice  float64
	OutputPrice float64
}{
	"claude-3-opus-20240229":   {15.0, 75.0},
	"claude-3-sonnet-20240229": {3.0, 15.0},
	"claude-3-haiku-20240307":  {0.25, 1.25},
	"claude-2.1":               {8.0, 24.0},
	"claude-2.0":               {8.0, 24.0},
	"claude-instant-1.2":       {0.8, 2.4},
}

// NewClaudeProvider creates a new Claude provider (backward compatible)
func NewClaudeProvider(baseConfig *types.ProviderConfig, logger *utils.Logger) *ClaudeProvider {
	// Convert to production config for compatibility
	prodConfig := types.NewProductionConfig(baseConfig)

	if prodConfig.BaseURL == "" {
		prodConfig.BaseURL = "https://api.anthropic.com/v1"
	}

	if prodConfig.Timeout == 0 {
		prodConfig.Timeout = 60 * time.Second
	}

	return &ClaudeProvider{
		config: prodConfig,
		logger: logger,
		httpClient: &http.Client{
			Timeout: prodConfig.Timeout,
		},
		rateLimits: &types.RateLimitInfo{
			RequestsPerMinute: prodConfig.RateLimit,
			ResetTime:         time.Now().Add(time.Minute),
		},
	}
}

// GetName returns the provider name
func (p *ClaudeProvider) GetName() string {
	return p.config.Name
}

// GetType returns the provider type
func (p *ClaudeProvider) GetType() string {
	return "anthropic"
}

// Call implements the Provider interface by wrapping ChatCompletion
func (p *ClaudeProvider) Call(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return p.ChatCompletion(ctx, req)
}

// GetConfig returns the provider configuration
func (p *ClaudeProvider) GetConfig() *types.ProviderConfig {
	return p.config.ProviderConfig
}

// ValidateConfig validates the provider configuration
func (p *ClaudeProvider) ValidateConfig(config *types.ProviderConfig) error {
	if config.APIKey == "" {
		return fmt.Errorf("Anthropic API key is required")
	}

	if config.Type != "anthropic" {
		return fmt.Errorf("invalid provider type: expected 'anthropic', got '%s'", config.Type)
	}

	return nil
}

// ChatCompletion sends a chat completion request to Claude
func (p *ClaudeProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Convert request to Claude format
	claudeReq := p.convertRequest(req)

	// Serialize request
	reqBody, err := json.Marshal(claudeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/messages", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("User-Agent", "LLM-Gateway/2.0")

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{
			Provider:  p.GetName(),
			Operation: "ChatCompletion",
			Message:   fmt.Sprintf("HTTP request failed: %v", err),
			Retryable: true,
		}
	}
	defer resp.Body.Close()

	// Update rate limits from headers
	p.updateRateLimits(resp.Header)

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle errors
	if resp.StatusCode != http.StatusOK {
		var errorResp claudeErrorResponse
		if err := json.Unmarshal(respBody, &errorResp); err == nil {
			return nil, &ProviderError{
				Provider:   p.GetName(),
				Operation:  "ChatCompletion",
				StatusCode: resp.StatusCode,
				Message:    errorResp.Error.Message,
				Retryable:  resp.StatusCode >= 500,
			}
		}
		return nil, &ProviderError{
			Provider:   p.GetName(),
			Operation:  "ChatCompletion",
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
			Retryable:  resp.StatusCode >= 500,
		}
	}

	// Parse response
	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert response to standard format
	return p.convertResponse(&claudeResp), nil
}

// HealthCheck performs a health check
func (p *ClaudeProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	start := time.Now()

	// Create a simple test request
	testReq := &claudeRequest{
		Model:     "claude-3-haiku-20240307",
		MaxTokens: 10,
		Messages: []claudeMessage{
			{Role: "user", Content: "Hi"},
		},
	}

	reqBody, err := json.Marshal(testReq)
	if err != nil {
		return &types.HealthStatus{
			IsHealthy:    false,
			ResponseTime: time.Since(start),
			ErrorMessage: fmt.Sprintf("Failed to create health check request: %v", err),
			LastChecked:  time.Now(),
			Endpoint:     p.config.BaseURL,
		}, nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/messages", bytes.NewBuffer(reqBody))
	if err != nil {
		return &types.HealthStatus{
			IsHealthy:    false,
			ResponseTime: time.Since(start),
			ErrorMessage: fmt.Sprintf("Failed to create health check request: %v", err),
			LastChecked:  time.Now(),
			Endpoint:     p.config.BaseURL,
		}, nil
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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
func (p *ClaudeProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	models := []*types.Model{
		{
			Name:               "claude-3-opus-20240229",
			DisplayName:        "Claude 3 Opus",
			Description:        "Most powerful model for highly complex tasks",
			ContextLength:      200000,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.000015,
			CostPerOutputToken: 0.000075,
			IsEnabled:          true,
		},
		{
			Name:               "claude-3-sonnet-20240229",
			DisplayName:        "Claude 3 Sonnet",
			Description:        "Balance of intelligence and speed for enterprise workloads",
			ContextLength:      200000,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.000003,
			CostPerOutputToken: 0.000015,
			IsEnabled:          true,
		},
		{
			Name:               "claude-3-haiku-20240307",
			DisplayName:        "Claude 3 Haiku",
			Description:        "Fastest and most compact model for near-instant responsiveness",
			ContextLength:      200000,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.00000025,
			CostPerOutputToken: 0.00000125,
			IsEnabled:          true,
		},
	}

	return models, nil
}

// EstimateCost estimates the cost for a request
func (p *ClaudeProvider) EstimateCost(req *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	pricing, exists := claudePricing[req.Model]
	if !exists {
		// Default to Claude 3 Haiku pricing
		pricing = claudePricing["claude-3-haiku-20240307"]
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

	inputCost := float64(inputTokens) * pricing.InputPrice / 1000000
	outputCost := float64(maxTokens) * pricing.OutputPrice / 1000000
	totalCost := inputCost + outputCost

	return &types.CostEstimate{
		InputTokens:   inputTokens,
		OutputTokens:  maxTokens,
		TotalTokens:   inputTokens + maxTokens,
		InputCost:     inputCost,
		OutputCost:    outputCost,
		TotalCost:     totalCost,
		Currency:      "USD",
		PricePerToken: (pricing.InputPrice + pricing.OutputPrice) / 2000000,
	}, nil
}

// GetRateLimit returns current rate limit information
func (p *ClaudeProvider) GetRateLimit() *types.RateLimitInfo {
	return p.rateLimits
}

// convertRequest converts standard request to Claude format
func (p *ClaudeProvider) convertRequest(req *types.ChatCompletionRequest) *claudeRequest {
	claudeReq := &claudeRequest{
		Model:       req.Model,
		MaxTokens:   1000, // Claude requires max_tokens
		Messages:    make([]claudeMessage, 0),
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}

	if req.MaxTokens != nil {
		claudeReq.MaxTokens = *req.MaxTokens
	}

	// Convert stop sequences
	if req.Stop != nil {
		switch stop := req.Stop.(type) {
		case string:
			claudeReq.StopSequences = []string{stop}
		case []string:
			claudeReq.StopSequences = stop
		case []interface{}:
			stopSeqs := make([]string, 0, len(stop))
			for _, s := range stop {
				if str, ok := s.(string); ok {
					stopSeqs = append(stopSeqs, str)
				}
			}
			claudeReq.StopSequences = stopSeqs
		}
	}

	// Convert messages - Claude has different role handling
	var systemMessage string
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			// Claude handles system messages separately
			systemMessage = msg.Content
		} else {
			claudeReq.Messages = append(claudeReq.Messages, claudeMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	if systemMessage != "" {
		claudeReq.System = systemMessage
	}

	return claudeReq
}

// convertResponse converts Claude response to standard format
func (p *ClaudeProvider) convertResponse(resp *claudeResponse) *types.ChatCompletionResponse {
	// Extract text content from Claude's content array
	var content string
	for _, c := range resp.Content {
		if c.Type == "text" {
			content = c.Text
			break
		}
	}

	choices := []types.Choice{
		{
			Index: 0,
			Message: types.Message{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: resp.StopReason,
		},
	}

	return &types.ChatCompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Format(time.RFC3339),
		Model:   resp.Model,
		Choices: choices,
		Usage: types.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
		Provider:  p.GetName(),
		LatencyMs: 0, // Will be set by caller
	}
}

// updateRateLimits updates rate limit information from response headers
func (p *ClaudeProvider) updateRateLimits(headers http.Header) {
	if remaining := headers.Get("anthropic-ratelimit-requests-remaining"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			p.rateLimits.RemainingRequests = val
		}
	}

	if remaining := headers.Get("anthropic-ratelimit-tokens-remaining"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			p.rateLimits.RemainingTokens = val
		}
	}

	if reset := headers.Get("anthropic-ratelimit-requests-reset"); reset != "" {
		if resetTime, err := time.Parse(time.RFC3339, reset); err == nil {
			p.rateLimits.ResetTime = resetTime
		}
	}
}
