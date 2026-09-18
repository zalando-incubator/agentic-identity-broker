package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/httpclient"
	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
	extprocfixtures "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
)

const (
	extprocApprovalBootstrapTimeout = 5 * time.Second
	extprocGRPCShutdownTimeout      = 5 * time.Second
)

// ExtProcEnvironment runs the production ExtProc composition against an end-user TestServer.
type ExtProcEnvironment struct {
	Address string

	issuer     *httptest.Server
	grpcServer *grpc.Server
	exchanger  *extprocserver.TokenExchanger
	authorizer authorization.Authorizer
	syncCancel context.CancelFunc
	syncDone   chan struct{}
	logger     *slog.Logger
	closeOnce  sync.Once
}

// NewExtProcEnvironment starts production ExtProc components wired to the supplied broker.
func NewExtProcEnvironment(brokerBaseURL, clientAssertion, policyPath string, logger *slog.Logger) (*ExtProcEnvironment, error) {
	if logger == nil {
		logger = slog.Default()
	}

	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": clientAssertion, "token_type": "Bearer", "expires_in": 3600})
	}))

	cfg := extprocfixtures.DefaultConfig()
	cfg.GRPC.Bind = "127.0.0.1"
	cfg.GRPC.Port = 0
	cfg.OAuth2.TokenEndpoint = brokerBaseURL + "/oauth2/token"
	cfg.OAuth2.Issuer = issuer.URL
	cfg.OAuth2.ClientCredentialsEndpoint = issuer.URL + "/oauth/token"
	cfg.OAuth2.TLS.AllowHTTP = true
	cfg.Authorization = extprocconfig.AuthorizationConfig{
		Enabled: true,
		Policy: extprocconfig.PolicyConfig{
			Path:     policyPath,
			Package:  "aib.extproc.authz",
			Decision: "result",
		},
		DefaultDecision:   "deny",
		EvaluationTimeout: 500 * time.Millisecond,
		MaxBodySize:       1024 * 1024,
	}
	cfg.ToolApprovals = extprocconfig.ToolApprovalsConfig{
		Enabled:                true,
		URL:                    brokerBaseURL,
		LongPollTimeoutSeconds: 1,
		ApprovalCacheIdleTTL:   5 * time.Minute,
		RequestTimeout:         5 * time.Second,
		MaxStaleness:           10 * time.Second,
	}
	cfg.Sessions.Extraction.HTTPHeader = "Mcp-Session-Id"

	environment := &ExtProcEnvironment{issuer: issuer, logger: logger}
	exchanger, err := extprocserver.NewTokenExchanger(cfg, logger)
	if err != nil {
		issuer.Close()
		return nil, fmt.Errorf("initialize ExtProc token exchanger: %w", err)
	}
	environment.exchanger = exchanger

	authorizer, err := authorization.NewOPAAuthorizer(&cfg.Authorization, logger)
	if err != nil {
		environment.Close()
		return nil, fmt.Errorf("initialize ExtProc OPA authorizer: %w", err)
	}
	environment.authorizer = authorizer

	approvalHTTPClient, err := httpclient.New(cfg, 0)
	if err != nil {
		environment.Close()
		return nil, fmt.Errorf("initialize ExtProc approval HTTP client: %w", err)
	}
	approvalClient, err := approval.NewClient(cfg.ToolApprovals.URL, cfg.ToolApprovals.RequestTimeout, exchanger, approvalHTTPClient)
	if err != nil {
		environment.Close()
		return nil, fmt.Errorf("initialize ExtProc approval client: %w", err)
	}
	approvalCache := approval.NewCache(cfg.ToolApprovals.ApprovalCacheIdleTTL, cfg.ToolApprovals.MaxStaleness)
	approvalGate := approval.NewGate(approvalCache, approvalClient)
	svc := extprocserver.NewServerWithApprovalGate(cfg, exchanger, authorizer, approvalGate, logger)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		environment.Close()
		return nil, fmt.Errorf("listen for ExtProc gRPC server: %w", err)
	}

	grpcServer := grpc.NewServer(
		grpc.MaxConcurrentStreams(uint32(cfg.GRPC.MaxConcurrentStreams)),
		grpc.KeepaliveParams(keepalive.ServerParameters{}),
	)
	extprocv3.RegisterExternalProcessorServer(grpcServer, svc)
	environment.grpcServer = grpcServer
	environment.Address = listener.Addr().String()

	syncer := approval.NewSyncer(approvalCache, approvalClient, time.Duration(cfg.ToolApprovals.LongPollTimeoutSeconds)*time.Second, logger)
	syncContext, syncCancel := context.WithCancel(context.Background())
	environment.syncCancel = syncCancel
	environment.syncDone = make(chan struct{})
	go func() {
		defer close(environment.syncDone)
		bootstrapContext, cancelBootstrap := context.WithTimeout(syncContext, extprocApprovalBootstrapTimeout)
		syncer.Bootstrap(bootstrapContext)
		cancelBootstrap()
		if syncContext.Err() != nil {
			return
		}
		syncer.Run(syncContext)
	}()

	go func() {
		if serveErr := grpcServer.Serve(listener); serveErr != nil && serveErr != grpc.ErrServerStopped {
			logger.Error("ExtProc gRPC server error", "error", serveErr)
		}
	}()

	return environment, nil
}

// Close stops the approval syncer and all ExtProc resources. It is safe to call repeatedly.
func (e *ExtProcEnvironment) Close() {
	if e == nil {
		return
	}
	e.closeOnce.Do(func() {
		if e.syncCancel != nil {
			e.syncCancel()
		}
		if e.syncDone != nil {
			<-e.syncDone
		}
		if e.grpcServer != nil {
			stopExtProcGRPCServer(e.grpcServer, e.logger)
		}
		if e.authorizer != nil {
			e.authorizer.Stop(context.Background())
		}
		if e.exchanger != nil {
			e.exchanger.Shutdown()
		}
		if e.issuer != nil {
			e.issuer.Close()
		}
	})
}

func stopExtProcGRPCServer(server *grpc.Server, logger *slog.Logger) {
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()

	timer := time.NewTimer(extprocGRPCShutdownTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		logger.Warn("ExtProc gRPC graceful shutdown timed out; forcing stop", "timeout", extprocGRPCShutdownTimeout)
		server.Stop()
	}
}
