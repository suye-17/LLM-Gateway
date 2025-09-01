// Package router implements routing service management
package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/llm-gateway/gateway/internal/providers"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// Service manages the routing system
type Service struct {
	router           *Router
	providerRegistry types.ProviderRegistry
	providerManager  *providers.ProviderManager
	config           *RouterConfig
	logger           *utils.Logger
	mu               sync.RWMutex
	started          bool
}

// NewService creates a new routing service
func NewService(config *RouterConfig, registry types.ProviderRegistry, logger *utils.Logger) *Service {
	router := NewRouter(config, registry, logger)
	providerManager := providers.NewProviderManager(registry, logger)

	return &Service{
		router:           router,
		providerRegistry: registry,
		providerManager:  providerManager,
		config:           config,
		logger:           logger,
	}
}

// Start starts the routing service
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("routing service already started")
	}

	// Start provider health checking
	s.providerManager.StartHealthChecking(s.config.HealthCheckInterval)

	// Initialize default providers
	if err := s.initializeDefaultProviders(); err != nil {
		return fmt.Errorf("failed to initialize default providers: %w", err)
	}

	s.started = true
	s.logger.Info("Routing service started successfully")

	return nil
}

// Stop stops the routing service
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	// Stop provider health checking
	s.providerManager.StopHealthChecking()

	s.started = false
	s.logger.Info("Routing service stopped")

	return nil
}

// RouteRequest routes a chat completion request to the best provider
func (s *Service) RouteRequest(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Route the request
	routingResult, err := s.router.RouteRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to route request: %w", err)
	}

	s.logger.WithField("provider", routingResult.ProviderName).
		WithField("strategy", routingResult.Strategy).
		WithField("reason", routingResult.Reason).
		Info("Request routed to provider")

	// Execute the request
	start := time.Now()
	resp, err := routingResult.Provider.Call(ctx, req)
	latency := time.Since(start)

	if err != nil {
		// Record failure
		s.router.RecordFailure(routingResult.ProviderName, err)
		return nil, fmt.Errorf("provider request failed: %w", err)
	}

	// Record success
	s.router.RecordSuccess(routingResult.ProviderName, latency)

	// Add routing metadata to response
	resp.LatencyMs = latency.Milliseconds()
	resp.Provider = routingResult.ProviderName

	s.logger.WithField("provider", routingResult.ProviderName).
		WithField("latency_ms", latency.Milliseconds()).
		WithField("tokens", resp.Usage.TotalTokens).
		Info("Request completed successfully")

	return resp, nil
}

// GetProviderStats returns statistics for all providers
func (s *Service) GetProviderStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Routing stats
	stats["routing"] = s.router.GetStats()

	// Provider health stats
	stats["health"] = s.providerManager.GetProviderStatus()

	// Provider list
	providers := s.providerRegistry.GetProviders()
	providerInfo := make([]map[string]interface{}, len(providers))
	for i, provider := range providers {
		providerInfo[i] = map[string]interface{}{
			"name":       provider.GetName(),
			"type":       provider.GetType(),
			"rate_limit": provider.GetRateLimit(),
		}
	}
	stats["providers"] = providerInfo

	return stats
}

// UpdateConfig updates the routing configuration
func (s *Service) UpdateConfig(config *RouterConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	s.router.UpdateConfig(config)

	s.logger.WithField("strategy", config.Strategy).Info("Routing configuration updated")
	return nil
}

// AddProvider dynamically adds a new provider
func (s *Service) AddProvider(providerConfig *types.ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var provider types.Provider
	var err error

	// Create provider based on type
	switch providerConfig.Type {
	case "openai":
		provider = providers.NewOpenAIProvider(providerConfig, s.logger)
	case "anthropic":
		provider = providers.NewClaudeProvider(providerConfig, s.logger)
	case "baidu":
		provider = providers.NewBaiduProvider(providerConfig, s.logger)
	default:
		return fmt.Errorf("unsupported provider type: %s", providerConfig.Type)
	}

	// Note: Provider validation is now handled within each provider's implementation

	// Register the provider
	if err = s.providerRegistry.RegisterProvider(provider); err != nil {
		return fmt.Errorf("failed to register provider: %w", err)
	}

	s.logger.WithField("provider", providerConfig.Name).
		WithField("type", providerConfig.Type).
		Info("Provider added successfully")

	return nil
}

// RemoveProvider dynamically removes a provider
func (s *Service) RemoveProvider(providerName string) error {
	// Note: This would require extending the registry interface
	// For now, just log the request
	s.logger.WithField("provider", providerName).
		Info("Provider removal requested (not implemented)")

	return fmt.Errorf("provider removal not implemented yet")
}

// HealthCheck performs a health check on all providers
func (s *Service) HealthCheck(ctx context.Context) map[string]*types.HealthStatus {
	return s.providerManager.GetProviderStatus()
}

// GetAvailableModels returns all available models from all providers
func (s *Service) GetAvailableModels(ctx context.Context) map[string][]*types.Model {
	providers := s.providerRegistry.GetProviders()
	modelsByProvider := make(map[string][]*types.Model)

	for _, provider := range providers {
		models, err := provider.GetModels(ctx)
		if err != nil {
			s.logger.WithField("provider", provider.GetName()).
				WithField("error", err.Error()).
				Warn("Failed to get models from provider")
			continue
		}
		modelsByProvider[provider.GetName()] = models
	}

	return modelsByProvider
}

