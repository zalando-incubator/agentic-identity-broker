package helpers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/encoding/gzip" // Accept the production exporter's gzip option.
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const LedgerOTLPScope = "agentic-identity-broker.ledger"

// OTLPLogRecord retains resource and scope attributes as well as the log itself,
// so credential assertions cannot accidentally inspect only the log body.
type OTLPLogRecord struct {
	Resource          *resourcev1.Resource
	ResourceSchemaURL string
	Scope             *commonv1.InstrumentationScope
	ScopeSchemaURL    string
	Record            *logsv1.LogRecord
	ExportStartedAt   time.Time
	CapturedAt        time.Time
}

// OTLPReceiver captures real gRPC exports on an isolated loopback listener.
// Register Close with the scenario's cleanup before starting broker telemetry.
type OTLPReceiver struct {
	endpoint     string
	server       *grpc.Server
	serveDone    chan struct{}
	shutdownDone chan struct{}
	closeOnce    sync.Once

	mu             sync.Mutex
	changed        chan struct{}
	records        []OTLPLogRecord
	ledgerAttempts int
	ledgerPauses   int
	unavailable    bool
	closed         bool
	serveErr       error
}

func NewOTLPReceiver() (*OTLPReceiver, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for OTLP logs: %w", err)
	}
	receiver := &OTLPReceiver{
		endpoint:     listener.Addr().String(),
		server:       grpc.NewServer(),
		serveDone:    make(chan struct{}),
		shutdownDone: make(chan struct{}),
		changed:      make(chan struct{}),
	}
	collectorlogsv1.RegisterLogsServiceServer(receiver.server, &otlpLogsService{receiver: receiver})
	go func() {
		err := receiver.server.Serve(listener)
		receiver.mu.Lock()
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			receiver.serveErr = err
		}
		receiver.notifyLocked()
		receiver.mu.Unlock()
		close(receiver.serveDone)
	}()
	return receiver, nil
}

func (r *OTLPReceiver) Endpoint() string { return r.endpoint }

// Close cancels blocked exports and waits at most five seconds for shutdown.
// Repeated and concurrent calls are safe.
func (r *OTLPReceiver) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.notifyLocked()
		r.mu.Unlock()
		go func() {
			r.server.Stop()
			<-r.serveDone
			close(r.shutdownDone)
		}()
	})
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-r.shutdownDone:
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.serveErr
	case <-timer.C:
		return errors.New("OTLP receiver shutdown timed out")
	}
}

func (r *OTLPReceiver) Records() []OTLPLogRecord { return r.snapshot(false) }

func (r *OTLPReceiver) LedgerRecords() []OTLPLogRecord { return r.snapshot(true) }

func (r *OTLPReceiver) snapshot(ledgerOnly bool) []OTLPLogRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]OTLPLogRecord, 0, len(r.records))
	for _, record := range r.records {
		if ledgerOnly && record.Scope.GetName() != LedgerOTLPScope {
			continue
		}
		copy := record
		if record.Resource != nil {
			copy.Resource = proto.Clone(record.Resource).(*resourcev1.Resource)
		}
		if record.Scope != nil {
			copy.Scope = proto.Clone(record.Scope).(*commonv1.InstrumentationScope)
		}
		copy.Record = proto.Clone(record.Record).(*logsv1.LogRecord)
		result = append(result, copy)
	}
	return result
}

// SetUnavailable makes all log exports fail with gRPC Unavailable, including
// exports currently blocked by PauseLedger. Rejected records are not captured.
func (r *OTLPReceiver) SetUnavailable(unavailable bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unavailable = unavailable
	r.notifyLocked()
}

// PauseLedger blocks capture and acknowledgement of requests containing ledger
// records. Other scopes remain usable. Each pause has an idempotent release;
// overlapping pauses must all be released. Client cancellation remains effective.
func (r *OTLPReceiver) PauseLedger() func() {
	r.mu.Lock()
	r.ledgerPauses++
	r.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.ledgerPauses--
			r.notifyLocked()
		})
	}
}

func (r *OTLPReceiver) LedgerAttempts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ledgerAttempts
}

// Attempts include successful, unavailable, and paused calls, counted on entry.
func (r *OTLPReceiver) WaitForLedgerAttempts(ctx context.Context, minimum int) error {
	if minimum < 0 {
		return errors.New("OTLP attempt minimum must not be negative")
	}
	for {
		r.mu.Lock()
		count := r.ledgerAttempts
		changed, closed, serveErr := r.changed, r.closed, r.serveErr
		r.mu.Unlock()
		if count >= minimum {
			return nil
		}
		if serveErr != nil {
			return fmt.Errorf("OTLP receiver stopped: %w", serveErr)
		}
		if closed {
			return net.ErrClosed
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (r *OTLPReceiver) notifyLocked() {
	close(r.changed)
	r.changed = make(chan struct{})
}

type otlpLogsService struct {
	collectorlogsv1.UnimplementedLogsServiceServer
	receiver *OTLPReceiver
}

func (s *otlpLogsService) Export(ctx context.Context, request *collectorlogsv1.ExportLogsServiceRequest) (*collectorlogsv1.ExportLogsServiceResponse, error) {
	exportStartedAt := time.Now().UTC()
	// Bound a paused call even if its client supplied no deadline.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ledger := false
	for _, resource := range request.GetResourceLogs() {
		for _, scope := range resource.GetScopeLogs() {
			if scope.GetScope().GetName() == LedgerOTLPScope && len(scope.GetLogRecords()) > 0 {
				ledger = true
			}
		}
	}
	r := s.receiver
	r.mu.Lock()
	if ledger {
		r.ledgerAttempts++
	}
	r.notifyLocked()
	for {
		if err := ctx.Err(); err != nil {
			r.mu.Unlock()
			return nil, status.FromContextError(err).Err()
		}
		if r.closed || r.unavailable {
			r.mu.Unlock()
			return nil, status.Error(codes.Unavailable, "OTLP receiver unavailable")
		}
		if !ledger || r.ledgerPauses == 0 {
			break
		}
		changed := r.changed
		r.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-changed:
		}
		r.mu.Lock()
	}
	// Own the protobuf graph; snapshots clone it again before exposing it.
	captured := proto.Clone(request).(*collectorlogsv1.ExportLogsServiceRequest)
	capturedAt := time.Now().UTC()
	for _, resource := range captured.GetResourceLogs() {
		for _, scope := range resource.GetScopeLogs() {
			for _, record := range scope.GetLogRecords() {
				if record == nil {
					continue
				}
				r.records = append(r.records, OTLPLogRecord{
					Resource:          resource.GetResource(),
					ResourceSchemaURL: resource.GetSchemaUrl(),
					Scope:             scope.GetScope(),
					ScopeSchemaURL:    scope.GetSchemaUrl(),
					Record:            record,
					ExportStartedAt:   exportStartedAt,
					CapturedAt:        capturedAt,
				})
			}
		}
	}
	r.mu.Unlock()
	return &collectorlogsv1.ExportLogsServiceResponse{}, nil
}
