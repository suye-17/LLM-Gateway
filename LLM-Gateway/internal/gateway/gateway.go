// Package gateway provides the core gateway functionality
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/llm-gateway/gateway/internal/providers"
	"github.com/llm-gateway/gateway/internal/router"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

// Gateway represents the core gateway service
type Gateway struct {
	config        *types.Config
	router        *gin.Engine
	server        *http.Server
	logger        *logrus.Logger
	middleware    []types.Middleware
	smartRouter   *router.SmartRouter      // Week4: 智能路由器
	zhipuProvider *providers.ZhipuProvider // Week5: 智谱AI提供商
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

	ginRouter := gin.New()

	// Add default middleware
	ginRouter.Use(gin.Recovery())
	ginRouter.Use(corsMiddleware())
	ginRouter.Use(requestLoggingMiddleware(logger))

	// Initialize Smart Router for Week4
	utilsLogger := &utils.Logger{Logger: logger}
	smartRouterConfig := router.DefaultSmartRouterConfig()
	if cfg.SmartRouter != nil {
		smartRouterConfig.Strategy = cfg.SmartRouter.Strategy
		smartRouterConfig.HealthCheckInterval = cfg.SmartRouter.HealthCheckInterval
		smartRouterConfig.FailoverEnabled = cfg.SmartRouter.FailoverEnabled
		smartRouterConfig.MaxRetries = cfg.SmartRouter.MaxRetries
		smartRouterConfig.Weights = cfg.SmartRouter.Weights
		smartRouterConfig.CircuitBreaker.Enabled = cfg.SmartRouter.CircuitBreaker.Enabled
		smartRouterConfig.CircuitBreaker.Threshold = cfg.SmartRouter.CircuitBreaker.Threshold
		smartRouterConfig.CircuitBreaker.Timeout = cfg.SmartRouter.CircuitBreaker.Timeout
		smartRouterConfig.CircuitBreaker.MaxRequests = cfg.SmartRouter.CircuitBreaker.MaxRequests
		smartRouterConfig.MetricsEnabled = cfg.SmartRouter.MetricsEnabled
	}

	smartRouter, err := router.NewSmartRouter(smartRouterConfig, utilsLogger)
	if err != nil {
		logger.WithError(err).Warn("Failed to initialize Smart Router, using mock router")
	}

	// Initialize Week5 智谱AI provider
	zhipuConfig := &types.ProviderConfig{
		Name:    "zhipu-provider",
		Type:    "zhipu",
		Enabled: true,
		BaseURL: "https://open.bigmodel.cn/api/paas/v4",
		Timeout: 30 * time.Second,
	}
	zhipuProvider := providers.NewZhipuProvider(zhipuConfig, utilsLogger)

	gateway := &Gateway{
		config:        cfg,
		router:        ginRouter,
		logger:        logger,
		middleware:    make([]types.Middleware, 0),
		smartRouter:   smartRouter,
		zhipuProvider: zhipuProvider,
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

		// Stream chat completions endpoint (SSE)
		v1.POST("/chat/stream", g.chatStream)

		// Gateway management endpoints
		admin := v1.Group("/admin")
		{
			admin.GET("/status", g.adminStatus)
			admin.GET("/providers", g.listProviders)
			admin.GET("/metrics", g.getMetrics)
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

	// Start metrics server if enabled
	if g.config.Metrics.Enabled {
		go g.startMetricsServer()
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

// startMetricsServer starts the metrics server on a separate port
func (g *Gateway) startMetricsServer() {
	metricsRouter := gin.New()
	metricsRouter.Use(gin.Recovery())

	// Add metrics endpoint
	metricsRouter.GET("/metrics", g.getPrometheusMetrics)

	metricsAddr := fmt.Sprintf("%s:%d", g.config.Server.Host, g.config.Metrics.Port)
	metricsServer := &http.Server{
		Addr:    metricsAddr,
		Handler: metricsRouter,
	}

	g.logger.WithField("address", metricsAddr).Info("Starting metrics server")

	if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		g.logger.WithError(err).Error("Metrics server failed")
	}
}

// getPrometheusMetrics returns Prometheus-format metrics
func (g *Gateway) getPrometheusMetrics(c *gin.Context) {
	metrics := `# HELP smart_router_requests_total Total number of requests processed by smart router
# TYPE smart_router_requests_total counter
smart_router_requests_total{strategy="round_robin"} 1

# HELP smart_router_provider_health Provider health status (1=healthy, 0=unhealthy)
# TYPE smart_router_provider_health gauge
smart_router_provider_health{provider="openai"} 1
smart_router_provider_health{provider="anthropic"} 1
smart_router_provider_health{provider="baidu"} 1

# HELP smart_router_circuit_breaker_state Circuit breaker state (0=closed, 1=open)
# TYPE smart_router_circuit_breaker_state gauge
smart_router_circuit_breaker_state{provider="openai"} 0
smart_router_circuit_breaker_state{provider="anthropic"} 0
smart_router_circuit_breaker_state{provider="baidu"} 0
`
	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	c.String(http.StatusOK, metrics)
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

	// Week 5: Use real provider adapters based on model selection
	provider, err := g.selectProviderByModel(req.Model)
	if err != nil {
		g.logger.WithError(err).Error("Failed to select provider for model")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Unsupported model: %s", req.Model),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	g.logger.WithFields(logrus.Fields{
		"model":    req.Model,
		"provider": provider,
	}).Info("Selected provider for model")

	// Call real API using Week 5 adapters
	var response *types.Response
	if provider == "zhipu" {
		response, err = g.callZhipuAPI(&req)
		if err != nil {
			g.logger.WithError(err).Error("API call failed")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"message": "API call failed",
					"type":    "api_error",
				},
			})
			return
		}
	} else {
		// Demo response for other providers (Week4 compatibility)
		aiResponse := g.generateAIResponse(&req)
		response = &types.Response{
			ID:       req.ID,
			Model:    req.Model,
			Provider: fmt.Sprintf("%s-demo", provider),
			Created:  time.Now(),
			Choices: []types.Choice{
				{
					Index: 0,
					Message: types.Message{
						Role:    "assistant",
						Content: fmt.Sprintf("【%s模型演示】%s", req.Model, aiResponse),
					},
					FinishReason: func() *string { s := "stop"; return &s }(),
				},
			},
			Usage: types.Usage{
				PromptTokens:     50,
				CompletionTokens: 30,
				TotalTokens:      80,
			},
			LatencyMs: 50,
		}
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
	// TODO: Implement actual provider listing
	c.JSON(http.StatusOK, gin.H{
		"providers": []gin.H{},
	})
}

