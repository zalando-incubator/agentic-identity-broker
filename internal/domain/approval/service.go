package approval

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/toolpattern"
)

// Domain errors for the approval service layer.
var (
	ErrApprovalNotFound       = errors.New("approval not found")
	ErrApprovalForbidden      = errors.New("acting principal does not own this approval")
	ErrApprovalGone           = errors.New("approval has expired")
	ErrApprovalNotPending     = errors.New("approval is not in pending state")
	ErrApprovalRateLimit      = errors.New("rate limit exceeded for this principal/agent pair")
	ErrApprovalNotConsumable  = errors.New("only once-persistence approved approvals can be consumed")
	ErrApprovalInvalidPattern = errors.New("requested approval pattern is invalid")
)

// CreateApprovalRequest contains the parameters for creating a pending approval.
type CreateApprovalRequest struct {
	Principal                id.Principal
	AgentID                  id.AgentID
	GatewayClientID          string
	ToolName                 string
	Arguments                map[string]any
	Description              string
	RiskLevel                string
	MCPSessionID             *string
	AgentSessionID           *string
	ToolInvocationID         *string
	OpenTelemetryTraceparent *string
}

// ApproveRequest carries the user's approval decision. A nil ParamsPattern means the approval
// covers only the reviewed argument values; a non-nil empty map leaves every argument unconstrained.
type ApproveRequest struct {
	Persistence   storage.ApprovalPersistence
	ToolPattern   string
	ParamsPattern map[string]string
}

// CreateApprovalResult contains the result of creating a pending approval.
type CreateApprovalResult struct {
	Approval *storage.ToolApproval
	IsNew    bool // true if newly created, false if existing (idempotent hit)
}

func canonicalRiskLevel(riskLevel string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(riskLevel))
	switch normalized {
	case "":
		return "", true
	case "low", "medium", "critical":
		return normalized, true
	default:
		return "critical", false
	}
}

func startLifecycleSpan(ctx context.Context, name string, traceparent *string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if traceparent != nil {
		origin := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier{"Traceparent": []string{*traceparent}})
		if originContext := trace.SpanContextFromContext(origin); originContext.IsValid() {
			opts = append(opts, trace.WithLinks(trace.Link{SpanContext: originContext}))
		}
	}
	return otel.Tracer("approval").Start(ctx, name, opts...)
}

// Service implements the business logic for tool approval operations.
// It depends on ports (repository interfaces), never on concrete adapter types.
type Service struct {
	approvals    ports.ToolApprovalRepository
	queries      ports.ToolApprovalQueryRepository
	metrics      ports.ToolApprovalMetricsRepository
	syncState    ports.ApprovalSyncStateRepository
	agents       ports.AgentRepository
	rateLimiter  *ApprovalRateLimiter
	creationMu   sync.Mutex
	broadcaster  *ApprovalSyncBroadcaster
	pendingTTL   time.Duration
	publicURL    string
	logger       *slog.Logger
	pendingGauge metric.Int64UpDownCounter
}

// NewService creates a new approval service with the given dependencies.
func NewService(
	approvals ports.ToolApprovalRepository,
	queries ports.ToolApprovalQueryRepository,
	metrics ports.ToolApprovalMetricsRepository,
	syncState ports.ApprovalSyncStateRepository,
	agents ports.AgentRepository,
	rateLimiter *ApprovalRateLimiter,
	broadcaster *ApprovalSyncBroadcaster,
	pendingTTL time.Duration,
	publicURL string,
	logger *slog.Logger,
) *Service {
	meter := otel.Meter("approval")
	pendingGauge, _ := meter.Int64UpDownCounter("approvals_pending_total",
		metric.WithDescription("Number of pending tool approvals"),
		metric.WithUnit("{approval}"),
	)
	return &Service{
		approvals:    approvals,
		queries:      queries,
		metrics:      metrics,
		syncState:    syncState,
		agents:       agents,
		rateLimiter:  rateLimiter,
		broadcaster:  broadcaster,
		pendingTTL:   pendingTTL,
		publicURL:    publicURL,
		logger:       logger,
		pendingGauge: pendingGauge,
	}
}

