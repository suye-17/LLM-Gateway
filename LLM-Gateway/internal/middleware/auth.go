// Package middleware provides HTTP middleware for the LLM Gateway
package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/llm-gateway/gateway/internal/auth"
	"github.com/llm-gateway/gateway/internal/storage"
	"github.com/llm-gateway/gateway/pkg/errors"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// AuthMiddleware provides authentication middleware
type AuthMiddleware struct {
	authService *auth.AuthService
	logger      *utils.Logger
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(authService *auth.AuthService, logger *utils.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
		logger:      logger,
	}
}

// RequireAuth middleware that requires authentication (JWT or API Key)
func (am *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, apiKey, err := am.authenticate(c)
		if err != nil {
			am.respondWithError(c, err)
			return
		}

		// Set user and API key in context
		c.Set("user", user)
		if apiKey != nil {
			c.Set("api_key", apiKey)
		}

		c.Next()
	}
}

// RequireAPIKey middleware that specifically requires API Key authentication
func (am *AuthMiddleware) RequireAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, apiKey, err := am.authenticateAPIKey(c)
		if err != nil {
			am.respondWithError(c, err)
			return
		}

		// Set user and API key in context
		c.Set("user", user)
		c.Set("api_key", apiKey)

		c.Next()
	}
}

// RequireAdmin middleware that requires admin privileges
func (am *AuthMiddleware) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, apiKey, err := am.authenticate(c)
		if err != nil {
			am.respondWithError(c, err)
			return
		}

		// Check if user is admin
		if !user.IsAdmin {
			am.logger.LogAuthFailure(c.Request.Context(), "insufficient_privileges",
				c.ClientIP(), c.Request.UserAgent())
			am.respondWithError(c, errors.ErrAccessDenied)
			return
		}

		// Set user and API key in context
		c.Set("user", user)
		if apiKey != nil {
			c.Set("api_key", apiKey)
		}

		c.Next()
	}
}

// OptionalAuth middleware that allows both authenticated and unauthenticated requests
func (am *AuthMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, apiKey, _ := am.authenticate(c)

		// Set user and API key in context if available
		if user != nil {
			c.Set("user", user)
		}
		if apiKey != nil {
			c.Set("api_key", apiKey)
		}

		c.Next()
	}
}

// authenticate attempts to authenticate the request using JWT or API Key
func (am *AuthMiddleware) authenticate(c *gin.Context) (*storage.User, *storage.APIKey, error) {
	// Try API Key authentication first
	if user, apiKey, err := am.authenticateAPIKey(c); err == nil {
		return user, apiKey, nil
	}

	// Try JWT authentication
	if user, err := am.authenticateJWT(c); err == nil {
		return user, nil, nil
	}

	return nil, nil, errors.ErrAuthenticationRequired
}

// authenticateAPIKey authenticates using API Key
func (am *AuthMiddleware) authenticateAPIKey(c *gin.Context) (*storage.User, *storage.APIKey, error) {
	// Check for API key in header
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		// Check for API key in Authorization header (Bearer format)
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			potentialAPIKey := strings.TrimPrefix(authHeader, "Bearer ")
			// Simple heuristic: if it's not a JWT (doesn't contain dots), treat as API key
			if !strings.Contains(potentialAPIKey, ".") {
				apiKey = potentialAPIKey
			}
		}
	}

	if apiKey == "" {
		return nil, nil, errors.NewGatewayError(errors.ErrUnauthorized, "API key required")
	}

	// Validate API key
	user, keyRecord, err := am.authService.ValidateAPIKey(c.Request.Context(), apiKey)
	if err != nil {
		return nil, nil, errors.NewGatewayError(errors.ErrInvalidAPIKey, err.Error())
	}

	return user, keyRecord, nil
}

// authenticateJWT authenticates using JWT token
func (am *AuthMiddleware) authenticateJWT(c *gin.Context) (*storage.User, error) {
	// Get token from Authorization header
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, errors.NewGatewayError(errors.ErrUnauthorized, "Bearer token required")
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == "" {
		return nil, errors.NewGatewayError(errors.ErrUnauthorized, "Token is empty")
	}

	// Validate JWT token
	user, err := am.authService.GetUserByToken(c.Request.Context(), tokenString)
	if err != nil {
		if strings.Contains(err.Error(), "expired") {
			return nil, errors.NewGatewayError(errors.ErrExpiredToken, "Token has expired")
		}
		return nil, errors.NewGatewayError(errors.ErrUnauthorized, "Invalid token")
	}

	return user, nil
}