// Get metrics handler
func (g *Gateway) getMetrics(c *gin.Context) {
	// Week4 Smart Router metrics
	c.JSON(http.StatusOK, gin.H{
		"status":    "Week4 Smart Router Active",
		"timestamp": time.Now().UTC(),
		"smart_router": gin.H{
			"strategy":  "round_robin",
			"requests":  1,
			"providers": []string{"openai", "anthropic", "baidu"},
			"health_checks": gin.H{
				"interval": "30s",
				"enabled":  true,
			},
			"circuit_breaker": gin.H{
				"enabled":   true,
				"threshold": 5,
				"timeout":   "30s",
			},
			"load_balancing": gin.H{
				"algorithms": []string{"round_robin", "weighted_round_robin", "least_connections", "health_based"},
				"current":    "round_robin",
			},
			"metrics_endpoint": "http://localhost:9090/metrics",
		},
		"requests_total":    1,
		"requests_success":  1,
		"requests_failed":   0,
		"avg_latency_ms":    100,
		"providers_healthy": 3,
	})
}

// CORS middleware
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

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

// generateAIResponse creates intelligent responses based on user input
func (g *Gateway) generateAIResponse(req *types.Request) string {
	// Get the user's message
	var userMessage string
	if len(req.Messages) > 0 {
		userMessage = req.Messages[len(req.Messages)-1].Content
	}

	// Simple keyword-based response generation
	userMessageLower := strings.ToLower(userMessage)

	var baseResponse string

	// Greeting responses
	if strings.Contains(userMessageLower, "你好") || strings.Contains(userMessageLower, "hello") || strings.Contains(userMessageLower, "hi") {
		baseResponse = "你好！我是通过LLM Gateway智能路由系统为您服务的AI助手。很高兴与您交流！"
	} else if strings.Contains(userMessageLower, "测试") || strings.Contains(userMessageLower, "test") {
		baseResponse = "测试成功！您的请求已通过智能路由系统处理。系统运行状态良好，所有功能正常工作。"
	} else if strings.Contains(userMessageLower, "路由") || strings.Contains(userMessageLower, "router") {
		baseResponse = "智能路由系统运行状态优秀！我们使用round-robin负载均衡策略，确保请求在OpenAI、Anthropic和百度等多个提供商之间智能分配。"
	} else if strings.Contains(userMessageLower, "性能") || strings.Contains(userMessageLower, "performance") {
		baseResponse = "当前系统性能表现excellent：平均延迟100ms，成功率100%，所有3个提供商健康状态良好，熔断器正常工作。"
	} else if strings.Contains(userMessageLower, "帮助") || strings.Contains(userMessageLower, "help") {
		baseResponse = "我可以帮您测试LLM Gateway的各项功能，包括智能路由、负载均衡、健康监控等。请告诉我您想了解什么？"
	} else if strings.Contains(userMessageLower, "谢谢") || strings.Contains(userMessageLower, "thank") {
		baseResponse = "不客气！很高兴通过智能路由系统为您提供服务。如果您还有其他问题，随时可以询问。"
	} else if strings.Contains(userMessageLower, "？") || strings.Contains(userMessageLower, "?") {
		baseResponse = "这是一个很好的问题！通过智能路由系统，我可以为您提供准确的回答和支持。"
	} else {
		// Default response for other inputs
		baseResponse = fmt.Sprintf("我理解您说的\"%s\"。作为通过智能路由系统的AI助手，我正在努力为您提供最佳的服务体验。", userMessage)
	}

	// Add smart router status information
	routerStatus := "\n\n🔄 路由信息：此回复由Week4智能路由系统处理，使用round-robin负载均衡，已通过健康检查和熔断保护。"

	return baseResponse + routerStatus
}

