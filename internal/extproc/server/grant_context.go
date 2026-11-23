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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/grantcontext"
)

// PermitResolver is the fourth ExtProc port (ADR 048). It returns a fresh broker
// delegation permit, a *grantcontext.DeniedError, or an error. hit reports a cache hit.
type PermitResolver interface {
	Resolve(ctx context.Context, subjectToken, resourceURI string) (result grantcontext.Result, hit bool, err error)
	Shutdown()
}

const grantContextUnavailableBody = `{"error":"service_unavailable","error_description":"grant context unavailable"}`
const grantContextBindingUnavailableBody = `{"error":"service_unavailable","error_description":"exchange result binding unavailable"}`

var errInvalidGrantTimestamp = errors.New("grant timestamp invalid")

func (s *Server) exchangeWithGrantContext(ctx context.Context, state *requestState, observed *ExchangeResult) (ExchangeResult, error) {
	updatedAt, err := time.Parse(time.RFC3339Nano, state.grantContext.Context.Grant.UpdatedAt)
	if err != nil {
		return ExchangeResult{}, errInvalidGrantTimestamp
	}
	binding := ExchangeResultBinding{Principal: state.grantContext.Context.Principal, AgentID: state.grantContext.Context.Agent.ID,
		Scopes: copyStrings(state.grantContext.Context.Resource.Scopes), GrantUpdatedAt: updatedAt}
	return s.exchanger.ExchangeBound(ctx, state.subjectToken, state.resourceURI, observed, binding)
}

// WithPermitResolver enables grant context. It requires an authorizer.
func (s *Server) WithPermitResolver(resolver PermitResolver) *Server {
	s.grantContextResolver = resolver
	return s
}

func (s *Server) grantContextEnabled() bool {
	return s.grantContextResolver != nil && s.authorizer != nil
}

// resolveGrantContext resolves and observes one delegation without credentials in logs.
func (s *Server) resolveGrantContext(ctx context.Context, logger *slog.Logger, subjectToken, resourceURI string) (*grantcontext.Result, error) {
	result, hit, err := s.grantContextResolver.Resolve(ctx, subjectToken, resourceURI)
	cache := "miss"
	if hit {
		cache = "hit"
	}
	logger = logger.With("operation", "grant_resolution", "grant_context.cache", cache)
	if err != nil {
		var denied *grantcontext.DeniedError
		if errors.As(err, &denied) {
			logDiagnostic(ctx, logger.With("event", "extproc.grant_context.denied", "grant_context.code", denied.Code), slog.LevelInfo, "delegation denied", NewDiagnostic(StageClientAuthorization, DetailGrantContextDenied))
		} else {
			category := "unavailable"
			var resolveErr *grantcontext.Error
			if errors.As(err, &resolveErr) {
				category = resolveErr.Category
			}
			logDiagnostic(ctx, logger.With("event", "extproc.grant_context.unavailable", "error_kind", category), slog.LevelWarn, "grant context unavailable", NewDiagnostic(StageClientAuthorization, DetailGrantContextUnavailable).asServerError(KindUnavailable))
		}
		return nil, err
	}
	logDiagnostic(ctx, logger, slog.LevelDebug, "delegation permitted", SuccessDiagnostic(ExchangeUnknown))
	return &result, nil
}

// grantContextFailureResponse maps a resolution failure to the client response: 403 with the
// consent URL for a denial, otherwise 503.
func grantContextFailureResponse(err error) (*extprocv3.ProcessingResponse, Diagnostic) {
	var denied *grantcontext.DeniedError
	if errors.As(err, &denied) {
		body, _ := json.Marshal(map[string]string{
			"error":             "access_denied",
			"error_description": "delegation denied: " + denied.Code,
			"error_uri":         denied.ConsentURL,
		})
		return immediateResponse(httpv3.StatusCode_Forbidden, string(body)), NewDiagnostic(StageClientAuthorization, DetailGrantContextDenied)
	}
	return immediateResponse(httpv3.StatusCode_ServiceUnavailable, grantContextUnavailableBody), NewDiagnostic(StageClientAuthorization, DetailGrantContextUnavailable).asServerError(KindUnavailable)
}

// grantContextExpired rejects a permit whose deadline has passed before a policy evaluation.
// The caller returns 503 without evaluating policy, forwarding or re-resolving.
func (s *Server) grantContextExpired(ctx context.Context, logger *slog.Logger, result *grantcontext.Result) (*extprocv3.ProcessingResponse, bool) {
	if s.now().Before(result.Deadline) {
		return nil, false
	}
	logDiagnostic(ctx, logger.With("event", "extproc.grant_context.permit_expired", "operation", "grant_resolution"), slog.LevelWarn, "delegation permit expired before policy evaluation", NewDiagnostic(StageClientAuthorization, DetailGrantContextUnavailable).asServerError(KindUnavailable))
	return immediateResponse(httpv3.StatusCode_ServiceUnavailable, grantContextUnavailableBody), true
}

// contextInputFromGrantContext builds the enabled-mode OPA context from a permit. It deep-copies
// every slice so policy inputs never alias the shared cached Result, and derives the
// permanently retained granted_permission_sets from the permit, never from exchange.
func contextInputFromGrantContext(result *grantcontext.Result, agentSessionID string) authorization.ContextInput {
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
		Resource: &authorization.ResourceContext{ServiceID: c.Resource.ServiceID, CanonicalID: c.Resource.CanonicalID,
			ResourceURI: c.Resource.ResourceURI, Scopes: copyStrings(c.Resource.Scopes)},
		Grant:          &authorization.GrantContext{ID: c.Grant.ID, UpdatedAt: c.Grant.UpdatedAt, ValidUntil: c.Grant.ValidUntil},
		PermissionSets: permissionSets,
	}
}

func (s *Server) forwardRequestBody(ctx context.Context, logger *slog.Logger, state *requestState, body *extprocv3.HttpBody) *extprocv3.ProcessingResponse {
	if state.grantContext != nil {
		if response, expired := s.grantContextExpired(ctx, logger, state.grantContext); expired {
			return response
		}
	}
	return echoRequestBody(body)
}

func copyStrings(values []string) []string {
	return append(make([]string, 0, len(values)), values...)
}

// exchangeAndResolve runs exchange and resolution concurrently on the body path and waits
// for both without cancelling either. Precedence: denial, resolution error, exchange error.
func (s *Server) exchangeAndResolve(ctx context.Context, logger *slog.Logger, subjectToken, resourceURI string) (ExchangeResult, *grantcontext.Result, error, error) {
	type resolved struct {
		result *grantcontext.Result
		err    error
	}
	resolution := make(chan resolved, 1)
	go func() {
		result, err := s.resolveGrantContext(ctx, logger, subjectToken, resourceURI)
		resolution <- resolved{result: result, err: err}
	}()
	exchangeResult, exchangeErr := s.exchanger.Exchange(ctx, subjectToken, resourceURI)
	r := <-resolution
	return exchangeResult, r.result, r.err, exchangeErr
}

func defaultNow() time.Time { return time.Now() }