// EstimateCost estimates the cost for a request across all available providers
func (s *Service) EstimateCost(req *types.ChatCompletionRequest) map[string]*types.CostEstimate {
	providers := s.providerRegistry.GetProviders()
	estimates := make(map[string]*types.CostEstimate)

	for _, provider := range providers {
		if estimate, err := provider.EstimateCost(req); err == nil {
			estimates[provider.GetName()] = estimate
		}
	}

	return estimates
}

// initializeDefaultProviders sets up default providers if none are configured
func (s *Service) initializeDefaultProviders() error {
	existingProviders := s.providerRegistry.GetProviders()
	if len(existingProviders) > 0 {
		s.logger.WithField("count", len(existingProviders)).
			Info("Using existing provider configuration")
		return nil
	}

	s.logger.Info("No providers configured, adding mock provider for testing")

	// Add a mock provider for testing
	mockConfig := &types.ProviderConfig{
		Name:     "mock-provider",
		Type:     "mock",
		Enabled:  true,
		BaseURL:  "http://localhost:8080/mock",
		Priority: 1,
		Weight:   100,
		Timeout:  30 * time.Second,
		Models:   []string{"gpt-3.5-turbo", "gpt-4"},
	}

	// For now, we don't have a mock provider implementation
	// This would be useful for testing
	s.logger.WithField("config", mockConfig).
		Info("Mock provider configuration prepared (implementation pending)")

	return nil
}

// MockProvider implements a mock provider for testing
type MockProvider struct {
	config *types.ProviderConfig
	logger *utils.Logger
}

// NewMockProvider creates a new mock provider
func NewMockProvider(config *types.ProviderConfig, logger *utils.Logger) *MockProvider {
	return &MockProvider{
		config: config,
		logger: logger,
	}
}

// GetName returns the provider name
func (p *MockProvider) GetName() string {
	return p.config.Name
}

// GetType returns the provider type
func (p *MockProvider) GetType() string {
	return "mock"
}

// ValidateConfig validates the provider configuration
func (p *MockProvider) ValidateConfig(config *types.ProviderConfig) error {
	return nil // Mock provider accepts any config
}

// ChatCompletion simulates a chat completion request
func (p *MockProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	// Simulate processing time
	time.Sleep(100 * time.Millisecond)

	userID := ""
	if req.User != nil {
		userID = *req.User
	}

	response := &types.ChatCompletionResponse{
		ID:      utils.GenerateRequestID(),
		Object:  "chat.completion",
		Created: time.Now().Format(time.RFC3339),
		Model:   req.Model,
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.Message{
					Role:    "assistant",
					Content: fmt.Sprintf("Mock response from %s! Request ID: %s, User: %s", p.GetName(), utils.GenerateRequestID(), userID),
				},
				FinishReason: func() *string { s := "stop"; return &s }(),
			},
		},
		Usage: types.Usage{
			PromptTokens:     len(req.Messages) * 10,
			CompletionTokens: 25,
			TotalTokens:      len(req.Messages)*10 + 25,
		},
		Provider:  p.GetName(),
		LatencyMs: 100,
	}

	return response, nil
}

// Call implements the Provider interface by wrapping ChatCompletion
func (p *MockProvider) Call(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return p.ChatCompletion(ctx, req)
}

// HealthCheck performs a mock health check
func (p *MockProvider) HealthCheck(ctx context.Context) (*types.HealthStatus, error) {
	return &types.HealthStatus{
		IsHealthy:    true,
		ResponseTime: 50 * time.Millisecond,
		LastChecked:  time.Now(),
		Endpoint:     p.config.BaseURL,
	}, nil
}

// GetModels returns mock models
func (p *MockProvider) GetModels(ctx context.Context) ([]*types.Model, error) {
	return []*types.Model{
		{
			Name:               "mock-gpt-3.5",
			DisplayName:        "Mock GPT-3.5",
			Description:        "Mock model for testing",
			ContextLength:      4096,
			SupportedModes:     `["chat"]`,
			CostPerInputToken:  0.001,
			CostPerOutputToken: 0.002,
			IsEnabled:          true,
		},
	}, nil
}

// EstimateCost returns mock cost estimate
func (p *MockProvider) EstimateCost(req *types.ChatCompletionRequest) (*types.CostEstimate, error) {
	inputTokens := len(req.Messages) * 10
	maxTokens := 100
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	return &types.CostEstimate{
		InputTokens:   inputTokens,
		OutputTokens:  maxTokens,
		TotalTokens:   inputTokens + maxTokens,
		InputCost:     float64(inputTokens) * 0.001 / 1000,
		OutputCost:    float64(maxTokens) * 0.002 / 1000,
		TotalCost:     float64(inputTokens)*0.001/1000 + float64(maxTokens)*0.002/1000,
		Currency:      "USD",
		PricePerToken: 0.0015 / 1000,
	}, nil
}

// GetRateLimit returns mock rate limit info
func (p *MockProvider) GetRateLimit() *types.RateLimitInfo {
	return &types.RateLimitInfo{
		RequestsPerMinute: 1000,
		TokensPerMinute:   100000,
		RemainingRequests: 950,
		RemainingTokens:   95000,
		ResetTime:         time.Now().Add(time.Minute),
	}
}

// GetConfig returns the provider configuration
func (p *MockProvider) GetConfig() *types.ProviderConfig {
	return p.config
}