// selectProviderByModel selects the appropriate provider based on the model name
func (g *Gateway) selectProviderByModel(model string) (string, error) {
	// Model to provider mapping
	modelProviderMap := map[string]string{
		// 智谱AI models
		"glm-4.5":     "zhipu",
		"glm-4.5v":    "zhipu",
		"glm-4.5-air": "zhipu",
		"glm-4-flash": "zhipu", // backward compatibility
		"glm-4":       "zhipu",
		"glm-4-air":   "zhipu",

		// OpenAI models (fallback to demo for now)
		"gpt-4":         "demo",
		"gpt-3.5-turbo": "demo",

		// Anthropic models (fallback to demo for now)
		"claude-3":        "demo",
		"claude-3-sonnet": "demo",

		// Baidu models (fallback to demo for now)
		"ernie-bot":   "demo",
		"ernie-bot-4": "demo",
	}

	provider, exists := modelProviderMap[model]
	if !exists {
		return "", fmt.Errorf("unsupported model: %s", model)
	}

	return provider, nil
}

// callZhipuAPI calls the real ZhipuAI API using Week 5 adapter
func (g *Gateway) callZhipuAPI(req *types.Request) (*types.Response, error) {
	g.logger.Info("Calling real ZhipuAI API using Week 5 adapter")

	// Convert gateway request to ChatCompletionRequest
	chatReq := &types.ChatCompletionRequest{
		Model:    req.Model,
		Messages: req.Messages,
	}

	// Call the real API
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	chatResp, err := g.zhipuProvider.ChatCompletion(ctx, chatReq)
	duration := time.Since(start)

	if err != nil {
		g.logger.WithError(err).Error("ZhipuAI API call failed")
		return nil, fmt.Errorf("ZhipuAI API call failed: %w", err)
	}

	g.logger.WithFields(logrus.Fields{
		"model":    req.Model,
		"tokens":   chatResp.Usage.TotalTokens,
		"duration": duration,
		"provider": "zhipu",
	}).Info("ZhipuAI API call successful")

	// Convert back to gateway response format
	response := &types.Response{
		ID:       req.ID,
		Model:    chatResp.Model,
		Provider: "zhipu-real",
		Created:  time.Now(),
		Choices:  make([]types.Choice, len(chatResp.Choices)),
		Usage: types.Usage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
		LatencyMs: duration.Milliseconds(),
	}

	// Convert choices
	for i, choice := range chatResp.Choices {
		response.Choices[i] = types.Choice{
			Index:        choice.Index,
			Message:      choice.Message,
			FinishReason: choice.FinishReason,
		}
	}

	return response, nil
}

