// Package gateway provides the core gateway functionality
package gateway

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/llm-gateway/gateway/pkg/types"
)

// Gateway represents the core gateway service
type Gateway struct {
	config     *types.Config
	router     *gin.Engine
	server     *http.Server
	logger     *logrus.Logger
	middleware []types.Middleware
}

// New creates a new Gateway instance
func New(cfg *types.Config) *Gateway {
	logger := logrus.New()

	// Configure logger based on config
	level, err := logrus.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	if cfg.Logging.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{})
	}

	// Set Gin mode based on log level
	if level == logrus.DebugLevel {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Add default middleware
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(requestLoggingMiddleware(logger))

	gateway := &Gateway{
		config:     cfg,
		router:     router,
		logger:     logger,
		middleware: make([]types.Middleware, 0),
	}

	// Setup routes
	gateway.setupRoutes()

	return gateway
}

// setupRoutes configures the API routes
func (g *Gateway) setupRoutes() {
	// Health check endpoint
	g.router.GET("/health", g.healthCheck)

	// API version 1
	v1 := g.router.Group("/v1")
	{
		// Chat completions endpoint (OpenAI compatible)
		v1.POST("/chat/completions", g.chatCompletions)

		// Gateway management endpoints
		admin := v1.Group("/admin")
		{
			admin.GET("/status", g.adminStatus)
			admin.GET("/providers", g.listProviders)
			admin.GET("/metrics", g.getMetrics)
			admin.GET("/config", g.getConfig)
			admin.PUT("/config", g.updateConfig)
			admin.POST("/providers/:id/test", g.testProvider)
			admin.PUT("/providers/:id", g.updateProvider)
		}
	}
}

// Start starts the gateway server
func (g *Gateway) Start() error {
	addr := fmt.Sprintf("%s:%d", g.config.Server.Host, g.config.Server.Port)

	g.server = &http.Server{
		Addr:         addr,
		Handler:      g.router,
		ReadTimeout:  g.config.Server.ReadTimeout,
		WriteTimeout: g.config.Server.WriteTimeout,
		IdleTimeout:  g.config.Server.IdleTimeout,
	}

	g.logger.WithField("address", addr).Info("Starting LLM Gateway server")

	if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

// Stop gracefully stops the gateway server
func (g *Gateway) Stop(ctx context.Context) error {
	g.logger.Info("Shutting down LLM Gateway server")

	if g.server != nil {
		return g.server.Shutdown(ctx)
	}

	return nil
}

// AddMiddleware adds a middleware to the gateway
func (g *Gateway) AddMiddleware(middleware types.Middleware) {
	g.middleware = append(g.middleware, middleware)
}

// Health check handler
func (g *Gateway) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
	})
}

// Chat completions handler (OpenAI compatible endpoint)
func (g *Gateway) chatCompletions(c *gin.Context) {
	var req types.Request

	if err := c.ShouldBindJSON(&req); err != nil {
		g.logger.WithError(err).Error("Failed to bind request")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Set request ID and timestamp
	req.ID = generateRequestID()
	req.Timestamp = time.Now()

	g.logger.WithFields(logrus.Fields{
		"request_id": req.ID,
		"model":      req.Model,
		"user_id":    req.UserID,
	}).Info("Processing chat completion request")

	// TODO: Implement actual provider routing and calling
	// For now, return a mock response
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
					Content: "This is a mock response from the LLM Gateway. Provider routing will be implemented soon.",
				},
				FinishReason: func() *string { s := "stop"; return &s }(),
			},
		},
		Usage: types.Usage{
			PromptTokens:     50,
			CompletionTokens: 20,
			TotalTokens:      70,
		},
		Latency: 100,
	}

	c.JSON(http.StatusOK, response)
}

// Admin status handler
func (g *Gateway) adminStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":     "running",
		"uptime":     time.Since(time.Now()), // TODO: Track actual uptime
		"version":    "1.0.0",
		"providers":  0, // TODO: Get actual provider count
		"middleware": len(g.middleware),
	})
}

