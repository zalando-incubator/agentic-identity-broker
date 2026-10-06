package oauth2server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ory/fosite"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type refreshRetryPayload struct {
	Purpose                           string              `json:"purpose"`
	Version                           int                 `json:"version"`
	SessionID                         id.RefreshSessionID `json:"session_id"`
	OriginalGrantID                   id.GrantID          `json:"original_grant_id"`
	OriginalTokenSignature            string              `json:"original_token_signature"`
	StartedAt                         time.Time           `json:"started_at"`
	Principal                         id.Principal        `json:"principal"`
	AgentID                           id.AgentID          `json:"agent_id"`
	ClientID                          id.ClientID         `json:"client_id"`
	PredecessorSignature              string              `json:"predecessor_signature"`
	SuccessorSignature                string              `json:"successor_signature"`
	OriginalRequestedScope            string              `json:"original_requested_scope"`
	Scope                             string              `json:"scope"`
	OriginalRequestContextFingerprint string              `json:"original_request_context_fingerprint"`
	AccessToken                       string              `json:"access_token"`
	RefreshToken                      string              `json:"refresh_token"`
	TokenType                         string              `json:"token_type"`
	AccessExpiresAt                   time.Time           `json:"access_expires_at"`
	RetryExpiresAt                    time.Time           `json:"retry_expires_at"`
}

func sealRefreshRetry(ctx context.Context, encryption ports.EncryptionPort, root *storage.RefreshSession, response ports.TokenResponse) ([]byte, error) {
	if root.PreviousSignature == nil || root.OriginalRequestedScope == nil || root.OriginalRequestContextFingerprint == nil || root.RetryAccessExpiresAt == nil || root.ReuseUntil == nil {
		return nil, errors.New("retry result requires complete predecessor evidence")
	}
	payload := refreshRetryPayload{
		Purpose: "refresh_retry", Version: 1, SessionID: root.ID,
		OriginalGrantID: root.OriginalGrantID, OriginalTokenSignature: root.OriginalTokenSignature, StartedAt: root.StartedAt,
		Principal: root.Principal, AgentID: root.AgentID, ClientID: root.ClientID,
		PredecessorSignature: *root.PreviousSignature, SuccessorSignature: root.CurrentSignature,
		OriginalRequestedScope: *root.OriginalRequestedScope, Scope: response.Scope,
		OriginalRequestContextFingerprint: *root.OriginalRequestContextFingerprint,
		AccessToken:                       response.AccessToken, RefreshToken: response.RefreshToken, TokenType: response.TokenType,
		AccessExpiresAt: *root.RetryAccessExpiresAt,
		RetryExpiresAt:  earliestRefreshDeadline(*root.ReuseUntil, *root.RetryAccessExpiresAt),
	}
	if err := validateRefreshRetryPayload(payload, root); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(payload) // #nosec G117 -- The result is encrypted before persistence and is never logged.
	if err != nil {
		return nil, errors.New("failed to encode refresh retry result")
	}
	return encryption.Encrypt(ctx, encoded, domainencryption.NewRefreshSessionBranchKeySubject(root.ID).EncryptionContext())
}

func openRefreshRetry(ctx context.Context, encryption ports.EncryptionPort, root *storage.RefreshSession) (*refreshRetryPayload, error) {
	if len(root.RetryCiphertext) == 0 {
		return nil, errors.New("refresh retry result is unavailable")
	}
	encoded, err := encryption.Decrypt(ctx, root.RetryCiphertext, domainencryption.NewRefreshSessionBranchKeySubject(root.ID).EncryptionContext())
	if err != nil {
		return nil, errors.New("failed to authenticate refresh retry result")
	}
	var payload refreshRetryPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return nil, errors.New("invalid refresh retry result encoding")
	}
	if err := validateRefreshRetryPayload(payload, root); err != nil {
		return nil, err
	}
	return &payload, nil
}

