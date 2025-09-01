// Package errors defines custom error types for the LLM Gateway
package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode represents different types of errors
type ErrorCode string

const (
	// Authentication and Authorization errors
	ErrUnauthorized     ErrorCode = "UNAUTHORIZED"
	ErrForbidden        ErrorCode = "FORBIDDEN"
	ErrInvalidAPIKey    ErrorCode = "INVALID_API_KEY"
	ErrExpiredToken     ErrorCode = "EXPIRED_TOKEN"
	
	// Request validation errors
	ErrInvalidRequest   ErrorCode = "INVALID_REQUEST"
	ErrMissingParameter ErrorCode = "MISSING_PARAMETER"
	ErrInvalidModel     ErrorCode = "INVALID_MODEL"
	
	// Rate limiting errors
	ErrRateLimited      ErrorCode = "RATE_LIMITED"
	ErrQuotaExceeded    ErrorCode = "QUOTA_EXCEEDED"
	
	// Provider errors
	ErrProviderUnavailable ErrorCode = "PROVIDER_UNAVAILABLE"
	ErrProviderTimeout     ErrorCode = "PROVIDER_TIMEOUT"
	ErrProviderError       ErrorCode = "PROVIDER_ERROR"
	
	// Internal errors
	ErrInternalServer   ErrorCode = "INTERNAL_SERVER_ERROR"
	ErrServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrDatabaseError    ErrorCode = "DATABASE_ERROR"
	ErrRedisError       ErrorCode = "REDIS_ERROR"
)

// GatewayError represents a gateway-specific error
type GatewayError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details string    `json:"details,omitempty"`
	HTTPStatusCode int `json:"-"`
}

// Error implements the error interface
func (e *GatewayError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewGatewayError creates a new gateway error
func NewGatewayError(code ErrorCode, message string) *GatewayError {
	return &GatewayError{
		Code:           code,
		Message:        message,
		HTTPStatusCode: getHTTPStatusCode(code),
	}
}

// NewGatewayErrorWithDetails creates a new gateway error with details
func NewGatewayErrorWithDetails(code ErrorCode, message, details string) *GatewayError {
	return &GatewayError{
		Code:           code,
		Message:        message,
		Details:        details,
		HTTPStatusCode: getHTTPStatusCode(code),
	}
}

// getHTTPStatusCode maps error codes to HTTP status codes
func getHTTPStatusCode(code ErrorCode) int {
	switch code {
	case ErrUnauthorized, ErrInvalidAPIKey, ErrExpiredToken:
		return http.StatusUnauthorized
	case ErrForbidden:
		return http.StatusForbidden
	case ErrInvalidRequest, ErrMissingParameter, ErrInvalidModel:
		return http.StatusBadRequest
	case ErrRateLimited, ErrQuotaExceeded:
		return http.StatusTooManyRequests
	case ErrProviderUnavailable, ErrServiceUnavailable:
		return http.StatusServiceUnavailable
	case ErrProviderTimeout:
		return http.StatusGatewayTimeout
	case ErrProviderError, ErrInternalServer, ErrDatabaseError, ErrRedisError:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// Common error instances
var (
	ErrAuthenticationRequired = NewGatewayError(ErrUnauthorized, "Authentication required")
	ErrInvalidCredentials     = NewGatewayError(ErrUnauthorized, "Invalid credentials")
	ErrAccessDenied          = NewGatewayError(ErrForbidden, "Access denied")
	ErrInvalidRequestFormat   = NewGatewayError(ErrInvalidRequest, "Invalid request format")
	ErrModelNotSupported      = NewGatewayError(ErrInvalidModel, "Model not supported")
)