package server

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// TokenBinding identifies the permit that first claims an exchange result, not exchange provenance.
type TokenBinding struct {
	Scopes         []string
	GrantUpdatedAt time.Time
}

func (TokenBinding) String() string         { return "token binding [redacted]" }
func (b TokenBinding) GoString() string     { return b.String() }
func (b TokenBinding) LogValue() slog.Value { return slog.StringValue(b.String()) }

// ErrTokenBindingConflict means the single permitted retry delivered an incompatible result.
var ErrTokenBindingConflict = errors.New("delegation token binding unavailable")

type tokenResultHandle struct {
	mu      sync.Mutex
	binding *TokenBinding
}

func (b TokenBinding) normalized() TokenBinding {
	b.Scopes = slices.Clone(b.Scopes)
	slices.Sort(b.Scopes)
	b.Scopes = slices.Compact(b.Scopes)
	return b
}

func (h *tokenResultHandle) bindOnce(binding TokenBinding) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.binding == nil {
		copy := binding.normalized()
		h.binding = &copy
	}
	return slices.Equal(h.binding.Scopes, binding.Scopes) && h.binding.GrantUpdatedAt.Equal(binding.GrantUpdatedAt)
}

func (te *TokenExchanger) ExchangeBound(ctx context.Context, subjectToken, resourceURI string, observed *ExchangeResult, binding TokenBinding) (ExchangeResult, error) {
	if err := ctx.Err(); err != nil {
		return ExchangeResult{}, NewOperationError(NewErrorMetadata(OperationExchange, KindCallerCanceled, DependencyLocal, 0, ""), err)
	}
	if _, err := te.ClientAssertion(); err != nil {
		return ExchangeResult{}, err
	}
	binding = binding.normalized()
	var result ExchangeResult
	if observed == nil {
		var err error
		result, err = te.Exchange(ctx, subjectToken, resourceURI)
		if err != nil {
			return ExchangeResult{}, err
		}
	} else {
		result = *observed
	}
	if result.handle.bindOnce(binding) {
		return result, nil
	}
	logger := loggerFromContext(ctx, te.logger).With(slog.String("delegation_binding", "mismatch"))
	logDiagnostic(ctx, logger, slog.LevelDebug, "delegation token binding mismatch", SuccessDiagnostic(ExchangeUnknown))

	key := tokenCacheKey{subjectToken: subjectToken, resourceURI: resourceURI}
	te.cacheMu.Lock()
	var replacement *cachedToken
	if entry := te.cache[key]; entry != nil {
		if entry.handle == result.handle {
			delete(te.cache, key)
		} else if !entry.isExpired() && entry.reAuthErr == nil {
			replacement = entry
		}
	}
	te.cacheMu.Unlock()
	if replacement != nil && replacement.handle.bindOnce(binding) {
		return replacement.result(), nil
	}

	// Retry once without stale fallback or incompatible success-cache reuse. Cooldown survives.
	result, err := te.exchange(ctx, subjectToken, resourceURI, false)
	if err != nil {
		return ExchangeResult{}, err
	}
	if !result.handle.bindOnce(binding) {
		return ExchangeResult{}, ErrTokenBindingConflict
	}
	return result, nil
}

func (c *cachedToken) result() ExchangeResult {
	return ExchangeResult{handle: c.handle, Token: c.accessToken, GrantedPermissionSets: cloneGrantedPermissionSets(c.grantedPermissionSets), Principal: c.principal, AgentID: c.agentID}
}
