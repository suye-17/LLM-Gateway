// Package gateway provides the enhanced gateway functionality with full data storage and auth
package gateway

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/llm-gateway/gateway/internal/auth"
	"github.com/llm-gateway/gateway/internal/middleware"
	"github.com/llm-gateway/gateway/internal/storage"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// EnhancedGateway represents the enhanced gateway service with full functionality
type EnhancedGateway struct {
	config      *types.Config
	router      *gin.Engine
	server      *http.Server
	logger      *utils.Logger
	db          *storage.Database
	redis       *storage.RedisClient
	authService *auth.AuthService

	// Handlers
	authHandlers  *AuthHandlers
	adminHandlers *AdminHandlers

	// Middleware
	authMiddleware      *middleware.AuthMiddleware
	rateLimitMiddleware *middleware.RateLimitMiddleware

	// Cache managers
	sessionManager *storage.SessionManager
	rateLimiter    *storage.RateLimiter
}

// NewEnhancedGateway creates a new enhanced gateway instance
func NewEnhancedGateway(cfg *types.Config) (*EnhancedGateway, error) {
	// Initialize logger
	logger := utils.NewLogger(&cfg.Logging)

	// Set Gin mode
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize database
	logger.Info("Initializing database connection...")
	db, err := storage.NewDatabase(&cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Run migrations
	if err := db.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Create default data
	if err := db.CreateDefaultData(); err != nil {
		return nil, fmt.Errorf("failed to create default data: %w", err)
	}

	// Initialize Redis
	logger.Info("Initializing Redis connection...")
	redis, err := storage.NewRedisClient(&cfg.Redis, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// Initialize authentication service
	authService := auth.NewAuthService(&cfg.Auth, logger, db)

	// Initialize cache managers
	sessionManager := storage.NewSessionManager(redis)
	rateLimiter := storage.NewRateLimiter(redis)

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(authService, logger)
	rateLimitMiddleware := middleware.NewRateLimitMiddleware(rateLimiter, logger)

	// Initialize handlers
	authHandlers := NewAuthHandlers(authService)
	adminHandlers := NewAdminHandlers(db, authService)

	// Create router
	router := gin.New()

	gateway := &EnhancedGateway{
		config:              cfg,
		router:              router,
		logger:              logger,
		db:                  db,
		redis:               redis,
		authService:         authService,
		authHandlers:        authHandlers,
		adminHandlers:       adminHandlers,
		authMiddleware:      authMiddleware,
		rateLimitMiddleware: rateLimitMiddleware,
		sessionManager:      sessionManager,
		rateLimiter:         rateLimiter,
	}

	// Setup middleware and routes
	gateway.setupMiddleware()
	gateway.setupRoutes()

	logger.Info("Enhanced gateway initialized successfully")
	return gateway, nil
}

// setupMiddleware configures global middleware
func (g *EnhancedGateway) setupMiddleware() {
	// Recovery middleware
	g.router.Use(gin.Recovery())

	// Request ID middleware
	g.router.Use(middleware.RequestID())

	// CORS middleware
	g.router.Use(middleware.CORS())

	// Custom logging middleware
	g.router.Use(g.loggingMiddleware())
}

// setupRoutes configures all API routes
func (g *EnhancedGateway) setupRoutes() {
	// Root health check (public)
	g.router.GET("/health", g.basicHealthCheck)
	g.router.GET("/health/detailed", g.healthCheckDetailed)

	// API version 1
	v1 := g.router.Group("/v1")
	{
		// Public authentication endpoints
		auth := v1.Group("/auth")
		{
			auth.POST("/register", g.authHandlers.RegisterUser)
			auth.POST("/login", g.authHandlers.Login)
			auth.POST("/refresh", g.authHandlers.RefreshToken)
		}

		// Protected user endpoints
		user := v1.Group("/user")
		user.Use(g.authMiddleware.RequireAuth())
		{
			user.GET("/profile", g.authHandlers.GetProfile)
			user.GET("/api-keys", g.authHandlers.ListAPIKeys)
			user.POST("/api-keys", g.authHandlers.CreateAPIKey)
			user.DELETE("/api-keys/:keyId", g.authHandlers.RevokeAPIKey)
		}

		// LLM API endpoints (require API key)
		llm := v1.Group("/chat")
		llm.Use(g.authMiddleware.RequireAPIKey())
		llm.Use(g.rateLimitMiddleware.RateLimit(100, "minute")) // 100 requests per minute
		{
			llm.POST("/completions", g.chatCompletions)
		}

		// Admin endpoints
		admin := v1.Group("/admin")
		admin.Use(g.authMiddleware.RequireAdmin())
		{
			admin.GET("/status", g.adminStatus)
			admin.GET("/users", g.adminHandlers.GetUsers)
			admin.GET("/users/:userId/stats", g.adminHandlers.GetUserStats)
			admin.GET("/stats", g.adminHandlers.GetSystemStats)
			admin.GET("/providers", g.listProviders)
			admin.GET("/metrics", g.getMetrics)
		}
	}
}

// Start starts the enhanced gateway server
func (g *EnhancedGateway) Start() error {
	addr := fmt.Sprintf("%s:%d", g.config.Server.Host, g.config.Server.Port)

	g.server = &http.Server{
		Addr:         addr,
		Handler:      g.router,
		ReadTimeout:  g.config.Server.ReadTimeout,
		WriteTimeout: g.config.Server.WriteTimeout,
		IdleTimeout:  g.config.Server.IdleTimeout,
	}

	g.logger.WithField("address", addr).Info("Starting Enhanced LLM Gateway server")

	if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

// Stop gracefully stops the enhanced gateway server
func (g *EnhancedGateway) Stop(ctx context.Context) error {
	g.logger.Info("Shutting down Enhanced LLM Gateway server")

	// Close database connection
	if g.db != nil {
		if err := g.db.Close(); err != nil {
			g.logger.WithError(err).Warn("Failed to close database connection")
		}
	}

	// Close Redis connection
	if g.redis != nil {
		if err := g.redis.Close(); err != nil {
			g.logger.WithError(err).Warn("Failed to close Redis connection")
		}
	}

	// Shutdown HTTP server
	if g.server != nil {
		return g.server.Shutdown(ctx)
	}

	return nil
}

// HTTP Handlers

// basicHealthCheck provides simple health check
func (g *EnhancedGateway) basicHealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "2.0.0",
	})
}

