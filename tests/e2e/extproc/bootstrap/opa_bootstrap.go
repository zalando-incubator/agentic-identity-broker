package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	. "github.com/onsi/gomega" //nolint:staticcheck

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/httpclient"
	extprocserver "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

// NewOPAAuthorizer creates a real OPA authorizer from the given config for use in E2E tests.
// Returns an error if OPA fails to initialize (e.g., policy file not found, syntax error).
func NewOPAAuthorizer(cfg *extprocconfig.Config, logger *slog.Logger) (authorization.Authorizer, error) {
	return authorization.NewOPAAuthorizer(&cfg.Authorization, logger)
}

// StartWithOPAConfig attempts to initialise an OPA authorizer from the given config.
// Returns an error if startup fails (e.g., invalid policy path, Rego syntax error).
// Used in US3 / US4 E2E tests that verify startup-failure behavior.
func StartWithOPAConfig(cfg *extprocconfig.Config, logger *slog.Logger) error {
	auth, err := authorization.NewOPAAuthorizer(&cfg.Authorization, logger)
	if err != nil {
		return fmt.Errorf("StartWithOPAConfig: %w", err)
	}
	// Successfully initialised — stop immediately (we only needed to verify startup succeeds/fails).
	auth.Stop(context.Background())
	return nil
}

// StartWithAuthorizer starts the test environment using a pre-built authorizer.
// The mock OAuth2 and token exchange servers are started as in Start(), but the
// authorizer is provided directly instead of being created from config.
func (e *TestEnvironment) StartWithAuthorizer(auth authorization.Authorizer) {
	e.startMockServers()

	exchanger, err := extprocserver.NewTokenExchanger(e.Config, e.logger)
	Expect(err).NotTo(HaveOccurred(), "failed to create token exchanger")
	e.exchanger = exchanger

	var approvalGate extprocserver.ApprovalGate
	if e.Config.ToolApprovals.Enabled {
		approvalHTTPClient, err := httpclient.New(e.Config, 0)
		Expect(err).NotTo(HaveOccurred(), "failed to create approval HTTP client")
		client, err := approval.NewClient(e.Config.ToolApprovals.URL, e.Config.ToolApprovals.RequestTimeout, exchanger, approvalHTTPClient)
		Expect(err).NotTo(HaveOccurred(), "failed to create approval client")
		e.approvalClient = client
		e.approvalCache = approval.NewCache(e.Config.ToolApprovals.ApprovalCacheIdleTTL, e.Config.ToolApprovals.MaxStaleness)
		approvalGate = approval.NewGate(e.approvalCache, client)
	}

	var svc *extprocserver.Server
	if approvalGate != nil {
		svc = extprocserver.NewServerWithApprovalGate(e.Config, exchanger, auth, approvalGate, e.logger)
	} else {
		svc = extprocserver.NewServerWithAuthorizer(e.Config, exchanger, auth, e.logger)
	}
	e.startGRPCServer(svc)
	e.startApprovalSync()
}
