package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

// ---------------------------------------------------------------------------
// Signal-interrupted startup regression tests
// ---------------------------------------------------------------------------

func TestIsSignalCancellation_SignalContextCanceled_ReturnsTrue(t *testing.T) {
	// Simulate SIGINT/SIGTERM: parent context is already cancelled, and
	// the init error is context.Canceled.
	sigCtx, cancel := context.WithCancel(context.Background())
	cancel() // signal fired

	assert.True(t, isSignalCancellation(sigCtx, context.Canceled),
		"must recognise context.Canceled after signal as a clean exit")
}

func TestIsSignalCancellation_SignalContextCanceledWrappedError_ReturnsTrue(t *testing.T) {
	sigCtx, cancel := context.WithCancel(context.Background())
	cancel()

	wrapped := fmt.Errorf("OTLP connect: %w", context.Canceled)
	assert.True(t, isSignalCancellation(sigCtx, wrapped),
		"must recognise wrapped context.Canceled after signal as a clean exit")
}

func TestIsSignalCancellation_RealInitFailure_WithSignal_ReturnsFalse(t *testing.T) {
	// Signal fired, but the init failure is NOT context.Canceled — this is a
	// real startup error that happened to coincide with a signal.
	sigCtx, cancel := context.WithCancel(context.Background())
	cancel()

	realErr := fmt.Errorf("OTLP connect: connection refused")
	assert.False(t, isSignalCancellation(sigCtx, realErr),
		"must NOT treat a non-cancellation error as a clean exit, even if signal fired")
}

func TestIsSignalCancellation_NoSignal_CanceledError_ReturnsFalse(t *testing.T) {
	// No signal, but the error is context.Canceled (e.g. from a timeout).
	// Without a signal, this is a real failure.
	sigCtx := context.Background()

	assert.False(t, isSignalCancellation(sigCtx, context.Canceled),
		"must NOT treat context.Canceled as clean exit when no signal was received")
}

func TestIsSignalCancellation_NoSignal_NoError_ReturnsFalse(t *testing.T) {
	assert.False(t, isSignalCancellation(context.Background(), nil),
		"nil error must not be treated as signal cancellation")
}

func TestIsSignalCancellation_DeadlineExceeded_ReturnsFalse(t *testing.T) {
	// DeadlineExceeded is intentionally not treated as signal cancellation:
	// accepting it would mask real init timeouts if a signal arrives in the
	// narrow window between the timeout firing and the check.
	sigCtx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.False(t, isSignalCancellation(sigCtx, context.DeadlineExceeded),
		"DeadlineExceeded must not be treated as signal cancellation")
}

type blockingApprovalSyncer struct {
	bootstrapStarted chan struct{}
	bootstrapRelease chan struct{}
	runStarted       chan struct{}
	stopped          chan struct{}
	bootstrapCalls   int
	runCalls         int
}

func (s *blockingApprovalSyncer) Bootstrap(context.Context) {
	s.bootstrapCalls++
	close(s.bootstrapStarted)
	<-s.bootstrapRelease
}

func (s *blockingApprovalSyncer) Run(ctx context.Context) {
	s.runCalls++
	close(s.runStarted)
	<-ctx.Done()
	close(s.stopped)
}

func TestStartApprovalSyncer_BootstrapsAsynchronouslyAndStopsOnce(t *testing.T) {
	syncer := &blockingApprovalSyncer{
		bootstrapStarted: make(chan struct{}),
		bootstrapRelease: make(chan struct{}),
		runStarted:       make(chan struct{}),
		stopped:          make(chan struct{}),
	}

	stop := startApprovalSyncer(context.Background(), syncer)
	select {
	case <-syncer.bootstrapStarted:
	case <-time.After(time.Second):
		t.Fatal("approval syncer bootstrap did not start asynchronously")
	}
	select {
	case <-syncer.runStarted:
		t.Fatal("syncer started polling before bootstrap completed")
	default:
	}

	close(syncer.bootstrapRelease)
	select {
	case <-syncer.runStarted:
	case <-time.After(time.Second):
		t.Fatal("approval syncer did not start after bootstrap")
	}
	stop()
	<-syncer.stopped
	stop()

	assert.Equal(t, 1, syncer.bootstrapCalls)
	assert.Equal(t, 1, syncer.runCalls)
}