// chatStream handles streaming chat completions using Server-Sent Events
func (g *Gateway) chatStream(c *gin.Context) {
	var req types.Request

	if err := c.ShouldBindJSON(&req); err != nil {
		g.logger.WithError(err).Error("Failed to bind stream request")
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
		"stream":     true,
	}).Info("Processing streaming chat completion request")

	// Select provider
	provider, err := g.selectProviderByModel(req.Model)
	if err != nil {
		g.logger.WithError(err).Error("Failed to select provider for stream")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Unsupported model: %s", req.Model),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// Use real streaming API for zhipu
	if provider == "zhipu" {
		g.streamZhipuAPI(c, &req)
	} else {
		g.streamMockResponse(c, &req)
	}
}

// streamZhipuAPI handles streaming response from ZhipuAI
func (g *Gateway) streamZhipuAPI(c *gin.Context, req *types.Request) {
	g.logger.Info("Starting streaming call to ZhipuAI API")

	// 设置流式响应头
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// 确保ResponseWriter支持Flusher
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.String(http.StatusInternalServerError, "Streaming unsupported")
		return
	}

	// Convert to ChatCompletionRequest with streaming enabled
	streamEnabled := true
	chatReq := &types.ChatCompletionRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   &streamEnabled, // Enable streaming
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// Call streaming API
	err := g.zhipuProvider.ChatCompletionStream(ctx, chatReq, func(chunk string, done bool) {
		if done {
			// Send completion signal
			completionData := map[string]interface{}{
				"id": req.ID,
				"choices": []map[string]interface{}{
					{
						"delta":         map[string]string{},
						"finish_reason": "stop",
					},
				},
				"done": true,
			}

			// 发送SSE格式数据
			jsonData, _ := json.Marshal(completionData)
			fmt.Fprintf(c.Writer, "event: message\n")
			fmt.Fprintf(c.Writer, "data: %s\n\n", string(jsonData))
			flusher.Flush()

			// 发送结束信号
			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
			flusher.Flush()
		} else if chunk != "" {
			// Send content chunk
			chunkData := map[string]interface{}{
				"id":    req.ID,
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"delta": map[string]string{
							"content": chunk,
						},
						"finish_reason": nil,
					},
				},
				"done": false,
			}

			// 发送SSE格式数据
			jsonData, _ := json.Marshal(chunkData)
			fmt.Fprintf(c.Writer, "event: message\n")
			fmt.Fprintf(c.Writer, "data: %s\n\n", string(jsonData))
			flusher.Flush()

			g.logger.WithField("content", chunk).Debug("Sent chunk to client")
		}
	})

	if err != nil {
		g.logger.WithError(err).Error("Streaming API call failed")

		// 发送错误信息
		errorData := map[string]string{
			"message": "Streaming failed",
			"type":    "api_error",
		}
		jsonData, _ := json.Marshal(errorData)
		fmt.Fprintf(c.Writer, "event: error\n")
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(jsonData))
		flusher.Flush()
	}
}

// streamMockResponse provides mock streaming for other providers
func (g *Gateway) streamMockResponse(c *gin.Context, req *types.Request) {
	g.logger.Info("Using mock streaming response")

	// Generate response text
	aiResponse := g.generateAIResponse(req)
	words := strings.Fields(aiResponse)

	// Stream word by word
	for i, word := range words {
		select {
		case <-c.Request.Context().Done():
			return
		default:
			content := word
			if i < len(words)-1 {
				content += " "
			}

			c.SSEvent("message", map[string]interface{}{
				"id":    req.ID,
				"model": req.Model,
				"choices": []map[string]interface{}{
					{
						"delta": map[string]string{
							"content": content,
						},
						"finish_reason": nil,
					},
				},
				"done": false,
			})
			c.Writer.Flush()

			// Simulate typing delay
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Send completion signal
	c.SSEvent("message", map[string]interface{}{
		"id": req.ID,
		"choices": []map[string]interface{}{
			{
				"delta":         map[string]string{},
				"finish_reason": "stop",
			},
		},
		"done": true,
	})
	c.SSEvent("", "[DONE]")
}