// List providers handler
func (g *Gateway) listProviders(c *gin.Context) {
	// TODO: Implement actual provider listing from registry
	providers := []gin.H{
		{
			"id":        "openai-1",
			"name":      "OpenAI GPT-3.5",
			"type":      "openai",
			"status":    "online",
			"endpoint":  "https://api.openai.com/v1",
			"model":     "gpt-3.5-turbo",
			"maxTokens": 4096,
			"timeout":   30000,
			"rateLimits": gin.H{
				"requestsPerMinute": 1000,
				"tokensPerMinute":   50000,
				"remainingRequests": 856,
				"remainingTokens":   42340,
				"resetTime":         "2024-01-01T15:30:00Z",
			},
			"health": gin.H{
				"lastCheck":    "2024-01-01T14:45:00Z",
				"responseTime": 45,
				"errorRate":    0.02,
			},
			"stats": gin.H{
				"totalRequests":   1234,
				"totalTokens":     56789,
				"avgResponseTime": 48,
				"successRate":     0.98,
			},
		},
		{
			"id":        "claude-1",
			"name":      "Anthropic Claude",
			"type":      "claude",
			"status":    "online",
			"endpoint":  "https://api.anthropic.com/v1",
			"model":     "claude-3-sonnet-20240229",
			"maxTokens": 4096,
			"timeout":   30000,
			"rateLimits": gin.H{
				"requestsPerMinute": 500,
				"tokensPerMinute":   25000,
				"remainingRequests": 423,
				"remainingTokens":   18650,
				"resetTime":         "2024-01-01T15:30:00Z",
			},
			"health": gin.H{
				"lastCheck":    "2024-01-01T14:45:00Z",
				"responseTime": 52,
				"errorRate":    0.01,
			},
			"stats": gin.H{
				"totalRequests":   892,
				"totalTokens":     34567,
				"avgResponseTime": 55,
				"successRate":     0.99,
			},
		},
		{
			"id":        "baidu-1",
			"name":      "百度文心一言",
			"type":      "baidu",
			"status":    "warning",
			"endpoint":  "https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop",
			"model":     "ERNIE-Bot-turbo",
			"maxTokens": 2048,
			"timeout":   30000,
			"rateLimits": gin.H{
				"requestsPerMinute": 300,
				"tokensPerMinute":   15000,
				"remainingRequests": 245,
				"remainingTokens":   12340,
				"resetTime":         "2024-01-01T15:30:00Z",
			},
			"health": gin.H{
				"lastCheck":    "2024-01-01T14:45:00Z",
				"responseTime": 78,
				"errorRate":    0.05,
			},
			"stats": gin.H{
				"totalRequests":   567,
				"totalTokens":     23456,
				"avgResponseTime": 82,
				"successRate":     0.95,
			},
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    providers,
	})
}

// Get metrics handler
func (g *Gateway) getMetrics(c *gin.Context) {
	// TODO: Implement actual metrics collection
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"totalRequests":     1234,
			"requestsPerSecond": 12.5,
			"avgResponseTime":   145,
			"errorRate":         0.02,
			"successRate":       0.98,
			"totalTokens":       56789,
			"tokensPerSecond":   45.2,
			"activeConnections": 23,
			"uptime":            "2h 15m",
		},
	})
}

// Get configuration handler
func (g *Gateway) getConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"server": gin.H{
				"host": g.config.Server.Host,
				"port": g.config.Server.Port,
				"cors": true,
			},
			"routing": gin.H{
				"strategy":              "round-robin",
				"providers":             []string{},
				"fallbackEnabled":       true,
				"circuitBreakerEnabled": true,
				"retryPolicy": gin.H{
					"maxRetries": 3,
					"retryDelay": 1000,
				},
			},
			"monitoring": gin.H{
				"enabled":  true,
				"interval": 30,
				"alerts": gin.H{
					"errorRate":    5.0,
					"responseTime": 1000,
				},
			},
			"rateLimit": gin.H{
				"enabled":           true,
				"requestsPerMinute": 1000,
			},
		},
	})
}

// Update configuration handler
func (g *Gateway) updateConfig(c *gin.Context) {
	var config map[string]interface{}
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_CONFIG",
				"message": "Invalid configuration format",
			},
		})
		return
	}

	// TODO: Implement real configuration update
	g.logger.Info("Configuration updated")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration updated successfully",
	})
}

// Test provider handler
func (g *Gateway) testProvider(c *gin.Context) {
	providerID := c.Param("id")

	// TODO: Implement real provider testing
	g.logger.WithField("provider_id", providerID).Info("Testing provider")

	// Simulate test result
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"status":       "online",
			"responseTime": 45,
			"testResult":   "success",
		},
	})
}

// Update provider handler
func (g *Gateway) updateProvider(c *gin.Context) {
	providerID := c.Param("id")

	var providerConfig map[string]interface{}
	if err := c.ShouldBindJSON(&providerConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_PROVIDER_CONFIG",
				"message": "Invalid provider configuration format",
			},
		})
		return
	}

	// TODO: Implement real provider update
	g.logger.WithField("provider_id", providerID).Info("Provider updated")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider updated successfully",
	})
}

// CORS middleware
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Requested-With")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

// Request logging middleware
func requestLoggingMiddleware(logger *logrus.Logger) gin.HandlerFunc {
	return gin.LoggerWithWriter(logger.Out)
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}
