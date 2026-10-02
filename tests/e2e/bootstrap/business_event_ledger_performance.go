//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	integrationbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"

	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
)

const LedgerPerformanceBaselineRevision = "d500f36378dd914f8a516604a08525f737e8ddff"

// LedgerPerformanceReference is the fixed, reproducible reference workload in
// specs/048-business-event-ledger/quickstart.md, not a deployment-load claim.
const (
	LedgerPerformancePrincipals      = 10000
	LedgerPerformanceAgents          = 100
	LedgerPerformanceServices        = 20
	LedgerPerformanceHistory         = 1000000
	LedgerPerformanceConcurrency     = 32
	LedgerPerformanceMeasuredActions = 100000
	LedgerPerformanceWarmup          = 2 * time.Minute
	LedgerPerformanceRepetitions     = 3
)

// LedgerPerformanceBinaries builds two actual broker executables, not two modes
// of the feature binary. The feature checkout is the caller's existing worktree.
// Close removes only the detached baseline worktree created by this helper.
type LedgerPerformanceBinaries struct {
	Baseline      string
	Feature       string
	FeatureSource string
	root          string
	checkout      string
}

func BuildLedgerPerformanceBinaries(ctx context.Context, root, temp string) (_ *LedgerPerformanceBinaries, err error) {
	checkout := filepath.Join(temp, "baseline-worktree")
	cmd := exec.CommandContext(ctx, "git", "-C", root, "worktree", "add", "--detach", checkout, LedgerPerformanceBaselineRevision)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		return nil, fmt.Errorf("checkout pinned pre-feature revision: %w (%s)", runErr, output)
	}
	result := &LedgerPerformanceBinaries{root: root, checkout: checkout, Baseline: filepath.Join(temp, "baseline-broker"), Feature: filepath.Join(temp, "feature-broker")}
	defer func() {
		if err != nil {
			err = errors.Join(err, result.Close())
		}
	}()
	revision := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD")
	revisionOutput, revisionErr := revision.Output()
	if revisionErr != nil {
		return nil, fmt.Errorf("read feature revision: %w", revisionErr)
	}
	result.FeatureSource = strings.TrimSpace(string(revisionOutput))
	status := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain", "--untracked-files=all")
	statusOutput, statusErr := status.Output()
	if statusErr != nil {
		return nil, fmt.Errorf("read feature worktree status: %w", statusErr)
	}
	if len(statusOutput) != 0 {
		result.FeatureSource += "+working-tree"
	}
	for _, version := range []struct{ dir, output string }{{checkout, result.Baseline}, {root, result.Feature}} {
		build := exec.CommandContext(ctx, "go", "build", "-o", version.output, "./cmd/agentic-identity-broker")
		build.Dir = version.dir
		if output, runErr := build.CombinedOutput(); runErr != nil {
			return nil, fmt.Errorf("build production broker in %s: %w (%s)", version.dir, runErr, output)
		}
	}
	return result, nil
}

func (b *LedgerPerformanceBinaries) BaselineCheckout() string { return b.checkout }

func (b *LedgerPerformanceBinaries) Close() error {
	if b == nil || b.checkout == "" {
		return nil
	}
	cmd := exec.Command("git", "-C", b.root, "worktree", "remove", b.checkout)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove benchmark-owned baseline checkout: %w (%s)", err, output)
	}
	b.checkout = ""
	return nil
}

// LedgerPerformanceBroker starts a production executable with identical
// PostgreSQL, HTTP, security, OTLP and pool settings for both versions.
// Passwords travel only through the child environment, never command arguments.
type LedgerPerformanceBroker struct {
	EndUserURL string
	AdminURL   string
	command    *exec.Cmd
	done       chan error
	output     bytes.Buffer
}

func freeLedgerPerformancePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func StartLedgerPerformanceBroker(ctx context.Context, binary, temp, databaseURL, upstreamURL, collector string) (*LedgerPerformanceBroker, error) {
	enduserPort, err := freeLedgerPerformancePort()
	if err != nil {
		return nil, err
	}
	adminPort, err := freeLedgerPerformancePort()
	if err != nil {
		return nil, err
	}
	if adminPort == enduserPort {
		return nil, errors.New("benchmark ports collided")
	}
	enduserURL := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(enduserPort))
	adminURL := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(adminPort))
	configPath := filepath.Join(temp, "broker.yaml")
	// The baseline ignores the new business_events section; neither executable
	// is run with a ledger-disable flag or with a different telemetry profile.
	config := fmt.Sprintf(`log:
  level: warn
  format: text
server:
  enduser:
    bind: 127.0.0.1
    port: %d
    public_url: %q
    authentication:
      preauth:
        principal_header_name: X-Remote-User
  admin:
    bind: 127.0.0.1
    port: %d
    public_url: %q
    authentication:
      preauth:
        principal_header_name: X-Remote-User
  shutdown:
    timeout: 10s
storage:
  backend: postgres
  timeouts:
    read: 10s
    write: 10s
third_party_oauth2:
  jwe_signing_key: ${IDENTITY_BROKER_JWE_SIGNING_KEY}
  state_token_ttl: 10m
  pkce_verifier_length: 32
encryption:
  memory:
    raw_key: ${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}
oauth2_authorization_server:
  mode: proxy
  proxy:
    upstream_issuer_uri: %q
    upstream_authorize_endpoint: %q
    upstream_token_endpoint: %q
    upstream_timeout: 10s
token_exchange:
  claim_extraction:
    principal_expression: subject_token.sub
    agent_id_expression: subject_token.azp
  authorization:
    type: cel
    cel:
      expression: "true"
      evaluation_timeout: 2s
security:
  skip_thirdparty_https_validation: true
approvals:
  pending_ttl: 20m
  sync_coalesce_window: 0s
  rate_limit:
    max_pending_per_pair: 100000
    max_requests_per_minute: 100000
telemetry:
  enabled: true
  service_name: ledger-performance
  traces:
    enabled: true
    sampling_rate: 1
    propagators: [tracecontext, baggage]
  metrics:
    enabled: true
    export_interval: 1s
  logs:
    enabled: true
  exporter:
    protocol: grpc
    endpoint: %q
    insecure: true
    timeout: 5s
business_events:
  retention: 2160h
  telemetry_copy_enabled: true
`, enduserPort, enduserURL, adminPort, adminURL, upstreamURL, upstreamURL+"/oauth/authorize", upstreamURL+"/oauth/token", collector)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		return nil, err
	}
	// DeferCleanup must drain the broker after the spec context is cancelled.
	cmd := exec.Command(binary, "--config", configPath) // #nosec G204 -- binary and private config path come from the benchmark's own builds.
	cmd.Dir = temp                                      // Never load a checkout's .env or implicit config.yaml.
	// Inherited broker/approval overrides must not silently change the fixed
	// workload. Both executables receive the same private configuration file.
	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "IDENTITY_BROKER_") && !strings.HasPrefix(entry, "APPROVAL_") {
			environment = append(environment, entry)
		}
	}
	cmd.Env = append(environment,
		"IDENTITY_BROKER_CONFIG_PATH="+configPath,
		"IDENTITY_BROKER_STORAGE_POSTGRES_URL="+databaseURL,
		"IDENTITY_BROKER_JWE_SIGNING_KEY=dGVzdC0zMi1ieXRlLWtleS1tdXN0LWJlLWV4YWN0LXg=",
		"IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY="+fixtures.TestKEKMaterialDeterministic(),
	)
	broker := &LedgerPerformanceBroker{command: cmd, EndUserURL: enduserURL, AdminURL: adminURL, done: make(chan error, 1)}
	cmd.Stdout, cmd.Stderr = &broker.output, &broker.output
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start production broker: %w", err)
	}
	go func() { broker.done <- cmd.Wait() }()
	readyContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 1 * time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		ok := true
		for _, address := range []string{broker.EndUserURL, broker.AdminURL} {
			response, requestErr := client.Get(address + "/health")
			if requestErr != nil || response.StatusCode != http.StatusOK {
				ok = false
			}
			if response != nil {
				_ = response.Body.Close()
			}
		}
		if ok {
			return broker, nil
		}
		select {
		case exit := <-broker.done:
			return nil, fmt.Errorf("production broker exited before readiness: %w", exit)
		case <-readyContext.Done():
			_ = broker.Close()
			return nil, fmt.Errorf("production broker did not become healthy: %w", readyContext.Err())
		case <-ticker.C:
		}
	}
}