func (s *Service) syncApprovalMutation(ctx context.Context) error {
	if atomicSync, ok := s.approvals.(ports.ApprovalMutationSyncRepository); ok && atomicSync.ApprovalMutationsSyncAtomically() {
		if s.broadcaster != nil {
			s.broadcaster.Broadcast()
		}
		return nil
	}
	if _, err := s.syncState.IncrementVersion(ctx); err != nil {
		return fmt.Errorf("increment sync version: %w", err)
	}
	if s.broadcaster != nil {
		s.broadcaster.Broadcast()
	}
	return nil
}

func (s *Service) resolveApprovalMutationError(ctx context.Context, approvalID id.ApprovalID, err error) error {
	var storageErr *storage.StorageError
	if !errors.As(err, &storageErr) || storageErr.Kind != storage.ErrorKindNotFound {
		return err
	}

	approval, reloadErr := s.approvals.Get(ctx, approvalID)
	if reloadErr != nil {
		var reloadStorageErr *storage.StorageError
		if errors.As(reloadErr, &reloadStorageErr) && reloadStorageErr.Kind == storage.ErrorKindNotFound {
			return ErrApprovalNotFound
		}
		return fmt.Errorf("re-read approval after mutation race: %w", reloadErr)
	}
	if approval.Status == storage.ApprovalStatusPending && approval.IsExpired(time.Now()) {
		return ErrApprovalGone
	}
	return ErrApprovalNotPending
}

// GetApproval retrieves an approval by ID, enforcing principal ownership and lazy expiry.
// Returns an enriched detail DTO for browser-facing approval views.
// Returns ErrApprovalNotFound if not found, ErrApprovalForbidden if principal mismatch,
// ErrApprovalGone if expired (SR-004, SR-008).
func (s *Service) GetApproval(ctx context.Context, approvalID id.ApprovalID, actingPrincipal id.Principal) (*ApprovalDetail, error) {

	approval, err := s.approvals.Get(ctx, approvalID)
	if err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("get approval: %w", err)
	}

	if approval.Principal != actingPrincipal {
		return nil, ErrApprovalForbidden
	}

	ctx, span := startLifecycleSpan(ctx, "approval.review", approval.OpenTelemetryTraceparent,
		trace.WithAttributes(
			attribute.String("approval.id", approvalID.String()),
			attribute.String("approval.principal", string(actingPrincipal)),
		),
	)
	defer span.End()

	if approval.IsExpired(time.Now()) && approval.Status == storage.ApprovalStatusPending {
		s.logger.Info("approval expired (lazy detection)",
			"approval_id", approvalID,
			"principal", actingPrincipal,
			"agent_id", approval.AgentID,
			"tool_name", approval.ToolName,
			"action", "expired",
			"expired_at", approval.ExpiresAt,
			"timestamp", time.Now(),
		)
		return nil, ErrApprovalGone
	}

	span.SetAttributes(
		attribute.String("approval.tool_name", approval.ToolName),
		attribute.String("approval.status", string(approval.Status)),
	)

	return s.enrichApproval(ctx, approval), nil
}

