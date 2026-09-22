package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/agentic-identity-broker/mock-upstream-oauth2-service/internal/config"
	"github.com/agentic-identity-broker/mock-upstream-oauth2-service/internal/server"
)

func main() {
	// Get config directory
	var configDir string

	// Allow override via command line argument
	if len(os.Args) > 1 {
		configDir = os.Args[1]
	} else {
		// Try current directory first (when running from mocks/upstream-oauth2-server)
		if _, err := os.Stat("config.yaml"); err == nil {
			configDir = "."
		} else {
			// Fall back to root-relative path (when running from project root)
			configDir = "mocks/upstream-oauth2-server"
		}
	}

	// Convert to absolute path
	abspath, err := filepath.Abs(configDir)
	if err != nil {
		log.Fatalf("Failed to get absolute path: %v", err)
	}

	// Load configuration
	cfg, err := config.Load(abspath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded: port=%d, bind=%q", cfg.Server.Port, cfg.Server.Bind) // #nosec G706 -- configuration bind value is Go-quoted before logging.

	// Create OAuth2 server
	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		if err := srv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\nShutting down gracefully...")

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to gracefully shutdown: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Server stopped")
}
