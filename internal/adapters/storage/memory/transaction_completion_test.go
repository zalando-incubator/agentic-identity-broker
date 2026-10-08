package memory

import (
	"context"
	"encoding/json"
	"io/fs"
	"sync"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

type admittedOperationContext struct {
	context.Context
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (ctx *admittedOperationContext) Done() <-chan struct{} {
	ctx.once.Do(func() {
		close(ctx.entered)
		<-ctx.resume
	})
	return ctx.Context.Done()
}

func TestTransactionCompletionWaitsForAdmittedWrite(t *testing.T) {
	for _, commit := range []bool{false, true} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			manager := NewTransactionManager()
			repo := NewAdapter(manager)
			owner, err := manager.BeginTX(context.Background())
			require.NoError(t, err)
			ctx := &admittedOperationContext{Context: owner, entered: make(chan struct{}), resume: make(chan struct{})}
			var resumeOnce sync.Once
			resume := func() { resumeOnce.Do(func() { close(ctx.resume) }) }
			defer resume()
			user := &ports.User{ID: id.NewUserID(), Email: "admitted@example.com"}
			writeDone := make(chan error, 1)
			go func() { writeDone <- repo.CreateUser(ctx, user) }()
			<-ctx.entered

			effectDone := make(chan error, 1)
			effects, ok := ports.StorageTransactionEffectsFromContext(owner)
			require.True(t, ok)
			require.NoError(t, effects.AfterCommit(func() {
				_, readErr := repo.GetUser(context.Background(), user.ID)
				effectDone <- readErr
			}))
			completion := make(chan error, 1)
			go func() {
				if commit {
					completion <- manager.Commit(owner)
				} else {
					completion <- manager.Rollback(owner)
				}
			}()
			scope, _ := memoryTransaction(owner)
			require.Eventually(t, func() bool {
				scope.owner.mu.Lock()
				defer scope.owner.mu.Unlock()
				return scope.completed
			}, time.Second, time.Millisecond)
			select {
			case err := <-completion:
				t.Fatalf("completion returned before the admitted write finished: %v", err)
			default:
			}
			_, err = repo.GetUser(owner, user.ID)
			require.Error(t, err, "completion must close admission")
			resume()
			require.NoError(t, <-writeDone)
			require.NoError(t, <-completion)
			stored, err := repo.GetUser(context.Background(), user.ID)
			if commit {
				require.NoError(t, err)
				require.Equal(t, user.ID, stored.ID)
				require.NoError(t, <-effectDone)
			} else {
				require.True(t, ports.IsNotFoundErr(err))
				select {
				case <-effectDone:
					t.Fatal("rollback released a commit effect")
				default:
				}
			}
		})
	}
}

func TestSessionDeleteWaitsForUncommittedAbsence(t *testing.T) {
	manager := NewTransactionManager()
	repo := NewInMemoryUserSessionRepository(manager)
	ctx := context.Background()
	session := refreshTestSession(id.Principal("delete-subject"), id.NewServiceID())
	require.NoError(t, repo.Create(ctx, session))
	owner, err := manager.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = manager.Rollback(owner) }()
	require.NoError(t, repo.Delete(owner, session.ID))

	blocked, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, repo.Delete(blocked, session.ID), context.DeadlineExceeded,
		"independent deletion must not report success for an uncommitted absence")
	require.NoError(t, manager.Rollback(owner))
	require.NoError(t, repo.Delete(ctx, session.ID))
	_, err = repo.Get(ctx, session.ID)
	require.True(t, ports.IsNotFoundErr(err))
}

type pausedEventValidator struct {
	ports.BusinessEventValidator
	entered, resume chan struct{}
}

func (validator *pausedEventValidator) Validate(event *model.BusinessEvent) (map[string]any, error) {
	close(validator.entered)
	<-validator.resume
	return validator.BusinessEventValidator.Validate(event)
}

func TestRollbackWaitsForAdmittedEventAppend(t *testing.T) {
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	manager := NewTransactionManager()
	repo := NewBusinessEventRepository(manager, registry)
	validator := &pausedEventValidator{BusinessEventValidator: registry, entered: make(chan struct{}), resume: make(chan struct{})}
	repo.ConfigureBusinessEventValidation(validator)
	contents, err := fs.ReadFile(eventschemas.Schemas, "examples.json")
	require.NoError(t, err)
	var events []model.BusinessEvent
	require.NoError(t, json.Unmarshal(contents, &events))
	event := events[0]
	event.ID = id.NewBusinessEventID()
	owner, err := manager.BeginTX(context.Background())
	require.NoError(t, err)
	appendDone := make(chan error, 1)
	go func() { appendDone <- repo.Append(owner, &event, true) }()
	<-validator.entered
	var resumeOnce sync.Once
	resume := func() { resumeOnce.Do(func() { close(validator.resume) }) }
	defer resume()
	rollbackDone := make(chan error, 1)
	go func() { rollbackDone <- manager.Rollback(owner) }()
	scope, _ := memoryTransaction(owner)
	require.Eventually(t, func() bool {
		scope.owner.mu.Lock()
		defer scope.owner.mu.Unlock()
		return scope.completed
	}, time.Second, time.Millisecond)
	select {
	case err := <-rollbackDone:
		t.Fatalf("rollback returned before admitted append finished: %v", err)
	default:
	}
	resume()
	require.Error(t, <-appendDone, "a joined append cannot commit after owner rollback")
	require.NoError(t, <-rollbackDone)
	key := model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
	_, err = repo.Get(context.Background(), key)
	require.True(t, ports.IsNotFoundErr(err))
	pending, err := repo.ListDue(context.Background(), 100)
	require.NoError(t, err)
	require.NotContains(t, pending, key)
}