// ApproveApproval transitions a pending approval to approved state.
// Enforces principal ownership, pattern authority, expiry check, and state machine invariants.
// Increments sync version and broadcasts change to long-poll subscribers.
func (s *Service) ApproveApproval(ctx context.Context, approvalID id.ApprovalID, actingPrincipal id.Principal, req ApproveRequest) (*storage.ToolApproval, error) {
	approval, err := s.approvals.Get(ctx, approvalID)
	if err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("get approval for approve: %w", err)
	}
	if approval.Principal != actingPrincipal {
		return nil, ErrApprovalForbidden
	}

	decision, err := resolveApprovalDecision(approval, req)
	if err != nil {
		return nil, err
	}
	ctx, span := startLifecycleSpan(ctx, "approval.approve", approval.OpenTelemetryTraceparent,
		trace.WithAttributes(
			attribute.String("approval.id", approvalID.String()),
			attribute.String("approval.principal", string(actingPrincipal)),
			attribute.String("approval.persistence", string(decision.Persistence)),
			attribute.String("approval.tool_pattern", decision.ToolPattern),
		),
	)
	defer span.End()

	now := time.Now()
	if err := approval.Approve(actingPrincipal, decision, now); err != nil {
		if errors.Is(err, storage.ErrApprovalExpired) {
			return nil, ErrApprovalGone
		}
		if errors.Is(err, storage.ErrApprovalNotPending) {
			return nil, ErrApprovalNotPending
		}
		return nil, fmt.Errorf("approve domain validation: %w", err)
	}
	result, err := s.approvals.Approve(ctx, approvalID, decision, now)
	if err != nil {
		return nil, s.resolveApprovalMutationError(ctx, approvalID, err)
	}
	s.pendingGauge.Add(ctx, -1, metric.WithAttributes(attribute.String("agent_id", result.AgentID.String())))
	if err := s.syncApprovalMutation(ctx); err != nil {
		return nil, err
	}
	s.logger.Info("approval approved", "approval_id", approvalID, "principal", actingPrincipal, "agent_id", result.AgentID, "tool_name", result.ToolName, "persistence", decision.Persistence, "action", "approved", "timestamp", now)
	return result, nil
}

func resolveApprovalDecision(approval *storage.ToolApproval, req ApproveRequest) (storage.ApprovalDecision, error) {
	toolPattern := approval.ToolName
	if req.ToolPattern != "" {
		toolPattern = req.ToolPattern
	}
	paramsPattern := req.ParamsPattern
	if paramsPattern == nil {
		paramsPattern = toolpattern.ExactParams(approval.Arguments)
	}
	if err := toolpattern.ValidateToolPattern(toolPattern); err != nil {
		return storage.ApprovalDecision{}, fmt.Errorf("%w: malformed tool glob: %v", ErrApprovalInvalidPattern, err)
	}
	if err := toolpattern.ValidateParamsPattern(paramsPattern); err != nil {
		return storage.ApprovalDecision{}, fmt.Errorf("%w: malformed argument glob: %v", ErrApprovalInvalidPattern, err)
	}
	if !toolpattern.Matches(toolPattern, paramsPattern, approval.ToolName, approval.Arguments) {
		return storage.ApprovalDecision{}, fmt.Errorf("%w: pattern does not cover the reviewed tool call", ErrApprovalInvalidPattern)
	}
	return storage.ApprovalDecision{Persistence: req.Persistence, ToolPattern: toolPattern, ParamsPattern: paramsPattern}, nil
}