type mockGRPCServerStopper struct {
	gracefulStarted chan struct{}
	gracefulRelease chan struct{}
	gracefulCalls   int
	stopCalls       int
}

func (m *mockGRPCServerStopper) GracefulStop() {
	m.gracefulCalls++
	if m.gracefulStarted != nil {
		select {
		case <-m.gracefulStarted:
		default:
			close(m.gracefulStarted)
		}
	}
	if m.gracefulRelease != nil {
		<-m.gracefulRelease
	}
}

func (m *mockGRPCServerStopper) Stop() {
	m.stopCalls++
	if m.gracefulRelease != nil {
		select {
		case <-m.gracefulRelease:
		default:
			close(m.gracefulRelease)
		}
	}
}

func TestStopGRPCServerWithTimeout_GracefulStopCompletes(t *testing.T) {
	stopper := &mockGRPCServerStopper{}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	stopGRPCServerWithTimeout(stopper, 50*time.Millisecond, logger)

	assert.Equal(t, 1, stopper.gracefulCalls)
	assert.Equal(t, 0, stopper.stopCalls)
}

func TestStopGRPCServerWithTimeout_ForceStopsAfterTimeout(t *testing.T) {
	stopper := &mockGRPCServerStopper{
		gracefulStarted: make(chan struct{}),
		gracefulRelease: make(chan struct{}),
	}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	done := make(chan struct{})
	go func() {
		stopGRPCServerWithTimeout(stopper, 10*time.Millisecond, logger)
		close(done)
	}()

	<-stopper.gracefulStarted
	<-done

	assert.Equal(t, 1, stopper.gracefulCalls)
	assert.Equal(t, 1, stopper.stopCalls)
}

func TestShutdownTelemetry_CallsShutdownWithTimeout(t *testing.T) {
	called := false
	var deadline time.Time
	var hasDeadline bool

	shutdownTelemetry(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), func(ctx context.Context) error {
		called = true
		deadline, hasDeadline = ctx.Deadline()
		return nil
	})

	require.True(t, called, "shutdown helper must invoke the shutdown callback")
	require.True(t, hasDeadline, "shutdown helper must bound the shutdown with a deadline")
	assert.WithinDuration(t, time.Now().Add(5*time.Second), deadline, 500*time.Millisecond,
		"shutdown helper must use the standard 5s shutdown timeout")
}

func TestShutdownTelemetry_LogsShutdownError(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	shutdownTelemetry(logger, func(context.Context) error {
		return fmt.Errorf("exporter close failed")
	})

	assert.Contains(t, logBuf.String(), "exporter close failed")
}

// recvOneProcessor accepts a single ProcessingRequest and answers it.
type recvOneProcessor struct {
	extprocv3.UnimplementedExternalProcessorServer
}

func (recvOneProcessor) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	return stream.Send(&extprocv3.ProcessingResponse{})
}

// A BUFFERED request body reaches ExtProc as one ProcessingRequest, so the gRPC
// server must accept every body that authorization.max_body_size allows.
func TestGRPCServerOptions_AcceptsBufferedBodyUpToMaxBodySize(t *testing.T) {
	tests := []struct {
		name        string
		maxBodySize int
		bodySize    int
	}{
		{name: "body above grpc-go default within max_body_size", maxBodySize: 30 << 20, bodySize: 5 << 20},
		{name: "default max_body_size keeps grpc-go default limit", maxBodySize: 1 << 20, bodySize: 3 << 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &extprocconfig.Config{}
			cfg.GRPC.MaxConcurrentStreams = 100
			cfg.Authorization.MaxBodySize = tt.maxBodySize

			srv := grpc.NewServer(grpcServerOptions(cfg)...)
			extprocv3.RegisterExternalProcessorServer(srv, recvOneProcessor{})
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			go func() { _ = srv.Serve(listener) }()
			t.Cleanup(srv.Stop)

			conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stream, err := extprocv3.NewExternalProcessorClient(conn).Process(ctx)
			require.NoError(t, err)

			require.NoError(t, stream.Send(&extprocv3.ProcessingRequest{
				Request: &extprocv3.ProcessingRequest_RequestBody{
					RequestBody: &extprocv3.HttpBody{Body: make([]byte, tt.bodySize), EndOfStream: true},
				},
			}))
			_, err = stream.Recv()
			require.NoError(t, err, "a %d-byte body within max_body_size=%d must reach ExtProc", tt.bodySize, tt.maxBodySize)
		})
	}
}
