// Package opa is the driven adapter that owns the one shared impersonation OPA engine. It builds
// immutable, broker-admitted revisions from managed bundles or a local bundle directory and
// evaluates rule decisions against them. It imports no other adapter and nothing from ExtProc.
package opa

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/v1/bundle"
	"github.com/open-policy-agent/opa/v1/logging"
	"github.com/open-policy-agent/opa/v1/plugins"
	bundleplugin "github.com/open-policy-agent/opa/v1/plugins/bundle"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/sdk"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/open-policy-agent/opa/v1/util"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const defaultEvaluationTimeout = 100 * time.Millisecond

// source is the shared engine. The active revision is swapped atomically; in-flight requests keep
// the revision they read.
type source struct {
	decisions []ports.ImpersonationPolicyDecisionRef
	admit     ports.ImpersonationPolicyAdmission
	telemetry *telemetry
	instance  string
	sequence  atomic.Uint64
	started   time.Time

	current    atomic.Pointer[revision]
	configured []string

	admitted     chan struct{}
	admittedOnce sync.Once
	sdkReady     chan struct{}
	final        chan struct{}
	localMode    bool

	mu            sync.Mutex
	lastRejection *rejection
	lastRequest   map[string]time.Time
	lastDownload  map[string]time.Time

	opa      *sdk.OPA
	stopOnce sync.Once
}

var _ ports.ImpersonationPolicySource = (*source)(nil)

// NewImpersonationPolicySource starts the shared engine without waiting for readiness. Managed
// mode reads the bootstrap and starts the OPA SDK; local mode reads the bundle directory once.
func NewImpersonationPolicySource(cfg ports.ImpersonationOPAEngineConfig, decisions []ports.ImpersonationPolicyDecisionRef,
	admit ports.ImpersonationPolicyAdmission, logger *slog.Logger,
) (ports.ImpersonationPolicySource, error) {
	if logger == nil {
		return nil, errors.New("impersonation policy logger is required")
	}
	if admit == nil {
		return nil, errors.New("impersonation policy admission is required")
	}
	s := &source{
		decisions:    append([]ports.ImpersonationPolicyDecisionRef(nil), decisions...),
		admit:        admit,
		telemetry:    newTelemetry(logger),
		instance:     uuid.NewString(),
		started:      time.Now(),
		admitted:     make(chan struct{}),
		sdkReady:     make(chan struct{}),
		final:        make(chan struct{}),
		lastRequest:  map[string]time.Time{},
		lastDownload: map[string]time.Time{},
	}
	s.telemetry.observeRevisionAge(func() (time.Time, bool) {
		if rev := s.current.Load(); rev != nil {
			return rev.admittedAt, true
		}
		return time.Time{}, false
	})
	var err error
	switch {
	case cfg.Policy.ConfigFile != "":
		err = s.startManaged(cfg.Policy.ConfigFile)
	case cfg.Policy.Path != "":
		err = s.loadLocal(cfg.Policy.Path)
	default:
		err = &configError{field: "policy", reason: reasonInvalidDocument}
	}
	if err != nil {
		s.Stop(context.Background())
		return nil, err
	}
	return s, nil
}

func (s *source) startManaged(path string) error {
	boot, err := readBootstrap(path)
	if err != nil {
		return err
	}
	s.configured = boot.bundles
	ctx := context.Background()
	store := inmem.New()
	if err := storage.Txn(ctx, store, storage.WriteParams, func(txn storage.Transaction) error {
		_, err := store.Register(ctx, txn, storage.TriggerConfig{OnCommit: func(ctx context.Context, txn storage.Transaction, event storage.TriggerEvent) {
			s.onCommit(ctx, store, txn, event)
		}})
		return err
	}); err != nil {
		return &configError{field: "policy.config_file", reason: reasonInvalidOPAConfiguration}
	}
	factories := map[string]plugins.Factory{}
	if boot.tokenFile != nil {
		factories[tokenPluginName] = &tokenPluginFactory{cfg: *boot.tokenFile, telemetry: s.telemetry}
	}
	opa, err := sdk.New(ctx, sdk.Options{
		ID:            s.instance,
		Config:        boot.reader(),
		Ready:         s.sdkReady,
		Store:         store,
		V1Compatible:  true,
		Logger:        logging.NewNoOpLogger(),
		ConsoleLogger: logging.NewNoOpLogger(),
		Plugins:       factories,
	})
	if err != nil {
		return &configError{field: "policy.config_file", reason: reasonInvalidOPAConfiguration}
	}
	s.opa = opa
	if plugin, ok := opa.Plugin(bundleplugin.Name).(*bundleplugin.Plugin); ok {
		plugin.Register(s.instance, s.onBundleStatus)
		s.telemetry.observeDownloadAge(s.downloadAges)
	}
	return nil
}

