package aws

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	keystoretypes "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographykeystoresmithygeneratedtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/branchkey"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// TestCreateBranchKeyPostCallContextExpiry guards the error-classification branch in
// CreateBranchKey (keystore.go:184-194).
//
// Three cases are required:
//  1. internal_timeout_expiry – adapter's own dynamoDB timeout sentinel fires → KEKUnavailable
//     with "dynamodb_timeout" in the message.
//  2. parent_deadline_expiry  – caller's deadline fires (not errDynamoDBTimeout) → KEKUnavailable
//     with generic "failed to create branch key" message (NOT "dynamodb_timeout").
//  3. create_key_failure      – CreateKey returns an error while ctx is still alive → generic
//     KEKUnavailable, no context classification at all.
func TestCreateBranchKeyPostCallContextExpiry(t *testing.T) {
	subject := encryption.NewServiceBranchKeySubject(id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"))
	sdkErr := errors.New("mock DynamoDB error")

	// internal_timeout_expiry: a non-zero dynamoDBTimeout causes CreateBranchKey to wrap the
	// caller's context with context.WithTimeoutCause.  The mock blocks on that derived
	// ctx.Done() so the adapter's own sentinel fires mid-call.  Asserting the configured
	// duration in the error message proves the real internal-timeout path was exercised rather
	// than a parent-context cancellation that happened to carry errDynamoDBTimeout as its cause.
	t.Run("internal_timeout_expiry", func(t *testing.T) {
		const internalTimeout = 1 * time.Millisecond

		ks := &KeyStore{
			dynamoDBTimeout: internalTimeout,
			createKeyFn: func(ctx context.Context, _ keystoretypes.CreateKeyInput) (*keystoretypes.CreateKeyOutput, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return nil, errors.New("test guard: context did not expire — WithTimeoutCause not applied")
				}
			},
		}

		_, err := ks.CreateBranchKey(context.Background(), subject)
		require.Error(t, err)
		assert.True(t, isKEKUnavailableError(err), "expected KEKUnavailable, got %v (type %T)", err, err)
		assert.Contains(t, err.Error(), "dynamodb_timeout="+internalTimeout.String())
	})

	t.Run("parent_deadline_expiry", func(t *testing.T) {
		ctx, cancelCause := context.WithCancelCause(context.Background())

		ks := &KeyStore{
			dynamoDBTimeout: 1 * time.Second,
			createKeyFn: func(_ context.Context, _ keystoretypes.CreateKeyInput) (*keystoretypes.CreateKeyOutput, error) {
				cancelCause(context.DeadlineExceeded)
				return nil, sdkErr
			},
		}

		_, err := ks.CreateBranchKey(ctx, subject)
		require.Error(t, err)
		assert.True(t, isKEKUnavailableError(err), "expected KEKUnavailable, got %v (type %T)", err, err)
		assert.NotContains(t, err.Error(), "dynamodb_timeout",
			"parent deadline must not be misclassified as adapter dynamodb_timeout")
		assert.Contains(t, err.Error(), "failed to create branch key")
	})

	t.Run("create_key_failure_no_context_error", func(t *testing.T) {
		ks := &KeyStore{
			dynamoDBTimeout: 0,
			createKeyFn: func(_ context.Context, _ keystoretypes.CreateKeyInput) (*keystoretypes.CreateKeyOutput, error) {
				return nil, errors.New("DynamoDB: provisioned throughput exceeded")
			},
		}

		_, err := ks.CreateBranchKey(context.Background(), subject)
		require.Error(t, err)
		assert.True(t, isKEKUnavailableError(err), "expected KEKUnavailable, got %v (type %T)", err, err)
		assert.Contains(t, err.Error(), "failed to create branch key")
	})
}

