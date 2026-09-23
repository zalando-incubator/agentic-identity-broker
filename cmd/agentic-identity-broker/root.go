// Package main is the entry point for the Agentic Identity Broker application.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	httpAdapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/routing"
	storageAdapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var rootCmd = &cobra.Command{
	Use:   "agentic-identity-broker",
	Short: "Agentic Identity Broker - Secure identity management for AI agents",
	Long: `Agentic Identity Broker provides secure identity management,
authentication, and authorization for AI agents and autonomous systems.`,
	RunE: run,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// run is the main execution function for the application.
func run(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create configuration loader
	loader := config.NewLoader()
	loader.SetCommand(cmd)

	// Load configuration
	cfg, err := loader.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Emit audit log
	emitAuditLog(loader)

	// Display startup summary
	displayStartupSummary(loader, cfg)

	// Initialize logger
	logger := initializeLogger(cfg.Log)
	logger.Info("Agentic Identity Broker starting",
		"log_level", cfg.Log.Level,
		"log_format", cfg.Log.Format)

	// Initialize storage adapter
	storage, err := storageAdapter.NewAdapter(&cfg.Storage)
	if err != nil {
		return fmt.Errorf("failed to create storage adapter: %w", err)
	}

	// Build application with all dependencies using builder pattern
	// Constitution Principle VI: Domain depends on ports, not adapters
	// Constitution Principle VII: Configuration-Driven Design
	application, err := app.NewBuilder().
		WithConfig(cfg).
		WithStorage(storage).
		WithLogger(logger).
		Build()
	if err != nil {
		return fmt.Errorf("failed to build application: %w", err)
	}
	defer func() {
		// Release background workers on startup failures as well as normal shutdown.
		if application.Shutdown != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.Shutdown.Timeout)
			defer shutdownCancel()
			if err := application.Shutdown(shutdownCtx); err != nil {
				logger.Error("Application shutdown error", "error", err)
			}
		}
	}()

	// Create route setup function for admin server
	adminRouteSetup := func(r chi.Router) {
		routing.SetupAdminRoutes(r, application.AdminHandlers, routing.AdminRouteConfig{
			CORS: cfg.Server.Admin.CORS,
		})
	}

	// Create route setup function for enduser server
	enduserRouteSetup := func(r chi.Router) {
		routing.SetupEnduserRoutes(r, application.EnduserHandlers, routing.EnduserRouteConfig{
			Authentication:               cfg.Server.EndUser.Authentication,
			JWTAuthenticator:             application.JWTAuthenticator,
			ApprovalRequestAuthenticator: application.ApprovalRequestAuthenticator,
			Logger:                       application.Logger,
			CORS:                         cfg.Server.EndUser.CORS,
			Telemetry:                    cfg.Telemetry,
		})
	}

	// Create server instances with route setup functions
	adminServer := httpAdapter.NewServer(
		httpAdapter.ServerConfig{
			Port:           cfg.Server.Admin.Port,
			Bind:           cfg.Server.Admin.Bind,
			PublicURL:      cfg.Server.Admin.PublicURL,
			Name:           "admin",
			Telemetry:      cfg.Telemetry,
			RequestContext: &cfg.RequestContext,
			Authentication: cfg.Server.Admin.Authentication,
		},
		adminRouteSetup,
		application.Logger,
	)

	enduserServer := httpAdapter.NewServer(
		httpAdapter.ServerConfig{
			Port:             cfg.Server.EndUser.Port,
			Bind:             cfg.Server.EndUser.Bind,
			PublicURL:        cfg.Server.EndUser.PublicURL,
			Name:             "enduser",
			Telemetry:        cfg.Telemetry,
			RequestContext:   &cfg.RequestContext,
			Authentication:   cfg.Server.EndUser.Authentication,
			JWTAuthenticator: application.JWTAuthenticator,
			HealthComponents: application.EnduserHealthComponents,
		},
		enduserRouteSetup,
		application.Logger,
	)

	// Setup signal handling for graceful shutdown
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start servers with atomic startup pattern (both must bind, or neither runs)
	// ADR 004-dual-server-isolation.md compliance
	logger.Info("Starting dual-port HTTP servers",
		"enduser_port", cfg.Server.EndUser.Port,
		"admin_port", cfg.Server.Admin.Port)

	// Use errgroup for concurrent startup with automatic context cancellation
	g, ctx := errgroup.WithContext(sigCtx)

	// Phase 1: Concurrent Listen (bind to ports)
	logger.Debug("Phase 1: Concurrent port binding")

	// Use channels to safely communicate listener values from goroutines
	enduserListenerCh := make(chan net.Listener, 1)
	adminListenerCh := make(chan net.Listener, 1)

	// Bind end-user server
	g.Go(func() error {
		logger.Debug("Binding end-user server")
		listener, err := enduserServer.Listen()
		if err != nil {
			logger.Error("End-user server bind failed", "error", err)
			return fmt.Errorf("enduser server bind failed: %w", err)
		}
		enduserListenerCh <- listener
		logger.Info("End-user server bound successfully")
		return nil
	})

	// Bind admin server
	g.Go(func() error {
		logger.Debug("Binding admin server")
		listener, err := adminServer.Listen()
		if err != nil {
			logger.Error("Admin server bind failed", "error", err)
			return fmt.Errorf("admin server bind failed: %w", err)
		}
		adminListenerCh <- listener
		logger.Info("Admin server bound successfully")
		return nil
	})

	// Wait for both binds to complete
	if err := g.Wait(); err != nil {
		logger.Error("Atomic startup failed during bind phase", "error", err)
		return err
	}

	// Receive listeners from channels (guaranteed to be safe after g.Wait())
	enduserListener := <-enduserListenerCh
	adminListener := <-adminListenerCh

	logger.Info("Phase 1 complete: Both servers bound successfully")

	// Phase 2: Concurrent Serve (start accepting connections)
	logger.Debug("Phase 2: Starting HTTP servers")

	g, ctx = errgroup.WithContext(ctx)

	// Serve end-user server
	g.Go(func() error {
		logger.Info("Starting end-user server")
		if err := enduserServer.Serve(ctx, enduserListener); err != nil {
			logger.Error("End-user server failed", "error", err)
			return fmt.Errorf("enduser server failed: %w", err)
		}
		return nil
	})

	// Serve admin server
	g.Go(func() error {
		logger.Info("Starting admin server")
		if err := adminServer.Serve(ctx, adminListener); err != nil {
			logger.Error("Admin server failed", "error", err)
			return fmt.Errorf("admin server failed: %w", err)
		}
		return nil
	})

	// Start approval sync subscriber if configured (PostgreSQL LISTEN/NOTIFY)
	if application.ApprovalSyncSubscriber != nil {
		g.Go(func() error {
			logger.Info("Starting approval sync subscriber (LISTEN/NOTIFY)")
			if err := application.ApprovalSyncSubscriber.Listen(ctx); err != nil && ctx.Err() == nil {
				logger.Error("Approval sync subscriber failed", "error", err)
				return fmt.Errorf("approval sync subscriber failed: %w", err)
			}
			return nil
		})
	}

	// Wait for either signal or server error
	var startErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		startErr = g.Wait()
	}()

	// Wait for signal
	<-sigCtx.Done()

	// Signal received, initiate graceful shutdown
	logger.Info("Shutdown signal received, initiating graceful shutdown")

	// Create context with shutdown timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.Shutdown.Timeout)
	defer shutdownCancel()

	// Shutdown both servers concurrently
	shutdownGroup := &errgroup.Group{}
	shutdownGroup.Go(func() error {
		return enduserServer.Shutdown(shutdownCtx, cfg.Server.Shutdown.Timeout)
	})
	shutdownGroup.Go(func() error {
		return adminServer.Shutdown(shutdownCtx, cfg.Server.Shutdown.Timeout)
	})

	if err := shutdownGroup.Wait(); err != nil {
		logger.Error("Shutdown error", "error", err)
		return fmt.Errorf("shutdown error: %w", err)
	}

	// Wait for servers to finish
	<-done
	if startErr != nil && startErr != context.Canceled {
		logger.Error("Server error", "error", startErr)
		return fmt.Errorf("server error: %w", startErr)
	}

	logger.Info("Servers shut down successfully")
	return nil
}

