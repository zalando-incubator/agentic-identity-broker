package security

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestWithSecurityContext_RoundTrip(t *testing.T) {
	t.Parallel()

	receivedAt := time.Date(2026, time.July, 3, 12, 34, 56, 0, time.UTC)
	want := SecurityContext{
		TraceID:       "0123456789abcdef0123456789abcdef",
		Actor:         "alice@example.com",
		CallingPeer:   "gateway-client-1",
		ClientIP:      "203.0.113.9",
		UserAgent:     "broker-tests/1.0",
		RequestMethod: "POST",
		RequestTarget: "/oauth2/token",
		ReceivedAt:    receivedAt,
	}

	parent := context.Background()
	ctx := WithSecurityContext(parent, want)

	got, ok := FromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, want, got)

	_, parentOK := FromContext(parent)
	assert.False(t, parentOK, "WithSecurityContext must not mutate the parent context")
}

func TestWithSecurityContext_UsesImmutableValueSemantics(t *testing.T) {
	t.Parallel()

	t.Run("mutating the original struct after storage does not affect later readers", func(t *testing.T) {
		t.Parallel()

		original := SecurityContext{
			TraceID:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Actor:         "alice@example.com",
			CallingPeer:   "gateway-client-1",
			ClientIP:      "203.0.113.10",
			UserAgent:     "broker-tests/1.0",
			RequestMethod: "GET",
			RequestTarget: "/api/me",
			ReceivedAt:    time.Date(2026, time.July, 3, 13, 0, 0, 0, time.UTC),
		}

		ctx := WithSecurityContext(context.Background(), original)
		original.Actor = "mallory@example.com"
		original.CallingPeer = "other-client"
		original.RequestTarget = "/tampered"

		got, ok := FromContext(ctx)
		require.True(t, ok)
		assert.Equal(t, id.NewPrincipal("alice@example.com"), got.Actor)
		assert.Equal(t, "gateway-client-1", got.CallingPeer)
		assert.Equal(t, "/api/me", got.RequestTarget)
	})

	t.Run("mutating a retrieved copy does not alter the stored context", func(t *testing.T) {
		t.Parallel()

		ctx := WithSecurityContext(context.Background(), SecurityContext{
			TraceID:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Actor:         "service-user",
			CallingPeer:   "gateway-client-2",
			ClientIP:      "198.51.100.22",
			UserAgent:     "broker-tests/2.0",
			RequestMethod: "PATCH",
			RequestTarget: "/api/users/123",
			ReceivedAt:    time.Date(2026, time.July, 3, 13, 30, 0, 0, time.UTC),
		})

		firstRead, ok := FromContext(ctx)
		require.True(t, ok)
		firstRead.Actor = "tampered"
		firstRead.CallingPeer = "tampered-peer"

		secondRead, ok := FromContext(ctx)
		require.True(t, ok)
		assert.Equal(t, id.NewPrincipal("service-user"), secondRead.Actor)
		assert.Equal(t, "gateway-client-2", secondRead.CallingPeer)
	})
}

func TestFromContext_Absent(t *testing.T) {
	t.Parallel()

	got, ok := FromContext(context.Background())
	assert.False(t, ok)
	assert.Equal(t, SecurityContext{}, got)
}

func TestFromContext_EmptyCallingPeerRoundTripsAsOmitted(t *testing.T) {
	t.Parallel()

	ctx := WithSecurityContext(context.Background(), SecurityContext{
		TraceID:       "cccccccccccccccccccccccccccccccc",
		Actor:         "anonymous",
		CallingPeer:   "",
		ClientIP:      "",
		UserAgent:     "",
		RequestMethod: "GET",
		RequestTarget: "/health",
		ReceivedAt:    time.Date(2026, time.July, 3, 14, 0, 0, 0, time.UTC),
	})

	got, ok := FromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, id.NewPrincipal("anonymous"), got.Actor)
	assert.Empty(t, got.CallingPeer, "calling_peer must stay omitted when no distinct peer exists")
}

func TestCaptureHolder_FinalizeOnceForDownstreamReaders(t *testing.T) {
	t.Parallel()

	receivedAt := time.Date(2026, time.July, 3, 15, 0, 0, 0, time.UTC)
	captured := TransportCapture{
		TraceID:       "dddddddddddddddddddddddddddddddd",
		ClientIP:      "203.0.113.44",
		UserAgent:     "delegated-client/1.0",
		RequestMethod: "POST",
		RequestTarget: "/oauth2/token",
		ReceivedAt:    receivedAt,
	}

	holder := NewCaptureHolder(captured)
	ctx := WithCaptureHolder(context.Background(), holder)

	_, ok := FromContext(ctx)
	assert.False(t, ok, "downstream readers must not observe transport-only state before delegated finalization")

	finalized, finalizedNow := holder.Finalize(id.NewPrincipal("subject-user@example.com"), "privileged-client-1")
	require.True(t, finalizedNow, "the delegated seam must finalize exactly once")

	expected := SecurityContext{
		TraceID:       "dddddddddddddddddddddddddddddddd",
		Actor:         "subject-user@example.com",
		CallingPeer:   "privileged-client-1",
		ClientIP:      "203.0.113.44",
		UserAgent:     "delegated-client/1.0",
		RequestMethod: "POST",
		RequestTarget: "/oauth2/token",
		ReceivedAt:    receivedAt,
	}
	assert.Equal(t, expected, finalized)

	got, ok := FromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, expected, got, "downstream readers must observe the finalized security context")

	again, finalizedAgain := holder.Finalize(id.NewPrincipal("other-user@example.com"), "other-client")
	assert.False(t, finalizedAgain, "once finalized, subsequent attempts must be ignored")
	assert.Equal(t, expected, again, "subsequent finalization attempts must return the original final context")

	gotAgain, ok := FromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, expected, gotAgain)
}

