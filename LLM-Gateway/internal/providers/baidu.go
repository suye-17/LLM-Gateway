// Package providers implements the Baidu Wenxin (ERNIE) provider adapter
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/llm-gateway/gateway/internal/config"
	"github.com/llm-gateway/gateway/pkg/cost"
	"github.com/llm-gateway/gateway/pkg/retry"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// BaiduProvider implements the Provider interface for Baidu Wenxin
type BaiduProvider struct {
	config         *types.ProductionConfig
	secureConfig   *types.SecureConfig
	logger         *utils.Logger
	httpClient     *http.Client
	rateLimits     *types.RateLimitInfo
	retryManager   retry.RetryManagerInterface
	costCalculator *cost.CostCalculator
	configManager  config.ConfigurationManager
	accessToken    string
	tokenExpiry    time.Time
}

// Baidu API structures
type baiduRequest struct {
	Messages       []baiduMessage `json:"messages"`
	Temperature    *float64       `json:"temperature,omitempty"`
	TopP           *float64       `json:"top_p,omitempty"`
	PenaltyScore   *float64       `json:"penalty_score,omitempty"`
	Stream         *bool          `json:"stream,omitempty"`
	System         string         `json:"system,omitempty"`
	Stop           []string       `json:"stop,omitempty"`
	DisableSearch  *bool          `json:"disable_search,omitempty"`
	EnableCitation *bool          `json:"enable_citation,omitempty"`
	UserID         string         `json:"user_id,omitempty"`
}

type baiduMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type baiduResponse struct {
	ID               string      `json:"id"`
	Object           string      `json:"object"`
	Created          int64       `json:"created"`
	SentenceID       int         `json:"sentence_id"`
	IsEnd            bool        `json:"is_end"`
	IsTruncated      bool        `json:"is_truncated"`
	Result           string      `json:"result"`
	NeedClearHistory bool        `json:"need_clear_history"`
	BanRound         int         `json:"ban_round"`
	Usage            baiduUsage  `json:"usage"`
	FunctionCall     interface{} `json:"function_call,omitempty"`
}

type baiduUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type baiduErrorResponse struct {
	Error     baiduError `json:"error"`
	ErrorCode int        `json:"error_code"`
	ErrorMsg  string     `json:"error_msg"`
}

type baiduError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type baiduTokenRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type baiduTokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// Baidu model pricing (estimated, RMB per 1K tokens)
var baiduPricing = map[string]struct {
	InputPrice  float64
	OutputPrice float64
}{
	"ernie-bot-turbo":    {0.008, 0.008},
	"ernie-bot":          {0.012, 0.012},
	"ernie-bot-4":        {0.120, 0.120},
	"ernie-3.5-8k":       {0.012, 0.012},
	"ernie-3.5-8k-0205":  {0.012, 0.012},
	"ernie-3.5-8k-1222":  {0.012, 0.012},
	"ernie-lite-8k-0922": {0.008, 0.008},
	"ernie-tiny-8k":      {0.001, 0.001},
}

// Model endpoint mappings
var baiduModelEndpoints = map[string]string{
	"ernie-bot-turbo":    "eb-instant",
	"ernie-bot":          "completions",
	"ernie-bot-4":        "completions_pro",
	"ernie-3.5-8k":       "ernie-3.5-8k",
	"ernie-3.5-8k-0205":  "ernie-3.5-8k-0205",
	"ernie-3.5-8k-1222":  "ernie-3.5-8k-1222",
	"ernie-lite-8k-0922": "ernie-lite-8k-0922",
	"ernie-tiny-8k":      "ernie-tiny-8k",
}

