// Package gateway implements the enhanced LLM Gateway v3.0 with intelligent routing
package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/llm-gateway/gateway/internal/auth"
	"github.com/llm-gateway/gateway/internal/middleware"
	"github.com/llm-gateway/gateway/internal/providers"
	"github.com/llm-gateway/gateway/internal/router"
	"github.com/llm-gateway/gateway/internal/storage"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// EnhancedGatewayV3 implements the third version of the LLM Gateway with intelligent routing
type EnhancedGatewayV3 struct {
	config              *types.Config
	router              *gin.Engine
	server              *http.Server
	logger              *utils.Logger
	db                  *storage.Database
	redis               *storage.RedisClient
	authService         *auth.AuthService
	authHandlers        *AuthHandlers
	adminHandlers       *AdminHandlers
	authMiddleware      *middleware.AuthMiddleware
	rateLimitMiddleware *middleware.RateLimitMiddleware
	sessionManager      *storage.SessionManager
	rateLimiter         *storage.RateLimiter

	// V3 additions
	providerRegistry providers.ProviderRegistry
	routingService   *router.Service
	configManager    *ConfigManager
}

// NewEnhancedGatewayV3 creates a new enhanced gateway v3.0 instance
func NewEnhancedGatewayV3(cfg *types.Config, logger *utils.Logger) (*EnhancedGatewayV3, error) {
	logger.Info("Initializing Enhanced LLM Gateway v3.0...")

	// Initialize database
	logger.Info("Initializing database connection...")
	db, err := storage.NewDatabase(&cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	logger.Info("Successfully connected to PostgreSQL database")

	// Run database migrations
	logger.Info("Starting database migration")
	if err := db.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}
	logger.Info("Database migration completed successfully")

	// Create default data
	logger.Info("Creating default data")
	if err := db.CreateDefaultData(); err != nil {
		logger.WithField("error", err.Error()).Warn("Failed to create default data")
	} else {
		logger.Info("Default data creation completed")
	}

	// Initialize Redis
	logger.Info("Initializing Redis connection...")
	redis, err := storage.NewRedisClient(&cfg.Redis, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}
	logger.Info("Successfully connected to Redis")

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

	// V3: Initialize provider registry and routing service
	providerRegistry := providers.NewDefaultRegistry(logger)

	routingConfig := &router.RouterConfig{
		Strategy:                router.StrategyRoundRobin,
		HealthCheckInterval:     30 * time.Second,
		MaxRetries:              3,
		RetryDelay:              100 * time.Millisecond,
		CircuitBreakerEnabled:   true,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   60 * time.Second,
		StickySessions:          false,
		PreferredProviders:      []string{},
		ModelAffinity:           make(map[string][]string),
	}

	routingService := router.NewService(routingConfig, providerRegistry, logger)

	// Initialize config manager
	configManager := NewConfigManager(cfg, logger)

	// Create router
	ginRouter := gin.New()

	gateway := &EnhancedGatewayV3{
		config:              cfg,
		router:              ginRouter,
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
		providerRegistry:    providerRegistry,
		routingService:      routingService,
		configManager:       configManager,
	}

	// Setup middleware and routes
	gateway.setupMiddleware()
	gateway.setupRoutes()

	// Initialize default providers
	if err := gateway.initializeProviders(); err != nil {
		logger.WithField("error", err.Error()).Warn("Failed to initialize providers")
	}

	logger.Info("Enhanced gateway v3.0 initialized successfully")
	return gateway, nil
}

// setupMiddleware configures global middleware
func (g *EnhancedGatewayV3) setupMiddleware() {
	// Basic middleware
	g.router.Use(gin.Logger())
	g.router.Use(gin.Recovery())

	// CORS middleware
	g.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Request logging middleware
	g.router.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		g.logger.WithHTTPRequest(c.Request.Method, c.Request.URL.Path, c.Request.UserAgent(), c.ClientIP()).
			WithField("status", c.Writer.Status()).
			WithField("duration_ms", duration.Milliseconds()).
			Info("HTTP request completed")
	})
}