func TestCreateBranchKeyLogsNonTimeoutFailure(t *testing.T) {
	subject := encryption.NewServiceBranchKeySubject(id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"))
	expectedBranchKeyID, err := branchkey.GenerateBranchKeyId(subject)
	require.NoError(t, err)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	ks := &KeyStore{
		createKeyFn: func(_ context.Context, _ keystoretypes.CreateKeyInput) (*keystoretypes.CreateKeyOutput, error) {
			return nil, errors.New("ProvisionedThroughputExceededException")
		},
	}

	_, err = ks.CreateBranchKey(context.Background(), subject)
	require.Error(t, err)

	assert.Contains(t, logBuf.String(), "\"msg\":\"branch_key_creation_failed\"")
	assert.Contains(t, logBuf.String(), "\"branch_key_id\":\""+expectedBranchKeyID+"\"")
	assert.Contains(t, logBuf.String(), "\"service_id\":\"550e8400-e29b-41d4-a716-446655440000\"")
	assert.Contains(t, logBuf.String(), "\"subject_kind\":\"service\"")
	assert.Contains(t, logBuf.String(), "\"reason\":\"create_key_failed\"")
}

func TestCreateBranchKeyAppliesTimeoutToGetActiveBranchKey(t *testing.T) {
	subject := encryption.NewServiceBranchKeySubject(id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"))
	observedCause := make(chan error, 1)

	ks := &KeyStore{
		dynamoDBTimeout: 1 * time.Millisecond,
		getActiveBranchKeyFn: func(ctx context.Context, _ keystoretypes.GetActiveBranchKeyInput) (*keystoretypes.GetActiveBranchKeyOutput, error) {
			select {
			case <-ctx.Done():
				observedCause <- context.Cause(ctx)
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
				observedCause <- nil
				return nil, errors.New("test guard: GetActiveBranchKey context did not expire — WithTimeoutCause not applied")
			}
		},
		createKeyFn: func(ctx context.Context, _ keystoretypes.CreateKeyInput) (*keystoretypes.CreateKeyOutput, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return nil, errors.New("test guard: CreateKey context did not expire")
			}
		},
	}

	_, err := ks.CreateBranchKey(context.Background(), subject)
	require.Error(t, err)
	assert.ErrorIs(t, <-observedCause, errDynamoDBTimeout)
	assert.Contains(t, err.Error(), "dynamodb_timeout=1ms")
}

func TestCreateBranchKeyGetActiveBranchKeyTimeoutDoesNotFallThroughToCreateKey(t *testing.T) {
	subject := encryption.NewServiceBranchKeySubject(id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"))
	createKeyCalled := false

	ks := &KeyStore{
		dynamoDBTimeout: 1 * time.Millisecond,
		getActiveBranchKeyFn: func(ctx context.Context, _ keystoretypes.GetActiveBranchKeyInput) (*keystoretypes.GetActiveBranchKeyOutput, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		createKeyFn: func(_ context.Context, _ keystoretypes.CreateKeyInput) (*keystoretypes.CreateKeyOutput, error) {
			createKeyCalled = true
			return nil, errors.New("create key should not be called after GetActiveBranchKey timeout")
		},
	}

	_, err := ks.CreateBranchKey(context.Background(), subject)
	require.Error(t, err)
	assert.False(t, createKeyCalled, "CreateKey should not run after GetActiveBranchKey times out")
	assert.True(t, isKEKUnavailableError(err), "expected KEKUnavailable, got %v (type %T)", err, err)
	assert.Contains(t, err.Error(), "getting active branch key")
	assert.Contains(t, err.Error(), "dynamodb_timeout=1ms")
}

func TestCreateBranchKeyQueuedCancellationSkipsSDK(t *testing.T) {
	subject := encryption.NewServiceBranchKeySubject(id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"))
	for _, withLookup := range []bool{true, false} {
		name := "create_key"
		if withLookup {
			name = "get_active_branch_key"
		}
		t.Run(name, func(t *testing.T) {
			require.NoError(t, lockSDK(context.Background()))
			held := true
			defer func() {
				if held {
					unlockSDK()
				}
			}()

			var sdkCalled atomic.Bool
			ks := &KeyStore{createKeyFn: func(context.Context, keystoretypes.CreateKeyInput) (*keystoretypes.CreateKeyOutput, error) {
				sdkCalled.Store(true)
				return nil, errors.New("CreateKey entered while gate held")
			}}
			if withLookup {
				ks.getActiveBranchKeyFn = func(context.Context, keystoretypes.GetActiveBranchKeyInput) (*keystoretypes.GetActiveBranchKeyOutput, error) {
					sdkCalled.Store(true)
					return nil, errors.New("GetActiveBranchKey entered while gate held")
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			watched := &waitingSDKContext{Context: ctx, waiting: make(chan struct{})}
			finished := make(chan error, 1)
			go func() {
				_, err := ks.CreateBranchKey(watched, subject)
				finished <- err
			}()
			select {
			case <-watched.waiting:
			case <-time.After(2 * time.Second):
				t.Fatal("branch-key operation did not reach the SDK gate")
			}
			cancel()
			select {
			case err := <-finished:
				require.ErrorIs(t, err, context.Canceled)
				assert.True(t, isKEKUnavailableError(err))
				assert.False(t, sdkCalled.Load(), "cancelled call entered the SDK")
			case <-time.After(2 * time.Second):
				t.Fatal("queued branch-key operation did not return before gate release")
			}
			unlockSDK()
			held = false
		})
	}
}
