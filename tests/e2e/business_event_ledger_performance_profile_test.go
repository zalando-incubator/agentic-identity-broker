//go:build integration

package e2e_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestLedgerDeploymentProfileHelpers(t *testing.T) {
	// Fictional approval exercises the harness, never release acceptance.
	var profile bootstrap.LedgerPerformanceProfile
	profile.Approval.Owner, profile.Approval.Date = "fictional-fixture-only", "2026-10-01"
	profile.Approval.Reference, profile.Approval.Location = "not-an-operator-approval", "test-fixture"
	profile.Actions, profile.ThroughputPerSecond, profile.Concurrency = 100000, 750, 7
	profile.Principals, profile.Agents, profile.Services = 8, 2, 4
	profile.PrincipalIDBytes = len("ledger-perf-00000@example.test") + 5
	profile.History, profile.PermissionSetServiceCounts = 150, []int{2, 1, 1}
	profile.Mix.Exchanges, profile.Mix.Approvals, profile.Mix.Grants, profile.Mix.Refreshes, profile.Mix.Admin = 35, 25, 15, 20, 5
	profile.EventBytes = map[string]bootstrap.LedgerPerformanceByteBounds{
		"token-exchanged": {Min: 100, Max: 3000}, "approval-approved": {Min: 100, Max: 3000},
		"grant-updated": {Min: 100, Max: 3000}, "session-refreshed": {Min: 100, Max: 3000}, "agent-updated": {Min: 100, Max: 3000},
	}
	profile.HistoryEventBytes = bootstrap.LedgerPerformanceByteBounds{Min: 100, Max: 3000}
	profile.Hardware.OS, profile.Hardware.Arch = runtime.GOOS, runtime.GOARCH
	profile.Hardware.CPUs, profile.Hardware.GOMAXPROCS = runtime.NumCPU(), runtime.GOMAXPROCS(0)
	profile.Hardware.MemoryBytes, profile.Hardware.Attestation = 16<<30, "fixture-hardware-only"
	profile.Postgres.Host, profile.Postgres.Port, profile.Postgres.SSLMode = "fixture-db.test", 5433, "verify-full"
	profile.Postgres.Settings = map[string]string{"server_version": "17.11", "max_connections": "100", "shared_buffers": "128MB", "synchronous_commit": "on"}
	profile.Network.ClientBroker, profile.Network.DatabaseHost, profile.Network.UpstreamHost = "loopback", profile.Postgres.Host, "loopback"
	profile.Pool.Open, profile.Pool.Idle = 25, 5
	profile.Telemetry.Enabled, profile.Telemetry.Traces, profile.Telemetry.Metrics, profile.Telemetry.Logs, profile.Telemetry.LedgerCopy = true, true, true, false, true
	profile.Telemetry.Receiver, profile.Telemetry.ReceiverHost = "local-ack", "127.0.0.1"
	base, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixture-not-approval.json")
	writeProfile := func(value any) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeProfile(profile)
	loaded, err := bootstrap.LoadLedgerPerformanceProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrap.LoadLedgerPerformanceProfile(""); err == nil || !strings.Contains(err.Error(), "AIB_LEDGER_PERFORMANCE_PROFILE") {
		t.Fatalf("missing approval must block acceptance: %v", err)
	}
	for _, tc := range []struct {
		name      string
		mutate    func(*bootstrap.LedgerPerformanceProfile)
		errorText string
	}{
		{"too few actions", func(p *bootstrap.LedgerPerformanceProfile) { p.Actions = 99900 }, ">=100000"},
		{"incomplete mix cycle", func(p *bootstrap.LedgerPerformanceProfile) { p.Actions = 100050 }, "divisible by 100"},
		{"incomplete mix", func(p *bootstrap.LedgerPerformanceProfile) { p.Mix.Admin = 0 }, "total 100"},
		{"missing approval", func(p *bootstrap.LedgerPerformanceProfile) { p.Approval.Owner = "" }, "approval owner"},
		{"invalid approval date", func(p *bootstrap.LedgerPerformanceProfile) { p.Approval.Date = "2026-14-01" }, "approval date"},
		{"inverted event sizes", func(p *bootstrap.LedgerPerformanceProfile) {
			p.EventBytes["token-exchanged"] = bootstrap.LedgerPerformanceByteBounds{Min: 3001, Max: 3000}
		}, "event_bytes"},
		{"missing selected event type", func(p *bootstrap.LedgerPerformanceProfile) { delete(p.EventBytes, "token-exchanged") }, "token-exchanged"},
		{"unavailable pool", func(p *bootstrap.LedgerPerformanceProfile) { p.Pool.Open = 40 }, "fixed at 25"},
		{"unavailable HTTP placement", func(p *bootstrap.LedgerPerformanceProfile) { p.Network.ClientBroker = "10.0.0.5" }, "loopback"},
		{"unavailable upstream placement", func(p *bootstrap.LedgerPerformanceProfile) { p.Network.UpstreamHost = "idp.test" }, "loopback"},
		{"unmeasurable traces", func(p *bootstrap.LedgerPerformanceProfile) { p.Telemetry.Traces = false }, "100% traces"},
		{"unmeasurable allocations", func(p *bootstrap.LedgerPerformanceProfile) { p.Telemetry.Metrics = false }, "allocation metrics"},
		{"unavailable TLS costs", func(p *bootstrap.LedgerPerformanceProfile) {
			p.Telemetry.Receiver = "external-grpc"
			p.Telemetry.ReceiverHost = "receiver.test"
			p.Telemetry.ReceiverPort = 4317
			p.Telemetry.ReceiverTLS = true
		}, "TLS telemetry is unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var candidate bootstrap.LedgerPerformanceProfile
			if err := json.Unmarshal(base, &candidate); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&candidate)
			writeProfile(candidate)
			if _, err := bootstrap.LoadLedgerPerformanceProfile(path); err == nil || !strings.Contains(err.Error(), tc.errorText) {
				t.Fatalf("invalid workload accepted or wrong error: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name      string
		mutate    func(map[string]any)
		errorText string
	}{
		{"unknown condition", func(p map[string]any) { p["compression"] = "gzip" }, "unknown field"},
		{"omitted false switch", func(p map[string]any) { delete(p["telemetry"].(map[string]any), "logs") }, "telemetry.logs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(base, &candidate); err != nil {
				t.Fatal(err)
			}
			tc.mutate(candidate)
			writeProfile(candidate)
			if _, err := bootstrap.LoadLedgerPerformanceProfile(path); err == nil || !strings.Contains(err.Error(), tc.errorText) {
				t.Fatalf("unreviewed condition accepted: %v", err)
			}
		})
	}

	t.Run("live service and scheduled mix", func(t *testing.T) {
		data := newLedgerPerformanceDataForProfile(nil, loaded)
		used := make(map[int]bool)
		for principal, value := range data.principals {
			if len(value) != loaded.PrincipalIDBytes {
				t.Fatal("principal padding does not match the declared event-size input")
			}
			serviceIndex := data.serviceIndex(principal)
			used[serviceIndex] = true
			live := data.services[serviceIndex]
			for index, entry := range data.grantedPermissionSets(principal) {
				for _, included := range entry.IncludedServiceIDs {
					if !slices.Contains(data.permissionSetServices[index], included) {
						t.Fatal("grant includes a service outside its permission set")
					}
				}
				if slices.Contains(data.permissionSetServices[index], live) && !slices.Contains(entry.IncludedServiceIDs, live) {
					t.Fatal("principal's live service is not granted")
				}
			}
		}
		if len(used) != 4 {
			t.Fatal("live actions do not exercise every configured service")
		}
		counts := make(map[string]int)
		for index := range 200 {
			counts[data.eventType(index)]++
		}
		for name, want := range map[string]int{"token-exchanged": 70, "approval-approved": 50, "grant-updated": 30, "session-refreshed": 40, "agent-updated": 10} {
			if counts[name] != want {
				t.Fatalf("%s scheduled %d actions, want %d", name, counts[name], want)
			}
		}
		reference := &ledgerPerformanceData{}
		counts = make(map[string]int)
		for index := range 100000 {
			counts[reference.eventType(index)]++
		}
		for name, want := range map[string]int{"token-exchanged": 50000, "approval-approved": 20000, "grant-updated": 10000, "session-refreshed": 10000, "agent-updated": 10000} {
			if counts[name] != want {
				t.Fatalf("reference %s scheduled %d actions, want %d", name, counts[name], want)
			}
		}
	})

	t.Run("host attestation", func(t *testing.T) {
		t.Setenv("AIB_LEDGER_PERFORMANCE_HOST_ATTESTATION", loaded.Hardware.Attestation)
		t.Setenv("AIB_LEDGER_PERFORMANCE_HOST_MEMORY_BYTES", strconv.FormatUint(loaded.Hardware.MemoryBytes, 10))
		if err := bootstrap.CheckLedgerPerformanceHost(loaded); err != nil {
			t.Fatal(err)
		}
		mismatch := *loaded
		mismatch.Hardware.CPUs++
		if err := bootstrap.CheckLedgerPerformanceHost(&mismatch); err == nil || !strings.Contains(err.Error(), "hardware mismatch") {
			t.Fatalf("wrong hardware accepted: %v", err)
		}
		t.Setenv("AIB_LEDGER_PERFORMANCE_HOST_ATTESTATION", "different-attestation")
		if err := bootstrap.CheckLedgerPerformanceHost(loaded); err == nil || !strings.Contains(err.Error(), "HOST_ATTESTATION") {
			t.Fatalf("wrong attestation accepted: %v", err)
		}
		t.Setenv("AIB_LEDGER_PERFORMANCE_HOST_ATTESTATION", loaded.Hardware.Attestation)
		t.Setenv("AIB_LEDGER_PERFORMANCE_HOST_MEMORY_BYTES", strconv.FormatUint(loaded.Hardware.MemoryBytes-1, 10))
		if err := bootstrap.CheckLedgerPerformanceHost(loaded); err == nil || !strings.Contains(err.Error(), "HOST_MEMORY_BYTES") {
			t.Fatalf("wrong memory accepted: %v", err)
		}
	})
	t.Run("external placement is not substituted", func(t *testing.T) {
		t.Setenv("AIB_LEDGER_PERFORMANCE_POSTGRES_ADMIN_URL", "")
		if _, err := bootstrap.NewExternalLedgerPerformancePostgres(context.Background(), loaded); err == nil || !strings.Contains(err.Error(), "AIB_LEDGER_PERFORMANCE_POSTGRES_ADMIN_URL") {
			t.Fatalf("missing deployment database was substituted: %v", err)
		}
		for _, connection := range []string{
			"postgres://admin:fixture@localhost:5433/postgres?sslmode=verify-full",
			"postgres://admin:fixture@fixture-db.test:5434/postgres?sslmode=verify-full",
			"postgres://admin:fixture@fixture-db.test:5433/postgres?sslmode=require",
		} {
			t.Setenv("AIB_LEDGER_PERFORMANCE_POSTGRES_ADMIN_URL", connection)
			if _, err := bootstrap.NewExternalLedgerPerformancePostgres(context.Background(), loaded); err == nil || !strings.Contains(err.Error(), "approved-host:port") {
				t.Fatalf("unapproved database placement accepted: %v", err)
			}
		}
		candidate := *loaded
		candidate.Telemetry.Receiver, candidate.Telemetry.ReceiverHost, candidate.Telemetry.ReceiverPort = "external-grpc", "receiver.test", 4317
		t.Setenv("AIB_LEDGER_PERFORMANCE_OTLP_ENDPOINT", "")
		if _, err := bootstrap.NewLedgerPerformanceCollectorForProfile(context.Background(), &candidate); err == nil || !strings.Contains(err.Error(), "AIB_LEDGER_PERFORMANCE_OTLP_ENDPOINT") {
			t.Fatalf("missing receiver was substituted: %v", err)
		}
		t.Setenv("AIB_LEDGER_PERFORMANCE_OTLP_ENDPOINT", "receiver.test:4318")
		if _, err := bootstrap.NewLedgerPerformanceCollectorForProfile(context.Background(), &candidate); err == nil || !strings.Contains(err.Error(), "approved receiver host:port") {
			t.Fatalf("unapproved receiver accepted: %v", err)
		}
	})
}

