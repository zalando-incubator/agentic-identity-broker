package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/delegation"
)

// DelegationResolver is the fourth ExtProc port (ADR 045). It returns a fresh broker
// delegation permit, a *delegation.DeniedError, or an error. hit reports a cache hit.
type DelegationResolver interface {
	Resolve(ctx context.Context, subjectToken, resourceURI string) (result delegation.Result, hit bool, err error)
	Shutdown()
}

const delegationUnavailableBody = `{"error":"service_unavailable","error_description":"delegation context unavailable"}`

// WithDelegationResolver enables delegation context. It requires an authorizer.
func (s *Server) WithDelegationResolver(resolver DelegationResolver) *Server {
	s.delegationResolver = resolver
	return s
}

func (s *Server) delegationEnabled() bool {
	return s.delegationResolver != nil && s.authorizer != nil
}

// resolveDelegation resolves and observes one delegation without credentials in logs.
func (s *Server) resolveDelegation(ctx context.Context, logger *slog.Logger, subjectToken, resourceURI string) (*delegation.Result, error) {
	result, hit, err := s.delegationResolver.Resolve(ctx, subjectToken, resourceURI)
	cache := "miss"
	if hit {
		cache = "hit"
	}
	logger = logger.With("operation", "delegation_resolution", "delegation.cache", cache)
	if err != nil {
		var denied *delegation.DeniedError
		if errors.As(err, &denied) {
			logDiagnostic(ctx, logger.With("delegation.code", denied.Code), slog.LevelInfo, "delegation denied", NewDiagnostic(StageClientAuthorization, DetailDelegationDenied))
		} else {
			category := "unavailable"
			var resolveErr *delegation.Error
			if errors.As(err, &resolveErr) {
				category = resolveErr.Category
			}
			logDiagnostic(ctx, logger.With("delegation.error", category), slog.LevelWarn, "delegation context unavailable", NewDiagnostic(StageClientAuthorization, DetailDelegationUnavailable))
		}
		return nil, err
	}
	return &result, nil
}

// delegationFailureResponse maps a resolution failure to the client response: 403 with the
// consent URL for a denial, otherwise 503.
func delegationFailureResponse(err error) (*extprocv3.ProcessingResponse, Diagnostic) {
	var denied *delegation.DeniedError
	if errors.As(err, &denied) {
		body, _ := json.Marshal(map[string]string{
			"error":             "access_denied",
			"error_description": "delegation denied: " + denied.Code,
			"error_uri":         denied.ConsentURL,
		})
		return immediateResponse(httpv3.StatusCode_Forbidden, string(body)), NewDiagnostic(StageClientAuthorization, DetailDelegationDenied)
	}
	return immediateResponse(httpv3.StatusCode_ServiceUnavailable, delegationUnavailableBody), NewDiagnostic(StageClientAuthorization, DetailDelegationUnavailable)
}

// delegationExpired rejects a permit whose deadline has passed before a policy evaluation.
// The caller returns 503 without evaluating policy, forwarding or re-resolving.
func (s *Server) delegationExpired(ctx context.Context, logger *slog.Logger, result *delegation.Result) (*extprocv3.ProcessingResponse, bool) {
	if s.now().Before(result.Deadline) {
		return nil, false
	}
	logDiagnostic(ctx, logger.With("operation", "delegation_resolution"), slog.LevelWarn, "delegation permit expired before policy evaluation", NewDiagnostic(StageClientAuthorization, DetailDelegationUnavailable))
	return immediateResponse(httpv3.StatusCode_ServiceUnavailable, delegationUnavailableBody), true
}

// contextInputFromDelegation builds the enabled-mode OPA context from a permit. It deep-copies
// every slice so policy inputs never alias the shared cached Result, and derives the
// permanently retained granted_permission_sets from the permit, never from exchange.
func contextInputFromDelegation(result *delegation.Result, agentSessionID string) authorization.ContextInput {
	c := result.Context
	granted := make(map[string][]string, len(c.PermissionSets))
	permissionSets := make([]authorization.PermissionSetContext, 0, len(c.PermissionSets))
	for _, ps := range c.PermissionSets {
		serviceIDs := make([]string, 0, len(ps.Services))
		services := make([]authorization.ServiceContext, 0, len(ps.Services))
		for _, svc := range ps.Services {
			serviceIDs = append(serviceIDs, svc.ID)
			services = append(services, authorization.ServiceContext{ID: svc.ID, CanonicalID: svc.CanonicalID, Scopes: copyStrings(svc.Scopes)})
		}
		granted[ps.ID] = serviceIDs
		permissionSets = append(permissionSets, authorization.PermissionSetContext{ID: ps.ID, CanonicalID: ps.CanonicalID, Services: services})
	}
	return authorization.ContextInput{
		GrantedPermissionSetsAvailable: true,
		GrantedPermissionSets:          granted,
		AgentSessionID:                 agentSessionID,
		Principal:                      c.Principal,
		Agent:                          &authorization.AgentContext{ID: c.Agent.ID, CanonicalID: c.Agent.CanonicalID},
		Resource: &authorization.ResourceContext{ID: c.Resource.ID, CanonicalID: c.Resource.CanonicalID,
			ResourceURI: c.Resource.ResourceURI, Scopes: copyStrings(c.Resource.Scopes)},
		Grant:          &authorization.GrantContext{ID: c.Grant.ID, UpdatedAt: c.Grant.UpdatedAt, ValidUntil: c.Grant.ValidUntil},
		PermissionSets: permissionSets,
	}
}

func copyStrings(values []string) []string {
	return append(make([]string, 0, len(values)), values...)
}

// exchangeAndResolve runs exchange and resolution concurrently on the body path and waits
// for both without cancelling either. Precedence: denial, resolution error, exchange error.
func (s *Server) exchangeAndResolve(ctx context.Context, logger *slog.Logger, subjectToken, resourceURI string) (ExchangeResult, *delegation.Result, error, error) {
	type resolved struct {
		result *delegation.Result
		err    error
	}
	resolution := make(chan resolved, 1)
	go func() {
		result, err := s.resolveDelegation(ctx, logger, subjectToken, resourceURI)
		resolution <- resolved{result: result, err: err}
	}()
	exchangeResult, exchangeErr := s.exchanger.Exchange(ctx, subjectToken, resourceURI)
	r := <-resolution
	return exchangeResult, r.result, r.err, exchangeErr
}

func defaultNow() time.Time { return time.Now() }
