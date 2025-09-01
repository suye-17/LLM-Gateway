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

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// OpenAIProvider implements the Provider interface for OpenAI
type OpenAIProvider struct {
	config     *types.ProviderConfig
	logger     *utils.Logger
	httpClient *http.Client
	rateLimits *types.RateLimitInfo
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

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(config *types.ProviderConfig, logger *utils.Logger) *OpenAIProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}

	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}

	return &OpenAIProvider{
		config: config,
		logger: logger,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		rateLimits: &types.RateLimitInfo{
			RequestsPerMinute: config.RateLimit,
			ResetTime:         time.Now().Add(time.Minute),
		},
	}
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
	return p.config
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

// ChatCompletion sends a chat completion request to OpenAI
func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Convert request to OpenAI format
	openAIReq := p.convertRequest(req)

	// Serialize request
	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
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
		var errorResp openAIErrorResponse
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
	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert response to standard format
	return p.convertResponse(&openAIResp), nil
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
