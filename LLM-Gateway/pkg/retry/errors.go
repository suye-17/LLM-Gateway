// Package retry provides error types and classification for retry logic
package retry

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/llm-gateway/gateway/pkg/types"
)

// ProviderRetryError represents an error from a provider with retry information
type ProviderRetryError struct {
	Provider      string              `json:"provider"`
	Operation     string              `json:"operation"`
	Category      types.ErrorCategory `json:"category"`
	StatusCode    int                 `json:"status_code"`
	Message       string              `json:"message"`
	Retryable     bool                `json:"retryable"`
	RetryAfter    int                 `json:"retry_after,omitempty"` // Seconds to wait before retry
	OriginalError error               `json:"-"`
}

func (e *ProviderRetryError) Error() string {
	return fmt.Sprintf("%s %s failed: %s (retryable: %v)", e.Provider, e.Operation, e.Message, e.Retryable)
}

func (e *ProviderRetryError) Unwrap() error {
	return e.OriginalError
}

// ClassifyError classifies an error and determines if it should be retried
func ClassifyError(err error, provider, operation string) *ProviderRetryError {
	if err == nil {
		return nil
	}

	retryError := &ProviderRetryError{
		Provider:      provider,
		Operation:     operation,
		Message:       err.Error(),
		OriginalError: err,
	}

	// Try to extract HTTP status code if available
	statusCode := extractStatusCode(err)
	retryError.StatusCode = statusCode

	// Classify error based on status code and message
	retryError.Category, retryError.Retryable = categorizeError(statusCode, err.Error())

	// Extract retry-after header if present
	retryError.RetryAfter = extractRetryAfter(err)

	return retryError
}

// categorizeError categorizes an error and determines if it's retryable
func categorizeError(statusCode int, message string) (types.ErrorCategory, bool) {
	messageLower := strings.ToLower(message)

	// HTTP status code based classification
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return types.ErrorAuth, false
	case statusCode == http.StatusTooManyRequests:
		return types.ErrorRateLimit, true
	case statusCode == http.StatusPaymentRequired:
		return types.ErrorQuota, false
	case statusCode >= 500:
		return types.ErrorServer, true
	case statusCode >= 400 && statusCode < 500:
		return types.ErrorClient, false
	}

	// Message-based classification for non-HTTP errors
	switch {
	case strings.Contains(messageLower, "unauthorized") ||
		strings.Contains(messageLower, "invalid api key") ||
		strings.Contains(messageLower, "authentication") ||
		strings.Contains(messageLower, "forbidden"):
		return types.ErrorAuth, false

	case strings.Contains(messageLower, "rate limit") ||
		strings.Contains(messageLower, "too many requests") ||
		strings.Contains(messageLower, "quota exceeded"):
		return types.ErrorRateLimit, true

	case strings.Contains(messageLower, "insufficient credits") ||
		strings.Contains(messageLower, "billing") ||
		strings.Contains(messageLower, "payment"):
		return types.ErrorQuota, false

	case strings.Contains(messageLower, "timeout") ||
		strings.Contains(messageLower, "deadline exceeded") ||
		strings.Contains(messageLower, "context canceled"):
		return types.ErrorTimeout, true

	case strings.Contains(messageLower, "connection refused") ||
		strings.Contains(messageLower, "network") ||
		strings.Contains(messageLower, "dns") ||
		strings.Contains(messageLower, "unreachable"):
		return types.ErrorNetwork, true

	case strings.Contains(messageLower, "internal server error") ||
		strings.Contains(messageLower, "service unavailable") ||
		strings.Contains(messageLower, "bad gateway") ||
		strings.Contains(messageLower, "gateway timeout"):
		return types.ErrorServer, true

	case strings.Contains(messageLower, "bad request") ||
		strings.Contains(messageLower, "invalid") ||
		strings.Contains(messageLower, "malformed"):
		return types.ErrorClient, false

	default:
		// Default to non-retryable for unknown errors
		return types.ErrorClient, false
	}
}

