//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	"google.golang.org/grpc/credentials/insecure"
)

const LedgerPerformanceBaselineRevision = "e6902cf296e617ff02e315a069d5dfb73f44bb51"

// LedgerPerformanceReference is the fixed, reproducible reference workload in
// specs/048-business-event-ledger/quickstart.md, not a deployment-load claim.
const (
	LedgerPerformancePrincipals        = 10000
	LedgerPerformanceAgents            = 100
	LedgerPerformanceServices          = 20
	LedgerPerformanceHistory           = 1000000
	LedgerPerformanceConcurrency       = 32
	LedgerPerformanceMeasuredActions   = 100000
	LedgerPerformanceWarmup            = 2 * time.Minute
	LedgerPerformanceRepetitions       = 3
	LedgerPerformanceReferencePoolSize = 40 // Identical test-only overlay in both versions, above concurrency 32.
)

type LedgerPerformanceByteBounds struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// LedgerPerformanceProfile is an operator-approved workload, not a replacement
// for the fixed reference. Missing fields and unknown JSON fields are errors.
type LedgerPerformanceProfile struct {
	Approval struct {
		Owner     string `json:"owner"`
		Date      string `json:"date"`
		Reference string `json:"reference"`
		Location  string `json:"location"`
	} `json:"approval"`
	Actions                    int   `json:"actions"`
	ThroughputPerSecond        int   `json:"throughput_per_second"`
	Concurrency                int   `json:"concurrency"`
	Principals                 int   `json:"principals"`
	Agents                     int   `json:"agents"`
	Services                   int   `json:"services"`
	History                    int   `json:"history"`
	PrincipalIDBytes           int   `json:"principal_id_bytes"`
	PermissionSetServiceCounts []int `json:"permission_set_service_counts"`
	Mix                        struct {
		Exchanges int `json:"exchanges"`
		Approvals int `json:"approvals"`
		Grants    int `json:"grants"`
		Refreshes int `json:"refreshes"`
		Admin     int `json:"admin"`
	} `json:"mix"`
	EventBytes        map[string]LedgerPerformanceByteBounds `json:"event_bytes"`
	HistoryEventBytes LedgerPerformanceByteBounds            `json:"history_event_bytes"`
	Hardware          struct {
		OS          string `json:"os"`
		Arch        string `json:"arch"`
		CPUs        int    `json:"cpus"`
		GOMAXPROCS  int    `json:"gomaxprocs"`
		MemoryBytes uint64 `json:"memory_bytes"`
		Attestation string `json:"attestation"`
	} `json:"hardware"`
	Postgres struct {
		Host     string            `json:"host"`
		Port     int               `json:"port"`
		SSLMode  string            `json:"sslmode"`
		Settings map[string]string `json:"settings"`
	} `json:"postgres"`
	Network struct {
		ClientBroker string `json:"client_broker"`
		DatabaseHost string `json:"database_host"`
		UpstreamHost string `json:"upstream_host"`
	} `json:"network"`
	Pool struct {
		Open int `json:"open"`
		Idle int `json:"idle"`
	} `json:"pool"`
	Telemetry struct {
		Enabled      bool   `json:"enabled"`
		Traces       bool   `json:"traces"`
		Metrics      bool   `json:"metrics"`
		Logs         bool   `json:"logs"`
		LedgerCopy   bool   `json:"ledger_copy"`
		Receiver     string `json:"receiver"`
		ReceiverTLS  bool   `json:"receiver_tls"`
		ReceiverHost string `json:"receiver_host"`
		ReceiverPort int    `json:"receiver_port"`
	} `json:"telemetry"`
}

