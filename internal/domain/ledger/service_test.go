package ledger

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type ledgerServiceEvents struct {
	appendFn func(context.Context, *model.BusinessEvent, bool) error
	queryFn  func(context.Context, model.BusinessEventQuery) ([]*model.BusinessEvent, error)
}

func (e ledgerServiceEvents) Append(ctx context.Context, event *model.BusinessEvent, queueDelivery bool) error {
	if e.appendFn == nil {
		panic("unexpected business event append")
	}
	return e.appendFn(ctx, event, queueDelivery)
}

func (e ledgerServiceEvents) Query(ctx context.Context, query model.BusinessEventQuery) ([]*model.BusinessEvent, error) {
	if e.queryFn == nil {
		panic("unexpected business event query")
	}
	return e.queryFn(ctx, query)
}

func (ledgerServiceEvents) Get(context.Context, model.BusinessEventKey) (*model.BusinessEvent, error) {
	panic("unexpected business event get")
}

type ledgerServiceTransactions struct {
	beginFn    func(context.Context) (context.Context, error)
	commitFn   func(context.Context) error
	rollbackFn func(context.Context) error
}

func (m ledgerServiceTransactions) BeginTX(ctx context.Context) (context.Context, error) {
	if m.beginFn == nil {
		panic("unexpected transaction begin")
	}
	return m.beginFn(ctx)
}

func (m ledgerServiceTransactions) Commit(ctx context.Context) error {
	if m.commitFn == nil {
		panic("unexpected transaction commit")
	}
	return m.commitFn(ctx)
}

func (m ledgerServiceTransactions) Rollback(ctx context.Context) error {
	if m.rollbackFn == nil {
		panic("unexpected transaction rollback")
	}
	return m.rollbackFn(ctx)
}

type ledgerServiceScopeKey struct{}

type ledgerServiceScope struct {
	committed  bool
	rolledBack bool
	callbacks  []func()
}

type ledgerServiceEffects struct{ scope *ledgerServiceScope }

func (e ledgerServiceEffects) AfterCommit(fn func()) error {
	e.scope.callbacks = append(e.scope.callbacks, fn)
	return nil
}

func ledgerServiceRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	return registry
}

func TestServiceWithTransactionPreservesFailureAndReportsRollbackFailure(t *testing.T) {
	for _, failureAt := range []string{"work", "commit"} {
		for _, rollbackFails := range []bool{false, true} {
			name := failureAt + "/clean rollback"
			if rollbackFails {
				name = failureAt + "/failed rollback"
			}
			t.Run(name, func(t *testing.T) {
				failure := errors.New("transaction failed")
				rollbackFailure := errors.New("rollback failed")
				scope := &ledgerServiceScope{}
				rollbacks := 0
				service := NewService(nil, nil, nil, ledgerServiceTransactions{
					beginFn: func(ctx context.Context) (context.Context, error) {
						return context.WithValue(ctx, ledgerServiceScopeKey{}, scope), nil
					},
					commitFn: func(ctx context.Context) error {
						require.Equal(t, "commit", failureAt)
						require.Same(t, scope, ctx.Value(ledgerServiceScopeKey{}))
						return failure
					},
					rollbackFn: func(ctx context.Context) error {
						require.Same(t, scope, ctx.Value(ledgerServiceScopeKey{}))
						rollbacks++
						if rollbackFails {
							return rollbackFailure
						}
						return nil
					},
				}, false)
				err := service.WithTransaction(context.Background(), ports.StorageTransactionHints{}, func(ctx context.Context) error {
					require.Same(t, scope, ctx.Value(ledgerServiceScopeKey{}))
					if failureAt == "work" {
						return failure
					}
					return nil
				})
				require.Equal(t, 1, rollbacks)
				require.ErrorIs(t, err, failure)
				if rollbackFails {
					require.ErrorIs(t, err, rollbackFailure)
					require.NotSame(t, failure, err, "an uncertain rollback must not appear to be only the work failure")
				} else {
					require.Same(t, failure, err, "a clean rollback preserves the exact work or commit error")
				}
			})
		}
	}
}

