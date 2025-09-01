// Enhanced LLM Gateway v3.0 - Main Entry Point
// Featuring intelligent routing, provider management, and circuit breakers
package main

import (
	"log"
	"os"

	"github.com/llm-gateway/gateway/internal/config"
	"github.com/llm-gateway/gateway/internal/gateway"
	"github.com/llm-gateway/gateway/pkg/types"
	"github.com/llm-gateway/gateway/pkg/utils"
)

func main() {
	log.Println("Starting Enhanced LLM Gateway v3.0...")

	// Setup logger
	logger := setupLogger()

	// Setup configuration
	cfg, err := setupConfig(logger)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Run server
	if err := runServer(cfg, logger); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func setupLogger() *utils.Logger {
	logConfig := &types.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "",
	}
	logger := utils.NewLogger(logConfig)
	logger.Info("Logger initialized")
	return logger
}

func setupConfig(logger *utils.Logger) (*types.Config, error) {
	configManager := config.NewManager()

	// Load configuration
	if err := configManager.Load(); err != nil {
		return nil, err
	}

	// Watch for configuration changes
	if err := configManager.Watch(nil); err != nil {
		logger.WithField("error", err.Error()).Warn("Failed to watch configuration file")
	}

	cfg := configManager.Get()

	// Override with environment variables if set
	if jwtSecret := os.Getenv("GATEWAY_AUTH_JWT_SECRET"); jwtSecret != "" {
		cfg.Auth.JWTSecret = jwtSecret
		logger.Info("JWT secret loaded from environment variable")
	}

	if dbPassword := os.Getenv("DATABASE_PASSWORD"); dbPassword != "" {
		cfg.Database.Password = dbPassword
		logger.Info("Database password loaded from environment variable")
	}

	if redisPassword := os.Getenv("REDIS_PASSWORD"); redisPassword != "" {
		cfg.Redis.Password = redisPassword
		logger.Info("Redis password loaded from environment variable")
	}

	logger.Info("Configuration loaded successfully")
	return cfg, nil
}

func runServer(cfg *types.Config, logger *utils.Logger) error {
	// Create enhanced gateway v3.0
	gatewayV3, err := gateway.NewEnhancedGatewayV3(cfg, logger)
	if err != nil {
		return err
	}

	// Display feature status
	log.Println("Enhanced LLM Gateway v3.0 started successfully!")
	log.Println("Features enabled:")
	log.Println("  ✅ PostgreSQL Database")
	log.Println("  ✅ Redis Cache")
	log.Println("  ✅ JWT Authentication")
	log.Println("  ✅ API Key Management")
	log.Println("  ✅ Rate Limiting")
	log.Println("  ✅ Request Logging")
	log.Println("  ✅ Admin Dashboard APIs")
	log.Println("  🚀 Intelligent Routing")
	log.Println("  🚀 Multi-Provider Support")
	log.Println("  🚀 Circuit Breakers")
	log.Println("  🚀 Load Balancing")
	log.Println("  🚀 Health Monitoring")
	log.Println("  🚀 Dynamic Configuration")

	// Run server with graceful shutdown
	return gatewayV3.Run()
}

