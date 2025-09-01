// Package providers implements the provider registry system
package providers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// DefaultRegistry implements the types.ProviderRegistry interface
type DefaultRegistry struct {
	providers map[string]types.Provider
	mu        sync.RWMutex
	logger    *utils.Logger
}

// NewDefaultRegistry creates a new provider registry
func NewDefaultRegistry(logger *utils.Logger) types.ProviderRegistry {
	return &DefaultRegistry{
		providers: make(map[string]types.Provider),
		logger:    logger,
	}
}

// RegisterProvider registers a new provider
func (r *DefaultRegistry) RegisterProvider(provider types.Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := provider.GetName()
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %s already registered", name)
	}

	r.providers[name] = provider
	r.logger.WithField("provider", name).Info("types.Provider registered successfully")

	return nil
}

// GetProvider returns a provider by name
func (r *DefaultRegistry) GetProvider(name string) (types.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", name)
	}

	return provider, nil
}

// GetProviders returns all registered providers
func (r *DefaultRegistry) GetProviders() []types.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]types.Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		providers = append(providers, provider)
	}

	return providers
}

// GetHealthyProviders returns only healthy providers
func (r *DefaultRegistry) GetHealthyProviders() []types.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var healthyProviders []types.Provider
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, provider := range r.providers {
		if health, err := provider.HealthCheck(ctx); err == nil && health.IsHealthy {
			healthyProviders = append(healthyProviders, provider)
		}
	}

	return healthyProviders
}

// GetProvidersByType returns providers of a specific type
func (r *DefaultRegistry) GetProvidersByType(providerType string) []types.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var providers []types.Provider
	for _, provider := range r.providers {
		if provider.GetType() == providerType {
			providers = append(providers, provider)
		}
	}

	return providers
}

// ProviderManager provides additional management capabilities
type ProviderManager struct {
	registry      types.ProviderRegistry
	healthChecker *HealthChecker
	logger        *utils.Logger
}

// NewProviderManager creates a new provider manager
func NewProviderManager(registry types.ProviderRegistry, logger *utils.Logger) *ProviderManager {
	return &ProviderManager{
		registry:      registry,
		healthChecker: NewHealthChecker(registry, logger),
		logger:        logger,
	}
}

// StartHealthChecking starts periodic health checking
func (m *ProviderManager) StartHealthChecking(interval time.Duration) {
	m.healthChecker.Start(interval)
}

// StopHealthChecking stops health checking
func (m *ProviderManager) StopHealthChecking() {
	m.healthChecker.Stop()
}

// GetProviderStatus returns the status of all providers
func (m *ProviderManager) GetProviderStatus() map[string]*types.HealthStatus {
	return m.healthChecker.GetAllStatus()
}

// HealthChecker manages health checking for all providers
type HealthChecker struct {
	registry  types.ProviderRegistry
	logger    *utils.Logger
	status    map[string]*types.HealthStatus
	statusMu  sync.RWMutex
	stopCh    chan struct{}
	stopped   bool
	stoppedMu sync.Mutex
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(registry types.ProviderRegistry, logger *utils.Logger) *HealthChecker {
	return &HealthChecker{
		registry: registry,
		logger:   logger,
		status:   make(map[string]*types.HealthStatus),
		stopCh:   make(chan struct{}),
	}
}

// Start begins periodic health checking
func (h *HealthChecker) Start(interval time.Duration) {
	h.stoppedMu.Lock()
	if h.stopped {
		h.stoppedMu.Unlock()
		return
	}
	h.stoppedMu.Unlock()

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()

		// Initial health check
		h.checkAllProviders()

		for {
			select {
			case <-ticker.C:
				h.checkAllProviders()
			case <-h.stopCh:
				return
			}
		}
	}()
}

// Stop stops health checking
func (h *HealthChecker) Stop() {
	h.stoppedMu.Lock()
	defer h.stoppedMu.Unlock()

	if !h.stopped {
		h.stopped = true
		close(h.stopCh)
	}
}

// checkAllProviders performs health checks on all providers
func (h *HealthChecker) checkAllProviders() {
	providers := h.registry.GetProviders()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for _, provider := range providers {
		wg.Add(1)
		go func(p types.Provider) {
			defer wg.Done()
			h.checkProvider(ctx, p)
		}(provider)
	}

	wg.Wait()
}

// checkProvider performs a health check on a single provider
func (h *HealthChecker) checkProvider(ctx context.Context, provider types.Provider) {
	start := time.Now()
	health, err := provider.HealthCheck(ctx)
	duration := time.Since(start)

	if err != nil {
		health = &types.HealthStatus{
			IsHealthy:    false,
			ResponseTime: duration,
			ErrorMessage: err.Error(),
			LastChecked:  time.Now(),
			Endpoint:     "unknown",
		}
	} else {
		health.LastChecked = time.Now()
		health.ResponseTime = duration
	}

	h.statusMu.Lock()
	h.status[provider.GetName()] = health
	h.statusMu.Unlock()

	if health.IsHealthy {
		h.logger.WithField("provider", provider.GetName()).
			WithField("healthy", health.IsHealthy).
			WithField("response_time", health.ResponseTime.Milliseconds()).
			Info("types.Provider health check completed")
	} else {
		h.logger.WithField("provider", provider.GetName()).
			WithField("healthy", health.IsHealthy).
			WithField("response_time", health.ResponseTime.Milliseconds()).
			Warn("types.Provider health check completed")
	}
}

// GetStatus returns the health status of a specific provider
func (h *HealthChecker) GetStatus(providerName string) (*types.HealthStatus, bool) {
	h.statusMu.RLock()
	defer h.statusMu.RUnlock()

	status, exists := h.status[providerName]
	return status, exists
}

// GetAllStatus returns the health status of all providers
func (h *HealthChecker) GetAllStatus() map[string]*types.HealthStatus {
	h.statusMu.RLock()
	defer h.statusMu.RUnlock()

	result := make(map[string]*types.HealthStatus)
	for name, status := range h.status {
		result[name] = status
	}

	return result
}
