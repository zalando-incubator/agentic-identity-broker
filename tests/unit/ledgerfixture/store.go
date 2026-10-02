package ledgerfixture

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

type Store struct {
	eventsMu     sync.Mutex
	Events       []*model.BusinessEvent
	AppendError  error
	CommitError  error
	Snapshot     func() func()
	BeforeCommit func()
}

type scopeKey struct{}
type scope struct {
	owner bool
	state *transaction
}
type transaction struct {
	events    []*model.BusinessEvent
	restore   func()
	callbacks []func()
	failed    bool
}

func (s *Store) Recorder(t *testing.T) *ledger.Service {
	t.Helper()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	return ledger.NewService(registry, s, nil, s, false)
}

func NewRecorder() *ledger.Service {
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	if err != nil {
		panic(err)
	}
	store := &Store{}
	return ledger.NewService(registry, store, nil, store, false)
}

func (s *Store) BeginTX(ctx context.Context) (context.Context, error) {
	if parent, ok := ctx.Value(scopeKey{}).(*scope); ok {
		return context.WithValue(ctx, scopeKey{}, &scope{state: parent.state}), nil
	}
	state := &transaction{}
	if s.Snapshot != nil {
		state.restore = s.Snapshot()
	}
	ctx = ports.WithStorageTransactionEffects(ctx, state)
	return context.WithValue(ctx, scopeKey{}, &scope{owner: true, state: state}), nil
}

func (s *Store) Commit(ctx context.Context) error {
	current := ctx.Value(scopeKey{}).(*scope)
	if current.state.failed {
		return errors.New("transaction is rollback-only")
	}
	if !current.owner {
		return nil
	}
	if s.CommitError != nil {
		return s.CommitError
	}
	if s.BeforeCommit != nil {
		s.BeforeCommit()
	}
	s.eventsMu.Lock()
	s.Events = append(s.Events, current.state.events...)
	s.eventsMu.Unlock()
	for _, fn := range current.state.callbacks {
		fn()
	}
	return nil
}

func (s *Store) Rollback(ctx context.Context) error {
	current := ctx.Value(scopeKey{}).(*scope)
	current.state.failed = true
	if current.owner && current.state.restore != nil {
		current.state.restore()
	}
	return nil
}

func (s *Store) Append(ctx context.Context, event *model.BusinessEvent, _ bool) error {
	if s.AppendError != nil {
		return s.AppendError
	}
	current, ok := ctx.Value(scopeKey{}).(*scope)
	if !ok {
		return errors.New("event append outside transaction")
	}
	copy := *event
	copy.PrepareRecording(time.Now().UTC())
	current.state.events = append(current.state.events, &copy)
	return nil
}

func (*Store) Query(context.Context, model.BusinessEventQuery) ([]*model.BusinessEvent, error) {
	panic("unexpected producer query")
}
func (*Store) Get(context.Context, model.BusinessEventKey) (*model.BusinessEvent, error) {
	panic("unexpected producer get")
}
func (s *transaction) AfterCommit(fn func()) error {
	s.callbacks = append(s.callbacks, fn)
	return nil
}