// setupRoutes configures API routes
func (g *EnhancedGatewayV3) setupRoutes() {
	// Health check endpoints
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

		// LLM API endpoints (require API key) - V3 enhanced with routing
		llm := v1.Group("/chat")
		llm.Use(g.authMiddleware.RequireAPIKey())
		llm.Use(g.rateLimitMiddleware.RateLimit(100, "minute"))
		{
			llm.POST("/completions", g.chatCompletionsV3)
		}

		// Provider management endpoints - V3 new
		providers := v1.Group("/providers")
		providers.Use(g.authMiddleware.RequireAdmin())
		{
			providers.GET("/", g.listProviders)
			providers.POST("/", g.addProvider)
			providers.GET("/:providerName", g.getProvider)
			providers.PUT("/:providerName", g.updateProvider)
			providers.DELETE("/:providerName", g.removeProvider)
			providers.GET("/:providerName/health", g.getProviderHealth)
			providers.GET("/:providerName/models", g.getProviderModels)
		}

		// Routing management endpoints - V3 new
		routing := v1.Group("/routing")
		routing.Use(g.authMiddleware.RequireAdmin())
		{
			routing.GET("/stats", g.getRoutingStats)
			routing.GET("/config", g.getRoutingConfig)
			routing.PUT("/config", g.updateRoutingConfig)
			routing.POST("/test", g.testRouting)
		}

		// Enhanced admin endpoints
		admin := v1.Group("/admin")
		admin.Use(g.authMiddleware.RequireAdmin())
		{
			admin.GET("/status", g.adminStatus)
			admin.GET("/users", g.adminHandlers.GetUsers)
			admin.GET("/users/:userId/stats", g.adminHandlers.GetUserStats)
			admin.GET("/stats", g.adminHandlers.GetSystemStats)
			admin.GET("/metrics", g.getMetrics)
			admin.GET("/config", g.getConfig)
			admin.PUT("/config", g.updateConfig)
		}
	}
}

// chatCompletionsV3 handles chat completion requests with intelligent routing
func (g *EnhancedGatewayV3) chatCompletionsV3(c *gin.Context) {
	var req types.ChatCompletionRequest
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

	// Generate request ID
	requestID := utils.GenerateRequestID()
	req.RequestID = requestID

	g.logger.WithField("request_id", requestID).
		WithField("model", req.Model).
		WithField("messages_count", len(req.Messages)).
		Info("Processing chat completion request")

	// Route the request using the intelligent routing system
	response, err := g.routingService.RouteRequest(c.Request.Context(), &req)
	if err != nil {
		g.logger.WithField("request_id", requestID).
			WithField("error", err.Error()).
			Error("Failed to route request")

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "ROUTING_FAILED",
				"message": "Failed to route request to available provider",
				"details": err.Error(),
			},
		})
		return
	}

	// Log the request to database
	go g.logRequestToDatabase(c.Request.Context(), &req, response, requestID)

	// Add routing metadata
	response.RequestID = requestID

	c.JSON(http.StatusOK, response)
}

// Provider management handlers
func (g *EnhancedGatewayV3) listProviders(c *gin.Context) {
	stats := g.routingService.GetProviderStats()
	c.JSON(http.StatusOK, gin.H{
		"providers": stats["providers"],
		"health":    stats["health"],
	})
}

func (g *EnhancedGatewayV3) addProvider(c *gin.Context) {
	var providerConfig types.ProviderConfig
	if err := c.ShouldBindJSON(&providerConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Invalid provider configuration",
				"details": err.Error(),
			},
		})
		return
	}

	if err := g.routingService.AddProvider(&providerConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "PROVIDER_ADD_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Provider added successfully",
		"provider": providerConfig.Name,
	})
}

func (g *EnhancedGatewayV3) getProvider(c *gin.Context) {
	providerName := c.Param("providerName")

	provider, err := g.providerRegistry.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "PROVIDER_NOT_FOUND",
				"message": fmt.Sprintf("Provider %s not found", providerName),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":       provider.GetName(),
		"type":       provider.GetType(),
		"rate_limit": provider.GetRateLimit(),
	})
}

func (g *EnhancedGatewayV3) updateProvider(c *gin.Context) {
	// TODO: Implement provider update
	c.JSON(http.StatusNotImplemented, gin.H{
		"message": "Provider update not implemented yet",
	})
}