func TestNormalizeActor_LengthBoundary(t *testing.T) {
	t.Parallel()

	atLimit := strings.Repeat("a", MaxActorRunes)
	overLimit := strings.Repeat("a", MaxActorRunes+1)

	assert.Equal(t, atLimit, NormalizeActor(atLimit),
		"an actor of exactly MaxActorRunes (256) runes must be accepted verbatim")
	assert.Equal(t, AnonymousActor, NormalizeActor(overLimit),
		"an actor of MaxActorRunes+1 (257) runes must fall back to anonymous")
}

func TestNormalizeActor_MultibyteLengthBoundary(t *testing.T) {
	t.Parallel()

	// 'é' is multi-byte but a single rune; the limit is counted in runes, not bytes.
	atLimit := strings.Repeat("é", MaxActorRunes)
	overLimit := strings.Repeat("é", MaxActorRunes+1)

	assert.Equal(t, atLimit, NormalizeActor(atLimit),
		"256 multi-byte runes must be accepted since the cap is rune-based")
	assert.Equal(t, AnonymousActor, NormalizeActor(overLimit),
		"257 multi-byte runes must fall back to anonymous")
}

func TestNormalizeActor_EmptyAndInvalid(t *testing.T) {
	t.Parallel()

	assert.Equal(t, AnonymousActor, NormalizeActor(""))
	assert.Equal(t, AnonymousActor, NormalizeActor("   "))
	assert.Equal(t, AnonymousActor, NormalizeActor("\xff\xfe"),
		"invalid UTF-8 must fall back to anonymous")
	assert.Equal(t, "actor@example.com", NormalizeActor("  actor@example.com  "),
		"surrounding whitespace must be trimmed")
}

func TestNormalizeCallingPeer_LengthBoundary(t *testing.T) {
	t.Parallel()

	atLimit := strings.Repeat("p", MaxActorRunes)
	overLimit := strings.Repeat("p", MaxActorRunes+1)

	assert.Equal(t, atLimit, NormalizeCallingPeer(atLimit, "actor"),
		"a calling peer of exactly MaxActorRunes (256) runes must be accepted verbatim")
	assert.Empty(t, NormalizeCallingPeer(overLimit, "actor"),
		"a calling peer of MaxActorRunes+1 (257) runes must be dropped")
}

func TestNormalizeCallingPeer_CollapsesWhenEqualToActor(t *testing.T) {
	t.Parallel()

	assert.Empty(t, NormalizeCallingPeer("actor@example.com", "actor@example.com"),
		"a calling peer identical to the actor must collapse to empty")
	assert.Equal(t, "peer@example.com", NormalizeCallingPeer("  peer@example.com  ", "actor@example.com"),
		"a distinct calling peer must be trimmed and retained")
}

func TestNewSecurityContext_NormalizesAndCompletes(t *testing.T) {
	t.Parallel()

	receivedAt := time.Date(2026, time.July, 4, 9, 0, 0, 0, time.UTC)
	capture := TransportCapture{
		TraceID:       "0123456789abcdef0123456789abcdef",
		ClientIP:      "203.0.113.10",
		UserAgent:     "client/1.0",
		RequestMethod: "POST",
		RequestTarget: "/oauth2/token",
		ReceivedAt:    receivedAt,
	}

	t.Run("normalizes actor and retains a distinct calling peer", func(t *testing.T) {
		t.Parallel()
		sc := NewSecurityContext(capture, id.NewPrincipal("  user@example.com  "), "peer@example.com")
		assert.Equal(t, id.NewPrincipal("user@example.com"), sc.Actor)
		assert.Equal(t, "peer@example.com", sc.CallingPeer)
		assert.Equal(t, capture.TraceID, sc.TraceID)
		assert.Equal(t, capture.ClientIP, sc.ClientIP)
		assert.Equal(t, capture.RequestTarget, sc.RequestTarget)
		assert.Equal(t, receivedAt, sc.ReceivedAt)
	})

	t.Run("oversized actor falls back to anonymous", func(t *testing.T) {
		t.Parallel()
		sc := NewSecurityContext(capture, id.NewPrincipal(strings.Repeat("a", MaxActorRunes+1)), "")
		assert.Equal(t, id.NewPrincipal(AnonymousActor), sc.Actor)
		assert.Empty(t, sc.CallingPeer)
	})

	t.Run("calling peer equal to actor collapses to empty", func(t *testing.T) {
		t.Parallel()
		sc := NewSecurityContext(capture, id.NewPrincipal("same@example.com"), "same@example.com")
		assert.Equal(t, id.NewPrincipal("same@example.com"), sc.Actor)
		assert.Empty(t, sc.CallingPeer)
	})

	t.Run("oversized user agent is truncated", func(t *testing.T) {
		t.Parallel()
		oversized := strings.Repeat("u", MaxUserAgentByte+50)
		sc := NewSecurityContext(TransportCapture{UserAgent: oversized}, id.NewPrincipal("actor"), "")
		assert.LessOrEqual(t, len(sc.UserAgent), MaxUserAgentByte)
	})
}
