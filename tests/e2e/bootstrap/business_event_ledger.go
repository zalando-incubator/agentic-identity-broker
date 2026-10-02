package bootstrap

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"sync"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jmoiron/sqlx"
	"github.com/onsi/ginkgo/v2"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type LedgerBackend string

const (
	LedgerMemory   LedgerBackend = "memory"
	LedgerPostgres LedgerBackend = "postgres"
)

type LedgerHarness struct {
	App           *app.App
	Admin         *TestServer
	EndUser       *TestServer
	Storage       *storageadapter.Adapter
	OwnerDB       *sqlx.DB
	ReaderDB      *sqlx.DB
	ErasureDB     *sqlx.DB
	ConnectionURL string
	closeOnce     sync.Once
	closeStorage  func() error
}

// NewLedgerHarness registers cleanup before returning an isolated backend iteration.
func NewLedgerHarness(backend LedgerBackend, config *ports.Config, logger *slog.Logger, schemas fs.FS, faults *LedgerStorageFaults, tracerProviders ...*sdktrace.TracerProvider) (*LedgerHarness, error) {
	if config == nil || logger == nil {
		return nil, errors.New("ledger bootstrap requires configuration and logger")
	}
	h := &LedgerHarness{}
	switch backend {
	case LedgerMemory:
		adapter, err := NewStorageFactory(logger).NewTestStorage()
		if err != nil {
			return nil, err
		}
		h.Storage = adapter
		h.closeStorage = func() error { return adapter.Close(context.Background()) }
	case LedgerPostgres:
		if err := openLedgerPostgres(h); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unknown ledger test backend")
	}
	ginkgo.DeferCleanup(h.Close)
	config.Storage.Backend = string(backend)
	config.Storage.Postgres.ConnectionURL = h.ConnectionURL
	if err := h.start(config, logger, schemas, faults, tracerProviders...); err != nil {
		_ = h.Close()
		return nil, err
	}
	return h, nil
}

func (h *LedgerHarness) start(config *ports.Config, logger *slog.Logger, schemas fs.FS, faults *LedgerStorageFaults, tracerProviders ...*sdktrace.TracerProvider) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer func() {
		if h.EndUser == nil {
			_ = listener.Close()
		}
	}()
	config.Server.EndUser.PublicURL = "http://" + listener.Addr().String()
	var provider ports.StorageProvider = h.Storage
	if faults != nil {
		provider = &ledgerFaultStorage{StorageProvider: provider, faults: faults}
	}
	builder := app.NewBuilder().WithConfig(config).WithStorage(provider).WithLogger(logger).WithStaticWebResourcesPath(webDistPath())
	if schemas != nil {
		builder.WithBusinessEventSchemas(schemas)
	}
	if len(tracerProviders) != 0 {
		builder.WithTracerProvider(tracerProviders[0])
	}
	application, err := builder.Build()
	if err != nil {
		return err
	}
	h.App = application
	h.Admin, err = NewAdminTestServer(application, logger)
	if err != nil {
		return err
	}
	h.EndUser, err = NewTestServerV2(application, logger, WithServerType(ServerTypeEndUser), withLedgerListener(listener))
	return err
}

func (h *LedgerHarness) Close() error {
	var result error
	h.closeOnce.Do(func() {
		// Both listeners drain before the shared application stops its workers.
		if h.Admin != nil {
			h.Admin.CloseHTTP()
		}
		if h.EndUser != nil {
			h.EndUser.CloseHTTP()
		}
		if h.App != nil && h.App.Shutdown != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			result = errors.Join(result, h.App.Shutdown(ctx))
			cancel()
		}
		if h.closeStorage != nil {
			result = errors.Join(result, h.closeStorage())
		}
	})
	return result
}

// LedgerStorageFaults intercepts production ports, never an HTTP debug endpoint.
// CommitReached is signalled only after the underlying owner commits.
// CommitRelease holds completion before domain logs and response construction.
type LedgerStorageFaults struct {
	mu                  sync.Mutex
	appendFailureType   string
	appendFailure       error
	commitReached       chan struct{}
	commitRelease       <-chan struct{}
	beforeCommitReached chan struct{}
	beforeCommitRelease <-chan struct{}
}

func (f *LedgerStorageFaults) FailAppend(eventType string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appendFailureType, f.appendFailure = eventType, err
}

func (f *LedgerStorageFaults) HoldNextCommit(release <-chan struct{}) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitReached = make(chan struct{})
	f.commitRelease = release
	return f.commitReached
}

func (f *LedgerStorageFaults) HoldBeforeNextCommit(release <-chan struct{}) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beforeCommitReached = make(chan struct{})
	f.beforeCommitRelease = release
	return f.beforeCommitReached
}

type ledgerFaultStorage struct {
	ports.StorageProvider
	faults *LedgerStorageFaults
}

type ledgerTransactionDepthKey struct{}

func (s *ledgerFaultStorage) BeginTX(ctx context.Context) (context.Context, error) {
	depth, _ := ctx.Value(ledgerTransactionDepthKey{}).(int)
	tx, err := s.StorageProvider.BeginTX(ctx)
	if err != nil {
		return nil, err
	}
	return context.WithValue(tx, ledgerTransactionDepthKey{}, depth+1), nil
}

func (s *ledgerFaultStorage) BusinessEvents() ports.BusinessEventRepository {
	return &ledgerFaultEvents{BusinessEventRepository: s.StorageProvider.BusinessEvents(), faults: s.faults}
}

func (s *ledgerFaultStorage) Commit(ctx context.Context) error {
	if depth, _ := ctx.Value(ledgerTransactionDepthKey{}).(int); depth == 1 {
		s.faults.mu.Lock()
		reached, release := s.faults.beforeCommitReached, s.faults.beforeCommitRelease
		s.faults.beforeCommitReached, s.faults.beforeCommitRelease = nil, nil
		s.faults.mu.Unlock()
		if reached != nil {
			close(reached)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	if err := s.StorageProvider.Commit(ctx); err != nil {
		return err
	}
	if depth, _ := ctx.Value(ledgerTransactionDepthKey{}).(int); depth != 1 {
		return nil
	}
	s.faults.mu.Lock()
	reached, release := s.faults.commitReached, s.faults.commitRelease
	s.faults.commitReached, s.faults.commitRelease = nil, nil
	s.faults.mu.Unlock()
	if reached == nil {
		return nil
	}
	close(reached)
	select {
	case <-release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type ledgerFaultEvents struct {
	ports.BusinessEventRepository
	faults *LedgerStorageFaults
}

func (r *ledgerFaultEvents) Append(ctx context.Context, event *model.BusinessEvent, queueDelivery bool) error {
	r.faults.mu.Lock()
	eventType, failure := r.faults.appendFailureType, r.faults.appendFailure
	r.faults.mu.Unlock()
	if failure != nil && (eventType == "" || event != nil && event.Type == eventType) {
		return failure
	}
	return r.BusinessEventRepository.Append(ctx, event, queueDelivery)
}
