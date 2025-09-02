// Package main provides the entry point for the LLM Gateway server
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/llm-gateway/gateway/internal/config"
	"github.com/llm-gateway/gateway/internal/gateway"
)

func main() {
	// Initialize configuration
	if err := config.InitDefault(); err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}

	cfg := config.Get()
	if cfg == nil {
		log.Fatal("Configuration is nil")
	}

	// Validate configuration
	configManager := config.Default()
	if err := configManager.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Create gateway instance
	gw := gateway.New(cfg)

	// Start server in a goroutine
	go func() {
		if err := gw.Start(); err != nil {
			log.Fatalf("Failed to start gateway: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Create a deadline for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown server
	if err := gw.Stop(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