// initializeLogger creates a structured logger based on configuration.
func initializeLogger(logCfg ports.LogConfig) *slog.Logger {
	var level slog.Level
	switch logCfg.Level {
	case ports.LogLevelDebug:
		level = slog.LevelDebug
	case ports.LogLevelInfo:
		level = slog.LevelInfo
	case ports.LogLevelWarn:
		level = slog.LevelWarn
	case ports.LogLevelError:
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler
	if logCfg.Format == ports.LogFormatJSON {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	return slog.New(handler)
}

// displayStartupSummary shows configuration details at startup.
// Displays all keys, values (redacted if sensitive), and sources.
func displayStartupSummary(loader *config.Loader, cfg interface{}) {
	fmt.Println("=== Configuration Summary ===")

	// Get all sources
	sources := loader.GetSources()

	// Build a map of keys to their source for display
	keyToSource := make(map[string]string)
	for _, source := range sources {
		for _, key := range source.Keys {
			// Higher precedence wins (last one in the slice)
			keyToSource[key] = formatSource(source)
		}
	}

	// Display configuration values
	if c, ok := cfg.(*ports.Config); ok {
		displayConfigValue("log.level", string(c.Log.Level), keyToSource["log.level"])
		displayConfigValue("log.format", string(c.Log.Format), keyToSource["log.format"])
		displayConfigValue("server.enduser.port", fmt.Sprintf("%d", c.Server.EndUser.Port), keyToSource["server.enduser.port"])
		displayConfigValue("server.enduser.bind", c.Server.EndUser.Bind, keyToSource["server.enduser.bind"])
		displayConfigValue("server.admin.port", fmt.Sprintf("%d", c.Server.Admin.Port), keyToSource["server.admin.port"])
		displayConfigValue("server.admin.bind", c.Server.Admin.Bind, keyToSource["server.admin.bind"])
		displayConfigValue("server.shutdown.timeout", c.Server.Shutdown.Timeout.String(), keyToSource["server.shutdown.timeout"])
	}

	fmt.Println("\n=== Configuration Sources ===")
	for _, source := range sources {
		fmt.Printf("  [%d] %s: %s (loaded at %s)\n",
			source.Precedence,
			source.Type,
			source.Path,
			source.LoadedAt.Format(time.RFC3339))
	}
	fmt.Println()
}

// displayConfigValue displays a single configuration value with redaction and source.
func displayConfigValue(key string, value string, source string) {
	displayValue := config.Redact(key, value)
	if source == "" {
		source = "default"
	}
	fmt.Printf("  %s: %v [source: %s]\n", key, displayValue, source)
}

// formatSource formats a ConfigSource for display.
func formatSource(source ports.ConfigSource) string {
	switch source.Type {
	case ports.SourceTypeDefault:
		return "default"
	case ports.SourceTypeEnvFile:
		return fmt.Sprintf("env (%s)", source.Path)
	case ports.SourceTypeYAML:
		return fmt.Sprintf("yaml (%s)", source.Path)
	case ports.SourceTypeCLI:
		return "cli"
	default:
		return string(source.Type)
	}
}

// emitAuditLog outputs a structured JSON audit log to stdout.
// Includes all configuration sources, keys loaded, and redacted keys.
func emitAuditLog(loader *config.Loader) {
	sources := loader.GetSources()

	// Build audit log structure
	auditLog := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"event":     "configuration_loaded",
		"sources":   make([]map[string]interface{}, 0, len(sources)),
	}

	// Collect all keys and identify redacted ones
	allKeys := make(map[string]bool)
	redactedKeys := make([]string, 0)

	for _, source := range sources {
		sourceData := map[string]interface{}{
			"type":       source.Type,
			"path":       source.Path,
			"precedence": source.Precedence,
			"loaded_at":  source.LoadedAt.Format(time.RFC3339),
			"keys":       source.Keys,
		}
		auditLog["sources"] = append(auditLog["sources"].([]map[string]interface{}), sourceData)

		// Track all keys
		for _, key := range source.Keys {
			allKeys[key] = true
			if config.IsSensitive(key) {
				redactedKeys = append(redactedKeys, key)
			}
		}
	}

	// Convert keys map to sorted slice
	keysSlice := make([]string, 0, len(allKeys))
	for key := range allKeys {
		keysSlice = append(keysSlice, key)
	}
	sort.Strings(keysSlice)
	sort.Strings(redactedKeys)

	auditLog["keys"] = keysSlice
	auditLog["redacted_keys"] = redactedKeys

	// Output as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(auditLog); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to emit audit log: %v\n", err)
	}
}

