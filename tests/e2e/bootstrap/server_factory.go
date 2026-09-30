// Package bootstrap provides test infrastructure that wraps production code.
// This package implements the VOLATILE layer which isolates E2E tests from
// implementation details while using PRODUCTION dependencies directly.
//
// Constitution Principle VIII: E2E tests use production DI and production adapters.
// This ensures E2E tests validate real code paths, not test-specific code.
package bootstrap

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ServerFactory creates production app.Builder instances with test-friendly defaults.
// This factory wraps production app.Builder and provides convenient methods for E2E testing.
//
// Key responsibilities:
// - Use PRODUCTION app.Builder (no custom DI logic)
// - Provide test-friendly configuration defaults
// - Handle logger creation
// - Validate dependencies before building
//
// Architecture:
// - This is a THIN WRAPPER around production code
// - NO test-specific logic or mocks
// - All wiring uses PRODUCTION adapters
type ServerFactory struct {
	config *ports.Config
	logger *slog.Logger
}

func webDistPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "../../../web/dist")
}

// NewServerFactory creates a new ServerFactory with required configuration.
//
// Parameters:
//   - config: Application configuration (REQUIRED)
//   - logger: Structured logger for app and factory diagnostics
//
// Returns: Configured factory ready to build App instances
//
// Preconditions:
//   - config MUST be non-nil and valid (caller responsible for validation)
//   - logger MUST be non-nil
//
// Postconditions:
//   - Factory is ready to build() multiple App instances
//   - Each build() call creates a fresh App with its own storage
func NewServerFactory(config *ports.Config, logger *slog.Logger) *ServerFactory {
	return &ServerFactory{
		config: config,
		logger: logger,
	}
}

// BuildApp creates a fully-wired application with production dependencies.
// This method wraps production app.Builder following builder pattern exactly.
//
// Parameters:
//   - storage: Storage adapter (PRODUCTION memory or postgres adapter)
//
// Returns:
//   - *app.App: Fully-wired application with all services initialized
//   - error: If validation fails or app.Builder.Build() fails
//
// Responsibilities:
// 1. Validate inputs (config, storage provided)
// 2. Create production app.Builder
// 3. Configure builder with config, storage, logger
// 4. Call production app.Builder.Build()
// 5. Return wired App or error
//
// Example usage:
//
//	factory := NewServerFactory(cfg, logger)
//	storage, _ := NewTestStorage()  // Fresh storage for each test
//	app, err := factory.BuildApp(storage)
//	require.NoError(t, err)
//	// app is ready to pass to TestServer
func (f *ServerFactory) BuildApp(storage interface{}) (*app.App, error) {
	// Validate inputs
	if storage == nil {
		return nil, fmt.Errorf("storage adapter is required")
	}

	if f.config == nil {
		return nil, fmt.Errorf("factory config is required")
	}

	if f.logger == nil {
		return nil, fmt.Errorf("factory logger is required")
	}

	// Create production app.Builder (no custom DI)
	builder := app.NewBuilder()

	storageAdapter, ok := storage.(ports.StorageProvider)
	if !ok {
		return nil, fmt.Errorf("storage must implement ports.StorageProvider")
	}

	// Configure with production pattern (using builder methods)
	builder.
		WithConfig(f.config).
		WithStorage(storageAdapter).
		WithLogger(f.logger).
		WithStaticWebResourcesPath(webDistPath())

	return builder.Build()
}

// BuildAppWithTracerProvider creates a fully-wired application with a custom
// TracerProvider for testing OTel instrumentation.
// The TracerProvider is registered as the global OTel provider and used to
// capture spans emitted during tests.
func (f *ServerFactory) BuildAppWithTracerProvider(storage interface{}, tp *sdktrace.TracerProvider) (*app.App, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage adapter is required")
	}
	if f.config == nil {
		return nil, fmt.Errorf("factory config is required")
	}
	if f.logger == nil {
		return nil, fmt.Errorf("factory logger is required")
	}

	storageAdapter, ok := storage.(ports.StorageProvider)
	if !ok {
		return nil, fmt.Errorf("storage must implement ports.StorageProvider")
	}

	return app.NewBuilder().
		WithConfig(f.config).
		WithStorage(storageAdapter).
		WithLogger(f.logger).
		WithStaticWebResourcesPath(webDistPath()).
		WithTracerProvider(tp).
		Build()
}

// BuildAppWithCIMDFetcher creates a fully-wired application with an injected CIMDFetcher.
// Used in CIMD E2E tests where the mock CIMD server uses a self-signed TLS certificate
// (e.g., httptest.NewTLSServer) that the production fetcher's system cert pool would reject.
func (f *ServerFactory) BuildAppWithCIMDFetcher(storage interface{}, cimdFetcher ports.CIMDFetcher) (*app.App, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage adapter is required")
	}
	if f.config == nil {
		return nil, fmt.Errorf("factory config is required")
	}
	if f.logger == nil {
		return nil, fmt.Errorf("factory logger is required")
	}

	storageAdapter, ok := storage.(ports.StorageProvider)
	if !ok {
		return nil, fmt.Errorf("storage must implement ports.StorageProvider")
	}

	return app.NewBuilder().
		WithConfig(f.config).
		WithStorage(storageAdapter).
		WithLogger(f.logger).
		WithStaticWebResourcesPath(webDistPath()).
		WithCIMDFetcher(cimdFetcher).
		Build()
}

// ValidateFactory checks factory is properly initialized.
// This is useful for early detection of misconfiguration in test setup.
//
// Returns error if:
//   - config is nil
//   - logger is nil
//
// This is optional - called automatically in BuildApp, but available for
// explicit validation in test setup code.
func (f *ServerFactory) ValidateFactory() error {
	if f.config == nil {
		return fmt.Errorf("factory config is nil")
	}

	if f.logger == nil {
		return fmt.Errorf("factory logger is nil")
	}

	return nil
}