// DenyApproval transitions a pending approval to denied state.
// Enforces principal ownership, expiry check, and state machine invariants.
// Increments sync version and broadcasts change to long-poll subscribers.
func (s *Service) DenyApproval(ctx context.Context, approvalID id.ApprovalID, actingPrincipal id.Principal, persistence *storage.ApprovalPersistence) (*storage.ToolApproval, error) {

	approval, err := s.approvals.Get(ctx, approvalID)
	if err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("get approval for deny: %w", err)
	}

	if approval.Principal != actingPrincipal {
		return nil, ErrApprovalForbidden
	}

	ctx, span := startLifecycleSpan(ctx, "approval.deny", approval.OpenTelemetryTraceparent,
		trace.WithAttributes(
			attribute.String("approval.id", approvalID.String()),
			attribute.String("approval.principal", string(actingPrincipal)),
		),
	)
	defer span.End()

	now := time.Now()
	if err := approval.Deny(actingPrincipal, persistence, now); err != nil {
		if errors.Is(err, storage.ErrApprovalExpired) {
			return nil, ErrApprovalGone
		}
		if errors.Is(err, storage.ErrApprovalNotPending) {
			return nil, ErrApprovalNotPending
		}
		return nil, fmt.Errorf("deny domain validation: %w", err)
	}

	result, err := s.approvals.Deny(ctx, approvalID, persistence, now)
	if err != nil {
		return nil, s.resolveApprovalMutationError(ctx, approvalID, err)
	}

	// Track pending approval resolution
	s.pendingGauge.Add(ctx, -1, metric.WithAttributes(
		attribute.String("agent_id", result.AgentID.String()),
	))

	if err := s.syncApprovalMutation(ctx); err != nil {
		return nil, err
	}

	span.SetAttributes(attribute.String("approval.tool_name", result.ToolName))

	s.logger.Info("approval denied",
		"approval_id", approvalID,
		"principal", actingPrincipal,
		"agent_id", result.AgentID,
		"tool_name", result.ToolName,
		"persistence", persistence,
		"action", "denied",
		"timestamp", now,
	)

	return result, nil
}

// CreatePendingApproval creates a new pending approval or returns an existing one (idempotent).
// Enforces rate limiting per (principal, agent) pair.
// Computes arguments hash for deduplication, constructs approval_url, sets TTL.
func (s *Service) CreatePendingApproval(ctx context.Context, req CreateApprovalRequest) (*CreateApprovalResult, error) {
	ctx, span := startLifecycleSpan(ctx, "approval.created", req.OpenTelemetryTraceparent,
		trace.WithAttributes(
			attribute.String("approval.principal", string(req.Principal)),
			attribute.String("approval.agent_id", req.AgentID.String()),
			attribute.String("approval.tool_name", req.ToolName),
		),
	)
	defer span.End()

	// ponytail: global lock preserves idempotency under concurrent retries; shard by principal/agent only if creation throughput requires it.
	s.creationMu.Lock()
	defer s.creationMu.Unlock()

	// Compute arguments hash for idempotency deduplication.
	argsHash := storage.ComputeArgumentsHash(req.Arguments)
	existing, err := s.findExistingPendingApproval(ctx, req.Principal, req.AgentID, req.ToolName, argsHash)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &CreateApprovalResult{Approval: existing, IsNew: false}, nil
	}

	// Rate limit check (per-minute token bucket).
	if s.rateLimiter != nil && !s.rateLimiter.AllowCreation(string(req.Principal), req.AgentID.String()) {
		existing, err := s.findExistingPendingApproval(ctx, req.Principal, req.AgentID, req.ToolName, argsHash)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return &CreateApprovalResult{Approval: existing, IsNew: false}, nil
		}
		return nil, ErrApprovalRateLimit
	}

	// Max pending per (principal, agent) pair check.
	if s.rateLimiter != nil && s.metrics != nil {
		pendingCount, err := s.metrics.CountPendingByPrincipalAndAgent(ctx, req.Principal, req.AgentID)
		if err != nil {
			s.logger.Warn("failed to count pending approvals for rate limit check",
				"principal", req.Principal,
				"agent_id", req.AgentID,
				"error", err,
			)
		} else if pendingCount >= s.rateLimiter.MaxPending() {
			return nil, ErrApprovalRateLimit
		}
	}
	canonicalRiskLevelValue, knownRiskLevel := canonicalRiskLevel(req.RiskLevel)
	if req.RiskLevel != "" && !knownRiskLevel && s.logger != nil {
		s.logger.Warn("unknown approval risk level, defaulting to critical",
			"provided_risk_level", req.RiskLevel,
			"principal", req.Principal,
			"agent_id", req.AgentID,
			"tool_name", req.ToolName,
		)
	}

	now := time.Now()
	expiresAt := now.Add(s.pendingTTL)
	approvalID := id.NewApprovalID()
	approvalURL := fmt.Sprintf("%s/approvals/%s", s.publicURL, approvalID.String())

	newApproval := &storage.ToolApproval{
		ID:                       approvalID,
		Principal:                req.Principal,
		AgentID:                  req.AgentID,
		GatewayClientID:          req.GatewayClientID,
		ToolName:                 req.ToolName,
		Arguments:                req.Arguments,
		ArgumentsHash:            argsHash,
		Description:              req.Description,
		RiskLevel:                canonicalRiskLevelValue,
		MCPSessionID:             req.MCPSessionID,
		AgentSessionID:           req.AgentSessionID,
		ToolInvocationID:         req.ToolInvocationID,
		OpenTelemetryTraceparent: req.OpenTelemetryTraceparent,
		Status:                   storage.ApprovalStatusPending,
		ApprovalURL:              approvalURL,
		CreatedAt:                now,
		ExpiresAt:                expiresAt,
	}
	newApproval.ApplyExactPatterns()

	// Create (idempotent — repo returns existing if duplicate pending found)
	result, err := s.approvals.Create(ctx, newApproval)
	if err != nil {
		return nil, fmt.Errorf("create pending approval: %w", err)
	}

	// Detect whether this is a new creation or idempotent hit
	isNew := result.ID == approvalID

	if isNew {
		// Track pending approval creation
		s.pendingGauge.Add(ctx, 1, metric.WithAttributes(
			attribute.String("agent_id", req.AgentID.String()),
		))

		if err := s.syncApprovalMutation(ctx); err != nil {
			return nil, err
		}

		s.logger.Info("approval created",
			"approval_id", result.ID,
			"principal", req.Principal,
			"agent_id", req.AgentID,
			"tool_name", req.ToolName,
			"action", "created",
			"expires_at", expiresAt,
			"timestamp", now,
		)
	} else {
		s.logger.Info("approval returned (idempotent)",
			"approval_id", result.ID,
			"principal", req.Principal,
			"agent_id", req.AgentID,
			"tool_name", req.ToolName,
		)
	}

	span.SetAttributes(
		attribute.String("approval.id", result.ID.String()),
		attribute.Bool("approval.is_new", isNew),
	)

	return &CreateApprovalResult{
		Approval: result,
		IsNew:    isNew,
	}, nil
}

