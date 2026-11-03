package principal_test

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testContextKey is a custom type for test context keys to avoid collisions
type testContextKey string

func TestWithPrincipal(t *testing.T) {
	ctx := context.Background()
	principals := []id.Principal{
		"alice@example.com",
		"user123",
		"alice.smith",
		"user+tag@example.com",
		"用户",      // Chinese characters
		"🚀rocket", // Emoji
	}

	for _, p := range principals {
		t.Run(p.String(), func(t *testing.T) {
			newCtx := principal.WithPrincipal(ctx, p)

			// Verify new context contains the principal
			retrieved, ok := principal.FromContext(newCtx)
			assert.True(t, ok, "principal should be present in context")
			assert.Equal(t, p, retrieved)

			// Verify parent context is unmodified (has no principal)
			_, ok = principal.FromContext(ctx)
			assert.False(t, ok, "parent context should not have principal")
		})
	}
}

func TestFromContext_Present(t *testing.T) {
	ctx := context.Background()
	ctx = principal.WithPrincipal(ctx, "alice@example.com")

	value, ok := principal.FromContext(ctx)

	assert.True(t, ok)
	assert.Equal(t, id.Principal("alice@example.com"), value)
}

func TestFromContext_Missing(t *testing.T) {
	ctx := context.Background()

	value, ok := principal.FromContext(ctx)

	assert.False(t, ok)
	assert.Empty(t, value)
}

func TestFromContext_WithOtherContextValues(t *testing.T) {
	// Create context with other values to ensure principal key doesn't collide
	ctx := context.Background()
	ctx = context.WithValue(ctx, testContextKey("other-key-1"), "value1")
	ctx = context.WithValue(ctx, testContextKey("other-key-2"), "value2")

	// Add principal
	ctx = principal.WithPrincipal(ctx, "alice@example.com")

	// Verify principal is present
	p, ok := principal.FromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, id.Principal("alice@example.com"), p)

	// Verify other values still accessible
	assert.Equal(t, "value1", ctx.Value(testContextKey("other-key-1")))
	assert.Equal(t, "value2", ctx.Value(testContextKey("other-key-2")))
}

func TestMustFromContext_Success(t *testing.T) {
	ctx := context.Background()
	ctx = principal.WithPrincipal(ctx, "alice@example.com")

	value := principal.MustFromContext(ctx)

	assert.Equal(t, id.Principal("alice@example.com"), value)
}

func TestMustFromContext_Panics(t *testing.T) {
	ctx := context.Background()

	assert.Panics(t, func() {
		principal.MustFromContext(ctx)
	})
}

func TestContextImmutability(t *testing.T) {
	// Verify that contexts are properly isolated and immutable
	ctx1 := context.Background()
	ctx2 := context.Background()

	ctx1WithPrincipal := principal.WithPrincipal(ctx1, "alice")
	ctx2WithPrincipal := principal.WithPrincipal(ctx2, "bob")

	// Verify each context has its own principal
	p1, ok1 := principal.FromContext(ctx1WithPrincipal)
	p2, ok2 := principal.FromContext(ctx2WithPrincipal)

	require.True(t, ok1)
	require.True(t, ok2)
	assert.Equal(t, id.Principal("alice"), p1)
	assert.Equal(t, id.Principal("bob"), p2)
	assert.NotEqual(t, p1, p2)
}

func TestContextCancellation(t *testing.T) {
	// Verify that cancellation signals are preserved through WithPrincipal
	ctx, cancel := context.WithCancel(context.Background())
	ctx = principal.WithPrincipal(ctx, "alice@example.com")

	// Context should not be cancelled initially
	assert.NoError(t, ctx.Err())

	// Cancel the context
	cancel()

	// Context should now be cancelled
	assert.Error(t, ctx.Err())

	// But principal should still be retrievable
	p, ok := principal.FromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, id.Principal("alice@example.com"), p)
}

// Benchmark context operations for performance validation
func BenchmarkWithPrincipal(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		principal.WithPrincipal(ctx, "alice@example.com")
	}
}

func BenchmarkFromContext(b *testing.B) {
	ctx := principal.WithPrincipal(context.Background(), "alice@example.com")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		principal.FromContext(ctx)
	}
}

func BenchmarkMustFromContext(b *testing.B) {
	ctx := principal.WithPrincipal(context.Background(), "alice@example.com")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		principal.MustFromContext(ctx)
	}
}
