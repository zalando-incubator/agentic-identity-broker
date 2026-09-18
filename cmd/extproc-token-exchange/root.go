// Package main is the entry point for the ExtProc Token Exchange service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	otelslog "go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var rootCmd = &cobra.Command{
	Use:   "extproc-token-exchange",
	Short: "Envoy ExtProc Token Exchange Service",
	Long: `ExtProc Token Exchange Service implements the Envoy External Processor (ExtProc)
gRPC interface for transparent OAuth2 token exchange. It intercepts incoming
requests, exchanges Bearer tokens via RFC 8693, and replaces the Authorization
header before the request reaches the upstream service.`,
	RunE: run,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

const grpcShutdownTimeout = 5 * time.Second

type grpcServerStopper interface {
	GracefulStop()
	Stop()
}

func stopGRPCServerWithTimeout(grpcSrv grpcServerStopper, timeout time.Duration, logger *slog.Logger) {
	done := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return
	case <-timer.C:
		if logger != nil {
			logger.Warn("gRPC graceful shutdown timed out, forcing stop", "timeout", timeout)
		}
		grpcSrv.Stop()
	}
}

// run is the main execution function for the ExtProc Token Exchange service.
func run(cmd *cobra.Command, _ []string) error {
	// 1. Load config — CLI flags (highest) → EXTPROC_* env vars → YAML file → defaults.
	cfg, err := extprocconfig.LoadWithCommand(cmd)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// 2. Initialize logger.
	logger := initLogger(cfg)
	logger.Info("ExtProc Token Exchange Service starting",
		"grpc_bind", cfg.GRPC.Bind,
		"grpc_port", cfg.GRPC.Port,
		"token_endpoint", cfg.OAuth2.TokenEndpoint,
		"issuer", cfg.OAuth2.Issuer,
		"client_id", cfg.OAuth2.ClientID,
		"client_secret", "[REDACTED]")

	// 3. Setup signal handling for graceful shutdown.
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 4. Initialize telemetry if enabled.
	var telemetryShutdown func(context.Context) error
	if cfg.Telemetry.Enabled {
		// SR-003: Log a warning if TLS is disabled (insecure mode)
		if cfg.Telemetry.Exporter.Insecure {
			logger.Warn("telemetry OTLP exporter running without TLS", "insecure", true)
		}
		telemetryCfg := mapTelemetryConfig(cfg.Telemetry)
		// initCtx bounds the exporter connection handshake; initCancel is intentional
		// cleanup of the init deadline and does not affect the long-lived provider.
		// Note: we reuse exporter.timeout (default 10s) as the init deadline. This is
		// a deliberate coupling — init establishes exporter connections, so the timeout
		// should be comparable. If decoupled init timing is needed, add a dedicated
		// telemetry.init_timeout config field.
		initCtx, initCancel := context.WithTimeout(sigCtx, cfg.Telemetry.Exporter.Timeout)
		shutdown, err := telemetry.NewProvider(initCtx, telemetryCfg, logger)
		initCancel()
		if err != nil {
			if isSignalCancellation(sigCtx, err) {
				logger.Info("Telemetry initialization interrupted by signal, shutting down")
				return nil
			}
			return fmt.Errorf("failed to initialize telemetry: %w", err)
		}
		telemetryShutdown = shutdown
		defer func() {
			shutdownTelemetry(logger, telemetryShutdown)
		}()
		logger.Info("Telemetry initialized", "endpoint", cfg.Telemetry.Exporter.Endpoint)

		// T047: Wire slog-to-OTel bridge when telemetry AND logs are both enabled.
		// This enables log-trace correlation (US5): structured log entries will carry
		// trace_id and span_id matching the active span context.
		// Note: slog.SetDefault is intentionally never restored — the process is
		// exiting when shutdown runs, and a bridged logger is correct for the
		// entire service lifetime.
		if cfg.Telemetry.Logs.Enabled {
			otelHandler := otelslog.NewHandler(cfg.Telemetry.ServiceName,
				otelslog.WithLoggerProvider(global.GetLoggerProvider()))
			logger = slog.New(telemetry.NewMultiHandler(logger.Handler(), otelHandler))
			slog.SetDefault(logger)
		}
	}

	// 5. Create TokenExchanger (performs startup client_credentials grant — fails fast on error).
	exchanger, err := extprocserver.NewTokenExchanger(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize token exchanger: %w", err)
	}
	defer exchanger.Shutdown()

	// 5. Optionally initialise OPA authorizer when authorization is enabled.
	var authorizer authorization.Authorizer
	if cfg.Authorization.Enabled {
		logger.Info("OPA authorization enabled", "policy_path", cfg.Authorization.Policy.Path)
		authorizer, err = authorization.NewOPAAuthorizer(&cfg.Authorization, logger)
		if err != nil {
			return fmt.Errorf("failed to initialize OPA authorizer: %w", err)
		}
		defer authorizer.Stop(sigCtx)
	}

	// 6. Create ExtProc server.
	var svc *extprocserver.Server
	if authorizer != nil {
		svc = extprocserver.NewServerWithAuthorizer(cfg, exchanger, authorizer, logger)
	} else {
		svc = extprocserver.NewServer(cfg, exchanger, logger)
	}

	// 7. Create gRPC server with max_concurrent_streams.
	grpcOpts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(uint32(cfg.GRPC.MaxConcurrentStreams)), // #nosec G115 -- Validate rejects values outside the uint32 range.
		grpc.KeepaliveParams(keepalive.ServerParameters{}),
	}
	grpcSrv := grpc.NewServer(grpcOpts...)
	extprocv3.RegisterExternalProcessorServer(grpcSrv, svc)

	// 8. Listen on configured bind:port.
	addr := fmt.Sprintf("%s:%d", cfg.GRPC.Bind, cfg.GRPC.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	logger.Info("gRPC server listening", "addr", addr)

	// 9. Serve in background goroutine.
	serveErr := make(chan error, 1)
	go func() {
		if err := grpcSrv.Serve(listener); err != nil {
			serveErr <- err
		}
	}()

	// 10. Wait for shutdown signal or serve error.
	select {
	case <-sigCtx.Done():
		logger.Info("Shutdown signal received, stopping gRPC server")
	case err := <-serveErr:
		return fmt.Errorf("gRPC server error: %w", err)
	}

	// 11. Graceful shutdown with a bounded fallback for long-lived streams.
	stopGRPCServerWithTimeout(grpcSrv, grpcShutdownTimeout, logger)
	logger.Info("gRPC server stopped")

	logger.Info("ExtProc Token Exchange Service stopped")
	return nil
}