func (s *Service) findExistingPendingApproval(ctx context.Context, principal id.Principal, agentID id.AgentID, toolName, argumentsHash string) (*storage.ToolApproval, error) {
	if s.queries == nil {
		return nil, nil
	}
	approvals, err := s.queries.ListActiveByPrincipalAndAgent(ctx, principal, agentID)
	if err != nil {
		return nil, fmt.Errorf("list active approvals: %w", err)
	}
	for _, approval := range approvals {
		if approval.ToolName == toolName && approval.ArgumentsHash == argumentsHash &&
			approval.Status == storage.ApprovalStatusPending && !approval.Consumed && !approval.IsExpired(time.Now()) {
			return approval, nil
		}
	}
	return nil, nil
}

// SyncPair represents a (principal, agent_id) pair with its active approvals.
type SyncPair struct {
	Principal id.Principal            `json:"principal"`
	AgentID   id.AgentID              `json:"agent_id"`
	Approvals []*storage.ToolApproval `json:"approvals"`
}

// SyncState represents the current sync state for long-poll responses.
type SyncState struct {
	Version int64       `json:"version"`
	Pairs   []*SyncPair `json:"pairs"`
}

// GetSyncState returns the current version and all active approvals grouped by (principal, agent_id) pair.
// Session-scoped approvals are included only for the supplied active agent sessions.
func (s *Service) GetSyncState(ctx context.Context, principalFilter *id.Principal, activeAgentSessionIDs []string) (*SyncState, error) {
	ctx, span := otel.Tracer("approval").Start(ctx, "approval.sync")
	defer span.End()

	version, err := s.syncState.GetVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("get sync version: %w", err)
	}

	approvals, err := s.queries.ListAllActive(ctx, principalFilter, activeAgentSessionIDs)
	if err != nil {
		return nil, fmt.Errorf("list active approvals: %w", err)
	}

	// Group by (principal, agent_id) pair
	type pairKey struct {
		Principal id.Principal
		AgentID   id.AgentID
	}
	pairMap := make(map[pairKey]*SyncPair)
	for _, a := range approvals {
		key := pairKey{Principal: a.Principal, AgentID: a.AgentID}
		pair, ok := pairMap[key]
		if !ok {
			pair = &SyncPair{
				Principal: a.Principal,
				AgentID:   a.AgentID,
			}
			pairMap[key] = pair
		}
		pair.Approvals = append(pair.Approvals, a)
	}

	pairs := make([]*SyncPair, 0, len(pairMap))
	for _, pair := range pairMap {
		pairs = append(pairs, pair)
	}

	span.SetAttributes(
		attribute.Int64("approval.sync_version", version),
		attribute.Int("approval.pair_count", len(pairs)),
	)

	return &SyncState{
		Version: version,
		Pairs:   pairs,
	}, nil
}