func (g *EnhancedGatewayV3) removeProvider(c *gin.Context) {
	providerName := c.Param("providerName")

	if err := g.routingService.RemoveProvider(providerName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "PROVIDER_REMOVE_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Provider %s removed successfully", providerName),
	})
}

func (g *EnhancedGatewayV3) getProviderHealth(c *gin.Context) {
	providerName := c.Param("providerName")

	provider, err := g.providerRegistry.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "PROVIDER_NOT_FOUND",
				"message": fmt.Sprintf("Provider %s not found", providerName),
			},
		})
		return
	}

	health, err := provider.HealthCheck(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "HEALTH_CHECK_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, health)
}

func (g *EnhancedGatewayV3) getProviderModels(c *gin.Context) {
	providerName := c.Param("providerName")

	provider, err := g.providerRegistry.GetProvider(providerName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "PROVIDER_NOT_FOUND",
				"message": fmt.Sprintf("Provider %s not found", providerName),
			},
		})
		return
	}

	models, err := provider.GetModels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "MODELS_FETCH_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"provider": providerName,
		"models":   models,
	})
}

// Routing management handlers
func (g *EnhancedGatewayV3) getRoutingStats(c *gin.Context) {
	stats := g.routingService.GetProviderStats()
	c.JSON(http.StatusOK, stats)
}

func (g *EnhancedGatewayV3) getRoutingConfig(c *gin.Context) {
	// TODO: Get current routing configuration
	c.JSON(http.StatusOK, gin.H{
		"strategy": "round_robin",
		"message":  "Routing config retrieval not fully implemented",
	})
}

func (g *EnhancedGatewayV3) updateRoutingConfig(c *gin.Context) {
	var config router.RouterConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Invalid routing configuration",
				"details": err.Error(),
			},
		})
		return
	}

	if err := g.routingService.UpdateConfig(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "CONFIG_UPDATE_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Routing configuration updated successfully",
	})
}

func (g *EnhancedGatewayV3) testRouting(c *gin.Context) {
	var req types.ChatCompletionRequest
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

	// Get cost estimates from all providers
	estimates := g.routingService.EstimateCost(&req)

	c.JSON(http.StatusOK, gin.H{
		"model":            req.Model,
		"cost_estimates":   estimates,
		"available_models": g.routingService.GetAvailableModels(c.Request.Context()),
	})
}

// Configuration management handlers
func (g *EnhancedGatewayV3) getConfig(c *gin.Context) {
	config := g.configManager.GetConfig()
	c.JSON(http.StatusOK, config)
}

func (g *EnhancedGatewayV3) updateConfig(c *gin.Context) {
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Invalid configuration format",
				"details": err.Error(),
			},
		})
		return
	}

	if err := g.configManager.UpdateConfig(updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "CONFIG_UPDATE_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Configuration updated successfully",
	})
}

// Enhanced health checks
func (g *EnhancedGatewayV3) basicHealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "3.0.0",
	})
}

func (g *EnhancedGatewayV3) healthCheckDetailed(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Check database health
	dbHealthy := true
	if err := g.db.Ping(); err != nil {
		dbHealthy = false
	}

	// Check Redis health
	redisHealthy := true
	if err := g.redis.Ping(ctx); err != nil {
		redisHealthy = false
	}

	// Check provider health
	providerHealth := g.routingService.HealthCheck(ctx)
	healthyProviders := 0
	totalProviders := len(providerHealth)
	for _, health := range providerHealth {
		if health.IsHealthy {
			healthyProviders++
		}
	}

	status := "healthy"
	if !dbHealthy || !redisHealthy || healthyProviders == 0 {
		status = "degraded"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    status,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "3.0.0",
		"checks": gin.H{
			"database": dbHealthy,
			"redis":    redisHealthy,
			"providers": gin.H{
				"healthy": healthyProviders,
				"total":   totalProviders,
				"details": providerHealth,
			},
		},
	})
}

