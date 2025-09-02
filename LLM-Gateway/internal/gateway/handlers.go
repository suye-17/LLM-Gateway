// Package gateway provides additional HTTP handlers for the LLM Gateway
package gateway

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/llm-gateway/gateway/internal/auth"
	"github.com/llm-gateway/gateway/internal/middleware"
	"github.com/llm-gateway/gateway/internal/storage"
	"github.com/llm-gateway/gateway/pkg/types"
)

// AuthHandlers provides authentication related HTTP handlers
type AuthHandlers struct {
	authService *auth.AuthService
}

// NewAuthHandlers creates new auth handlers
func NewAuthHandlers(authService *auth.AuthService) *AuthHandlers {
	return &AuthHandlers{
		authService: authService,
	}
}

// RegisterUser handles user registration
func (h *AuthHandlers) RegisterUser(c *gin.Context) {
	var req types.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Invalid request format",
				"details": err.Error(),
			},
		})
		return
	}

	response, err := h.authService.Register(c.Request.Context(), &req)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "username already exists" || err.Error() == "email already exists" {
			statusCode = http.StatusConflict
		}
		c.JSON(statusCode, gin.H{
			"error": gin.H{
				"code":    "REGISTRATION_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, response)
}

// Login handles user login
func (h *AuthHandlers) Login(c *gin.Context) {
	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Invalid request format",
				"details": err.Error(),
			},
		})
		return
	}

	response, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "AUTHENTICATION_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// RefreshToken handles token refresh
func (h *AuthHandlers) RefreshToken(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Invalid request format",
			},
		})
		return
	}

	response, err := h.authService.RefreshToken(c.Request.Context(), req.Token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "TOKEN_REFRESH_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// CreateAPIKey handles API key creation
func (h *AuthHandlers) CreateAPIKey(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "User not found in context",
			},
		})
		return
	}

	var req auth.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Invalid request format",
				"details": err.Error(),
			},
		})
		return
	}

	response, err := h.authService.CreateAPIKey(c.Request.Context(), user.ID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "API_KEY_CREATION_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, response)
}

// ListAPIKeys handles listing user's API keys
func (h *AuthHandlers) ListAPIKeys(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "User not found in context",
			},
		})
		return
	}

	apiKeys, err := h.authService.ListAPIKeys(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "API_KEY_LIST_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"api_keys": apiKeys,
	})
}

// RevokeAPIKey handles API key revocation
func (h *AuthHandlers) RevokeAPIKey(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "User not found in context",
			},
		})
		return
	}

	keyIDStr := c.Param("keyId")
	keyID, err := strconv.ParseUint(keyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_KEY_ID",
				"message": "Invalid API key ID",
			},
		})
		return
	}

	err = h.authService.RevokeAPIKey(c.Request.Context(), user.ID, uint(keyID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "API_KEY_REVOCATION_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "API key revoked successfully",
	})
}

// GetProfile handles getting user profile
func (h *AuthHandlers) GetProfile(c *gin.Context) {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "User not found in context",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": auth.UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			IsAdmin:  user.IsAdmin,
		},
	})
}

// AdminHandlers provides admin-only HTTP handlers
type AdminHandlers struct {
	db          *storage.Database
	authService *auth.AuthService
}

// NewAdminHandlers creates new admin handlers
func NewAdminHandlers(db *storage.Database, authService *auth.AuthService) *AdminHandlers {
	return &AdminHandlers{
		db:          db,
		authService: authService,
	}
}

// GetUsers handles listing all users (admin only)
func (h *AdminHandlers) GetUsers(c *gin.Context) {
	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	users, err := h.db.UserRepo().List(offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "USER_LIST_FAILED",
				"message": "Failed to retrieve users",
			},
		})
		return
	}

	// Remove password fields from response
	for i := range users {
		users[i].Password = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
		},
	})
}

// GetSystemStats handles getting system statistics (admin only)
func (h *AdminHandlers) GetSystemStats(c *gin.Context) {
	// Get time range parameters
	startTime := time.Now().AddDate(0, 0, -30) // Default: last 30 days
	endTime := time.Now()

	if start := c.Query("start_time"); start != "" {
		if parsed, err := time.Parse(time.RFC3339, start); err == nil {
			startTime = parsed
		}
	}

	if end := c.Query("end_time"); end != "" {
		if parsed, err := time.Parse(time.RFC3339, end); err == nil {
			endTime = parsed
		}
	}

	stats, err := h.db.RequestRepo().GetStats(nil, startTime, endTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "STATS_FAILED",
				"message": "Failed to retrieve system statistics",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"stats": stats,
		"time_range": gin.H{
			"start_time": startTime,
			"end_time":   endTime,
		},
	})
}

// (This method is now implemented in gateway.go as part of Gateway)

// GetUserStats handles getting user-specific statistics
func (h *AdminHandlers) GetUserStats(c *gin.Context) {
	userIDStr := c.Param("userId")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_USER_ID",
				"message": "Invalid user ID",
			},
		})
		return
	}

	// Get time range parameters
	startTime := time.Now().AddDate(0, 0, -30) // Default: last 30 days
	endTime := time.Now()

	if start := c.Query("start_time"); start != "" {
		if parsed, err := time.Parse(time.RFC3339, start); err == nil {
			startTime = parsed
		}
	}

	if end := c.Query("end_time"); end != "" {
		if parsed, err := time.Parse(time.RFC3339, end); err == nil {
			endTime = parsed
		}
	}

	userIDUint := uint(userID)
	stats, err := h.db.RequestRepo().GetStats(&userIDUint, startTime, endTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "USER_STATS_FAILED",
				"message": "Failed to retrieve user statistics",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": userID,
		"stats":   stats,
		"time_range": gin.H{
			"start_time": startTime,
			"end_time":   endTime,
		},
	})
}