func validateRefreshRetryPayload(payload refreshRetryPayload, root *storage.RefreshSession) error {
	if root.PreviousSignature == nil || root.OriginalRequestedScope == nil || root.OriginalRequestContextFingerprint == nil || root.RetryAccessExpiresAt == nil || root.RetryExpiresAt == nil || root.ReuseUntil == nil {
		return errors.New("refresh retry result lacks authoritative predecessor evidence")
	}
	if payload.Purpose != "refresh_retry" || payload.Version != 1 || payload.SessionID != root.ID || payload.OriginalGrantID != root.OriginalGrantID || payload.OriginalTokenSignature != root.OriginalTokenSignature || !payload.StartedAt.Equal(root.StartedAt) {
		return errors.New("refresh retry result has invalid origin binding")
	}
	if payload.Principal != root.Principal || payload.AgentID != root.AgentID || payload.ClientID != root.ClientID || payload.PredecessorSignature != *root.PreviousSignature || payload.SuccessorSignature != root.CurrentSignature || sha256Hex(payload.RefreshToken) != root.CurrentSignature {
		return errors.New("refresh retry result has invalid ownership or lineage binding")
	}
	expectedScope := *root.OriginalRequestedScope
	if expectedScope == "" {
		expectedScope = root.Scope
	}
	if payload.OriginalRequestedScope != *root.OriginalRequestedScope || payload.Scope != expectedScope || payload.OriginalRequestContextFingerprint != *root.OriginalRequestContextFingerprint {
		return errors.New("refresh retry result has invalid request binding")
	}
	originalDeadline := earliestRefreshDeadline(*root.ReuseUntil, *root.RetryAccessExpiresAt)
	if !payload.AccessExpiresAt.Equal(*root.RetryAccessExpiresAt) || !payload.RetryExpiresAt.Equal(originalDeadline) || root.RetryExpiresAt.After(payload.RetryExpiresAt) {
		return errors.New("refresh retry result has invalid deadline binding")
	}
	if payload.AccessToken == "" || strings.Count(payload.AccessToken, ".") != 2 || payload.RefreshToken == "" || payload.TokenType != "Bearer" {
		return errors.New("refresh retry result has invalid token values")
	}
	return nil
}

func stageRefreshSuccessor(root *storage.RefreshSession, policy storage.RefreshSessionPolicy, signature, predecessor, requested, fingerprint string, at, accessExpiresAt time.Time) error {
	if policy.AbsoluteLifetime > 0 {
		candidate := root.StartedAt.Add(policy.AbsoluteLifetime)
		if !at.Before(candidate) {
			return fosite.ErrInvalidGrant
		}
		if root.AbsoluteExpiresAt == nil || candidate.Before(*root.AbsoluteExpiresAt) {
			root.AbsoluteExpiresAt = &candidate
		}
	}
	reuseUntil := at.Add(policy.ReuseInterval)
	inactivity := at.Add(policy.InactivityLifetime)
	deadline := earliestRefreshDeadline(reuseUntil, accessExpiresAt, inactivity)
	if root.AbsoluteExpiresAt != nil {
		deadline = earliestRefreshDeadline(deadline, *root.AbsoluteExpiresAt)
	}
	root.PreviousSignature = &predecessor
	root.PreviousConsumedAt = &at
	root.ReuseUntil = &reuseUntil
	root.OriginalRequestedScope = &requested
	root.OriginalRequestContextFingerprint = &fingerprint
	root.RetryCiphertext = nil
	root.RetryAccessExpiresAt = &accessExpiresAt
	root.RetryExpiresAt = &deadline
	root.RetryCount = 0
	root.LastFreshAt = at
	root.InactivityExpiresAt = inactivity
	root.CurrentSignature = signature
	if inactivity.After(root.RetainUntil) {
		root.RetainUntil = inactivity
	}
	return nil
}

func prohibitedRefreshRetry(root *storage.RefreshSession, signature, requestedScope string, at time.Time, policy storage.RefreshSessionPolicy) bool {
	return policy.ReuseInterval <= 0 || root.PreviousSignature == nil || *root.PreviousSignature != signature ||
		root.OriginalRequestedScope == nil || *root.OriginalRequestedScope != requestedScope || root.RetryCount >= 3 ||
		root.PreviousConsumedAt == nil || !at.Before(root.PreviousConsumedAt.Add(policy.ReuseInterval)) ||
		root.ReuseUntil == nil || !at.Before(*root.ReuseUntil)
}