func TestServiceRecordRejectsInvalidEventsBeforeStartingWork(t *testing.T) {
	registry := ledgerServiceRegistry(t)
	const canary = "credential-bearing-input-canary"
	for _, tc := range []struct {
		name  string
		event *model.BusinessEvent
	}{
		{"nil event", nil},
		{"unknown type", func() *model.BusinessEvent {
			event := validLedgerEvent()
			event.Type = "agentic-identity-broker." + canary
			return event
		}()},
		{"unregistered payload", func() *model.BusinessEvent {
			event := validLedgerEvent()
			event.Data["access_token"] = canary
			return event
		}()},
		{"untrusted reason", func() *model.BusinessEvent {
			event := validLedgerEvent()
			event.ReasonAdmin = canary
			return event
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := NewService(registry, ledgerServiceEvents{
				appendFn: func(context.Context, *model.BusinessEvent, bool) error {
					t.Fatal("invalid facts must never be appended")
					return nil
				},
			}, nil, ledgerServiceTransactions{
				beginFn: func(context.Context) (context.Context, error) {
					t.Fatal("invalid facts must never start a transaction")
					return nil, nil
				},
			}, true)
			err := service.Record(context.Background(), tc.event)
			require.Error(t, err)
			require.NotContains(t, err.Error(), canary)
		})
	}
}

func TestServiceRecordDoesNotAppendWhenOwnerCannotBegin(t *testing.T) {
	registry := ledgerServiceRegistry(t)
	event := validLedgerEvent()
	beginFailure := errors.New("transaction unavailable")
	service := NewService(registry, ledgerServiceEvents{
		appendFn: func(context.Context, *model.BusinessEvent, bool) error {
			t.Fatal("a failed owner must not append an event outside its transaction")
			return nil
		},
	}, nil, ledgerServiceTransactions{
		beginFn: func(context.Context) (context.Context, error) { return nil, beginFailure },
	}, false)
	require.ErrorIs(t, service.Record(context.Background(), event), beginFailure)
}

func TestServiceRecordAppendFailureRollsBackWithoutLeakingEventValues(t *testing.T) {
	registry := ledgerServiceRegistry(t)
	event := validLedgerEvent()
	const canary = "credential-bearing-input-canary"
	principal := event.Subject
	*principal = id.NewPrincipal("principal-" + canary)
	require.NoError(t, NewService(registry, nil, nil, nil, false).Validate(event))

	appendFailure := errors.New("ledger append unavailable")
	scope := &ledgerServiceScope{}
	begins, appends := 0, 0
	service := NewService(registry, ledgerServiceEvents{
		appendFn: func(ctx context.Context, got *model.BusinessEvent, queueDelivery bool) error {
			appends++
			require.Same(t, scope, ctx.Value(ledgerServiceScopeKey{}))
			require.Equal(t, event.ID, got.ID)
			require.Equal(t, event.Type, got.Type)
			require.False(t, queueDelivery, "copy disablement must not disable persistence")
			return appendFailure
		},
	}, nil, ledgerServiceTransactions{
		beginFn: func(ctx context.Context) (context.Context, error) {
			begins++
			return context.WithValue(ctx, ledgerServiceScopeKey{}, scope), nil
		},
		rollbackFn: func(ctx context.Context) error {
			require.Same(t, scope, ctx.Value(ledgerServiceScopeKey{}))
			scope.rolledBack = true
			return nil
		},
		commitFn: func(context.Context) error {
			t.Fatal("append failure must not commit business work")
			return nil
		},
	}, false)

	err := service.Record(context.Background(), event)
	require.ErrorIs(t, err, appendFailure)
	require.NotContains(t, err.Error(), canary)
	require.Equal(t, 1, begins)
	require.Equal(t, 1, appends, "a failed append must not automatically retry the occurrence")
	require.True(t, scope.rolledBack, "a failed occurrence cannot leave its owner active")
}

func TestServiceRecordReturnsOnlyAfterOwnerCommits(t *testing.T) {
	registry := ledgerServiceRegistry(t)
	event := validLedgerEvent()
	scope := &ledgerServiceScope{}
	commitEntered := make(chan struct{})
	releaseCommit := make(chan struct{})
	persisted := make(chan struct{})
	result := make(chan error, 1)
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseCommit) }) })

	service := NewService(registry, ledgerServiceEvents{
		appendFn: func(ctx context.Context, got *model.BusinessEvent, queueDelivery bool) error {
			if got.ID != event.ID || got.Type != event.Type || !queueDelivery || ctx.Value(ledgerServiceScopeKey{}) != scope {
				return errors.New("append not joined to the owner or copy flag lost")
			}
			effects, ok := ports.StorageTransactionEffectsFromContext(ctx)
			if !ok {
				return errors.New("append has no transaction effects")
			}
			return effects.AfterCommit(func() { close(persisted) })
		},
	}, nil, ledgerServiceTransactions{
		beginFn: func(ctx context.Context) (context.Context, error) {
			ctx = context.WithValue(ctx, ledgerServiceScopeKey{}, scope)
			return ports.WithStorageTransactionEffects(ctx, ledgerServiceEffects{scope}), nil
		},
		commitFn: func(ctx context.Context) error {
			if ctx.Value(ledgerServiceScopeKey{}) != scope {
				return errors.New("wrong transaction committed")
			}
			close(commitEntered)
			<-releaseCommit
			scope.committed = true
			for _, fn := range scope.callbacks {
				fn()
			}
			return nil
		},
		rollbackFn: func(context.Context) error {
			return errors.New("unexpected rollback of valid occurrence")
		},
	}, true)

	go func() { result <- service.Record(context.Background(), event) }()
	select {
	case <-commitEntered:
	case err := <-result:
		t.Fatalf("record returned before owner commit: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("record never reached owner commit")
	}
	select {
	case err := <-result:
		t.Fatalf("record returned before owner commit: %v", err)
	case <-persisted:
		t.Fatal("post-commit effect ran before the commit boundary")
	default:
	}
	releaseOnce.Do(func() { close(releaseCommit) })
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("record did not finish after owner commit")
	}
	require.True(t, scope.committed)
	select {
	case <-persisted:
	default:
		t.Fatal("owner commit did not publish the occurrence")
	}
}

