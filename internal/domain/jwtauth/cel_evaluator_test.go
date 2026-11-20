package jwtauth

import (
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger creates a logger that discards output (for clean test output).
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&discardWriter{}, nil))
}

type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (n int, err error) { return len(p), nil }

// TestCELEvaluator_NewCELEvaluator_CompileErrors tests that invalid CEL expressions
// are detected at construction time (fail-fast).
func TestCELEvaluator_NewCELEvaluator_CompileErrors(t *testing.T) {
	tests := []struct {
		name        string
		config      CELEvaluatorConfig
		expectError string
	}{
		{
			name: "invalid principal expression syntax",
			config: CELEvaluatorConfig{
				PrincipalExpression: "claims[[[invalid",
			},
			expectError: "invalid principal_expression",
		},
		{
			name: "invalid display name expression syntax",
			config: CELEvaluatorConfig{
				PrincipalExpression:   "claims.sub",
				DisplayNameExpression: "claims..name",
			},
			expectError: "invalid display_name_expression",
		},
		{
			name: "invalid email expression syntax",
			config: CELEvaluatorConfig{
				PrincipalExpression: "claims.sub",
				EmailExpression:     "undefined_var.email",
			},
			expectError: "invalid email_expression",
		},
		{
			name: "invalid picture URL expression syntax",
			config: CELEvaluatorConfig{
				PrincipalExpression:  "claims.sub",
				PictureURLExpression: "bad syntax +++",
			},
			expectError: "invalid picture_url_expression",
		},
		{
			name: "empty principal expression uses default claims.sub",
			config: CELEvaluatorConfig{
				PrincipalExpression: "", // should default to "claims.sub"
			},
			expectError: "", // no error expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator, err := NewCELEvaluator(tt.config, testLogger())

			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				assert.Nil(t, evaluator)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, evaluator)
			}
		})
	}
}

// TestCELEvaluator_ExtractClaims_StringExtraction tests successful extraction of
// string values from JWT claims using CEL expressions.
func TestCELEvaluator_ExtractClaims_StringExtraction(t *testing.T) {
	tests := []struct {
		name              string
		config            CELEvaluatorConfig
		claims            map[string]interface{}
		expectedPrincipal id.Principal
		expectedDisplay   *string
		expectedEmail     *string
		expectedPicture   *string
	}{
		{
			name: "extract all fields from standard claims",
			config: CELEvaluatorConfig{
				PrincipalExpression:   "claims.sub",
				DisplayNameExpression: "claims.name",
				EmailExpression:       "claims.email",
				PictureURLExpression:  "claims.picture",
			},
			claims: map[string]interface{}{
				"sub":     "user123",
				"name":    "Alice Smith",
				"email":   "alice@example.com",
				"picture": "https://example.com/alice.jpg",
			},
			expectedPrincipal: "user123",
			expectedDisplay:   strPtr("Alice Smith"),
			expectedEmail:     strPtr("alice@example.com"),
			expectedPicture:   strPtr("https://example.com/alice.jpg"),
		},
		{
			name: "extract principal only (no optional expressions configured)",
			config: CELEvaluatorConfig{
				PrincipalExpression: "claims.sub",
			},
			claims: map[string]interface{}{
				"sub":   "user456",
				"name":  "Bob",
				"email": "bob@example.com",
			},
			expectedPrincipal: "user456",
			expectedDisplay:   nil,
			expectedEmail:     nil,
			expectedPicture:   nil,
		},
		{
			name: "default principal expression (claims.sub)",
			config: CELEvaluatorConfig{
				PrincipalExpression: "", // defaults to claims.sub
			},
			claims: map[string]interface{}{
				"sub": "default-user",
			},
			expectedPrincipal: "default-user",
			expectedDisplay:   nil,
			expectedEmail:     nil,
			expectedPicture:   nil,
		},
		{
			name: "nested claim extraction",
			config: CELEvaluatorConfig{
				PrincipalExpression:   "claims.sub",
				DisplayNameExpression: "claims.given_name",
			},
			claims: map[string]interface{}{
				"sub":        "user789",
				"given_name": "Charlie",
			},
			expectedPrincipal: "user789",
			expectedDisplay:   strPtr("Charlie"),
			expectedEmail:     nil,
			expectedPicture:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator, err := NewCELEvaluator(tt.config, testLogger())
			require.NoError(t, err)

			result, err := evaluator.ExtractClaims(t.Context(), tt.claims)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, tt.expectedPrincipal, result.Principal)
			assert.Equal(t, tt.expectedDisplay, result.DisplayName)
			assert.Equal(t, tt.expectedEmail, result.Email)
			assert.Equal(t, tt.expectedPicture, result.PictureURL)
			assert.Equal(t, tt.claims, result.Claims)
		})
	}
}