func init() {
	// Configuration file path flag
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file path (overrides IDENTITY_BROKER_CONFIG_PATH)")

	// Logging configuration flags
	rootCmd.PersistentFlags().String("log-level", "", "log level: debug, info, warn, error")
	rootCmd.PersistentFlags().String("log-format", "", "log format: text, json")

	// Server configuration flags
	rootCmd.PersistentFlags().Int("server.enduser.port", 0, "end-user server port (default: 8000)")
	rootCmd.PersistentFlags().String("server.enduser.bind", "", "end-user server bind address (default: ::)")
	rootCmd.PersistentFlags().Int("server.admin.port", 0, "admin server port (default: 14000)")
	rootCmd.PersistentFlags().String("server.admin.bind", "", "admin server bind address (default: ::)")
	rootCmd.PersistentFlags().Duration("server.shutdown.timeout", 0, "graceful shutdown timeout (default: 30s)")

	// Request security-context configuration flags
	rootCmd.PersistentFlags().Bool("request_context.trusted_proxy.enabled", false, "trust configured forwarded header for client IP derivation")
	rootCmd.PersistentFlags().String("request_context.trusted_proxy.forwarded_header", "", "forwarded header name for client IP derivation")
	rootCmd.PersistentFlags().Bool("request_context.trace.response_enabled", true, "emit traceresponse header on HTTP responses")
}