// healthCheckDetailed provides detailed health check information
func (g *EnhancedGateway) healthCheckDetailed(c *gin.Context) {
	health := gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "2.0.0",
		"services":  gin.H{},
	}

	// Check database connectivity
	if g.db != nil {
		if err := g.db.Ping(); err != nil {
			health["status"] = "degraded"
			health["services"].(gin.H)["database"] = gin.H{
				"status": "unhealthy",
				"error":  err.Error(),
			}
		} else {
			health["services"].(gin.H)["database"] = gin.H{
				"status": "healthy",
			}
		}
	}

	// Check Redis connectivity
	if g.redis != nil {
		ctx := c.Request.Context()
		if err := g.redis.Ping(ctx); err != nil {
			health["status"] = "degraded"
			health["services"].(gin.H)["redis"] = gin.H{
				"status": "unhealthy",
				"error":  err.Error(),
			}
		} else {
			health["services"].(gin.H)["redis"] = gin.H{
				"status": "healthy",
			}
		}
	}

	// Determine overall status code
	statusCode := http.StatusOK
	if health["status"] == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, health)
}

// chatCompletions handles the enhanced chat completions with full logging and auth
func (g *EnhancedGateway) chatCompletions(c *gin.Context) {
	startTime := time.Now()
	requestID := middleware.GetRequestIDFromContext(c)

	var req types.Request
	if err := c.ShouldBindJSON(&req); err != nil {
		g.logger.WithRequestID(requestID).WithError(err).Error("Failed to bind request")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Set request metadata
	req.ID = requestID
	req.Timestamp = startTime

	// Get user info from context
	user, _ := middleware.GetUserFromContext(c)
	apiKey, _ := middleware.GetAPIKeyFromContext(c)

	if user != nil {
		req.UserID = fmt.Sprintf("%d", user.ID)
	}

	g.logger.LogAPIRequest(c.Request.Context(), c.Request.Method, c.Request.URL.Path,
		c.Request.UserAgent(), c.ClientIP(), requestID, startTime)

	// TODO: Implement actual provider routing and calling
	// For now, return an enhanced mock response
	response := &types.Response{
		ID:       req.ID,
		Model:    req.Model,
		Provider: "mock",
		Created:  time.Now(),
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.Message{
					Role:    "assistant",
					Content: fmt.Sprintf("Enhanced LLM Gateway v2.0 response! Request ID: %s, User: %s", requestID, req.UserID),
				},
				FinishReason: func() *string { s := "stop"; return &s }(),
			},
		},
		Usage: types.Usage{
			PromptTokens:     len(req.Messages) * 10, // Rough estimation
			CompletionTokens: 25,
			TotalTokens:      len(req.Messages)*10 + 25,
		},
		Latency: time.Since(startTime).Milliseconds(),
	}

	// Log the request to database
	go g.logRequestToDatabase(req, response, user, apiKey, c.ClientIP(), c.Request.UserAgent())

	duration := time.Since(startTime)
	g.logger.LogAPIResponse(c.Request.Context(), requestID, http.StatusOK, duration, 0)

	c.JSON(http.StatusOK, response)
}