// respondWithError sends an error response
func (am *AuthMiddleware) respondWithError(c *gin.Context, err error) {
	var gatewayErr *errors.GatewayError
	var ok bool

	if gatewayErr, ok = err.(*errors.GatewayError); !ok {
		gatewayErr = errors.NewGatewayError(errors.ErrUnauthorized, err.Error())
	}

	c.JSON(gatewayErr.HTTPStatusCode, gin.H{
		"error": gin.H{
			"code":    gatewayErr.Code,
			"message": gatewayErr.Message,
			"details": gatewayErr.Details,
		},
	})
	c.Abort()
}

// GetUserFromContext extracts user from gin context
func GetUserFromContext(c *gin.Context) (*storage.User, bool) {
	user, exists := c.Get("user")
	if !exists {
		return nil, false
	}

	userObj, ok := user.(*storage.User)
	return userObj, ok
}

// GetAPIKeyFromContext extracts API key from gin context
func GetAPIKeyFromContext(c *gin.Context) (*storage.APIKey, bool) {
	apiKey, exists := c.Get("api_key")
	if !exists {
		return nil, false
	}

	keyObj, ok := apiKey.(*storage.APIKey)
	return keyObj, ok
}

// GetUserIDFromContext extracts user ID from gin context
func GetUserIDFromContext(c *gin.Context) (uint, bool) {
	user, ok := GetUserFromContext(c)
	if !ok {
		return 0, false
	}
	return user.ID, true
}

// IsAdminFromContext checks if the current user is an admin
func IsAdminFromContext(c *gin.Context) bool {
	user, ok := GetUserFromContext(c)
	if !ok {
		return false
	}
	return user.IsAdmin
}

// RequestID middleware adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request ID already exists in header
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			// Generate new request ID
			requestID = utils.GenerateRequestID()
		}

		// Set request ID in context and response header
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		c.Next()
	}
}

// GetRequestIDFromContext extracts request ID from gin context
func GetRequestIDFromContext(c *gin.Context) string {
	requestID, exists := c.Get("request_id")
	if !exists {
		return ""
	}

	if id, ok := requestID.(string); ok {
		return id
	}

	return ""
}

// CORS middleware for handling Cross-Origin Resource Sharing
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-API-Key, X-Request-ID")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

// RateLimitMiddleware provides rate limiting functionality
type RateLimitMiddleware struct {
	rateLimiter *storage.RateLimiter
	logger      *utils.Logger
}

// NewRateLimitMiddleware creates a new rate limiting middleware
func NewRateLimitMiddleware(rateLimiter *storage.RateLimiter, logger *utils.Logger) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		rateLimiter: rateLimiter,
		logger:      logger,
	}
}

// RateLimit middleware that enforces rate limiting
func (rlm *RateLimitMiddleware) RateLimit(limit int64, window string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Build rate limit key based on user or IP
		var key string
		if user, ok := GetUserFromContext(c); ok {
			key = fmt.Sprintf("user:%d:%s", user.ID, c.Request.URL.Path)
		} else {
			key = fmt.Sprintf("ip:%s:%s", c.ClientIP(), c.Request.URL.Path)
		}

		// Check rate limit
		windowDuration := parseDuration(window)
		allowed, err := rlm.rateLimiter.Allow(c.Request.Context(), key, limit, windowDuration)
		if err != nil {
			rlm.logger.WithError(err).Error("Rate limit check failed")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "INTERNAL_ERROR",
					"message": "Rate limit check failed",
				},
			})
			c.Abort()
			return
		}

		if !allowed {
			rlm.logger.LogRateLimitExceeded(c.Request.Context(),
				getUserIDString(c), getAPIKeyString(c), c.Request.URL.Path)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "RATE_LIMITED",
					"message": "Rate limit exceeded",
				},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Helper functions
func getUserIDString(c *gin.Context) string {
	if user, ok := GetUserFromContext(c); ok {
		return fmt.Sprintf("%d", user.ID)
	}
	return ""
}

func getAPIKeyString(c *gin.Context) string {
	if apiKey, ok := GetAPIKeyFromContext(c); ok {
		return apiKey.Key[:8] + "****" // Masked for logging
	}
	return ""
}

func parseDuration(window string) time.Duration {
	switch window {
	case "minute":
		return time.Minute
	case "hour":
		return time.Hour
	case "day":
		return 24 * time.Hour
	default:
		return time.Minute
	}
}
