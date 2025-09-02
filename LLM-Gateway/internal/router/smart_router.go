// Package router implements the intelligent routing system with strategy pattern
package router

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/llm-gateway/gateway/internal/router/strategies"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

var (
	ErrNoAvailableProvider = errors.New("no available providers")
	ErrInvalidStrategy     = errors.New("invalid routing strategy")
	ErrProviderNotFound    = errors.New("provider not found")
)

// SmartRouter implements intelligent routing with pluggable strategies
type SmartRouter struct {
	config           *SmartRouterConfig
	strategy         strategies.LoadBalanceStrategy
	healthChecker    HealthChecker
	metricsCollector MetricsCollector
	providers        map[string]types.Provider
	logger           *utils.Logger
	mutex            sync.RWMutex

	// Runtime state
	started bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewSmartRouter creates a new intelligent router instance
func NewSmartRouter(config *SmartRouterConfig, logger *utils.Logger) (*SmartRouter, error) {
	if config == nil {
		config = DefaultSmartRouterConfig()
	}

	if err := config.ValidateConfig(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	router := &SmartRouter{
		config:    config.Clone(),
		providers: make(map[string]types.Provider),
		logger:    logger,
		started:   false,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Initialize components
	if err := router.initializeComponents(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	return router, nil
}

// initializeComponents initializes the router components
func (sr *SmartRouter) initializeComponents() error {
	var err error

	// Initialize strategy
	if sr.strategy, err = sr.createStrategy(sr.config.Strategy); err != nil {
		return fmt.Errorf("failed to create strategy: %w", err)
	}

	// Initialize health checker
	if sr.healthChecker, err = NewHealthChecker(DefaultHealthCheckConfig(), sr.logger); err != nil {
		return fmt.Errorf("failed to create health checker: %w", err)
	}

	// Initialize metrics collector if enabled
	if sr.config.MetricsEnabled {
		if sr.metricsCollector, err = NewMetricsCollector(); err != nil {
			return fmt.Errorf("failed to create metrics collector: %w", err)
		}
	}

	return nil
}

// RouteRequest routes a request to the best available provider
func (sr *SmartRouter) RouteRequest(ctx context.Context, req *types.Request) (*SmartRoutingResult, error) {
	startTime := time.Now()

	sr.mutex.RLock()
	providers := sr.getAvailableProviders()
	sr.mutex.RUnlock()

	if len(providers) == 0 {
		if sr.metricsCollector != nil {
			sr.metricsCollector.RecordRouting("", time.Since(startTime), false)
		}
		return nil, ErrNoAvailableProvider
	}

	// Get healthy providers
	healthyProviders := sr.healthChecker.GetHealthyProviders()
	if len(healthyProviders) == 0 {
		// Fall back to all providers if none are healthy
		healthyProviders = sr.convertToProviderSlice(providers)
		sr.logger.Warn("No healthy providers available, falling back to all providers")
	}

	// Select provider using strategy
	strategyStart := time.Now()
	selectedProvider, err := sr.strategy.SelectProvider(healthyProviders, req)
	strategyLatency := time.Since(strategyStart)

	if err != nil {
		if sr.metricsCollector != nil {
			sr.metricsCollector.RecordRouting("", time.Since(startTime), false)
			sr.metricsCollector.RecordStrategy(sr.strategy.GetStrategyName(), strategyLatency)
		}
		return nil, fmt.Errorf("strategy selection failed: %w", err)
	}

	// Record metrics
	totalLatency := time.Since(startTime)
	if sr.metricsCollector != nil {
		sr.metricsCollector.RecordRouting((*selectedProvider).GetName(), totalLatency, true)
		sr.metricsCollector.RecordStrategy(sr.strategy.GetStrategyName(), strategyLatency)
	}

	// Create routing result
	result := &SmartRoutingResult{
		Provider:      *selectedProvider,
		ProviderName:  (*selectedProvider).GetName(),
		Reason:        fmt.Sprintf("Selected by %s strategy", sr.strategy.GetStrategyName()),
		Attempts:      1,
		BackupUsed:    len(healthyProviders) < len(providers),
		Strategy:      sr.strategy.GetStrategyName(),
		LoadFactor:    sr.calculateLoadFactor(*selectedProvider),
		SelectionTime: totalLatency,
	}

	return result, nil
}

// AddProvider adds a new provider to the router
func (sr *SmartRouter) AddProvider(provider types.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	sr.mutex.Lock()
	defer sr.mutex.Unlock()

	providerName := provider.GetName()
	if _, exists := sr.providers[providerName]; exists {
		return fmt.Errorf("provider %s already exists", providerName)
	}

	sr.providers[providerName] = provider
	sr.logger.Info(fmt.Sprintf("Added provider: %s", providerName))

	// Add to health checker
	if err := sr.healthChecker.AddProvider(&provider); err != nil {
		sr.logger.Error(fmt.Sprintf("Failed to add provider to health checker: %v", err))
	}

	return nil
}

// RemoveProvider removes a provider from the router
func (sr *SmartRouter) RemoveProvider(providerID string) error {
	sr.mutex.Lock()
	defer sr.mutex.Unlock()

	if _, exists := sr.providers[providerID]; !exists {
		return ErrProviderNotFound
	}

	delete(sr.providers, providerID)
	sr.logger.Info(fmt.Sprintf("Removed provider: %s", providerID))

	return nil
}

// UpdateProviderWeight updates the weight of a specific provider
func (sr *SmartRouter) UpdateProviderWeight(providerID string, weight int) error {
	if weight < 0 {
		return fmt.Errorf("weight cannot be negative")
	}

	sr.mutex.Lock()
	defer sr.mutex.Unlock()

	if _, exists := sr.providers[providerID]; !exists {
		return ErrProviderNotFound
	}

	sr.config.Weights[providerID] = weight

	// Update strategy weights
	if err := sr.strategy.UpdateWeights(sr.config.Weights); err != nil {
		return fmt.Errorf("failed to update strategy weights: %w", err)
	}

	sr.logger.Info(fmt.Sprintf("Updated weight for provider %s to %d", providerID, weight))

	return nil
}

// UpdateConfig updates the router configuration
func (sr *SmartRouter) UpdateConfig(config *SmartRouterConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if err := config.ValidateConfig(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	sr.mutex.Lock()
	defer sr.mutex.Unlock()

	// Check if strategy changed
	if config.Strategy != sr.config.Strategy {
		newStrategy, err := sr.createStrategy(config.Strategy)
		if err != nil {
			return fmt.Errorf("failed to create new strategy: %w", err)
		}
		sr.strategy = newStrategy
		sr.logger.Info(fmt.Sprintf("Changed strategy to: %s", config.Strategy))
	}

	// Update configuration
	sr.config = config.Clone()

	// Update strategy weights
	if err := sr.strategy.UpdateWeights(sr.config.Weights); err != nil {
		return fmt.Errorf("failed to update strategy weights: %w", err)
	}

	sr.logger.Info("Router configuration updated")

	return nil
}

// GetMetrics returns router metrics
func (sr *SmartRouter) GetMetrics() *RoutingMetrics {
	if sr.metricsCollector == nil {
		return &RoutingMetrics{}
	}
	return sr.metricsCollector.GetRoutingMetrics()
}

// GetHealthStatus returns health status of all providers
func (sr *SmartRouter) GetHealthStatus() map[string]*HealthResult {
	return sr.healthChecker.GetAllHealthResults()
}

// Start starts the router and its components
func (sr *SmartRouter) Start() error {
	sr.mutex.Lock()
	defer sr.mutex.Unlock()

	if sr.started {
		return fmt.Errorf("router already started")
	}

	// Start health checker
	if err := sr.healthChecker.StartHealthCheck(); err != nil {
		return fmt.Errorf("failed to start health checker: %w", err)
	}

	sr.started = true
	sr.logger.Info("Smart router started")

	return nil
}

// Stop stops the router and its components
func (sr *SmartRouter) Stop() error {
	sr.mutex.Lock()
	defer sr.mutex.Unlock()

	if !sr.started {
		return nil
	}

	// Stop health checker
	if err := sr.healthChecker.StopHealthCheck(); err != nil {
		sr.logger.Error(fmt.Sprintf("Failed to stop health checker: %v", err))
	}

	// Cancel context
	sr.cancel()

	sr.started = false
	sr.logger.Info("Smart router stopped")

	return nil
}

// getAvailableProviders returns all registered providers
func (sr *SmartRouter) getAvailableProviders() []types.Provider {
	providers := make([]types.Provider, 0, len(sr.providers))
	for _, provider := range sr.providers {
		providers = append(providers, provider)
	}
	return providers
}

// convertToProviderSlice converts a slice of Provider interfaces to *types.Provider
func (sr *SmartRouter) convertToProviderSlice(providers []types.Provider) []*types.Provider {
	result := make([]*types.Provider, len(providers))
	for i, provider := range providers {
		result[i] = &provider
	}
	return result
}

// calculateLoadFactor calculates the load factor for a provider
func (sr *SmartRouter) calculateLoadFactor(provider types.Provider) float64 {
	if sr.metricsCollector == nil {
		return 0.0
	}

	metrics := sr.metricsCollector.GetProviderMetrics()
	providerMetrics, exists := metrics[provider.GetName()]
	if !exists {
		return 0.0
	}

	if providerMetrics.RequestCount == 0 {
		return 0.0
	}

	if providerMetrics.RequestCount+providerMetrics.SuccessCount == 0 {
		return 0.0
	}

	return float64(providerMetrics.RequestCount) / float64(providerMetrics.RequestCount+providerMetrics.SuccessCount)
}

// createStrategy creates a load balance strategy based on name
func (sr *SmartRouter) createStrategy(strategyName string) (strategies.LoadBalanceStrategy, error) {
	switch strategyName {
	case "round_robin":
		return strategies.NewRoundRobinStrategy(), nil
	case "weighted_round_robin":
		return strategies.NewWeightedRoundRobinStrategy(sr.config.Weights), nil
	case "least_connections":
		return strategies.NewLeastConnectionsStrategy(), nil
	case "health_based":
		return strategies.NewHealthBasedStrategy(), nil
	default:
		return nil, fmt.Errorf("unsupported strategy: %s", strategyName)
	}
}