// Start starts the enhanced gateway v3.0 server
func (g *EnhancedGatewayV3) Start() error {
	// Start routing service
	if err := g.routingService.Start(); err != nil {
		return fmt.Errorf("failed to start routing service: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", g.config.Server.Host, g.config.Server.Port)

	g.server = &http.Server{
		Addr:         addr,
		Handler:      g.router,
		ReadTimeout:  g.config.Server.ReadTimeout,
		WriteTimeout: g.config.Server.WriteTimeout,
		IdleTimeout:  g.config.Server.IdleTimeout,
	}

	g.logger.WithField("address", addr).Info("Starting Enhanced LLM Gateway v3.0 server")

	if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the gateway
func (g *EnhancedGatewayV3) Shutdown(ctx context.Context) error {
	g.logger.Info("Shutting down Enhanced LLM Gateway v3.0...")

	// Stop routing service
	if err := g.routingService.Stop(); err != nil {
		g.logger.WithField("error", err.Error()).Warn("Failed to stop routing service")
	}

	// Shutdown HTTP server
	if err := g.server.Shutdown(ctx); err != nil {
		g.logger.WithField("error", err.Error()).Error("Failed to shutdown HTTP server")
		return err
	}

	// Close database connection
	if err := g.db.Close(); err != nil {
		g.logger.WithField("error", err.Error()).Warn("Failed to close database connection")
	}

	// Close Redis connection
	if err := g.redis.Close(); err != nil {
		g.logger.WithField("error", err.Error()).Warn("Failed to close Redis connection")
	}

	g.logger.Info("Enhanced LLM Gateway v3.0 shutdown completed")
	return nil
}

// Run starts the gateway and handles graceful shutdown
func (g *EnhancedGatewayV3) Run() error {
	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Start()
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case sig := <-sigCh:
		g.logger.WithField("signal", sig.String()).Info("Received shutdown signal")

		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		return g.Shutdown(ctx)
	}

	return nil
}

// Helper methods
func (g *EnhancedGatewayV3) initializeProviders() error {
	// Add mock provider for testing
	mockConfig := &types.ProviderConfig{
		Name:         "mock-openai",
		Type:         "mock",
		Enabled:      true,
		BaseURL:      "http://localhost:8080/mock",
		Priority:     1,
		Weight:       100,
		Timeout:      30 * time.Second,
		Models:       []string{"gpt-3.5-turbo", "gpt-4"},
		CustomConfig: make(map[string]string),
	}

	mockProvider := router.NewMockProvider(mockConfig, g.logger)
	return g.providerRegistry.RegisterProvider(mockProvider)
}

func (g *EnhancedGatewayV3) logRequestToDatabase(ctx context.Context, req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse, requestID string) {
	// Extract user info from context if available
	var userID *uint
	if user := g.authService.GetUserFromContext(ctx); user != nil {
		userID = &user.ID
	}

	requestLog := &storage.Request{
		RequestID:        requestID,
		UserID:           userID,
		Method:           "POST",
		Path:             "/v1/chat/completions",
		ModelName:        req.Model,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
		StatusCode:       200,
		ResponseTimeMs:   resp.LatencyMs,
		RequestTime:      time.Now(),
		ResponseAt:       time.Now(),
	}

	if err := g.db.RequestRepo().Create(requestLog); err != nil {
		g.logger.WithField("request_id", requestID).
			WithField("error", err.Error()).
			Warn("Failed to log request to database")
	}
}

func (g *EnhancedGatewayV3) adminStatus(c *gin.Context) {
	uptime := time.Since(time.Now()) // This would be calculated from start time

	c.JSON(http.StatusOK, gin.H{
		"status":  "running",
		"version": "3.0.0",
		"uptime":  uptime.String(),
		"features": gin.H{
			"authentication":      true,
			"database":            true,
			"redis":               true,
			"rate_limiting":       true,
			"monitoring":          true,
			"intelligent_routing": true,
			"provider_management": true,
			"circuit_breakers":    true,
		},
		"providers":  len(g.providerRegistry.GetProviders()),
		"middleware": 6,
	})
}

func (g *EnhancedGatewayV3) getMetrics(c *gin.Context) {
	stats := g.routingService.GetProviderStats()
	c.JSON(http.StatusOK, gin.H{
		"routing_stats":   stats["routing"],
		"provider_health": stats["health"],
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	})
}