func TestServiceRecordSpansExcludePoolCheckoutAndCommit(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	count := func(name string) int {
		n := 0
		for _, span := range recorder.Ended() {
			if span.Name() == name {
				n++
			}
		}
		return n
	}
	service := NewService(ledgerServiceRegistry(t), ledgerServiceEvents{
		appendFn: func(context.Context, *model.BusinessEvent, bool) error { return nil },
	}, nil, ledgerServiceTransactions{
		beginFn: func(ctx context.Context) (context.Context, error) {
			require.Equal(t, 1, count("ledger.record.validate"), "preflight validation must complete before checkout")
			require.Zero(t, count("ledger.record"), "checkout must not count toward append")
			return ctx, nil
		},
		commitFn: func(context.Context) error {
			require.Equal(t, 1, count("ledger.record"), "append must finish before physical commit")
			require.Zero(t, count("ledger.transaction"), "transaction must include physical commit")
			return nil
		},
	}, false)
	require.NoError(t, service.Record(context.Background(), validLedgerEvent()))
	require.Equal(t, 1, count("ledger.transaction"))
}

func TestServiceRecordCommitErrorCannotReportSuccess(t *testing.T) {
	registry := ledgerServiceRegistry(t)
	event := validLedgerEvent()
	principal := id.NewPrincipal("principal-credential-bearing-input-canary")
	event.Subject = &principal
	scope := &ledgerServiceScope{}
	commitFailure := errors.New("commit unavailable")
	begins, appends, commits := 0, 0, 0
	service := NewService(registry, ledgerServiceEvents{
		appendFn: func(ctx context.Context, got *model.BusinessEvent, _ bool) error {
			appends++
			require.Equal(t, event.ID, got.ID)
			require.Equal(t, event.Type, got.Type)
			require.Same(t, scope, ctx.Value(ledgerServiceScopeKey{}))
			return nil
		},
	}, nil, ledgerServiceTransactions{
		beginFn: func(ctx context.Context) (context.Context, error) {
			begins++
			return context.WithValue(ctx, ledgerServiceScopeKey{}, scope), nil
		},
		commitFn: func(context.Context) error {
			commits++
			return commitFailure
		},
		rollbackFn: func(context.Context) error {
			scope.rolledBack = true
			return nil
		},
	}, true)

	err := service.Record(context.Background(), event)
	require.ErrorIs(t, err, commitFailure)
	require.False(t, scope.committed)
	require.NotContains(t, err.Error(), "credential-bearing-input-canary")
	require.Equal(t, 1, begins, "commit failure must not retry the occurrence")
	require.Equal(t, 1, appends, "commit failure must not append a second occurrence")
	require.Equal(t, 1, commits, "a failed commit must not be retried automatically")
}

