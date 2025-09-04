// Package retry implements retry mechanisms with exponential backoff for LLM provider requests
package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// RetryManager manages retry logic with exponential backoff
type RetryManager struct {
	policy *types.RetryPolicy
	logger *utils.Logger
	stats  *RetryStats
	mutex  sync.RWMutex
}

// RetryStats tracks retry statistics
type RetryStats struct {
	TotalAttempts     int64   `json:"total_attempts"`
	TotalRetries      int64   `json:"total_retries"`
	SuccessfulRetries int64   `json:"successful_retries"`
	FailedRetries     int64   `json:"failed_retries"`
	AverageAttempts   float64 `json:"average_attempts"`
}

// RetryableOperation represents an operation that can be retried
type RetryableOperation func(ctx context.Context, attempt int) error

// RetryManagerInterface defines the contract for retry management
type RetryManagerInterface interface {
	ExecuteWithRetry(ctx context.Context, operation RetryableOperation) error
	ShouldRetry(err error, attempt int) bool
	CalculateDelay(attempt int) time.Duration
	GetRetryStats() *RetryStats
	ResetStats()
}

// NewRetryManager creates a new retry manager with the given policy
func NewRetryManager(policy *types.RetryPolicy, logger *utils.Logger) *RetryManager {
	if policy == nil {
		policy = types.DefaultRetryPolicy()
	}

	return &RetryManager{
		policy: policy,
		logger: logger,
		stats:  &RetryStats{},
	}
}

// ExecuteWithRetry executes an operation with retry logic
func (rm *RetryManager) ExecuteWithRetry(ctx context.Context, operation RetryableOperation) error {
	var lastErr error
	attempt := 0

	// Track total attempts
	defer func() {
		atomic.AddInt64(&rm.stats.TotalAttempts, 1)
		if attempt > 1 {
			atomic.AddInt64(&rm.stats.TotalRetries, int64(attempt-1))
			if lastErr == nil {
				atomic.AddInt64(&rm.stats.SuccessfulRetries, 1)
			} else {
				atomic.AddInt64(&rm.stats.FailedRetries, 1)
			}
		}
		rm.updateAverageAttempts(float64(attempt))
	}()

	for attempt = 1; attempt <= rm.policy.MaxRetries+1; attempt++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		// Execute the operation
		err := operation(ctx, attempt)
		if err == nil {
			// Success!
			if attempt > 1 {
				rm.logger.WithField("attempts", attempt).Info("Operation succeeded after retry")
			}
			return nil
		}

		lastErr = err

		// Check if we should retry
		if attempt > rm.policy.MaxRetries || !rm.ShouldRetry(err, attempt) {
			rm.logger.WithField("attempts", attempt).WithError(err).Warn("Operation failed, no more retries")
			break
		}

		// Calculate delay for next attempt
		delay := rm.CalculateDelay(attempt)
		rm.logger.WithField("attempt", attempt).WithField("delay", delay).WithError(err).Info("Operation failed, retrying after delay")

		// Wait with context cancellation check
		if err := rm.waitWithContext(ctx, delay); err != nil {
			return fmt.Errorf("retry cancelled: %w", err)
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", attempt-1, lastErr)
}

// ShouldRetry determines if an error is retryable
func (rm *RetryManager) ShouldRetry(err error, attempt int) bool {
	if attempt > rm.policy.MaxRetries {
		return false
	}

	// Check if it's a provider error with retry information
	if providerErr, ok := err.(*ProviderRetryError); ok {
		return providerErr.Retryable
	}

	// Check error message against retryable error patterns
	errorMsg := err.Error()
	for _, retryablePattern := range rm.policy.RetryableErrors {
		if contains(errorMsg, retryablePattern) {
			return true
		}
	}

	// Default to not retry for unknown errors
	return false
}

// CalculateDelay calculates the delay for the next retry attempt using exponential backoff with jitter
func (rm *RetryManager) CalculateDelay(attempt int) time.Duration {
	// Exponential backoff: BaseDelay * (BackoffFactor ^ attempt)
	delay := float64(rm.policy.BaseDelay) * math.Pow(rm.policy.BackoffFactor, float64(attempt))

	// Apply maximum delay limit
	maxDelay := float64(rm.policy.MaxDelay)
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±10% randomization) to prevent thundering herd
	jitter := delay * 0.1 * (rand.Float64()*2 - 1) // -10% to +10%
	delay += jitter

	// Ensure delay is not negative
	if delay < 0 {
		delay = float64(rm.policy.BaseDelay)
	}

	return time.Duration(delay)
}

// GetRetryStats returns current retry statistics
func (rm *RetryManager) GetRetryStats() *RetryStats {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	return &RetryStats{
		TotalAttempts:     atomic.LoadInt64(&rm.stats.TotalAttempts),
		TotalRetries:      atomic.LoadInt64(&rm.stats.TotalRetries),
		SuccessfulRetries: atomic.LoadInt64(&rm.stats.SuccessfulRetries),
		FailedRetries:     atomic.LoadInt64(&rm.stats.FailedRetries),
		AverageAttempts:   rm.stats.AverageAttempts,
	}
}

// ResetStats resets retry statistics
func (rm *RetryManager) ResetStats() {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	atomic.StoreInt64(&rm.stats.TotalAttempts, 0)
	atomic.StoreInt64(&rm.stats.TotalRetries, 0)
	atomic.StoreInt64(&rm.stats.SuccessfulRetries, 0)
	atomic.StoreInt64(&rm.stats.FailedRetries, 0)
	rm.stats.AverageAttempts = 0
}

// waitWithContext waits for the specified duration while respecting context cancellation
func (rm *RetryManager) waitWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// updateAverageAttempts updates the moving average of attempts per operation
func (rm *RetryManager) updateAverageAttempts(attempts float64) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	totalOps := atomic.LoadInt64(&rm.stats.TotalAttempts)
	if totalOps > 0 {
		// Simple moving average
		rm.stats.AverageAttempts = (rm.stats.AverageAttempts*float64(totalOps-1) + attempts) / float64(totalOps)
	} else {
		rm.stats.AverageAttempts = attempts
	}
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsInner(s, substr)))
}

func containsInner(s, substr string) bool {
	for i := 1; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