// LoadLedgerPerformanceProfile requires approval and rejects partial profiles
// before any expensive broker build or benchmark database is created.
func LoadLedgerPerformanceProfile(path string) (*LedgerPerformanceProfile, error) {
	if path == "" {
		return nil, errors.New("AIB_LEDGER_PERFORMANCE_PROFILE is required: no operator-approved deployment profile is available")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open operator-approved profile: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode operator-approved profile: %w", err)
	}
	for _, path := range []string{
		"approval.owner", "approval.date", "approval.reference", "approval.location",
		"actions", "throughput_per_second", "concurrency", "principals", "agents", "services", "history", "principal_id_bytes", "permission_set_service_counts",
		"mix.exchanges", "mix.approvals", "mix.grants", "mix.refreshes", "mix.admin", "event_bytes", "history_event_bytes.min", "history_event_bytes.max",
		"hardware.os", "hardware.arch", "hardware.cpus", "hardware.gomaxprocs", "hardware.memory_bytes", "hardware.attestation",
		"postgres.host", "postgres.port", "postgres.sslmode", "postgres.settings", "network.client_broker", "network.database_host", "network.upstream_host", "pool.open", "pool.idle",
		"telemetry.enabled", "telemetry.traces", "telemetry.metrics", "telemetry.logs", "telemetry.ledger_copy", "telemetry.receiver", "telemetry.receiver_tls", "telemetry.receiver_host", "telemetry.receiver_port",
	} {
		parts := strings.Split(path, ".")
		current := fields
		for i, part := range parts {
			value, present := current[part]
			if !present || string(value) == "null" {
				return nil, fmt.Errorf("operator-approved profile requires %s", path)
			}
			if i < len(parts)-1 {
				var nested map[string]json.RawMessage
				if err := json.Unmarshal(value, &nested); err != nil {
					return nil, fmt.Errorf("operator-approved profile requires object %s: %w", part, err)
				}
				current = nested
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var profile LedgerPerformanceProfile
	if err := decoder.Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode operator-approved profile: %w", err)
	}
	if profile.Approval.Owner == "" || profile.Approval.Reference == "" || profile.Approval.Location == "" || profile.Approval.Date == "" {
		return nil, errors.New("operator-approved profile requires approval owner, date, reference, and location")
	}
	if _, err := time.Parse("2006-01-02", profile.Approval.Date); err != nil {
		return nil, fmt.Errorf("invalid profile approval date: %w", err)
	}
	if profile.Actions < LedgerPerformanceMeasuredActions || profile.Actions%100 != 0 || profile.Concurrency <= 0 || profile.Principals <= 0 || profile.Agents <= 0 || profile.Services < 2 || profile.History <= 0 || profile.ThroughputPerSecond <= 0 || profile.ThroughputPerSecond > 1000000000 {
		return nil, errors.New("profile requires >=100000 measured actions divisible by 100, positive throughput/concurrency/dataset/history, and >=2 services")
	}
	if profile.History > int(^uint(0)>>1)-profile.Actions {
		return nil, errors.New("approved history plus measured actions exceeds supported count")
	}
	if profile.Principals < profile.Agents || profile.Principals < profile.Services {
		return nil, errors.New("approved principal cardinality must cover every declared agent and service")
	}
	longest := len(fmt.Sprintf("ledger-perf-%05d@example.test", profile.Principals-1))
	if profile.PrincipalIDBytes < longest || profile.PrincipalIDBytes > 255 {
		return nil, errors.New("profile principal_id_bytes cannot represent its principal cardinality (max 255)")
	}
	if len(profile.PermissionSetServiceCounts) == 0 {
		return nil, errors.New("profile permission_set_service_counts is required")
	}
	covered := make([]bool, profile.Services)
	start := 0
	for _, count := range profile.PermissionSetServiceCounts {
		if count <= 0 || count > profile.Services {
			return nil, errors.New("each permission-set service count must be between 1 and the service cardinality")
		}
		for i := range count {
			covered[(start+i)%profile.Services] = true
		}
		start += count
	}
	for _, present := range covered {
		if !present {
			return nil, errors.New("permission sets must collectively cover every configured service")
		}
	}
	weights := []int{profile.Mix.Exchanges, profile.Mix.Approvals, profile.Mix.Grants, profile.Mix.Refreshes, profile.Mix.Admin}
	types := []string{"token-exchanged", "approval-approved", "grant-updated", "session-refreshed", "agent-updated"}
	total, measuredTypes := 0, 0
	for i, weight := range weights {
		if weight < 0 {
			return nil, errors.New("profile mix weights cannot be negative")
		}
		total += weight
		if weight == 0 {
			continue
		}
		measuredTypes++
		bounds, ok := profile.EventBytes[types[i]]
		if !ok || bounds.Min <= 0 || bounds.Max < bounds.Min {
			return nil, fmt.Errorf("profile event_bytes requires positive min/max bounds for %s", types[i])
		}
	}
	if total != 100 || len(profile.EventBytes) != measuredTypes {
		return nil, errors.New("profile mix must total 100 and event_bytes must cover exactly its selected workflow types")
	}
	if profile.HistoryEventBytes.Min <= 0 || profile.HistoryEventBytes.Max < profile.HistoryEventBytes.Min {
		return nil, errors.New("profile history_event_bytes requires positive min/max bounds")
	}
	if profile.Hardware.OS == "" || profile.Hardware.Arch == "" || profile.Hardware.CPUs <= 0 ||
		profile.Hardware.GOMAXPROCS <= 0 || profile.Hardware.MemoryBytes == 0 || profile.Hardware.Attestation == "" ||
		profile.Postgres.Host == "" || profile.Network.ClientBroker != "loopback" ||
		profile.Network.DatabaseHost != profile.Postgres.Host || profile.Network.UpstreamHost != "loopback" {
		return nil, errors.New("profile requires attested OS/arch/cpus/gomaxprocs/memory, matching PostgreSQL host, loopback client/broker and mock upstream; unsupported placement needs an external test runner")
	}
	for _, setting := range []string{"server_version", "max_connections", "shared_buffers", "synchronous_commit"} {
		if profile.Postgres.Settings[setting] == "" {
			return nil, fmt.Errorf("profile requires PostgreSQL setting %s", setting)
		}
	}
	if profile.Postgres.Port < 1 || profile.Postgres.Port > 65535 || profile.Postgres.SSLMode == "" {
		return nil, errors.New("profile PostgreSQL port and sslmode are required")
	}
	for name, expected := range profile.Postgres.Settings {
		if name == "" || expected == "" {
			return nil, errors.New("profile PostgreSQL settings cannot have empty names or values")
		}
	}
	if profile.Pool.Open != 25 || profile.Pool.Idle != 5 {
		return nil, errors.New("production PostgreSQL pool is fixed at 25 open/5 idle; requested deployment pool unavailable")
	}
	if !profile.Telemetry.Enabled || !profile.Telemetry.Traces || !profile.Telemetry.Metrics {
		return nil, errors.New("acceptance requires enabled telemetry, 100% traces, and allocation metrics; requested deployment telemetry is unmeasurable")
	}
	if profile.Telemetry.Receiver != "local-ack" && profile.Telemetry.Receiver != "external-grpc" {
		return nil, errors.New("profile telemetry receiver must be local-ack or external-grpc")
	}
	if profile.Telemetry.ReceiverTLS {
		return nil, errors.New("TLS telemetry is unsupported: the capture proxy cannot measure the broker's own TLS costs")
	}
	if profile.Telemetry.ReceiverHost == "" || (profile.Telemetry.Receiver == "local-ack" && profile.Telemetry.ReceiverHost != "127.0.0.1") {
		return nil, errors.New("local acknowledging receiver requires host 127.0.0.1; external receiver requires an approved receiver host")
	}
	if (profile.Telemetry.Receiver == "local-ack" && profile.Telemetry.ReceiverPort != 0) ||
		(profile.Telemetry.Receiver == "external-grpc" && (profile.Telemetry.ReceiverPort < 1 || profile.Telemetry.ReceiverPort > 65535)) {
		return nil, errors.New("profile receiver_port must be 0 for a local ephemeral receiver or a valid approved external port")
	}
	return &profile, nil
}

// CheckLedgerPerformanceHost never treats a declared host as a measured host.
// Physical memory needs an explicit operator attestation because Go cannot
// portably discover the physical allocation of the deployment host.
func CheckLedgerPerformanceHost(profile *LedgerPerformanceProfile) error {
	if runtime.GOOS != profile.Hardware.OS || runtime.GOARCH != profile.Hardware.Arch || runtime.NumCPU() != profile.Hardware.CPUs || runtime.GOMAXPROCS(0) != profile.Hardware.GOMAXPROCS {
		return fmt.Errorf("deployment hardware mismatch: running %s/%s cpus=%d gomaxprocs=%d", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0))
	}
	if os.Getenv("AIB_LEDGER_PERFORMANCE_HOST_ATTESTATION") == "" || os.Getenv("AIB_LEDGER_PERFORMANCE_HOST_ATTESTATION") != profile.Hardware.Attestation {
		return errors.New("AIB_LEDGER_PERFORMANCE_HOST_ATTESTATION must attest the approved deployment hardware and memory")
	}
	memory, err := strconv.ParseUint(os.Getenv("AIB_LEDGER_PERFORMANCE_HOST_MEMORY_BYTES"), 10, 64)
	if err != nil || memory != profile.Hardware.MemoryBytes {
		return errors.New("AIB_LEDGER_PERFORMANCE_HOST_MEMORY_BYTES must match the operator-attested physical deployment memory")
	}
	return nil
}

// CheckLedgerPerformancePostgres compares the connected server, not a label in
// the report, with the database conditions from the approved profile.
func CheckLedgerPerformancePostgres(ctx context.Context, db *sqlx.DB, profile *LedgerPerformanceProfile) error {
	for key, expected := range profile.Postgres.Settings {
		var observed string
		if err := db.GetContext(ctx, &observed, "SELECT current_setting($1)", key); err != nil {
			return fmt.Errorf("inspect PostgreSQL setting %s: %w", key, err)
		}
		if observed != expected {
			return fmt.Errorf("PostgreSQL %s=%q; approved profile requires %q", key, observed, expected)
		}
	}
	return nil
}

// LedgerPerformanceBinaries builds separate brokers from pinned baseline and
// feature code. A positive poolSize applies one identical test-only overlay to
// both; zero builds both unmodified for an approved deployment workload.
// Close removes only the detached baseline worktree created by this helper.
type LedgerPerformanceBinaries struct {
	Baseline      string
	Feature       string
	FeatureSource string
	PoolSize      int // Zero for the unmodified deployment binaries.
	root          string
	checkout      string
}

func BuildLedgerPerformanceBinaries(ctx context.Context, root, temp string, poolSize int) (_ *LedgerPerformanceBinaries, err error) {
	checkout := filepath.Join(temp, "baseline-worktree")
	cmd := exec.CommandContext(ctx, "git", "-C", root, "worktree", "add", "--detach", checkout, LedgerPerformanceBaselineRevision)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		return nil, fmt.Errorf("checkout pinned pre-feature revision: %w (%s)", runErr, output)
	}
	result := &LedgerPerformanceBinaries{root: root, checkout: checkout, Baseline: filepath.Join(temp, "baseline-broker"), Feature: filepath.Join(temp, "feature-broker"), PoolSize: poolSize}
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
		args := []string{"build", "-o", version.output, "./cmd/agentic-identity-broker"}
		if poolSize > 0 {
			overlay, overlayErr := ledgerPerformancePoolOverlay(version.dir, temp, poolSize)
			if overlayErr != nil {
				return nil, overlayErr
			}
			args = []string{"build", "-overlay", overlay, "-o", version.output, "./cmd/agentic-identity-broker"}
		}
		build := exec.CommandContext(ctx, "go", args...)
		build.Dir = version.dir
		if output, runErr := build.CombinedOutput(); runErr != nil {
			return nil, fmt.Errorf("build broker in %s: %w (%s)", version.dir, runErr, output)
		}
	}
	return result, nil
}

