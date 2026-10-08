package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// SecurityContextMiddleware captures the request security context. cfg is the
// already-resolved configuration; resolve caller-supplied config once at the
// composition root (NewHandler) via ResolveRequestContextConfig and pass the
// concrete value here, so this middleware carries no nil/default ambiguity.
func SecurityContextMiddleware(resolved ports.RequestContextConfig, reuseSpanTraceID bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := traceIDFromContext(r.Context(), reuseSpanTraceID)
			capture := security.TransportCapture{
				TraceID:       traceID,
				ClientIP:      resolveClientIP(r, resolved),
				UserAgent:     security.TruncateUserAgent(r.UserAgent()),
				RequestMethod: r.Method,
				RequestTarget: requestTarget(r),
				ReceivedAt:    time.Now().UTC(),
			}

			ctx := ledger.CaptureRequestSpan(r.Context())
			if shouldDeferSecurityContextFinalization(r) {
				ctx = security.WithCaptureHolder(ctx, security.NewCaptureHolder(capture))
			} else {
				ctx = security.WithSecurityContext(ctx, security.NewSecurityContext(capture, actorFromContext(ctx), ""))
			}

			if resolved.Trace.ResponseEnabled {
				w.Header().Set("traceresponse", TraceResponseValue(ctx, traceID))
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TraceResponseValue(ctx context.Context, traceID string) string {
	spanContext := trace.SpanContextFromContext(ctx)
	childID := randomHex(8)
	flagHigh, flagLow := byte('0'), byte('0')

	if spanContext.IsValid() && !spanContext.IsRemote() {
		childID = spanContext.SpanID().String()
		flagHigh, flagLow = hexPair(byte(spanContext.TraceFlags()))
		if traceID == "" {
			traceID = spanContext.TraceID().String()
		}
	}

	if traceID == "" {
		traceID = randomHex(16)
	}

	var value [55]byte
	copy(value[0:3], "00-")
	copy(value[3:35], traceID)
	value[35] = '-'
	copy(value[36:52], childID)
	value[52] = '-'
	value[53], value[54] = flagHigh, flagLow
	return string(value[:])
}

// ResolveRequestContextConfig layers the caller-supplied config over the package
// defaults, returning a fully resolved value. A nil cfg yields the defaults.
func ResolveRequestContextConfig(cfg *ports.RequestContextConfig) ports.RequestContextConfig {
	resolved := ports.DefaultRequestContextConfig()
	if cfg == nil {
		return resolved
	}

	resolved.TrustedProxy.Enabled = cfg.TrustedProxy.Enabled
	if strings.TrimSpace(cfg.TrustedProxy.ForwardedHeader) != "" {
		resolved.TrustedProxy.ForwardedHeader = cfg.TrustedProxy.ForwardedHeader
	}
	resolved.Trace.ResponseEnabled = cfg.Trace.ResponseEnabled

	return resolved
}

func traceIDFromContext(ctx context.Context, reuseSpanTraceID bool) string {
	if reuseSpanTraceID {
		spanContext := trace.SpanContextFromContext(ctx)
		if spanContext.IsValid() {
			return spanContext.TraceID().String()
		}
	}

	return randomHex(16)
}

func requestTarget(r *http.Request) string {
	target := r.URL.EscapedPath()
	if target == "" {
		target = r.URL.Path
	}
	if target == "" {
		return "/"
	}

	return target
}

func actorFromContext(ctx context.Context) string {
	actor, ok := principal.FromContext(ctx)
	if !ok {
		return security.AnonymousActor
	}

	return security.NormalizeActor(actor)
}

func shouldDeferSecurityContextFinalization(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == "/oauth2/token"
}

// FinalizeRequestSecurityContext finalizes a still-open deferred capture holder
// using the request principal as actor (anonymous sentinel when absent) and no
// calling peer. It is the HTTP-lifecycle finalization for requests that carry no
// distinct calling peer, layered over the domain primitive
// security.FinalizeCaptureHolder.
//
// It is the single seam shared by the perimeter (LoggingMiddleware after the
// handler returns) and by handlers that must finalize earlier so their in-handler
// audit logs carry actor and trace_id (the /oauth2/token non-exchange grant path).
// Idempotent: a no-op once the context has already been finalized.
func FinalizeRequestSecurityContext(ctx context.Context) {
	if _, ok := security.FromContext(ctx); ok {
		return
	}

	actor, ok := principal.FromContext(ctx)
	if !ok {
		actor = security.AnonymousActor
	}
	_, _ = security.FinalizeCaptureHolder(ctx, actor, "")
}

func hexPair(value byte) (byte, byte) {
	const digits = "0123456789abcdef"
	return digits[value>>4], digits[value&0x0f]
}

func randomHex(byteLen int) string {
	if byteLen == 8 {
		var raw [8]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
		}
		var encoded [16]byte
		hex.Encode(encoded[:], raw[:])
		return string(encoded[:])
	}

	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strings.ReplaceAll(uuid.NewString(), "-", "")[:32]
	}
	var encoded [32]byte
	hex.Encode(encoded[:], raw[:])
	return string(encoded[:])
}