func (b *LedgerPerformanceBroker) Close() error {
	if b == nil || b.command == nil || b.command.Process == nil {
		return nil
	}
	if err := b.command.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case err := <-b.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("production broker exited abnormally: %w", err)
		}
		return nil
	case <-time.After(15 * time.Second):
		_ = b.command.Process.Kill()
		<-b.done
		return errors.New("production broker shutdown exceeded 15 seconds")
	}
}

// The collector acknowledges real OTLP exports. ledger.record spans must
// cover validation, serialization, event/delivery-reference append and the
// attributable transaction work; HTTP and SQL timing are not substitutes.
// Missing or duplicate spans fail acceptance, never imply zero duration.
type LedgerPerformanceCollector struct {
	server   *grpc.Server
	listener net.Listener
	mu       sync.Mutex
	traces   map[string][]time.Duration
	alloc    []LedgerPerformanceAllocation
}

type ledgerPerformanceTraces struct {
	collecttraces.UnimplementedTraceServiceServer
	collector *LedgerPerformanceCollector
}

type ledgerPerformanceMetrics struct {
	collectmetrics.UnimplementedMetricsServiceServer
	collector *LedgerPerformanceCollector
}

type ledgerPerformanceLogs struct {
	collectlogs.UnimplementedLogsServiceServer
}

type LedgerPerformanceAllocation struct {
	Bytes, Objects uint64
	At             time.Time
}

func NewLedgerPerformanceCollector() (*LedgerPerformanceCollector, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	collector := &LedgerPerformanceCollector{server: grpc.NewServer(), listener: listener, traces: make(map[string][]time.Duration)}
	collecttraces.RegisterTraceServiceServer(collector.server, &ledgerPerformanceTraces{collector: collector})
	collectmetrics.RegisterMetricsServiceServer(collector.server, &ledgerPerformanceMetrics{collector: collector})
	collectlogs.RegisterLogsServiceServer(collector.server, &ledgerPerformanceLogs{})
	go func() { _ = collector.server.Serve(listener) }()
	return collector, nil
}

func (c *LedgerPerformanceCollector) Endpoint() string { return c.listener.Addr().String() }
func (c *LedgerPerformanceCollector) Close()           { c.server.Stop() }

func (service *ledgerPerformanceTraces) Export(_ context.Context, req *collecttraces.ExportTraceServiceRequest) (*collecttraces.ExportTraceServiceResponse, error) {
	c := service.collector
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, resource := range req.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if span.Name == "ledger.record" && span.StartTimeUnixNano != 0 && span.EndTimeUnixNano > span.StartTimeUnixNano {
					traceID := fmt.Sprintf("%x", span.TraceId)
					c.traces[traceID] = append(c.traces[traceID], time.Duration(span.EndTimeUnixNano-span.StartTimeUnixNano))
				}
			}
		}
	}
	return &collecttraces.ExportTraceServiceResponse{}, nil
}