func TestServiceRecordInExistingOwnerDoesNotPublishEarly(t *testing.T) {
	registry := ledgerServiceRegistry(t)
	event := validLedgerEvent()
	scope := &ledgerServiceScope{}
	root := context.Background()
	physicalCommits := 0
	published := false
	transactions := ledgerServiceTransactions{
		beginFn: func(ctx context.Context) (context.Context, error) {
			if ctx.Value(ledgerServiceScopeKey{}) == scope {
				return context.WithValue(ctx, ledgerServiceJoinedKey{}, true), nil
			}
			return ports.WithStorageTransactionEffects(context.WithValue(ctx, ledgerServiceScopeKey{}, scope), ledgerServiceEffects{scope}), nil
		},
		commitFn: func(ctx context.Context) error {
			if ctx.Value(ledgerServiceJoinedKey{}) == true {
				return nil
			}
			physicalCommits++
			scope.committed = true
			for _, fn := range scope.callbacks {
				fn()
			}
			return nil
		},
		rollbackFn: func(context.Context) error { return errors.New("unexpected rollback") },
	}
	service := NewService(registry, ledgerServiceEvents{
		appendFn: func(ctx context.Context, got *model.BusinessEvent, _ bool) error {
			require.Equal(t, event.ID, got.ID)
			require.Equal(t, event.Type, got.Type)
			require.Same(t, scope, ctx.Value(ledgerServiceScopeKey{}))
			effects, ok := ports.StorageTransactionEffectsFromContext(ctx)
			require.True(t, ok)
			return effects.AfterCommit(func() { published = true })
		},
	}, nil, transactions, false)
	ownerCtx, err := transactions.BeginTX(root)
	require.NoError(t, err)
	require.NoError(t, service.Record(ownerCtx, event))
	require.Equal(t, 0, physicalCommits, "joined scope cannot commit another caller's business mutation")
	require.False(t, published, "joined scope cannot announce a still-rollbackable occurrence")
	require.NoError(t, transactions.Commit(ownerCtx))
	require.Equal(t, 1, physicalCommits)
	require.True(t, published)
}

type ledgerServiceJoinedKey struct{}

func TestServiceRecordIndependentOutcomesUseFreshOwnerAfterRollback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typeName string
		outcome  model.BusinessEventOutcome
		reason   string
	}{
		{"failure", "token-request-failed", model.BusinessEventFailure, "A token request fails without a more specific exchange-denial classification."},
		{"denial", "token-exchange-denied", model.BusinessEventDenied, "An exchange is refused by authentication or authorization controls."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := ledgerServiceRegistry(t)
			event := validLedgerEvent()
			event.Type = model.BusinessEventTypePrefix + tc.typeName
			event.AgentID, event.GrantID = id.AgentID{}, id.GrantID{}
			event.Outcome, event.ReasonUser, event.ReasonAdmin = tc.outcome, tc.reason, tc.reason
			event.Data = map[string]any{"reason_code": "authorization_failed"}
			if tc.typeName == "token-exchange-denied" {
				gateway := "verified-gateway-client"
				event.Actor = model.BusinessEventActor{Kind: "gateway", ID: &gateway}
			}
			require.NoError(t, NewService(registry, nil, nil, nil, false).Validate(event))

			root := context.Background()
			owner, independent := &ledgerServiceScope{}, &ledgerServiceScope{}
			begins, appends, commits := 0, 0, 0
			var order []string
			transactions := ledgerServiceTransactions{
				beginFn: func(ctx context.Context) (context.Context, error) {
					if ctx.Value(ledgerServiceScopeKey{}) != nil {
						return nil, errors.New("independent outcome attempted to join a failed owner")
					}
					begins++
					if begins == 1 {
						return context.WithValue(ctx, ledgerServiceScopeKey{}, owner), nil
					}
					if !owner.rolledBack || begins != 2 {
						return nil, errors.New("independent outcome started before rollback or retried")
					}
					order = append(order, "independent begin")
					return context.WithValue(ctx, ledgerServiceScopeKey{}, independent), nil
				},
				rollbackFn: func(ctx context.Context) error {
					require.Same(t, owner, ctx.Value(ledgerServiceScopeKey{}))
					owner.rolledBack = true
					order = append(order, "owner rollback")
					return nil
				},
				commitFn: func(ctx context.Context) error {
					require.Same(t, independent, ctx.Value(ledgerServiceScopeKey{}))
					commits++
					independent.committed = true
					order = append(order, "independent commit")
					return nil
				},
			}
			service := NewService(registry, ledgerServiceEvents{
				appendFn: func(ctx context.Context, got *model.BusinessEvent, _ bool) error {
					appends++
					require.Equal(t, event.ID, got.ID)
					require.Equal(t, event.Type, got.Type)
					require.Same(t, independent, ctx.Value(ledgerServiceScopeKey{}), "terminal outcome cannot append in failed owner")
					require.True(t, owner.rolledBack)
					order = append(order, "independent append")
					return nil
				},
			}, nil, transactions, false)
			ownerCtx, err := transactions.BeginTX(root)
			require.NoError(t, err)
			require.NoError(t, transactions.Rollback(ownerCtx))
			require.NoError(t, service.Record(root, event), "independent terminal outcome must commit after failed mutation ends")
			require.Equal(t, []string{"owner rollback", "independent begin", "independent append", "independent commit"}, order)
			require.Equal(t, 2, begins)
			require.Equal(t, 1, appends)
			require.Equal(t, 1, commits)
			require.True(t, independent.committed)
		})
	}
}