// retryEligibility checks classification without reading the encrypted result.
// Only the owner-side caller may turn prohibited reuse into a revocation.
func (p *Provider) retryEligibility(owner context.Context, at time.Time, input *refreshExchangeInput, root *storage.RefreshSession) (bool, error) {
	input.auditStage = "prohibited_reuse"
	if prohibitedRefreshRetry(root, input.signature, input.requestedScope, at, p.refreshDeps.Policy) {
		return false, nil
	}
	successor, err := p.refreshDeps.Tokens.FindBySignature(owner, root.CurrentSignature)
	if err != nil {
		return false, fosite.ErrServerError.WithWrap(err)
	}
	if successor.UsedAt != nil || successor.SessionID != root.ID {
		return false, nil
	}
	input.auditStage = "cached_access_expired"
	if root.RetryAccessExpiresAt == nil || !at.Before(*root.RetryAccessExpiresAt) {
		return false, fosite.ErrInvalidGrant
	}
	input.auditStage = "prohibited_reuse"
	return root.RetryExpiresAt != nil && at.Before(*root.RetryExpiresAt), nil
}

func (p *Provider) retryRefreshInOwner(owner context.Context, at time.Time, input *refreshExchangeInput, root *storage.RefreshSession, client fosite.Client, payload *refreshRetryPayload) (*ports.TokenResponse, error, error) {
	eligible, err := p.retryEligibility(owner, at, input, root)
	if err != nil {
		return nil, nil, err
	}
	if !eligible {
		if err := p.refreshDeps.Revocations.RevokeByID(owner, root.ID, at, storage.RefreshReasonProhibitedReuse); err != nil {
			return nil, nil, fosite.ErrServerError.WithWrap(err)
		}
		return nil, fosite.ErrInvalidGrant, nil
	}
	input.auditStage = "retry_payload"
	if payload == nil || validateRefreshRetryPayload(*payload, root) != nil {
		return nil, nil, fosite.ErrServerError.WithDebug("refresh retry result changed during preparation")
	}
	input.auditStage = "response_scope"
	if err := validateProviderResponseScopes(client, fosite.Arguments(strings.Fields(payload.Scope))); err != nil {
		return nil, nil, err
	}
	secondAt, err := p.refreshDeps.Clock.Now(owner)
	if err != nil {
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	_, currentRoot, token, secondClient, err := p.authorizedRefreshInOwner(owner, secondAt, input)
	if err != nil {
		return nil, nil, err
	}
	if token.UsedAt == nil {
		return nil, nil, fosite.ErrServerError.WithDebug("refresh retry token changed classification before commit")
	}
	eligible, err = p.retryEligibility(owner, secondAt, input, currentRoot)
	if err != nil {
		return nil, nil, err
	}
	if !eligible {
		if err := p.refreshDeps.Revocations.RevokeByID(owner, root.ID, secondAt, storage.RefreshReasonProhibitedReuse); err != nil {
			return nil, nil, fosite.ErrServerError.WithWrap(err)
		}
		return nil, fosite.ErrInvalidGrant, nil
	}
	input.auditStage = "retry_payload"
	if err := validateRefreshRetryPayload(*payload, currentRoot); err != nil {
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	if !secondAt.Before(payload.RetryExpiresAt) {
		return nil, nil, fosite.ErrInvalidGrant
	}
	input.auditStage = "response_scope"
	if err := validateProviderResponseScopes(secondClient, fosite.Arguments(strings.Fields(payload.Scope))); err != nil {
		return nil, nil, err
	}
	input.auditStage = "retry_counter_commit"
	currentRoot.RetryCount++
	if err := p.refreshDeps.Sessions.Save(owner, currentRoot); err != nil {
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	return &ports.TokenResponse{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, TokenType: payload.TokenType, Scope: payload.Scope, ExpiresIn: int64(payload.AccessExpiresAt.Sub(secondAt) / time.Second)}, nil, nil
}