// initLogger creates a structured slog.Logger from the service configuration.
func initLogger(cfg *extprocconfig.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	var handler slog.Handler
	if cfg.Log.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	return slog.New(handler)
}

// isSignalCancellation returns true when err is context.Canceled and the signal
// context is already done — i.e., the cancellation was caused by SIGINT/SIGTERM
// rather than an independent init failure.
//
// Note: context.DeadlineExceeded is intentionally NOT matched here. While a
// signal-cancelled parent context can theoretically produce DeadlineExceeded in
// a derived timeout context, accepting it would mask real init timeouts if a
// signal arrives in the narrow window between the timeout firing and this check.
func isSignalCancellation(sigCtx context.Context, err error) bool {
	return sigCtx.Err() != nil && errors.Is(err, context.Canceled)
}

func shutdownTelemetry(logger *slog.Logger, shutdown func(context.Context) error) {
	if shutdown == nil {
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(shutdownCtx); err != nil && logger != nil {
		logger.Warn("error during telemetry shutdown", "error", err)
	}
}

// mapTelemetryConfig converts ExtProc's local TelemetryConfig to the ports.TelemetryConfig
// interface type for use with the telemetry adapter. This function lives in cmd/ to keep
// internal/extproc/config free of imports from internal/ports (architecture boundary).
func mapTelemetryConfig(c extprocconfig.TelemetryConfig) ports.TelemetryConfig {
	return ports.TelemetryConfig{
		Enabled:            c.Enabled,
		ServiceName:        c.ServiceName,
		ResourceAttributes: c.ResourceAttributes,
		Traces: ports.TracesConfig{
			Enabled:      c.Traces.Enabled,
			SamplingRate: c.Traces.SamplingRate,
			Propagators:  c.Traces.Propagators,
		},
		Metrics: ports.MetricsConfig{
			Enabled:        c.Metrics.Enabled,
			ExportInterval: c.Metrics.ExportInterval,
		},
		Logs: ports.LogsConfig{
			Enabled: c.Logs.Enabled,
		},
		Exporter: ports.OTLPExporterConfig{
			Protocol:    ports.OTLPProtocol(c.Exporter.Protocol),
			Endpoint:    c.Exporter.Endpoint,
			Headers:     c.Exporter.Headers,
			Timeout:     c.Exporter.Timeout,
			Insecure:    c.Exporter.Insecure,
			Compression: ports.OTLPCompression(c.Exporter.Compression),
		},
	}
}

func init() {
	extprocconfig.RegisterFlags(rootCmd)
}
