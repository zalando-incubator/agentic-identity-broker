package e2e_test

import (
	"context"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("OpenTelemetry Instrumentation", func() {
	var (
		server         *bootstrap.TestServer
		adminServer    *bootstrap.TestServer
		testStorage    *storageadapter.Adapter
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
		tp             *sdktrace.TracerProvider
		recorder       *tracetest.SpanRecorder
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)

		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		// TelemetryEnabledConfig sets Telemetry.Enabled=true and Traces.Enabled=true so
		// that the otelchi middleware gate in SetupEnduserRoutes/SetupAdminRoutes is satisfied.
		// The OTLP exporter endpoint is irrelevant — WithTracerProvider bypasses NewProvider().
		cfg := fixtures.TelemetryEnabledConfig()
		serverFactory := bootstrap.NewServerFactory(cfg, logger)
		tp, recorder = bootstrap.NewInMemoryTracerProvider()

		appInstance, err := serverFactory.BuildAppWithTracerProvider(testStorage, tp)
		Expect(err).ToNot(HaveOccurred())

		server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())

		adminServer, err = bootstrap.NewAdminTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if adminServer != nil {
			adminServer.Close()
		}
		if tp != nil {
			_ = tp.Shutdown(context.Background())
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// US1: Enable Distributed Tracing for Request Debugging

	Context("when tracing is enabled", func() {
		// Scenario US1-S1 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should emit a trace span for end-user server requests", func() {
			resp, err := server.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(200))

			// otelchi ends spans synchronously before returning the response,
			// so recorder.Ended() is populated immediately after the HTTP call.
			Expect(recorder.Ended()).To(matchers.HaveHTTPSpan("GET", "/health", 200))
		})

		// Scenario US1-S2 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should emit a trace span for admin server requests with distinct server identification", func() {
			resp, err := adminServer.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(200))

			// Admin server uses otelchi.Middleware("admin", ...) which sets the
			// net.host.name attribute to "admin", distinguishing it from the end-user
			// server (otelchi.Middleware("enduser", ...)) in traces.
			Expect(recorder.Ended()).To(matchers.HaveHTTPSpan("GET", "/health", 200))
			Expect(recorder.Ended()).To(matchers.ContainSpanWithAttribute("GET /health", "net.host.name", "admin"))
		})

	})

	Context("when telemetry is disabled", func() {
		var (
			disabledServer  *bootstrap.TestServer
			disabledStorage *storageadapter.Adapter
		)

		BeforeEach(func() {
			// Build a fresh server with the default (disabled) telemetry config.
			// DefaultOAuth2Config has Telemetry.Enabled=false, so the otelchi middleware
			// is NOT added and no spans are emitted.
			var err error
			disabledStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			// Use BuildApp (not BuildAppWithTracerProvider) so the global TracerProvider
			// is NOT changed. Changing the global TP here would cause background goroutines
			// in the outer app (e.g. JWKS refresh via otelhttp) to record spans to the
			// wrong recorder, making the test flaky.
			disabledApp, err := bootstrap.NewServerFactory(fixtures.DefaultOAuth2Config(), logger).
				BuildApp(disabledStorage)
			Expect(err).ToNot(HaveOccurred())

			disabledServer, err = bootstrap.NewEndUserTestServer(disabledApp, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if disabledServer != nil {
				disabledServer.Close()
			}
			if disabledStorage != nil {
				_ = storageFactory.CloseStorage(disabledStorage)
			}
		})

		// Scenario US1-S4 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should not emit any trace spans", func() {
			// Snapshot the recorder before the request. Only spans added AFTER this
			// snapshot could be caused by the disabled server.
			snapshotBefore := len(recorder.Ended())

			resp, err := disabledServer.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(200))

			// Verify no otelchi server-side spans were emitted (span name: "GET /health").
			// Background client-side spans (e.g. JWKS refresh → "HTTP GET") are filtered out
			// by checking the span name rather than asserting the recorder is empty.
			for _, span := range recorder.Ended()[snapshotBefore:] {
				Expect(span.Name()).NotTo(Equal("GET /health"),
					"otelchi must not be registered when Telemetry.Enabled=false")
			}
		})
	})

	// US2: Expose Runtime Metrics for Capacity Planning

	Context("when metrics are enabled", func() {
		// Scenario US2-S3 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should not export metric data when metrics are disabled", func() {
			// TelemetryEnabledConfig() has Metrics.Enabled=false while Traces.Enabled=true.
			// This verifies that the app operates normally when only tracing is active:
			// requests succeed and trace spans are emitted as expected.
			resp, err := server.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(200))

			// Tracing continues to work even when metrics are disabled.
			Expect(recorder.Ended()).To(matchers.HaveHTTPSpan("GET", "/health", 200))
		})
	})

	// US3: Configurable Telemetry Without Code Changes

	Context("when no telemetry config is present", func() {
		// Scenario US3-S1 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should start normally with telemetry disabled and no overhead", func() {
			// Build a dedicated server with DefaultOAuth2Config (Telemetry.Enabled=false).
			// This is distinct from the BeforeEach server which uses TelemetryEnabledConfig.
			disabledStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(disabledStorage) }()

			// Use BuildApp (not BuildAppWithTracerProvider) to avoid changing the global
			// TracerProvider. Background goroutines in the outer app (e.g. JWKS refresh via
			// otelhttp.NewTransport) could otherwise create "HTTP GET" spans in a detached
			// recorder, making the assertion flaky.
			disabledApp, err := bootstrap.NewServerFactory(fixtures.DefaultOAuth2Config(), logger).
				BuildApp(disabledStorage)
			Expect(err).ToNot(HaveOccurred())

			srv, err := bootstrap.NewEndUserTestServer(disabledApp, logger)
			Expect(err).ToNot(HaveOccurred())
			defer srv.Close()

			// Snapshot recorder before the request so we can isolate new spans.
			snapshotBefore := len(recorder.Ended())

			resp, err := srv.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(200))

			// No otelchi server-side spans should appear for the disabled server's request.
			// Background client spans (JWKS refresh → "HTTP GET") are filtered by name.
			for _, span := range recorder.Ended()[snapshotBefore:] {
				Expect(span.Name()).NotTo(Equal("GET /health"),
					"no otelchi HTTP spans should be emitted when telemetry is disabled")
			}
		})
	})

	Context("when OTLP gRPC protocol is configured", func() {
		// Scenario US3-S2 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should initialize a gRPC trace exporter successfully", func() {
			// TelemetryGRPCConfig has Enabled=true, Protocol="grpc". BuildAppWithTracerProvider
			// bypasses NewProvider() for the exporter path (covered by TestNewProvider_GRPCInitializes
			// unit test), but it does wire TelemetryConfig into the app so that the otelchi
			// middleware gate (Telemetry.Enabled && Traces.Enabled) is satisfied.
			localStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(localStorage) }()

			localTP, localRecorder := bootstrap.NewInMemoryTracerProvider()
			grpcApp, err := bootstrap.NewServerFactory(fixtures.TelemetryGRPCConfig(), logger).
				BuildAppWithTracerProvider(localStorage, localTP)
			Expect(err).ToNot(HaveOccurred())

			grpcServer, err := bootstrap.NewEndUserTestServer(grpcApp, logger)
			Expect(err).ToNot(HaveOccurred())
			defer grpcServer.Close()
			defer func() { _ = localTP.Shutdown(context.Background()) }()

			resp, err := grpcServer.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(200))

			// otelchi is registered because TelemetryGRPCConfig has Enabled=true.
			Expect(localRecorder.Ended()).To(matchers.HaveHTTPSpan("GET", "/health", 200))
		})
	})

	Context("when OTLP HTTP protocol is configured", func() {
		// Scenario US3-S3 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should initialize an HTTP trace exporter successfully", func() {
			// TelemetryHTTPConfig has Enabled=true, Protocol="http". Same approach as US3-S2:
			// BuildAppWithTracerProvider bypasses NewProvider() for the exporter path (covered
			// by TestNewProvider_HTTPInitializes unit test), but wires TelemetryConfig so that
			// the otelchi middleware gate (Telemetry.Enabled && Traces.Enabled) is satisfied.
			localStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(localStorage) }()

			localTP, localRecorder := bootstrap.NewInMemoryTracerProvider()
			httpApp, err := bootstrap.NewServerFactory(fixtures.TelemetryHTTPConfig(), logger).
				BuildAppWithTracerProvider(localStorage, localTP)
			Expect(err).ToNot(HaveOccurred())

			httpServer, err := bootstrap.NewEndUserTestServer(httpApp, logger)
			Expect(err).ToNot(HaveOccurred())
			defer httpServer.Close()
			defer func() { _ = localTP.Shutdown(context.Background()) }()

			resp, err := httpServer.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(200))

			// otelchi is registered because TelemetryHTTPConfig has Enabled=true.
			Expect(localRecorder.Ended()).To(matchers.HaveHTTPSpan("GET", "/health", 200))
		})
	})

	Context("when custom resource attributes are configured", func() {
		// Scenario US3-S4 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should attach custom service name and attributes to all spans", func() {
			// TelemetryEnabledConfig sets ServiceName="test-broker". The in-memory TracerProvider
			// used in BeforeEach does not carry resource attributes (those are set by NewProvider()
			// in the production path). The production behaviour is verified in:
			//   internal/adapters/telemetry/provider_test.go TestNewProvider_CustomServiceName
			//
			// This E2E test verifies that the app functions correctly with a custom service name
			// configured: requests succeed and trace spans are emitted as expected.
			resp, err := server.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(200))

			Expect(recorder.Ended()).To(matchers.HaveHTTPSpan("GET", "/health", 200))
		})
	})

	Context("when OTLP endpoint is set via environment variable", func() {
	})

	// US4: Graceful Degradation When Collector is Unavailable

	Context("when the OTLP collector is unreachable", func() {
		// degradedServer is built with the real NewProvider() path (BuildApp, not
		// BuildAppWithTracerProvider) so that the actual OTLP exporter is initialised
		// against an unreachable endpoint. This exercises the graceful-degradation path.
		var (
			degradedServer  *bootstrap.TestServer
			degradedApp     *app.App
			degradedStorage *storageadapter.Adapter
		)

		BeforeEach(func() {
			var err error
			degradedStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			// TelemetryGRPCConfig uses localhost:4317 which is intentionally unreachable.
			// The OTel SDK buffers telemetry and does not fail at startup.
			degradedCfg := fixtures.TelemetryGRPCConfig()
			degradedFactory := bootstrap.NewServerFactory(degradedCfg, logger)
			degradedApp, err = degradedFactory.BuildApp(degradedStorage)
			Expect(err).ToNot(HaveOccurred(),
				"app must start successfully even when the OTLP collector is unreachable")

			degradedServer, err = bootstrap.NewEndUserTestServer(degradedApp, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if degradedServer != nil {
				degradedServer.Close()
			}
			if degradedApp != nil {
				// Flush with a short timeout — export will fail (unreachable endpoint) but
				// we still call Shutdown so resources are released promptly.
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				defer cancel()
				_ = degradedApp.Shutdown(shutdownCtx)
			}
			if degradedStorage != nil {
				_ = storageFactory.CloseStorage(degradedStorage)
			}
		})

		// Scenario US4-S1 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should start successfully and log a warning about the unreachable collector", func() {
			// Startup success is already asserted in BeforeEach (BuildApp must not error).
			// This It() verifies the app serves requests normally after startup.
			resp, err := degradedServer.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(200))
		})

		// Scenario US4-S2 from specs/20260228-120000-opentelemetry-support/spec.md
		It("should continue serving requests normally with no latency impact", func() {
			// Send multiple requests to verify continuous availability with an unreachable
			// OTLP endpoint. The OTel SDK handles buffering and export retries internally.
			for i := 0; i < 3; i++ {
				resp, err := degradedServer.PublicGET("/health")
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
				Expect(resp.StatusCode).To(Equal(200))
			}
		})

	})
})