// GetVersion returns the current sync version without fetching approvals.
func (s *Service) GetVersion(ctx context.Context) (int64, error) {
	return s.syncState.GetVersion(ctx)
}

// GetBroadcaster returns the approval sync broadcaster for handler subscription.
func (s *Service) GetBroadcaster() *ApprovalSyncBroadcaster {
	return s.broadcaster
}

// ConsumeApproval marks a once-persistence approved approval as consumed.
// Returns the updated approval. Idempotent: already-consumed approvals return 200.
// Returns ErrApprovalNotFound if not found, ErrApprovalForbidden for wrong principal,
// ErrApprovalNotConsumable if not a once-persistence approved approval.
func (s *Service) ConsumeApproval(ctx context.Context, approvalID id.ApprovalID, actingPrincipal id.Principal) (*storage.ToolApproval, error) {

	// Fetch the approval to validate ownership and state
	existing, err := s.approvals.Get(ctx, approvalID)
	if err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("get approval: %w", err)
	}

	// Verify principal ownership
	if existing.Principal != actingPrincipal {
		return nil, ErrApprovalForbidden
	}

	ctx, span := startLifecycleSpan(ctx, "approval.consume", existing.OpenTelemetryTraceparent,
		trace.WithAttributes(attribute.String("approval.id", approvalID.String())),
	)
	defer span.End()

	// Already consumed — idempotent return
	if existing.Consumed {
		return existing, nil
	}

	// Only once-persistence approved approvals can be consumed
	if existing.Status != storage.ApprovalStatusApproved ||
		existing.Persistence == nil ||
		*existing.Persistence != storage.ApprovalPersistenceOnce {
		return nil, ErrApprovalNotConsumable
	}

	now := time.Now().UTC()
	result, err := s.approvals.Consume(ctx, approvalID, now)
	if err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
			reloaded, reloadErr := s.approvals.Get(ctx, approvalID)
			if reloadErr != nil {
				var reloadStorageErr *storage.StorageError
				if errors.As(reloadErr, &reloadStorageErr) && reloadStorageErr.Kind == storage.ErrorKindNotFound {
					return nil, ErrApprovalNotFound
				}
				return nil, fmt.Errorf("re-read approval after consume race: %w", reloadErr)
			}
			if reloaded.Principal != actingPrincipal {
				return nil, ErrApprovalForbidden
			}
			if reloaded.Consumed {
				return reloaded, nil
			}
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("consume approval: %w", err)
	}

	if err := s.syncApprovalMutation(ctx); err != nil {
		return nil, err
	}

	s.logger.Info("approval consumed",
		"approval_id", approvalID,
		"principal", actingPrincipal,
		"agent_id", result.AgentID,
		"tool_name", result.ToolName,
		"timestamp", now.Format(time.RFC3339),
	)

	span.SetAttributes(
		attribute.String("approval.agent_id", result.AgentID.String()),
		attribute.String("approval.tool_name", result.ToolName),
	)

	return result, nil
}