// Cumulative Go allocation bytes/objects are sampled at the receiver. Deltas
// require samples on both sides of the measured workload.
func (service *ledgerPerformanceMetrics) Export(_ context.Context, req *collectmetrics.ExportMetricsServiceRequest) (*collectmetrics.ExportMetricsServiceResponse, error) {
	c := service.collector
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, resource := range req.ResourceMetrics {
		var sample LedgerPerformanceAllocation
		var bytesOK, objectsOK bool
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				var sum *metricsv1.Sum = metric.GetSum()
				if sum == nil {
					continue
				}
				for _, point := range sum.DataPoints {
					if metric.Name == "go.memory.allocated" {
						sample.Bytes = uint64(point.GetAsInt())
						bytesOK = true
					} else if metric.Name == "go.memory.allocations" {
						sample.Objects = uint64(point.GetAsInt())
						objectsOK = true
					}
				}
			}
		}
		if bytesOK && objectsOK {
			sample.At = time.Now()
			c.alloc = append(c.alloc, sample)
		}
	}
	return &collectmetrics.ExportMetricsServiceResponse{}, nil
}

func (ledgerPerformanceLogs) Export(context.Context, *collectlogs.ExportLogsServiceRequest) (*collectlogs.ExportLogsServiceResponse, error) {
	return &collectlogs.ExportLogsServiceResponse{}, nil
}

func (c *LedgerPerformanceCollector) RecordingTimes(traceIDs []string) ([]time.Duration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]time.Duration, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		spans := c.traces[traceID]
		if len(spans) != 1 {
			return nil, fmt.Errorf("recording trace %s had %d ledger.record spans, want exactly one", traceID, len(spans))
		}
		result = append(result, spans[0])
	}
	return result, nil
}

func (c *LedgerPerformanceCollector) Allocations(start, end time.Time) (LedgerPerformanceAllocation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var before, after *LedgerPerformanceAllocation
	for i := range c.alloc {
		item := &c.alloc[i]
		if !item.At.After(start) {
			before = item
		}
		if !item.At.Before(end) {
			after = item
			break
		}
	}
	if before == nil || after == nil || after.Bytes < before.Bytes || after.Objects < before.Objects {
		return LedgerPerformanceAllocation{}, errors.New("missing cumulative allocation measurements bracketing the measured workload")
	}
	return LedgerPerformanceAllocation{Bytes: after.Bytes - before.Bytes, Objects: after.Objects - before.Objects}, nil
}

// LedgerPerformanceStats keeps full observed samples until percentile extraction.
// Sorting copies only once, outside the timed workload.
type LedgerPerformanceStats struct {
	Samples           []time.Duration
	P50, P95, P99     time.Duration
	Throughput        float64
	Errors, Rollbacks int
	Allocations       LedgerPerformanceAllocation
}

func SummarizeLedgerPerformance(samples []time.Duration, elapsed time.Duration, allocation LedgerPerformanceAllocation, errors, rollbacks int) LedgerPerformanceStats {
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	pct := func(percent int) time.Duration {
		if len(sorted) == 0 {
			return 0
		}
		return sorted[(len(sorted)*percent+99)/100-1]
	}
	return LedgerPerformanceStats{Samples: samples, P50: pct(50), P95: pct(95), P99: pct(99), Throughput: float64(len(samples)) / elapsed.Seconds(), Errors: errors, Rollbacks: rollbacks, Allocations: allocation}
}

// Suppress unauthenticated downstream logs in the high-volume harness. Callers
// still inspect HTTP status and completed-action counts separately.
func DrainLedgerPerformanceResponse(response *http.Response) error {
	if response == nil {
		return errors.New("nil broker response")
	}
	_, err := io.Copy(io.Discard, response.Body)
	return errors.Join(err, response.Body.Close())
}