// extractStatusCode extracts HTTP status code from error message if available
func extractStatusCode(err error) int {
	message := err.Error()

	// Common patterns for HTTP status codes in error messages
	patterns := []string{
		"status code: ",
		"status: ",
		"HTTP ",
		"code ",
	}

	for _, pattern := range patterns {
		if idx := strings.Index(message, pattern); idx != -1 {
			start := idx + len(pattern)
			if start < len(message) {
				// Extract the number part
				var statusCode int
				if _, err := fmt.Sscanf(message[start:], "%d", &statusCode); err == nil {
					return statusCode
				}
			}
		}
	}

	return 0 // No status code found
}

// extractRetryAfter extracts retry-after value from error message if available
func extractRetryAfter(err error) int {
	message := strings.ToLower(err.Error())

	patterns := []string{
		"retry after ",
		"retry-after: ",
		"wait ",
		"try again in ",
	}

	for _, pattern := range patterns {
		if idx := strings.Index(message, pattern); idx != -1 {
			start := idx + len(pattern)
			if start < len(message) {
				// Extract the number part (assume seconds)
				var seconds int
				if _, err := fmt.Sscanf(message[start:], "%d", &seconds); err == nil {
					return seconds
				}
			}
		}
	}

	return 0 // No retry-after found
}

// NewProviderRetryError creates a new ProviderRetryError with the specified parameters
func NewProviderRetryError(provider, operation string, category types.ErrorCategory, message string, retryable bool) *ProviderRetryError {
	return &ProviderRetryError{
		Provider:  provider,
		Operation: operation,
		Category:  category,
		Message:   message,
		Retryable: retryable,
	}
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	if retryErr, ok := err.(*ProviderRetryError); ok {
		return retryErr.Retryable
	}

	// Classify unknown errors and check if retryable
	classified := ClassifyError(err, "unknown", "unknown")
	return classified.Retryable
}

// ClassifyZhipuError classifies Zhipu GLM specific errors
func ClassifyZhipuError(statusCode int, errorCode, message string) *ProviderRetryError {
	retryError := &ProviderRetryError{
		Provider:   "zhipu",
		Operation:  "chat_completion",
		StatusCode: statusCode,
		Message:    fmt.Sprintf("[%s] %s", errorCode, message),
	}

	// Classify based on Zhipu error codes
	switch errorCode {
	case "1001", "1002": // API key errors
		retryError.Category = types.ErrorAuth
		retryError.Retryable = false
	case "1003", "1004": // Request format errors
		retryError.Category = types.ErrorClient
		retryError.Retryable = false
	case "1013": // Rate limit exceeded
		retryError.Category = types.ErrorRateLimit
		retryError.Retryable = true
		retryError.RetryAfter = 60 // Wait 1 minute
	case "1301", "1302": // Service temporarily unavailable
		retryError.Category = types.ErrorServer
		retryError.Retryable = true
		retryError.RetryAfter = 30
	case "50001", "50002": // Server internal errors
		retryError.Category = types.ErrorServer
		retryError.Retryable = true
	default:
		// Fallback to HTTP status code classification
		return ClassifyHTTPError(statusCode, message)
	}

	return retryError
}

// ClassifyHTTPError classifies generic HTTP errors
func ClassifyHTTPError(statusCode int, message string) *ProviderRetryError {
	retryError := &ProviderRetryError{
		Provider:   "http",
		Operation:  "request",
		StatusCode: statusCode,
		Message:    message,
	}

	switch {
	case statusCode >= 200 && statusCode < 300:
		// Success - shouldn't be here
		retryError.Category = types.ErrorNetwork // Use network as fallback
		retryError.Retryable = false
	case statusCode == 401 || statusCode == 403:
		// Authentication/Authorization errors
		retryError.Category = types.ErrorAuth
		retryError.Retryable = false
	case statusCode == 400 || statusCode == 422:
		// Bad request errors
		retryError.Category = types.ErrorClient
		retryError.Retryable = false
	case statusCode == 429:
		// Rate limit errors
		retryError.Category = types.ErrorRateLimit
		retryError.Retryable = true
		retryError.RetryAfter = 60
	case statusCode >= 500:
		// Server errors
		retryError.Category = types.ErrorServer
		retryError.Retryable = true
	case statusCode >= 400:
		// Other client errors
		retryError.Category = types.ErrorClient
		retryError.Retryable = false
	default:
		// Unknown status codes
		retryError.Category = types.ErrorNetwork
		retryError.Retryable = false
	}

	return retryError
}