// ApprovalDetail is the user-facing approval DTO shared by detail and list endpoints.
// It exposes agent/session context needed by the UI while hiding internal-only fields.
type ApprovalDetail struct {
	ID               id.ApprovalID                `json:"id"`
	Principal        id.Principal                 `json:"principal"`
	AgentID          id.AgentID                   `json:"agent_id"`
	AgentDisplayName string                       `json:"agent_display_name,omitempty"`
	MCPSessionID     *string                      `json:"mcp_session_id,omitempty"`
	AgentSessionID   *string                      `json:"agent_session_id,omitempty"`
	ToolInvocationID *string                      `json:"tool_invocation_id,omitempty"`
	ToolName         string                       `json:"tool_name"`
	Arguments        map[string]any               `json:"arguments"`
	ToolPattern      string                       `json:"tool_pattern"`
	ParamsPattern    map[string]string            `json:"params_pattern"`
	Description      string                       `json:"description"`
	RiskLevel        string                       `json:"risk_level"`
	Status           storage.ApprovalStatus       `json:"status"`
	Persistence      *storage.ApprovalPersistence `json:"persistence"`
	Consumed         bool                         `json:"consumed"`
	ApprovalURL      string                       `json:"approval_url"`
	CreatedAt        time.Time                    `json:"created_at"`
	ApprovedAt       *time.Time                   `json:"approved_at,omitempty"`
	DeniedAt         *time.Time                   `json:"denied_at,omitempty"`
	ConsumedAt       *time.Time                   `json:"consumed_at,omitempty"`
	ExpiresAt        time.Time                    `json:"expires_at"`
}

func newApprovalDetail(approval *storage.ToolApproval, agentDisplayName string) *ApprovalDetail {
	if approval == nil {
		return nil
	}

	return &ApprovalDetail{
		ID:               approval.ID,
		Principal:        approval.Principal,
		AgentID:          approval.AgentID,
		AgentDisplayName: agentDisplayName,
		MCPSessionID:     approval.MCPSessionID,
		AgentSessionID:   approval.AgentSessionID,
		ToolInvocationID: approval.ToolInvocationID,
		ToolName:         approval.ToolName,
		Arguments:        approval.Arguments,
		ToolPattern:      approval.ToolPattern,
		ParamsPattern:    approval.ParamsPattern,
		Description:      approval.Description,
		RiskLevel:        approval.RiskLevel,
		Status:           approval.Status,
		Persistence:      approval.Persistence,
		Consumed:         approval.Consumed,
		ApprovalURL:      approval.ApprovalURL,
		CreatedAt:        approval.CreatedAt,
		ApprovedAt:       approval.ApprovedAt,
		DeniedAt:         approval.DeniedAt,
		ConsumedAt:       approval.ConsumedAt,
		ExpiresAt:        approval.ExpiresAt,
	}
}

func (s *Service) enrichApproval(ctx context.Context, approval *storage.ToolApproval) *ApprovalDetail {
	if approval == nil {
		return nil
	}

	agentDisplayName := ""
	if s.agents != nil {
		if agent, err := s.agents.Get(ctx, approval.AgentID); err == nil {
			agentDisplayName = agent.DisplayName
		}
	}

	return newApprovalDetail(approval, agentDisplayName)
}

// enrichApprovals resolves agent display names for a slice of approvals.
// Agent IDs that cannot be resolved are silently skipped — the UI falls back to the raw UUID.
func (s *Service) enrichApprovals(ctx context.Context, approvals []*storage.ToolApproval) []*ApprovalDetail {
	// Collect unique agent IDs
	agentNames := make(map[id.AgentID]string)
	for _, a := range approvals {
		agentNames[a.AgentID] = ""
	}
	if s.agents != nil {
		for agentID := range agentNames {
			if agent, err := s.agents.Get(ctx, agentID); err == nil {
				agentNames[agentID] = agent.DisplayName
			}
		}
	}

	details := make([]*ApprovalDetail, len(approvals))
	for i, a := range approvals {
		details[i] = newApprovalDetail(a, agentNames[a.AgentID])
	}
	return details
}