// adminStatus provides enhanced admin status
func (g *EnhancedGateway) adminStatus(c *gin.Context) {
	uptime := time.Since(time.Now()) // TODO: Track actual uptime

	stats := gin.H{
		"status":  "running",
		"uptime":  uptime.String(),
		"version": "2.0.0",
		"features": gin.H{
			"database":       true,
			"redis":          true,
			"authentication": true,
			"rate_limiting":  true,
			"monitoring":     true,
		},
		"providers":  0, // TODO: Get actual provider count
		"middleware": 5, // Current middleware count
	}

	c.JSON(http.StatusOK, stats)
}

// listProviders handles provider listing (placeholder)
func (g *EnhancedGateway) listProviders(c *gin.Context) {
	// TODO: Implement actual provider listing from database
	c.JSON(http.StatusOK, gin.H{
		"providers": []gin.H{},
		"message":   "Provider management will be implemented in Week 3",
	})
}

// getMetrics handles metrics retrieval (placeholder)
func (g *EnhancedGateway) getMetrics(c *gin.Context) {
	// TODO: Implement actual metrics collection
	c.JSON(http.StatusOK, gin.H{
		"requests_total":    0,
		"requests_success":  0,
		"requests_failed":   0,
		"avg_latency_ms":    0,
		"providers_healthy": 0,
		"message":           "Detailed metrics will be implemented in Week 3",
	})
}

// Middleware

// loggingMiddleware provides structured request logging
func (g *EnhancedGateway) loggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf(`{"time":"%s","method":"%s","path":"%s","status":%d,"latency":"%s","ip":"%s","user_agent":"%s"}%s`,
				param.TimeStamp.Format(time.RFC3339),
				param.Method,
				param.Path,
				param.StatusCode,
				param.Latency,
				param.ClientIP,
				param.Request.UserAgent(),
				"\n",
			)
		},
		Output: g.logger.Logger.Out,
	})
}

// Helper functions

// logRequestToDatabase logs the request details to database
func (g *EnhancedGateway) logRequestToDatabase(req types.Request, resp *types.Response, user *storage.User, apiKey *storage.APIKey, clientIP, userAgent string) {
	logEntry := &storage.Request{
		RequestID:        req.ID,
		Method:           "POST",
		Path:             "/v1/chat/completions",
		ClientIP:         clientIP,
		UserAgent:        userAgent,
		ModelName:        req.Model,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
		StatusCode:       200,
		ResponseTimeMs:   resp.Latency,
		Cost:             float64(resp.Usage.TotalTokens) * 0.001, // Mock cost calculation
		RequestTime:      req.Timestamp,
		ResponseAt:       resp.Created,
	}

	if user != nil {
		logEntry.UserID = &user.ID
	}

	if apiKey != nil {
		logEntry.APIKeyID = &apiKey.ID
	}

	if err := g.db.RequestRepo().Create(logEntry); err != nil {
		g.logger.WithError(err).Error("Failed to log request to database")
	}
}