// NewBaiduProvider creates a new Baidu provider (backward compatible)
func NewBaiduProvider(baseConfig *types.ProviderConfig, logger *utils.Logger) *BaiduProvider {
	// Convert to production config for compatibility
	prodConfig := types.NewProductionConfig(baseConfig)

	if prodConfig.BaseURL == "" {
		prodConfig.BaseURL = "https://aip.baidubce.com"
	}

	if prodConfig.Timeout == 0 {
		prodConfig.Timeout = 60 * time.Second
	}

	return &BaiduProvider{
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
func (p *BaiduProvider) GetName() string {
	return p.config.Name
}

// GetType returns the provider type
func (p *BaiduProvider) GetType() string {
	return "baidu"
}

// Call implements the Provider interface by wrapping ChatCompletion
func (p *BaiduProvider) Call(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return p.ChatCompletion(ctx, req)
}

// GetConfig returns the provider configuration
func (p *BaiduProvider) GetConfig() *types.ProviderConfig {
	return p.config.ProviderConfig
}

// ValidateConfig validates the provider configuration
func (p *BaiduProvider) ValidateConfig(config *types.ProviderConfig) error {
	if config.APIKey == "" {
		return fmt.Errorf("Baidu API key (client_id) is required")
	}

	clientSecret, exists := config.CustomConfig["client_secret"]
	if !exists || clientSecret == "" {
		return fmt.Errorf("Baidu client_secret is required in custom_config")
	}

	if config.Type != "baidu" {
		return fmt.Errorf("invalid provider type: expected 'baidu', got '%s'", config.Type)
	}

	return nil
}

// ChatCompletion sends a chat completion request to Baidu
func (p *BaiduProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Ensure we have a valid access token
	if err := p.ensureAccessToken(ctx); err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Get endpoint for the model
	endpoint, exists := baiduModelEndpoints[req.Model]
	if !exists {
		endpoint = "completions" // default endpoint
	}

	// Convert request to Baidu format
	baiduReq := p.convertRequest(req)

	// Serialize request
	reqBody, err := json.Marshal(baiduReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	requestURL := fmt.Sprintf("%s/rpc/2.0/ai_custom/v1/wenxinworkshop/chat/%s?access_token=%s",
		p.config.BaseURL, endpoint, p.accessToken)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", requestURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
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
		var errorResp baiduErrorResponse
		if err := json.Unmarshal(respBody, &errorResp); err == nil {
			message := errorResp.ErrorMsg
			if errorResp.Error.Message != "" {
				message = errorResp.Error.Message
			}
			return nil, &ProviderError{
				Provider:   p.GetName(),
				Operation:  "ChatCompletion",
				StatusCode: resp.StatusCode,
				Message:    message,
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
	var baiduResp baiduResponse
	if err := json.Unmarshal(respBody, &baiduResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Convert response to standard format
	return p.convertResponse(&baiduResp, req.Model), nil
}

// HealthCheck performs a health check
func (p *BaiduProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	start := time.Now()

	// Check if we can get an access token
	if err := p.ensureAccessToken(ctx); err != nil {
		return &types.HealthStatus{
			IsHealthy:    false,
			ResponseTime: time.Since(start),
			ErrorMessage: fmt.Sprintf("Failed to get access token: %v", err),
			LastChecked:  time.Now(),
			Endpoint:     p.config.BaseURL,
		}, nil
	}

	// Create a simple test request
	testReq := &baiduRequest{
		Messages: []baiduMessage{
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

	requestURL := fmt.Sprintf("%s/rpc/2.0/ai_custom/v1/wenxinworkshop/chat/eb-instant?access_token=%s",
		p.config.BaseURL, p.accessToken)

	req, err := http.NewRequestWithContext(ctx, "POST", requestURL, bytes.NewBuffer(reqBody))
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
func (p *BaiduProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	models := []*types.Model{
		{
			Name:               "ernie-bot-4",
			DisplayName:        "ERNIE Bot 4.0",
			Description:        "百度自研的大语言模型，覆盖海量中文数据",
			ContextLength:      8192,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.00012,
			CostPerOutputToken: 0.00012,
			IsEnabled:          true,
		},
		{
			Name:               "ernie-3.5-8k",
			DisplayName:        "ERNIE 3.5 8K",
			Description:        "在ERNIE 3.5基础上增强了对话能力",
			ContextLength:      8192,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.000012,
			CostPerOutputToken: 0.000012,
			IsEnabled:          true,
		},
		{
			Name:               "ernie-bot-turbo",
			DisplayName:        "ERNIE Bot Turbo",
			Description:        "百度自研的高性能大语言模型",
			ContextLength:      8192,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.000008,
			CostPerOutputToken: 0.000008,
			IsEnabled:          true,
		},
		{
			Name:               "ernie-lite-8k-0922",
			DisplayName:        "ERNIE Lite 8K",
			Description:        "轻量化的ERNIE模型，响应速度快",
			ContextLength:      8192,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.000008,
			CostPerOutputToken: 0.000008,
			IsEnabled:          true,
		},
	}

	return models, nil
}

// EstimateCost estimates the cost for a request
func (p *BaiduProvider) EstimateCost(req *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	pricing, exists := baiduPricing[req.Model]
	if !exists {
		// Default to ERNIE Bot Turbo pricing
		pricing = baiduPricing["ernie-bot-turbo"]
	}

	// Rough token estimation (1 token ≈ 2 characters for Chinese)
	inputText := ""
	for _, msg := range req.Messages {
		inputText += msg.Content + " "
	}

	inputTokens := len([]rune(inputText)) / 2 // Use runes for Chinese character counting
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
		Currency:      "CNY",
		PricePerToken: (pricing.InputPrice + pricing.OutputPrice) / 2000,
	}, nil
}

// GetRateLimit returns current rate limit information
func (p *BaiduProvider) GetRateLimit() *types.RateLimitInfo {
	return p.rateLimits
}

// ensureAccessToken ensures we have a valid access token
func (p *BaiduProvider) ensureAccessToken(ctx context.Context) error {
	// Check if token is still valid
	if p.accessToken != "" && time.Now().Before(p.tokenExpiry) {
		return nil
	}

	// Get client secret from config
	clientSecret, exists := p.config.CustomConfig["client_secret"]
	if !exists {
		return fmt.Errorf("client_secret not found in config")
	}

	// Request new token
	tokenURL := "https://aip.baidubce.com/oauth/2.0/token"
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", p.config.APIKey)
	data.Set("client_secret", clientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}

	var tokenResp baiduTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return fmt.Errorf("failed to unmarshal token response: %w", err)
	}

	if tokenResp.Error != "" {
		return fmt.Errorf("token request error: %s - %s", tokenResp.Error, tokenResp.ErrorDescription)
	}

	p.accessToken = tokenResp.AccessToken
	p.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-300) * time.Second) // 5 minutes buffer

	return nil
}

// convertRequest converts standard request to Baidu format
func (p *BaiduProvider) convertRequest(req *types.ChatCompletionRequest) *baiduRequest {
	baiduReq := &baiduRequest{
		Messages:    make([]baiduMessage, 0),
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}

	// Convert stop sequences
	if req.Stop != nil {
		switch stop := req.Stop.(type) {
		case string:
			baiduReq.Stop = []string{stop}
		case []string:
			baiduReq.Stop = stop
		case []interface{}:
			stopSeqs := make([]string, 0, len(stop))
			for _, s := range stop {
				if str, ok := s.(string); ok {
					stopSeqs = append(stopSeqs, str)
				}
			}
			baiduReq.Stop = stopSeqs
		}
	}

	// Convert messages - Baidu handles system messages differently
	var systemMessage string
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemMessage = msg.Content
		} else {
			baiduReq.Messages = append(baiduReq.Messages, baiduMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	if systemMessage != "" {
		baiduReq.System = systemMessage
	}

	return baiduReq
}

// convertResponse converts Baidu response to standard format
func (p *BaiduProvider) convertResponse(resp *baiduResponse, model string) *types.ChatCompletionResponse {
	choices := []types.Choice{
		{
			Index: 0,
			Message: types.Message{
				Role:    "assistant",
				Content: resp.Result,
			},
			FinishReason: nil, // Baidu doesn't provide finish reason in the same format
		},
	}

	// Convert finish reason
	if resp.IsEnd {
		reason := "stop"
		choices[0].FinishReason = &reason
	}

	return &types.ChatCompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Format(time.RFC3339),
		Model:   model,
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
func (p *BaiduProvider) updateRateLimits(headers http.Header) {
	// Baidu doesn't provide rate limit headers in the same format
	// This is a placeholder for when they add such headers
	if remaining := headers.Get("x-ratelimit-remaining"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			p.rateLimits.RemainingRequests = val
		}
	}
}