// ListPendingApprovals returns all pending (not yet actioned) approvals for a principal.
func (s *Service) ListPendingApprovals(ctx context.Context, principal id.Principal) ([]*ApprovalDetail, error) {
	ctx, span := otel.Tracer("approval").Start(ctx, "approval.list_pending",
		trace.WithAttributes(
			attribute.String("approval.principal", string(principal)),
		),
	)
	defer span.End()

	all, err := s.queries.ListAllActive(ctx, &principal, nil)
	if err != nil {
		return nil, fmt.Errorf("list active approvals: %w", err)
	}

	var pending []*storage.ToolApproval
	for _, a := range all {
		if a.Status == storage.ApprovalStatusPending {
			pending = append(pending, a)
		}
	}
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].CreatedAt.After(pending[j].CreatedAt)
	})

	span.SetAttributes(attribute.Int("approval.count", len(pending)))
	return s.enrichApprovals(ctx, pending), nil
}

// ListPermanentApprovals returns all permanent approvals and denials for a principal.
func (s *Service) ListPermanentApprovals(ctx context.Context, principal id.Principal) ([]*ApprovalDetail, error) {
	ctx, span := otel.Tracer("approval").Start(ctx, "approval.list_permanent",
		trace.WithAttributes(
			attribute.String("approval.principal", string(principal)),
		),
	)
	defer span.End()

	approvals, err := s.queries.ListPermanentByPrincipal(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("list permanent approvals: %w", err)
	}

	span.SetAttributes(attribute.Int("approval.count", len(approvals)))
	return s.enrichApprovals(ctx, approvals), nil
}

// ErrApprovalNotRevocable is returned when attempting to revoke a non-permanent approval.
var ErrApprovalNotRevocable = errors.New("only permanent approvals can be revoked")

// RevokePermanentApproval transitions a permanent approval to denied status.
// Returns ErrApprovalNotFound, ErrApprovalForbidden, or ErrApprovalNotRevocable.
func (s *Service) RevokePermanentApproval(ctx context.Context, approvalID id.ApprovalID, actingPrincipal id.Principal) (*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("approval").Start(ctx, "approval.revoke",
		trace.WithAttributes(
			attribute.String("approval.id", approvalID.String()),
		),
	)
	defer span.End()

	existing, err := s.approvals.Get(ctx, approvalID)
	if err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("get approval: %w", err)
	}

	if existing.Principal != actingPrincipal {
		return nil, ErrApprovalForbidden
	}

	if existing.Persistence == nil || *existing.Persistence != storage.ApprovalPersistencePermanent {
		return nil, ErrApprovalNotRevocable
	}

	now := time.Now().UTC()
	result, err := s.approvals.RevokePermanent(ctx, approvalID, now)
	if err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
			current, getErr := s.approvals.Get(ctx, approvalID)
			if getErr == nil && current.Principal == actingPrincipal {
				return nil, ErrApprovalNotRevocable
			}
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("revoke approval: %w", err)
	}

	if err := s.syncApprovalMutation(ctx); err != nil {
		return nil, err
	}

	s.logger.Info("permanent approval revoked",
		"approval_id", approvalID,
		"principal", actingPrincipal,
		"agent_id", result.AgentID,
		"tool_name", result.ToolName,
		"timestamp", now.Format(time.RFC3339),
	)

	span.SetAttributes(
		attribute.String("approval.agent_id", result.AgentID.String()),
		attribute.String("approval.tool_name", result.ToolName),
	)

	return result, nil
}