// TestCELEvaluator_ExtractClaims_NonStringHandling tests that non-string CEL results
// for optional fields are treated as nil (with warning log), while non-string principal
// results cause authentication failure.
func TestCELEvaluator_ExtractClaims_NonStringHandling(t *testing.T) {
	t.Run("non-string principal causes error", func(t *testing.T) {
		// Configure principal expression that returns an integer
		config := CELEvaluatorConfig{
			PrincipalExpression: "claims.user_id",
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)

		claims := map[string]interface{}{
			"user_id": 12345, // integer, not string
		}
		result, err := evaluator.ExtractClaims(t.Context(), claims)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrClaimExtraction)
	})

	t.Run("non-string optional display name treated as nil", func(t *testing.T) {
		config := CELEvaluatorConfig{
			PrincipalExpression:   "claims.sub",
			DisplayNameExpression: "claims.display_id",
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)

		claims := map[string]interface{}{
			"sub":        "user123",
			"display_id": 99, // integer, not string
		}
		result, err := evaluator.ExtractClaims(t.Context(), claims)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, id.Principal("user123"), result.Principal)
		assert.Nil(t, result.DisplayName) // non-string → treated as absent
	})

	t.Run("non-string optional email treated as nil", func(t *testing.T) {
		config := CELEvaluatorConfig{
			PrincipalExpression: "claims.sub",
			EmailExpression:     "claims.verified",
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)

		claims := map[string]interface{}{
			"sub":      "user123",
			"verified": true, // bool, not string
		}
		result, err := evaluator.ExtractClaims(t.Context(), claims)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Nil(t, result.Email) // bool → treated as absent
	})

	t.Run("empty principal causes error", func(t *testing.T) {
		config := CELEvaluatorConfig{
			PrincipalExpression: "claims.sub",
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)

		claims := map[string]interface{}{
			"sub": "", // empty string
		}
		result, err := evaluator.ExtractClaims(t.Context(), claims)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrClaimExtraction)
	})

	t.Run("missing claim for principal causes error", func(t *testing.T) {
		config := CELEvaluatorConfig{
			PrincipalExpression: "claims.sub",
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)

		claims := map[string]interface{}{
			"iss": "some-issuer",
			// "sub" is missing
		}
		result, err := evaluator.ExtractClaims(t.Context(), claims)
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("missing claim for optional field returns nil", func(t *testing.T) {
		config := CELEvaluatorConfig{
			PrincipalExpression:   "claims.sub",
			DisplayNameExpression: "claims.name",
			EmailExpression:       "claims.email",
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)

		claims := map[string]interface{}{
			"sub": "user123",
			// "name" and "email" missing
		}
		result, err := evaluator.ExtractClaims(t.Context(), claims)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, id.Principal("user123"), result.Principal)
		assert.Nil(t, result.DisplayName)
		assert.Nil(t, result.Email)
	})
}

// TestCELEvaluator_ExtractClaims_Timeout tests that CEL evaluation respects timeouts.
func TestCELEvaluator_ExtractClaims_Timeout(t *testing.T) {
	t.Run("evaluation completes within timeout", func(t *testing.T) {
		config := CELEvaluatorConfig{
			PrincipalExpression: "claims.sub",
			EvaluationTimeout:   1 * time.Second,
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)

		claims := map[string]interface{}{
			"sub": "fast-user",
		}
		result, err := evaluator.ExtractClaims(t.Context(), claims)
		require.NoError(t, err)
		assert.Equal(t, id.Principal("fast-user"), result.Principal)
	})

	t.Run("default timeout is 100ms", func(t *testing.T) {
		config := CELEvaluatorConfig{
			PrincipalExpression: "claims.sub",
			// No timeout set — defaults to 100ms
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)
		assert.Equal(t, 100*time.Millisecond, evaluator.evaluationTimeout)
	})
}

// TestCELEvaluator_PrincipalRequired tests that the principal expression is always
// required and must produce a non-empty string.
func TestCELEvaluator_PrincipalRequired(t *testing.T) {
	t.Run("principal expression always compiled even when empty (defaults to claims.sub)", func(t *testing.T) {
		config := CELEvaluatorConfig{
			PrincipalExpression: "",
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)

		claims := map[string]interface{}{
			"sub": "default-principal",
		}
		result, err := evaluator.ExtractClaims(t.Context(), claims)
		require.NoError(t, err)
		assert.Equal(t, id.Principal("default-principal"), result.Principal)
	})

	t.Run("principal must be non-empty string", func(t *testing.T) {
		config := CELEvaluatorConfig{
			PrincipalExpression: "claims.sub",
		}
		evaluator, err := NewCELEvaluator(config, testLogger())
		require.NoError(t, err)

		claims := map[string]interface{}{
			"sub": "",
		}
		_, err = evaluator.ExtractClaims(t.Context(), claims)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrClaimExtraction)
	})
}

// strPtr is a helper to create a *string from a string literal.
func strPtr(s string) *string {
	return &s
}
