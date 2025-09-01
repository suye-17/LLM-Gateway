// Package router implements circuit breaker pattern for fault tolerance
package router

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	threshold       int           // Failure threshold to open circuit
	timeout         time.Duration // Time to wait before trying half-open
	state           CircuitState
	failureCount    int
	successCount    int
	lastFailureTime time.Time
	mu              sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		timeout:   timeout,
		state:     StateClosed,
	}
}

// IsOpen returns true if the circuit is open
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state == StateOpen {
		// Check if we should transition to half-open
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			// Double-check after acquiring write lock
			if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.timeout {
				cb.state = StateHalfOpen
				cb.successCount = 0
				cb.failureCount = 0
			}
			cb.mu.Unlock()
			cb.mu.RLock()
		}
	}

	return cb.state == StateOpen
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successCount++

	switch cb.state {
	case StateHalfOpen:
		// In half-open state, a few successes will close the circuit
		if cb.successCount >= 3 {
			cb.state = StateClosed
			cb.failureCount = 0
			cb.successCount = 0
		}
	case StateClosed:
		// Reset failure count on success
		cb.failureCount = 0
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failureCount >= cb.threshold {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		// Any failure in half-open state should open the circuit
		cb.state = StateOpen
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:           cb.state,
		FailureCount:    cb.failureCount,
		SuccessCount:    cb.successCount,
		LastFailureTime: cb.lastFailureTime,
		Threshold:       cb.threshold,
		Timeout:         cb.timeout,
	}
}

// CircuitBreakerStats contains statistics about a circuit breaker
type CircuitBreakerStats struct {
	State           CircuitState  `json:"state"`
	FailureCount    int           `json:"failure_count"`
	SuccessCount    int           `json:"success_count"`
	LastFailureTime time.Time     `json:"last_failure_time"`
	Threshold       int           `json:"threshold"`
	Timeout         time.Duration `json:"timeout"`
}

// String returns a string representation of the circuit state
func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler for CircuitState
func (s CircuitState) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// AdaptiveCircuitBreaker implements an adaptive circuit breaker
// that adjusts its threshold based on request patterns
type AdaptiveCircuitBreaker struct {
	*CircuitBreaker
	minThreshold   int
	maxThreshold   int
	adaptionPeriod time.Duration
	requestCount   int
	lastAdaptation time.Time
	successRate    float64
}

// NewAdaptiveCircuitBreaker creates a new adaptive circuit breaker
func NewAdaptiveCircuitBreaker(minThreshold, maxThreshold int, timeout, adaptionPeriod time.Duration) *AdaptiveCircuitBreaker {
	return &AdaptiveCircuitBreaker{
		CircuitBreaker: NewCircuitBreaker(minThreshold, timeout),
		minThreshold:   minThreshold,
		maxThreshold:   maxThreshold,
		adaptionPeriod: adaptionPeriod,
		lastAdaptation: time.Now(),
	}
}

// RecordSuccess records a successful operation and adapts threshold
func (acb *AdaptiveCircuitBreaker) RecordSuccess() {
	acb.CircuitBreaker.RecordSuccess()
	acb.adapt()
}

// RecordFailure records a failed operation and adapts threshold
func (acb *AdaptiveCircuitBreaker) RecordFailure() {
	acb.CircuitBreaker.RecordFailure()
	acb.adapt()
}

// adapt adjusts the threshold based on recent success rate
func (acb *AdaptiveCircuitBreaker) adapt() {
	acb.mu.Lock()
	defer acb.mu.Unlock()

	acb.requestCount++

	// Only adapt periodically
	if time.Since(acb.lastAdaptation) < acb.adaptionPeriod {
		return
	}

	if acb.requestCount > 0 {
		acb.successRate = float64(acb.successCount) / float64(acb.requestCount)

		// Adjust threshold based on success rate
		if acb.successRate > 0.95 {
			// High success rate - can tolerate more failures
			newThreshold := acb.threshold + 1
			if newThreshold <= acb.maxThreshold {
				acb.threshold = newThreshold
			}
		} else if acb.successRate < 0.80 {
			// Low success rate - be more sensitive
			newThreshold := acb.threshold - 1
			if newThreshold >= acb.minThreshold {
				acb.threshold = newThreshold
			}
		}
	}

	// Reset counters for next period
	acb.requestCount = 0
	acb.successCount = 0
	acb.lastAdaptation = time.Now()
}
