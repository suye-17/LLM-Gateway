// Package router implements health checking functionality
package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// DefaultHealthChecker implements the HealthChecker interface
type DefaultHealthChecker struct {
	providers map[string]*types.Provider
	results   map[string]*HealthResult
	config    *HealthCheckConfig
	logger    *utils.Logger
	ticker    *time.Ticker
	ctx       context.Context
	cancel    context.CancelFunc
	mutex     sync.RWMutex

	// Health tracking
	failureCounts map[string]int
	successCounts map[string]int

	// Runtime state
	running bool
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(config *HealthCheckConfig, logger *utils.Logger) (HealthChecker, error) {
	if config == nil {
		config = DefaultHealthCheckConfig()
	}

	if err := config.ValidateHealthCheckConfig(); err != nil {
		return nil, fmt.Errorf("invalid health check config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	checker := &DefaultHealthChecker{
		providers:     make(map[string]*types.Provider),
		results:       make(map[string]*HealthResult),
		config:        config.Clone(),
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
		failureCounts: make(map[string]int),
		successCounts: make(map[string]int),
		running:       false,
	}

	return checker, nil
}

// StartHealthCheck starts the health checking routine
func (hc *DefaultHealthChecker) StartHealthCheck() error {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	if hc.running {
		return fmt.Errorf("health checker already running")
	}

	hc.ticker = time.NewTicker(hc.config.Interval)
	hc.running = true

	// Start the health check goroutine
	go hc.healthCheckLoop()

	hc.logger.Info(fmt.Sprintf("Health checker started with interval: %v", hc.config.Interval))

	return nil
}

// StopHealthCheck stops the health checking routine
func (hc *DefaultHealthChecker) StopHealthCheck() error {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	if !hc.running {
		return nil
	}

	hc.running = false

	if hc.ticker != nil {
		hc.ticker.Stop()
		hc.ticker = nil
	}

	hc.cancel()

	hc.logger.Info("Health checker stopped")

	return nil
}

// CheckProvider performs immediate health check on a provider
func (hc *DefaultHealthChecker) CheckProvider(providerID string) (*HealthResult, error) {
	hc.mutex.RLock()
	provider, exists := hc.providers[providerID]
	hc.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("provider %s not found", providerID)
	}

	return hc.performHealthCheck(providerID, provider)
}

// GetHealthyProviders returns list of healthy providers
func (hc *DefaultHealthChecker) GetHealthyProviders() []*types.Provider {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	var healthy []*types.Provider

	for providerID, result := range hc.results {
		if result.IsHealthy {
			if provider, exists := hc.providers[providerID]; exists {
				healthy = append(healthy, provider)
			}
		}
	}

	return healthy
}

// GetProviderHealth returns health status of a specific provider
func (hc *DefaultHealthChecker) GetProviderHealth(providerID string) (*HealthResult, error) {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	if result, exists := hc.results[providerID]; exists {
		// Return a copy to avoid race conditions
		return &HealthResult{
			ProviderID:   result.ProviderID,
			IsHealthy:    result.IsHealthy,
			ResponseTime: result.ResponseTime,
			ErrorCount:   result.ErrorCount,
			LastCheck:    result.LastCheck,
			ErrorRate:    result.ErrorRate,
			Status:       result.Status,
			ErrorMessage: result.ErrorMessage,
		}, nil
	}

	return nil, fmt.Errorf("no health data for provider %s", providerID)
}

// GetAllHealthResults returns health status of all providers
func (hc *DefaultHealthChecker) GetAllHealthResults() map[string]*HealthResult {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	results := make(map[string]*HealthResult)

	for providerID, result := range hc.results {
		results[providerID] = &HealthResult{
			ProviderID:   result.ProviderID,
			IsHealthy:    result.IsHealthy,
			ResponseTime: result.ResponseTime,
			ErrorCount:   result.ErrorCount,
			LastCheck:    result.LastCheck,
			ErrorRate:    result.ErrorRate,
			Status:       result.Status,
			ErrorMessage: result.ErrorMessage,
		}
	}

	return results
}

// UpdateConfig updates the health check configuration
func (hc *DefaultHealthChecker) UpdateConfig(config *HealthCheckConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if err := config.ValidateHealthCheckConfig(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	oldInterval := hc.config.Interval
	hc.config = config.Clone()

	// Restart ticker if interval changed and checker is running
	if hc.running && oldInterval != config.Interval {
		if hc.ticker != nil {
			hc.ticker.Stop()
		}
		hc.ticker = time.NewTicker(config.Interval)
	}

	hc.logger.Info("Health check configuration updated")

	return nil
}

// AddProvider adds a provider to be health checked
func (hc *DefaultHealthChecker) AddProvider(provider *types.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	providerID := (*provider).GetName()
	hc.providers[providerID] = provider

	// Initialize health state
	hc.failureCounts[providerID] = 0
	hc.successCounts[providerID] = 0

	// Initialize with healthy status
	hc.results[providerID] = &HealthResult{
		ProviderID:   providerID,
		IsHealthy:    true,
		ResponseTime: 0,
		ErrorCount:   0,
		LastCheck:    time.Now(),
		ErrorRate:    0.0,
		Status:       "unknown",
	}

	hc.logger.Info(fmt.Sprintf("Added provider to health checker: %s", providerID))

	return nil
}

// RemoveProvider removes a provider from health checking
func (hc *DefaultHealthChecker) RemoveProvider(providerID string) error {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	delete(hc.providers, providerID)
	delete(hc.results, providerID)
	delete(hc.failureCounts, providerID)
	delete(hc.successCounts, providerID)

	hc.logger.Info(fmt.Sprintf("Removed provider from health checker: %s", providerID))

	return nil
}

// healthCheckLoop runs the periodic health checking
func (hc *DefaultHealthChecker) healthCheckLoop() {
	hc.logger.Info("Health check loop started")
	
	// Safety check
	if hc.ticker == nil {
		hc.logger.Error("Health checker ticker is nil")
		return
	}
	
	for {
		select {
		case <-hc.ctx.Done():
			hc.logger.Info("Health check loop stopped")
			return
		case <-hc.ticker.C:
			hc.performAllHealthChecks()
		}
	}
}

// performAllHealthChecks performs health checks on all providers
func (hc *DefaultHealthChecker) performAllHealthChecks() {
	hc.mutex.RLock()
	providers := make(map[string]*types.Provider)
	for id, provider := range hc.providers {
		providers[id] = provider
	}
	hc.mutex.RUnlock()

	// Perform health checks concurrently
	var wg sync.WaitGroup

	for providerID, provider := range providers {
		wg.Add(1)
		go func(id string, p *types.Provider) {
			defer wg.Done()

			result, err := hc.performHealthCheck(id, p)
			if err != nil {
				hc.logger.Error(fmt.Sprintf("Health check failed for provider %s: %v", id, err))
				return
			}

			hc.updateHealthResult(id, result)
		}(providerID, provider)
	}

	wg.Wait()
}

// performHealthCheck performs a single health check
func (hc *DefaultHealthChecker) performHealthCheck(providerID string, provider *types.Provider) (*HealthResult, error) {
	start := time.Now()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), hc.config.Timeout)
	defer cancel()

	// Perform health check
	healthStatus, err := (*provider).HealthCheck(ctx)
	responseTime := time.Since(start)

	result := &HealthResult{
		ProviderID:   providerID,
		ResponseTime: responseTime,
		LastCheck:    time.Now(),
	}

	if err != nil {
		result.IsHealthy = false
		result.Status = "error"
		result.ErrorMessage = err.Error()
		return result, nil
	}

	if healthStatus != nil {
		result.IsHealthy = healthStatus.IsHealthy
		result.Status = "ok"
		if healthStatus.ErrorMessage != "" {
			result.ErrorMessage = healthStatus.ErrorMessage
			result.Status = "error"
		}
		if healthStatus.IsHealthy {
			result.Status = "healthy"
		} else {
			result.Status = "unhealthy"
		}
	} else {
		// If no health status returned, consider it healthy if no error
		result.IsHealthy = true
		result.Status = "ok"
	}

	return result, nil
}