// onCommit builds a candidate from the committing transaction: its modules, data, and activated
// bundles all come from one snapshot. A trigger failure rejects the candidate.
func (s *source) onCommit(ctx context.Context, store storage.Store, txn storage.Transaction, event storage.TriggerEvent) {
	if !event.PolicyChanged() && !event.DataChanged() {
		return
	}
	defer func() {
		if recover() != nil {
			s.reject(&rejection{reason: reasonSnapshotFailed}, "")
		}
	}()
	in, err := readSnapshot(ctx, store, txn)
	if err != nil {
		s.reject(&rejection{reason: reasonSnapshotFailed}, in.label)
		return
	}
	if len(in.activated) == 0 {
		return
	}
	in.configured = s.configured
	s.consider(ctx, in)
}

// candidateInput is the policy and data of one candidate revision.
type candidateInput struct {
	modules    map[string]string
	data       map[string]any
	label      string
	configured []string
	activated  []string
}

func readSnapshot(ctx context.Context, store storage.Store, txn storage.Transaction) (candidateInput, error) {
	in := candidateInput{modules: map[string]string{}}
	names, err := bundle.ReadBundleNamesFromStore(ctx, store, txn)
	if err != nil && !storage.IsNotFound(err) {
		return in, err
	}
	sort.Strings(names)
	in.activated = names
	labels := make([]string, 0, len(names))
	for _, name := range names {
		revision, err := bundle.ReadBundleRevisionFromStore(ctx, store, txn, name)
		if err != nil && !storage.IsNotFound(err) {
			return in, err
		}
		labels = append(labels, name+"="+revision)
	}
	in.label = strings.Join(labels, ",")
	ids, err := store.ListPolicies(ctx, txn)
	if err != nil {
		return in, err
	}
	for _, id := range ids {
		raw, err := store.GetPolicy(ctx, txn, id)
		if err != nil {
			return in, err
		}
		in.modules[id] = string(raw)
	}
	data, err := store.Read(ctx, txn, storage.RootPath)
	if err != nil {
		return in, err
	}
	copied, ok := data.(map[string]any)
	if !ok {
		return in, errors.New("data document is not an object")
	}
	in.data = copied
	return in, nil
}

// consider builds a candidate and admits it, or keeps the active revision and records why not.
func (s *source) consider(ctx context.Context, in candidateInput) bool {
	start := time.Now()
	rev, rej := s.build(ctx, in)
	if rej != nil {
		s.reject(rej, in.label)
		return false
	}
	rev.admissionID = fmt.Sprintf("%s-%d", s.instance, s.sequence.Add(1))
	rev.admittedAt = time.Now()
	s.current.Store(rev)
	s.telemetry.revisionAdmitted(rev, len(in.activated), len(s.decisions), time.Since(start))
	s.admittedOnce.Do(func() { close(s.admitted) })
	return true
}