// ledgerPerformancePoolOverlay changes only benchmark binaries. The production
// adapter retains its 25/5 pool; both reference binaries use the same 40/40 pool.
func ledgerPerformancePoolOverlay(checkout, temp string, size int) (string, error) {
	original := filepath.Join(checkout, "internal/adapters/storage/postgres/adapter.go")
	raw, err := os.ReadFile(original)
	if err != nil {
		return "", err
	}
	text := string(raw)
	for _, edit := range []struct{ from, to string }{
		{"\t\"context\"", "\t\"context\"\n\t\"go.opentelemetry.io/otel\"\n\t\"go.opentelemetry.io/otel/metric\""},
		{"db.SetMaxOpenConns(25)\n\tdb.SetMaxIdleConns(5)", fmt.Sprintf("db.SetMaxOpenConns(%d)\n\tdb.SetMaxIdleConns(%d)", size, size)},
		{"\ta.db = db", fmt.Sprintf("\tif err := warmLedgerPerformancePool(ctx, db, %d); err != nil {\n\t\t_ = db.Close()\n\t\treturn fmt.Errorf(\"benchmark pool warmup: %%w\", err)\n\t}\n\ta.db = db", size)},
	} {
		if strings.Count(text, edit.from) != 1 {
			return "", fmt.Errorf("benchmark overlay source changed at %q", edit.from)
		}
		text = strings.Replace(text, edit.from, edit.to, 1)
	}
	text += `
// The benchmark holds every connection simultaneously before returning it to
// the idle pool; initial actions therefore reuse initialized PG backends.
func warmLedgerPerformancePool(ctx context.Context, db *sqlx.DB, size int) error {
	var fill func(int) error
	fill = func(remaining int) error {
		if remaining == 0 { return nil }
		conn, err := db.Conn(ctx)
		if err != nil { return err }
		defer conn.Close()
		return fill(remaining - 1)
	}
	if err := fill(size); err != nil { return err }
	meter := otel.Meter("ledger-performance.pool")
	waits, err := meter.Int64ObservableGauge("ledger.performance.pool.wait_count")
	if err != nil { return err }
	waitNanos, err := meter.Int64ObservableGauge("ledger.performance.pool.wait_nanoseconds")
	if err != nil { return err }
	_, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		stats := db.Stats()
		observer.ObserveInt64(waits, stats.WaitCount)
		observer.ObserveInt64(waitNanos, int64(stats.WaitDuration))
		return nil
	}, waits, waitNanos)
	return err
}
`
	modified := filepath.Join(temp, filepath.Base(checkout)+"-pool-adapter.go")
	if err := os.WriteFile(modified, []byte(text), 0600); err != nil {
		return "", err
	}
	file := filepath.Join(temp, filepath.Base(checkout)+"-pool-overlay.json")
	payload, err := json.Marshal(struct {
		Replace map[string]string `json:"Replace"`
	}{Replace: map[string]string{original: modified}})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(file, payload, 0600); err != nil {
		return "", err
	}
	return file, nil
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

// LedgerPerformanceBroker starts the actual baseline or feature executable with
// identical PostgreSQL, HTTP, security, OTLP and pool settings. Passwords travel
// only through the child environment, never command arguments.
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

func StartLedgerPerformanceBrokerWithProfile(ctx context.Context, binary, temp, databaseURL, upstreamURL, collector string, profile *LedgerPerformanceProfile, deliveryDiagnostic bool) (*LedgerPerformanceBroker, error) {
	logsEnabled, ledgerCopy := true, deliveryDiagnostic
	if profile != nil {
		logsEnabled, ledgerCopy = profile.Telemetry.Logs, profile.Telemetry.LedgerCopy
	}
	if profile != nil {
		db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
		if err != nil {
			return nil, errors.New("approved benchmark runtime database is unavailable")
		}
		checkErr := CheckLedgerPerformancePostgres(ctx, db, profile)
		_ = db.Close()
		if checkErr != nil {
			return nil, checkErr
		}
	}
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
    enabled: %t
  exporter:
    protocol: grpc
    endpoint: %q
    insecure: true
    timeout: 5s
business_events:
  retention: 2160h
  telemetry_copy_enabled: %t
`, enduserPort, enduserURL, adminPort, adminURL, upstreamURL, upstreamURL+"/oauth/authorize", upstreamURL+"/oauth/token", logsEnabled, collector, ledgerCopy)
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
		if profile != nil && strings.HasPrefix(entry, "GOMAXPROCS=") {
			continue
		}
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
	if profile != nil {
		cmd.Env = append(cmd.Env, "GOMAXPROCS="+strconv.Itoa(profile.Hardware.GOMAXPROCS))
	}
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

// Validation and append are distinct measured stages. Their per-action sum
// excludes pool checkout, advisory-gate waits, and physical commit. The full
// owning-transaction span includes those costs. Missing or duplicate spans
// fail measurement, never imply zero duration.
type LedgerPerformanceCollector struct {
	server   *grpc.Server
	listener net.Listener
	forward  *grpc.ClientConn
	mu       sync.Mutex
	traces   map[string]map[string][]time.Duration
	alloc    []LedgerPerformanceAllocation
	pool     []LedgerPerformancePoolSample
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
	collector *LedgerPerformanceCollector
}

type LedgerPerformanceAllocation struct {
	Bytes, Objects uint64
	At             time.Time
}

type LedgerPerformancePoolSample struct {
	WaitCount    int64         `json:"wait_count"`
	WaitDuration time.Duration `json:"wait_duration_nanoseconds"`
	At           time.Time     `json:"-"`
}

type LedgerPerformanceRecordings struct {
	Validation, Append, Total, Transaction []time.Duration
}

// The capture proxy preserves the external OTLP receiver's real response and
// backpressure while retaining spans/allocations needed for acceptance.
func NewLedgerPerformanceCollectorForProfile(ctx context.Context, profile *LedgerPerformanceProfile) (*LedgerPerformanceCollector, error) {
	if profile != nil && profile.Telemetry.ReceiverTLS {
		return nil, errors.New("TLS telemetry is unsupported: the capture proxy cannot measure the broker's own TLS costs")
	}
	if profile == nil || profile.Telemetry.Receiver == "local-ack" {
		return newLedgerPerformanceCollector(nil)
	}
	endpoint := os.Getenv("AIB_LEDGER_PERFORMANCE_OTLP_ENDPOINT")
	if endpoint == "" {
		return nil, errors.New("AIB_LEDGER_PERFORMANCE_OTLP_ENDPOINT is required for the approved external receiver")
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || host != profile.Telemetry.ReceiverHost || port != strconv.Itoa(profile.Telemetry.ReceiverPort) {
		return nil, errors.New("AIB_LEDGER_PERFORMANCE_OTLP_ENDPOINT must use the approved receiver host:port")
	}
	connect, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	upstream, err := grpc.DialContext(connect, endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("approved OTLP receiver unavailable: %w", err)
	}
	collector, err := newLedgerPerformanceCollector(upstream)
	if err != nil {
		_ = upstream.Close()
		return nil, err
	}
	return collector, nil
}

func newLedgerPerformanceCollector(forward *grpc.ClientConn) (*LedgerPerformanceCollector, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	collector := &LedgerPerformanceCollector{server: grpc.NewServer(), listener: listener, traces: make(map[string]map[string][]time.Duration), forward: forward}
	collecttraces.RegisterTraceServiceServer(collector.server, &ledgerPerformanceTraces{collector: collector})
	collectmetrics.RegisterMetricsServiceServer(collector.server, &ledgerPerformanceMetrics{collector: collector})
	collectlogs.RegisterLogsServiceServer(collector.server, &ledgerPerformanceLogs{collector: collector})
	go func() { _ = collector.server.Serve(listener) }()
	return collector, nil
}

func (c *LedgerPerformanceCollector) Endpoint() string { return c.listener.Addr().String() }
func (c *LedgerPerformanceCollector) Close() {
	c.server.Stop()
	if c.forward != nil {
		_ = c.forward.Close()
	}
}

func (service *ledgerPerformanceTraces) Export(ctx context.Context, req *collecttraces.ExportTraceServiceRequest) (*collecttraces.ExportTraceServiceResponse, error) {
	c := service.collector
	response := &collecttraces.ExportTraceServiceResponse{}
	if c.forward != nil {
		var err error
		response, err = collecttraces.NewTraceServiceClient(c.forward).Export(ctx, req)
		if err != nil {
			return nil, err
		}
		if response.GetPartialSuccess().GetRejectedSpans() != 0 {
			return nil, errors.New("approved OTLP receiver rejected recording spans")
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, resource := range req.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if (span.Name == "ledger.record.validate" || span.Name == "ledger.record" || span.Name == "ledger.transaction") && span.StartTimeUnixNano != 0 && span.EndTimeUnixNano > span.StartTimeUnixNano {
					traceID := fmt.Sprintf("%x", span.TraceId)
					if c.traces[traceID] == nil {
						c.traces[traceID] = make(map[string][]time.Duration, 3)
					}
					c.traces[traceID][span.Name] = append(c.traces[traceID][span.Name], time.Duration(span.EndTimeUnixNano-span.StartTimeUnixNano))
				}
			}
		}
	}
	return response, nil
}

// Cumulative Go allocation bytes/objects are sampled at the receiver. Deltas
// require samples on both sides of the measured workload.
func (service *ledgerPerformanceMetrics) Export(ctx context.Context, req *collectmetrics.ExportMetricsServiceRequest) (*collectmetrics.ExportMetricsServiceResponse, error) {
	c := service.collector
	response := &collectmetrics.ExportMetricsServiceResponse{}
	if c.forward != nil {
		var err error
		response, err = collectmetrics.NewMetricsServiceClient(c.forward).Export(ctx, req)
		if err != nil {
			return nil, err
		}
		if response.GetPartialSuccess().GetRejectedDataPoints() != 0 {
			return nil, errors.New("approved OTLP receiver rejected allocation metrics")
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, resource := range req.ResourceMetrics {
		var sample LedgerPerformanceAllocation
		var pool LedgerPerformancePoolSample
		var bytesOK, objectsOK, waitCountOK, waitDurationOK bool
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if gauge := metric.GetGauge(); gauge != nil {
					for _, point := range gauge.DataPoints {
						switch metric.Name {
						case "ledger.performance.pool.wait_count":
							pool.WaitCount, waitCountOK = point.GetAsInt(), true
						case "ledger.performance.pool.wait_nanoseconds":
							pool.WaitDuration, waitDurationOK = time.Duration(point.GetAsInt()), true
						}
					}
					continue
				}
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
		if waitCountOK && waitDurationOK {
			pool.At = time.Now()
			c.pool = append(c.pool, pool)
		}
	}
	return response, nil
}

func (service *ledgerPerformanceLogs) Export(ctx context.Context, req *collectlogs.ExportLogsServiceRequest) (*collectlogs.ExportLogsServiceResponse, error) {
	if service.collector.forward == nil {
		return &collectlogs.ExportLogsServiceResponse{}, nil
	}
	response, err := collectlogs.NewLogsServiceClient(service.collector.forward).Export(ctx, req)
	if err != nil {
		return nil, err
	}
	if response.GetPartialSuccess().GetRejectedLogRecords() != 0 {
		return nil, errors.New("approved OTLP receiver rejected ledger logs")
	}
	return response, nil
}

func (c *LedgerPerformanceCollector) RecordingTimes(traceIDs []string) (LedgerPerformanceRecordings, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, traceID := range traceIDs {
		spans := c.traces[traceID]
		for _, name := range []string{"ledger.record.validate", "ledger.record", "ledger.transaction"} {
			if len(spans[name]) != 1 {
				return LedgerPerformanceRecordings{}, fmt.Errorf("recording trace %s had %d %s spans, want exactly one", traceID, len(spans[name]), name)
			}
		}
	}
	result := LedgerPerformanceRecordings{
		Validation: make([]time.Duration, len(traceIDs)), Append: make([]time.Duration, len(traceIDs)),
		Total: make([]time.Duration, len(traceIDs)), Transaction: make([]time.Duration, len(traceIDs)),
	}
	for index, traceID := range traceIDs {
		spans := c.traces[traceID]
		validation, appendTime := spans["ledger.record.validate"][0], spans["ledger.record"][0]
		result.Validation[index] = validation
		result.Append[index] = appendTime
		result.Total[index] = validation + appendTime
		result.Transaction[index] = spans["ledger.transaction"][0]
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

func (c *LedgerPerformanceCollector) PoolWaits(start, end time.Time) (LedgerPerformancePoolSample, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var before, after *LedgerPerformancePoolSample
	for i := range c.pool {
		item := &c.pool[i]
		if !item.At.After(start) {
			before = item
		}
		if !item.At.Before(end) {
			after = item
			break
		}
	}
	if before == nil || after == nil || after.WaitCount < before.WaitCount || after.WaitDuration < before.WaitDuration {
		return LedgerPerformancePoolSample{}, errors.New("missing pool wait counters bracketing measured workload")
	}
	return LedgerPerformancePoolSample{WaitCount: after.WaitCount - before.WaitCount, WaitDuration: after.WaitDuration - before.WaitDuration}, nil
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

// LedgerPerformancePostgres is the existing template-clone contract, shared by
// the reference container and an operator-provided PostgreSQL server.
type LedgerPerformancePostgres interface {
	SetupDatabaseFromTemplate(integrationbootstrap.PostgresTestHandle, string, func(string)) (string, string, func())
	ConnectionString(string) string
	ExecuteSQL(integrationbootstrap.PostgresTestHandle, string, string)
}

type ExternalLedgerPerformancePostgres struct {
	adminURL  string
	identity  string
	templates map[string]string
	mu        sync.Mutex
	sequence  int
}

// NewExternalLedgerPerformancePostgres refuses the local test container for a
// deployment run. The operator provides a dedicated PostgreSQL admin URL with
// CREATE DATABASE and CREATE ROLE privileges; only benchmark-owned DBs/roles
// are created or deleted.
func NewExternalLedgerPerformancePostgres(ctx context.Context, profile *LedgerPerformanceProfile) (*ExternalLedgerPerformancePostgres, error) {
	raw := os.Getenv("AIB_LEDGER_PERFORMANCE_POSTGRES_ADMIN_URL")
	if raw == "" {
		return nil, errors.New("AIB_LEDGER_PERFORMANCE_POSTGRES_ADMIN_URL is required for the approved deployment database")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" || parsed.User == nil || parsed.Path != "/postgres" || parsed.Hostname() != profile.Postgres.Host || parsed.Port() != strconv.Itoa(profile.Postgres.Port) || parsed.Query().Get("sslmode") != profile.Postgres.SSLMode {
		return nil, errors.New("deployment PostgreSQL admin URL must use postgres://USER:PASSWORD@approved-host:port/postgres?sslmode=approved-mode")
	}
	connection, err := sqlx.ConnectContext(ctx, "pgx", raw)
	if err != nil {
		return nil, fmt.Errorf("approved deployment PostgreSQL is unavailable: %w", err)
	}
	defer connection.Close()
	if err := CheckLedgerPerformancePostgres(ctx, connection, profile); err != nil {
		return nil, err
	}
	identity := make([]byte, 8)
	if _, err := rand.Read(identity); err != nil {
		return nil, err
	}
	return &ExternalLedgerPerformancePostgres{adminURL: raw, identity: hex.EncodeToString(identity), templates: make(map[string]string)}, nil
}

func (pg *ExternalLedgerPerformancePostgres) ConnectionString(name string) string {
	parsed, _ := url.Parse(pg.adminURL) // Checked during construction.
	parsed.Path = "/" + name
	return parsed.String()
}

func (pg *ExternalLedgerPerformancePostgres) exec(ctx context.Context, query string) error {
	conn, err := pgx.Connect(ctx, pg.adminURL)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	_, err = conn.Exec(ctx, query)
	return err
}

func (pg *ExternalLedgerPerformancePostgres) create(name, template string) error {
	return pg.exec(context.Background(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+pgx.Identifier{template}.Sanitize())
}

func (pg *ExternalLedgerPerformancePostgres) drop(name string) error {
	// Only the unpredictable benchmark prefix may be dropped.
	if !strings.HasPrefix(name, "ledgerp_"+pg.identity+"_") {
		return errors.New("refusing to drop a database not owned by this performance run")
	}
	return pg.exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
}

func (pg *ExternalLedgerPerformancePostgres) SetupDatabaseFromTemplate(t integrationbootstrap.PostgresTestHandle, key string, provision func(string)) (string, string, func()) {
	t.Helper()
	pg.mu.Lock()
	defer pg.mu.Unlock()
	template, ok := pg.templates[key]
	if !ok {
		pg.sequence++
		template = fmt.Sprintf("ledgerp_%s_%d_template", pg.identity, pg.sequence)
		if err := pg.create(template, "template0"); err != nil {
			t.Fatalf("create approved PostgreSQL template: %v", err)
		}
		pg.templates[key] = template // Close cleans it if provisioning fails.
		provision(template)
	}
	pg.sequence++
	name := fmt.Sprintf("ledgerp_%s_%d", pg.identity, pg.sequence)
	if err := pg.create(name, template); err != nil {
		t.Fatalf("clone approved PostgreSQL template: %v", err)
	}
	return name, pg.ConnectionString(name), func() {
		if err := pg.drop(name); err != nil {
			t.Fatalf("drop benchmark-owned database: %v", err)
		}
	}
}

func (pg *ExternalLedgerPerformancePostgres) ExecuteSQL(t integrationbootstrap.PostgresTestHandle, database, query string) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), pg.ConnectionString(database))
	if err != nil {
		t.Fatalf("connect to approved PostgreSQL database: %v", err)
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(context.Background(), query); err != nil {
		t.Fatalf("execute approved PostgreSQL database setup: %v", err)
	}
}

func (pg *ExternalLedgerPerformancePostgres) Close() error {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	var failures []error
	for _, name := range pg.templates {
		if err := pg.drop(name); err != nil {
			failures = append(failures, err)
		}
	}
	pg.templates = nil
	return errors.Join(failures...)
}

// OpenLedgerPerformanceDatabase clones an actual migrated PostgreSQL database.
// The binary receives only a separate DML role; test setup and historical
// seeding retain the migration-owner connection. The baseline schema has no
// business_events table and no ledger privileges to synthesize one.
func OpenLedgerPerformanceDatabase(ctx context.Context, t integrationbootstrap.PostgresTestHandle, pg LedgerPerformancePostgres, template string, provision func(string)) (ownerURL, runtimeURL string, cleanup func(), err error) {
	dbName, ownerURL, drop := pg.SetupDatabaseFromTemplate(t, template, provision)
	owner, err := sqlx.ConnectContext(ctx, "pgx", ownerURL)
	if err != nil {
		drop()
		return "", "", nil, errors.New("benchmark cannot connect to migrated database as owner")
	}
	role := dbName + "_runtime"
	quoted := pgx.Identifier{role}.Sanitize()
	secret := make([]byte, 24)
	if _, err = rand.Read(secret); err != nil {
		_ = owner.Close()
		drop()
		return "", "", nil, fmt.Errorf("generate benchmark runtime credential: %w", err)
	}
	password := hex.EncodeToString(secret)
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
	if _, err = owner.ExecContext(ctx, "CREATE ROLE "+quoted+" LOGIN PASSWORD '"+password+"'"); err != nil {
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
	parsed.User = url.UserPassword(role, password)
	runtimeURL = parsed.String()
	cleanup = func() {
		_ = owner.Close()
		drop()
		pg.ExecuteSQL(t, "postgres", "DROP ROLE "+quoted)
	}
	return ownerURL, runtimeURL, cleanup, nil
}