// OpenLedgerPerformanceDatabase clones an actual migrated PostgreSQL database.
// The binary receives only a separate DML role; test setup and historical
// seeding retain the migration-owner connection. The baseline schema has no
// business_events table and no ledger privileges to synthesize one.
func OpenLedgerPerformanceDatabase(ctx context.Context, t integrationbootstrap.PostgresTestHandle, pg *integrationbootstrap.SharedPostgres, template string, provision func(string)) (ownerURL, runtimeURL string, cleanup func(), err error) {
	dbName, ownerURL, drop := pg.SetupDatabaseFromTemplate(t, template, provision)
	owner, err := sqlx.ConnectContext(ctx, "pgx", ownerURL)
	if err != nil {
		drop()
		return "", "", nil, errors.New("benchmark cannot connect to migrated database as owner")
	}
	role := dbName + "_runtime"
	quoted := pgx.Identifier{role}.Sanitize()
	roleCreated := false
	defer func() {
		if err != nil {
			_ = owner.Close()
			drop()
			if roleCreated {
				pg.ExecuteSQL(t, "postgres", "DROP ROLE "+quoted)
			}
		}
	}()
	if _, err = owner.ExecContext(ctx, "CREATE ROLE "+quoted+" LOGIN PASSWORD 'ledger-performance-test-only'"); err != nil {
		return "", "", nil, errors.New("benchmark cannot create isolated runtime database role")
	}
	roleCreated = true
	if _, err = owner.ExecContext(ctx, "GRANT USAGE ON SCHEMA public TO "+quoted); err != nil {
		return "", "", nil, errors.New("benchmark cannot grant runtime schema usage")
	}
	if _, err = owner.ExecContext(ctx, "GRANT SELECT ON public.schema_migrations TO "+quoted); err != nil {
		return "", "", nil, errors.New("benchmark cannot grant read-only migration metadata")
	}
	var businessTables []string
	if err = owner.SelectContext(ctx, &businessTables, `SELECT format('%I.%I', schemaname, tablename) FROM pg_tables
		WHERE schemaname='public' AND tablename NOT IN ('schema_migrations', 'business_event_policy')
		AND tablename NOT LIKE 'business_events%' AND tablename NOT LIKE 'business_event_delivery_pending%'`); err != nil {
		return "", "", nil, errors.New("benchmark cannot discover business tables")
	}
	for _, table := range businessTables {
		if _, err = owner.ExecContext(ctx, "GRANT SELECT, INSERT, UPDATE, DELETE ON "+table+" TO "+quoted); err != nil {
			return "", "", nil, errors.New("benchmark cannot grant business-state DML")
		}
	}
	var ledgerExists bool
	if err = owner.GetContext(ctx, &ledgerExists, "SELECT to_regclass('public.business_events') IS NOT NULL"); err != nil {
		return "", "", nil, errors.New("benchmark cannot inspect ledger parent")
	}
	if ledgerExists {
		var validPolicy bool
		if err = owner.GetContext(ctx, &validPolicy, "SELECT count(*) = 1 FROM public.business_event_policy WHERE singleton AND retention_microseconds > 0"); err != nil || !validPolicy {
			return "", "", nil, errors.New("benchmark requires an initialized ledger retention policy")
		}
		if _, err = owner.ExecContext(ctx, "GRANT SELECT, UPDATE (retention_microseconds) ON public.business_event_policy TO "+quoted); err != nil {
			return "", "", nil, errors.New("benchmark cannot grant validated retention policy update")
		}
		if _, err = owner.ExecContext(ctx, "GRANT SELECT, INSERT ON public.business_events TO "+quoted+"; GRANT SELECT, INSERT, UPDATE, DELETE ON public.business_event_delivery_pending TO "+quoted); err != nil {
			return "", "", nil, errors.New("benchmark cannot grant append and delivery-reference DML")
		}
	}
	parsed, err := url.Parse(ownerURL)
	if err != nil {
		return "", "", nil, errors.New("benchmark cannot parse owner connection URL")
	}
	parsed.User = url.UserPassword(role, "ledger-performance-test-only")
	runtimeURL = parsed.String()
	cleanup = func() {
		_ = owner.Close()
		drop()
		pg.ExecuteSQL(t, "postgres", "DROP ROLE "+quoted)
	}
	return ownerURL, runtimeURL, cleanup, nil
}