// updateHealthResult updates the health result and applies thresholds
func (hc *DefaultHealthChecker) updateHealthResult(providerID string, result *HealthResult) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	// Get current counters
	failureCount := hc.failureCounts[providerID]
	successCount := hc.successCounts[providerID]

	// Update counters based on result
	if result.IsHealthy {
		successCount++
		failureCount = 0 // Reset failure count on success
	} else {
		failureCount++
		successCount = 0 // Reset success count on failure
	}

	// Apply thresholds
	if !result.IsHealthy && failureCount >= hc.config.FailureThreshold {
		result.IsHealthy = false
		result.Status = "unhealthy"
	} else if result.IsHealthy && successCount >= hc.config.SuccessThreshold {
		result.IsHealthy = true
		result.Status = "healthy"
	} else {
		// Keep previous health status during threshold transition
		if prevResult, exists := hc.results[providerID]; exists {
			result.IsHealthy = prevResult.IsHealthy
		}
	}

	// Calculate error rate
	totalChecks := failureCount + successCount
	if totalChecks > 0 {
		result.ErrorRate = float64(failureCount) / float64(totalChecks)
	}
	result.ErrorCount = int64(failureCount)

	// Update stored values
	hc.failureCounts[providerID] = failureCount
	hc.successCounts[providerID] = successCount
	hc.results[providerID] = result

	// Log status changes
	if prevResult, exists := hc.results[providerID]; exists {
		if prevResult.IsHealthy != result.IsHealthy {
			status := "healthy"
			if !result.IsHealthy {
				status = "unhealthy"
			}
			hc.logger.Info(fmt.Sprintf("Provider %s status changed to %s", providerID, status))
		}
	}
}
