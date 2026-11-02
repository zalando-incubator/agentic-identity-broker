package security

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

const (
	AnonymousActor   = "anonymous"
	MaxActorRunes    = 256
	MaxUserAgentByte = 1024
)

type SecurityContext struct {
	TraceID       string
	Actor         id.Principal
	CallingPeer   string
	ClientIP      string
	UserAgent     string
	RequestMethod string
	RequestTarget string
	ReceivedAt    time.Time
}

type TransportCapture struct {
	TraceID       string
	ClientIP      string
	UserAgent     string
	RequestMethod string
	RequestTarget string
	ReceivedAt    time.Time
}

type securityContextKey struct{}

type captureHolderKey struct{}

type CaptureHolder struct {
	captured  TransportCapture
	mu        sync.RWMutex
	finalized bool
	final     SecurityContext
}

func WithSecurityContext(ctx context.Context, sc SecurityContext) context.Context {
	return context.WithValue(ctx, securityContextKey{}, sc)
}

func FromContext(ctx context.Context) (SecurityContext, bool) {
	if sc, ok := ctx.Value(securityContextKey{}).(SecurityContext); ok {
		return sc, true
	}

	holder, ok := CaptureHolderFromContext(ctx)
	if !ok {
		return SecurityContext{}, false
	}

	return holder.Finalized()
}

// NewSecurityContext builds a normalized, complete SecurityContext from a
// transport capture plus the resolved actor and calling peer. Actor is
// normalized (empty/invalid/over-long → anonymous); the calling peer is dropped
// when absent, invalid, over-long, or equal to the actor; the user agent is
// truncated. It is the single construction seam for both the immediate
// (ordinary HTTP) and deferred (token-exchange) finalization paths, so the two
// cannot drift.
func NewSecurityContext(capture TransportCapture, actor, callingPeer string) SecurityContext {
	normalizedActor := NormalizeActor(actor)
	return SecurityContext{
		TraceID:       capture.TraceID,
		Actor:         id.NewPrincipal(normalizedActor),
		CallingPeer:   NormalizeCallingPeer(callingPeer, normalizedActor),
		ClientIP:      capture.ClientIP,
		UserAgent:     TruncateUserAgent(capture.UserAgent),
		RequestMethod: capture.RequestMethod,
		RequestTarget: capture.RequestTarget,
		ReceivedAt:    capture.ReceivedAt,
	}
}

func NewCaptureHolder(captured TransportCapture) *CaptureHolder {
	return &CaptureHolder{captured: captured}
}

func WithCaptureHolder(ctx context.Context, holder *CaptureHolder) context.Context {
	return context.WithValue(ctx, captureHolderKey{}, holder)
}

func CaptureHolderFromContext(ctx context.Context) (*CaptureHolder, bool) {
	holder, ok := ctx.Value(captureHolderKey{}).(*CaptureHolder)
	if !ok || holder == nil {
		return nil, false
	}

	return holder, true
}

func FinalizeCaptureHolder(ctx context.Context, actor, callingPeer string) (SecurityContext, bool) {
	holder, ok := CaptureHolderFromContext(ctx)
	if !ok {
		return SecurityContext{}, false
	}

	return holder.Finalize(actor, callingPeer)
}

func (h *CaptureHolder) Finalize(actor, callingPeer string) (SecurityContext, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.finalized {
		return h.final, false
	}

	h.final = NewSecurityContext(h.captured, actor, callingPeer)
	h.finalized = true

	return h.final, true
}

func (h *CaptureHolder) Finalized() (SecurityContext, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.finalized {
		return SecurityContext{}, false
	}

	return h.final, true
}

func (h *CaptureHolder) Capture() TransportCapture {
	return h.captured
}

func NormalizeActor(actor string) string {
	trimmed := strings.TrimSpace(actor)
	if trimmed == "" || !utf8.ValidString(trimmed) || utf8.RuneCountInString(trimmed) > MaxActorRunes {
		return AnonymousActor
	}

	return trimmed
}

func NormalizeCallingPeer(callingPeer, actor string) string {
	trimmed := strings.TrimSpace(callingPeer)
	if trimmed == "" || !utf8.ValidString(trimmed) || utf8.RuneCountInString(trimmed) > MaxActorRunes {
		return ""
	}
	if trimmed == actor {
		return ""
	}

	return trimmed
}

func TruncateUserAgent(userAgent string) string {
	if len(userAgent) <= MaxUserAgentByte {
		return userAgent
	}

	truncated := userAgent[:MaxUserAgentByte]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}

	return truncated
}