func TestLedgerPerformanceCollectorStagesAndPoolWaits(t *testing.T) {
	ctx := t.Context()
	collector, err := bootstrap.NewLedgerPerformanceCollectorForProfile(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(collector.Close)
	connection, err := grpc.DialContext(ctx, collector.Endpoint(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := collecttraces.NewTraceServiceClient(connection)
	traceID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	start := uint64(time.Now().UnixNano())
	span := func(name string, offset, duration time.Duration) *tracesv1.Span {
		return &tracesv1.Span{Name: name, TraceId: traceID, StartTimeUnixNano: start + uint64(offset), EndTimeUnixNano: start + uint64(offset+duration)}
	}
	export := func(spans ...*tracesv1.Span) {
		t.Helper()
		_, err := client.Export(ctx, &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracesv1.ResourceSpans{{ScopeSpans: []*tracesv1.ScopeSpans{{Spans: spans}}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	export(span("ledger.record.validate", 0, 2*time.Millisecond), span("ledger.record", 20*time.Millisecond, 3*time.Millisecond), span("ledger.transaction", 3*time.Millisecond, 40*time.Millisecond))
	got, err := collector.RecordingTimes([]string{hex.EncodeToString(traceID)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Total) != 1 || got.Validation[0] != 2*time.Millisecond || got.Append[0] != 3*time.Millisecond || got.Total[0] != 5*time.Millisecond || got.Transaction[0] != 40*time.Millisecond {
		t.Fatalf("recording stages included checkout or commit: %+v", got)
	}
	export(span("ledger.record", 50*time.Millisecond, time.Millisecond))
	if _, err := collector.RecordingTimes([]string{hex.EncodeToString(traceID)}); err == nil {
		t.Fatal("duplicate append span was accepted as one completed action")
	}
	metrics := collectmetrics.NewMetricsServiceClient(connection)
	emitPool := func(count int64, wait time.Duration) {
		t.Helper()
		point := func(value int64) *metricsv1.NumberDataPoint {
			return &metricsv1.NumberDataPoint{Value: &metricsv1.NumberDataPoint_AsInt{AsInt: value}}
		}
		gauge := func(name string, value int64) *metricsv1.Metric {
			return &metricsv1.Metric{Name: name, Data: &metricsv1.Metric_Gauge{Gauge: &metricsv1.Gauge{DataPoints: []*metricsv1.NumberDataPoint{point(value)}}}}
		}
		_, err := metrics.Export(ctx, &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricsv1.ResourceMetrics{{ScopeMetrics: []*metricsv1.ScopeMetrics{{Metrics: []*metricsv1.Metric{
			gauge("ledger.performance.pool.wait_count", count), gauge("ledger.performance.pool.wait_nanoseconds", int64(wait)),
		}}}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	emitPool(2, time.Millisecond)
	boundary := time.Now()
	emitPool(5, 9*time.Millisecond)
	pools, err := collector.PoolWaits(boundary, boundary)
	if err != nil {
		t.Fatal(err)
	}
	if pools.WaitCount != 3 || pools.WaitDuration != 8*time.Millisecond {
		t.Fatalf("wrong measured pool wait delta: %+v", pools)
	}
}