func (s *source) build(ctx context.Context, in candidateInput) (*revision, *rejection) {
	activated := make(map[string]bool, len(in.activated))
	for _, name := range in.activated {
		activated[name] = true
	}
	for _, name := range in.configured {
		if !activated[name] {
			return nil, &rejection{reason: reasonBundlesIncomplete}
		}
	}
	modules, rej := parseModules(in.modules)
	if rej != nil {
		return nil, rej
	}
	compiled, rej := compileCandidate(modules)
	if rej != nil {
		return nil, rej
	}
	if err := s.admit(compiled.decisionPresence(s.decisions)); err != nil {
		rej := &rejection{reason: reasonDecisionAbsent}
		var admissionErr *ports.ImpersonationPolicyAdmissionError
		if errors.As(err, &admissionErr) {
			rej.rule = admissionErr.Rule
		}
		return nil, rej
	}
	var data any = in.data
	if err := util.RoundTrip(&data); err != nil {
		return nil, &rejection{reason: reasonSnapshotFailed}
	}
	object, ok := data.(map[string]any)
	if !ok {
		object = map[string]any{}
	}
	store := inmem.NewFromObject(object)
	queries := make(map[string]decisionQuery, len(s.decisions))
	for _, decision := range s.decisions {
		ref, err := decisionRef(decision)
		if err != nil {
			return nil, &rejection{reason: reasonSnapshotFailed}
		}
		query, err := rego.New(
			rego.Query(ref.String()),
			rego.Compiler(compiled.compiler),
			rego.Store(store),
			rego.StrictBuiltinErrors(true),
			rego.EnablePrintStatements(false),
		).PrepareForEval(ctx)
		if err != nil {
			return nil, &rejection{reason: reasonSnapshotFailed}
		}
		timeout := decision.EvaluationTimeout
		if timeout <= 0 {
			timeout = defaultEvaluationTimeout
		}
		queries[decision.Rule] = decisionQuery{query: query, timeout: timeout}
	}
	return &revision{label: in.label, queries: queries, telemetry: s.telemetry}, nil
}

func (s *source) reject(rej *rejection, label string) {
	s.mu.Lock()
	s.lastRejection = rej
	s.mu.Unlock()
	s.telemetry.revisionRejected(rej, label)
}

// AwaitReady waits for SDK readiness and the first admitted revision, which contains every
// configured bundle in its own snapshot.
func (s *source) AwaitReady(ctx context.Context) error {
	for _, ready := range []<-chan struct{}{s.sdkReady, s.admitted} {
		select {
		case <-ready:
		case <-s.final:
			return s.notReady("local_bundle_invalid")
		case <-ctx.Done():
			return s.notReady("timeout")
		}
	}
	return nil
}

func (s *source) notReady(reason string) error {
	s.mu.Lock()
	last := s.lastRejection
	s.mu.Unlock()
	err := &ports.ImpersonationPolicyNotReadyError{}
	if last != nil {
		err.Reason = string(last.reason)
		err.Rule = last.rule
	}
	s.telemetry.startupFailed(reason, err.Rule, time.Since(s.started))
	return err
}

// Current returns the active revision, or nil before the first admission.
func (s *source) Current() ports.ImpersonationPolicyRevision {
	if rev := s.current.Load(); rev != nil {
		return rev
	}
	return nil
}

// Stop stops the SDK and unregisters the gauges. It is safe to call more than once.
func (s *source) Stop(ctx context.Context) {
	s.stopOnce.Do(func() {
		if s.opa != nil {
			s.opa.Stop(ctx)
		}
		s.telemetry.close()
	})
}

// onBundleStatus records each completed download attempt reported by OPA's bundle plugin.
func (s *source) onBundleStatus(status bundleplugin.Status) {
	s.mu.Lock()
	if status.LastRequest.IsZero() || !status.LastRequest.After(s.lastRequest[status.Name]) {
		s.mu.Unlock()
		return
	}
	s.lastRequest[status.Name] = status.LastRequest
	if !status.LastSuccessfulDownload.IsZero() {
		s.lastDownload[status.Name] = status.LastSuccessfulDownload
	}
	s.mu.Unlock()

	if status.Code == "" {
		s.telemetry.bundleDownload(status.Name, downloadSuccess, "")
		return
	}
	kind := downloadTransport
	switch {
	case status.HTTPCode != "":
		kind = downloadHTTPStatus
	case !status.LastSuccessfulDownload.IsZero() && !status.LastSuccessfulDownload.Before(status.LastRequest):
		kind = downloadInvalidBundle
	}
	s.telemetry.bundleDownload(status.Name, downloadFailure, kind)
}

func (s *source) downloadAges() map[string]time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	ages := make(map[string]time.Time, len(s.lastDownload))
	for name, at := range s.lastDownload {
		ages[name] = at
	}
	return ages
}
