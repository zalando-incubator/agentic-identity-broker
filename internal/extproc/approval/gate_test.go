package approval

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type readCall struct {
	principal string
	sessions  []string
}

type createCall struct {
	subjectToken string
	request      CreateRequest
}

type brokerStub struct {
	mu sync.Mutex

	pairs      []Pair
	etag       string
	readErr    error
	createURL  string
	createErr  error
	consumeErr error

	readDelay   time.Duration
	createDelay time.Duration

	readCalls     []readCall
	createCalls   []createCall
	consumeCalls  int
	consumeTokens []string
	clientTime    time.Duration
}

func (b *brokerStub) Read(ctx context.Context, principal string, sessions []string) ([]Pair, string, error) {
	started := time.Now()
	if err := waitForBrokerDelay(ctx, b.readDelay); err != nil {
		return nil, "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clientTime += time.Since(started)
	b.readCalls = append(b.readCalls, readCall{principal: principal, sessions: append([]string(nil), sessions...)})
	return b.pairs, b.etag, b.readErr
}

func (b *brokerStub) Create(ctx context.Context, subjectToken string, request CreateRequest) (string, error) {
	started := time.Now()
	if err := waitForBrokerDelay(ctx, b.createDelay); err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clientTime += time.Since(started)
	b.createCalls = append(b.createCalls, createCall{subjectToken: subjectToken, request: request})
	return b.createURL, b.createErr
}

func (b *brokerStub) Consume(_ context.Context, subjectToken, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consumeCalls++
	b.consumeTokens = append(b.consumeTokens, subjectToken)
	return b.consumeErr
}

func (b *brokerStub) calls() (reads []readCall, creates []createCall, consumes int, clientTime time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]readCall(nil), b.readCalls...), append([]createCall(nil), b.createCalls...), b.consumeCalls, b.clientTime
}

func waitForBrokerDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func testInvocation() Invocation {
	return Invocation{
		Identity:       Identity{Principal: "alice", AgentID: "agent"},
		ToolName:       "create_issue",
		Arguments:      map[string]any{"repo": "acme/app"},
		AgentSessionID: "agent-session",
		MCPSessionID:   "mcp-session",
		RequestID:      "request-id",
		SubjectToken:   "subject-token",
		RiskLevel:      "medium",
	}
}

func TestGateServesCachedApprovalWithoutBrokerRoundTrip(t *testing.T) {
	cache := NewCache(time.Minute, time.Minute)
	invocation := testInvocation()
	cache.Replace([]Pair{{
		Identity:  invocation.Identity,
		Approvals: []Record{approvedRecord("permanent", invocation.ToolName, map[string]string{"repo": "acme/*"}, "permanent", time.Now().UTC())},
	}}, `"v1"`)
	broker := &brokerStub{createURL: "https://broker.example/approval"}

	outcome := NewGate(cache, broker).Evaluate(context.Background(), invocation)
	assert.True(t, outcome.Proceed)
	assert.Empty(t, outcome.URL)
	assert.Empty(t, outcome.Reason)
	reads, creates, consumes, _ := broker.calls()
	assert.Empty(t, reads)
	assert.Empty(t, creates)
	assert.Zero(t, consumes)
}

func TestGateRefreshesAuthoritativelyWhenCacheIsStale(t *testing.T) {
	invocation := testInvocation()
	cache := NewCache(time.Minute, 50*time.Millisecond)
	cache.Replace([]Pair{{
		Identity:  invocation.Identity,
		Approvals: []Record{approvedRecord("permanent", invocation.ToolName, map[string]string{"repo": "acme/*"}, "permanent", time.Now().UTC())},
	}}, `"v1"`)
	time.Sleep(80 * time.Millisecond)
	broker := &brokerStub{readErr: errors.New("unavailable")}

	outcome := NewGate(cache, broker).Evaluate(context.Background(), invocation)

	assert.False(t, outcome.Proceed)
	assert.Empty(t, outcome.URL)
	assert.Equal(t, "approval state could not be refreshed", outcome.Reason)
	reads, creates, consumes, _ := broker.calls()
	assert.Len(t, reads, 1)
	assert.Empty(t, creates)
	assert.Zero(t, consumes)
}

func TestGateRefreshesAuthoritativelyBeforeCreating(t *testing.T) {
	invocation := testInvocation()
	broker := &brokerStub{pairs: []Pair{{
		Identity:  invocation.Identity,
		Approvals: []Record{approvedRecord("refreshed", invocation.ToolName, map[string]string{"repo": "acme/*"}, "permanent", time.Now().UTC())},
	}}, etag: `"v2"`, createURL: "https://broker.example/approval"}

	outcome := NewGate(NewCache(time.Minute, time.Minute), broker).Evaluate(context.Background(), invocation)
	require.True(t, outcome.Proceed)
	reads, creates, consumes, _ := broker.calls()
	require.Len(t, reads, 1)
	assert.Equal(t, "alice", reads[0].principal)
	assert.Equal(t, []string{"agent-session"}, reads[0].sessions)
	assert.Empty(t, creates)
	assert.Zero(t, consumes)
}

func TestGateFailsClosedWhenDependenciesOrRefreshAreUnavailable(t *testing.T) {
	invocation := testInvocation()
	tests := []struct {
		name   string
		gate   *Gate
		input  Invocation
		reason string
	}{
		{
			name:   "missing identity",
			gate:   NewGate(NewCache(time.Minute, time.Minute), &brokerStub{}),
			input:  Invocation{ToolName: "tool", Arguments: map[string]any{}, SubjectToken: "subject"},
			reason: "approval identity unavailable",
		},
		{
			name:   "missing subject token",
			gate:   NewGate(NewCache(time.Minute, time.Minute), &brokerStub{}),
			input:  Invocation{Identity: invocation.Identity, ToolName: "tool", Arguments: map[string]any{}},
			reason: "approval subject token unavailable",
		},
		{
			name:   "broker read fails",
			gate:   NewGate(NewCache(time.Minute, time.Minute), &brokerStub{readErr: errors.New("unavailable")}),
			input:  invocation,
			reason: "approval state could not be refreshed",
		},
		{
			name:   "gate is not configured",
			gate:   NewGate(nil, nil),
			input:  invocation,
			reason: "approval gate unavailable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome := test.gate.Evaluate(context.Background(), test.input)
			assert.False(t, outcome.Proceed)
			assert.Empty(t, outcome.URL)
			assert.Equal(t, test.reason, outcome.Reason)
		})
	}
}

func TestGateMapsRateLimitToRetryReason(t *testing.T) {
	broker := &brokerStub{createErr: &StatusError{Operation: "create", StatusCode: http.StatusTooManyRequests}}

	outcome := NewGate(NewCache(time.Minute, time.Minute), broker).Evaluate(context.Background(), testInvocation())

	require.False(t, outcome.Proceed)
	require.Empty(t, outcome.URL)
	require.Equal(t, "approval could not be initiated — broker rate limit; retry shortly", outcome.Reason)
}

func TestGateCreatesOnlyAfterConfirmedMiss(t *testing.T) {
	invocation := testInvocation()
	broker := &brokerStub{etag: `"v2"`, createURL: "https://broker.example/approval"}
	outcome := NewGate(NewCache(time.Minute, time.Minute), broker).Evaluate(context.Background(), invocation)

	require.Equal(t, "https://broker.example/approval", outcome.URL)
	assert.False(t, outcome.Proceed)
	reads, creates, consumes, _ := broker.calls()
	require.Len(t, reads, 1)
	require.Len(t, creates, 1)
	assert.Zero(t, consumes)
	assert.Equal(t, invocation.SubjectToken, creates[0].subjectToken)
	assert.Equal(t, invocation.ToolName, creates[0].request.ToolName)
	assert.Equal(t, invocation.Arguments, creates[0].request.Arguments)
	assert.Equal(t, invocation.MCPSessionID, creates[0].request.Metadata.MCPSessionID)
	assert.Equal(t, invocation.AgentSessionID, creates[0].request.Metadata.AgentSessionID)
	assert.Equal(t, invocation.RequestID, creates[0].request.Metadata.InvocationID)
	assert.Equal(t, invocation.RiskLevel, creates[0].request.RiskLevel)
	assert.Equal(t, "Approval required for create_issue", creates[0].request.Metadata.Description)
}

func TestGateNeverFabricatesElicitation(t *testing.T) {
	invocation := testInvocation()
	for _, test := range []struct {
		name   string
		broker *brokerStub
	}{
		{name: "create fails", broker: &brokerStub{createErr: errors.New("invalid subject token")}},
		{name: "create returns no URL", broker: &brokerStub{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome := NewGate(NewCache(time.Minute, time.Minute), test.broker).Evaluate(context.Background(), invocation)
			assert.False(t, outcome.Proceed)
			assert.Empty(t, outcome.URL)
			assert.Equal(t, "approval could not be initiated", outcome.Reason)
			_, creates, _, _ := test.broker.calls()
			assert.Len(t, creates, 1)
		})
	}
}

func TestGateReliesOnBrokerDedupForStaleCacheMisses(t *testing.T) {
	invocation := testInvocation()
	broker := &brokerStub{createURL: "https://broker.example/approval"}
	gate := NewGate(NewCache(time.Minute, time.Minute), broker)

	first := gate.Evaluate(context.Background(), invocation)
	second := gate.Evaluate(context.Background(), invocation)
	assert.Equal(t, first.URL, second.URL)
	reads, creates, _, _ := broker.calls()
	assert.Len(t, reads, 2)
	assert.Len(t, creates, 2)
}

func TestGateConsumesOnceApprovalBeforeProceedingAndReleasesOnFailure(t *testing.T) {
	invocation := testInvocation()
	cache := NewCache(time.Minute, time.Minute)
	cache.Replace([]Pair{{
		Identity:  invocation.Identity,
		Approvals: []Record{approvedRecord("once", invocation.ToolName, map[string]string{}, "once", time.Now().UTC())},
	}}, `"v1"`)
	broker := &brokerStub{consumeErr: errors.New("broker unavailable")}
	gate := NewGate(cache, broker)

	outcome := gate.Evaluate(context.Background(), invocation)
	assert.False(t, outcome.Proceed)
	assert.Equal(t, "approval could not be consumed", outcome.Reason)

	broker.mu.Lock()
	broker.consumeErr = nil
	broker.mu.Unlock()
	outcome = gate.Evaluate(context.Background(), invocation)
	assert.True(t, outcome.Proceed)
	_, _, consumes, _ := broker.calls()
	assert.Equal(t, 2, consumes)
	_, matched := cache.Match(invocation.Identity, invocation.AgentSessionID, invocation.ToolName, invocation.Arguments)
	assert.False(t, matched, "a confirmed consume must make the once approval permanently unmatchable")
}

func TestGateProcessingExcludesControllableBrokerDuration(t *testing.T) {
	invocation := testInvocation()
	broker := &brokerStub{
		createURL:   "https://broker.example/approval",
		readDelay:   25 * time.Millisecond,
		createDelay: 25 * time.Millisecond,
	}
	started := time.Now()
	outcome := NewGate(NewCache(time.Minute, time.Minute), broker).Evaluate(context.Background(), invocation)
	elapsed := time.Since(started)
	_, _, _, clientTime := broker.calls()

	require.Equal(t, "https://broker.example/approval", outcome.URL)
	processingTime := elapsed - clientTime
	assert.Less(t, processingTime, 500*time.Millisecond)
}

func TestGateEvaluateEmitsDecisionTelemetry(t *testing.T) {
	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	previousMeterProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(meterProvider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previousMeterProvider)
		_ = meterProvider.Shutdown(context.Background())
	})

	spanExporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	previousTracerProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tracerProvider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousTracerProvider)
		_ = tracerProvider.Shutdown(context.Background())
	})

	cachedInvocation := testInvocation()
	cached := NewCache(time.Minute, time.Minute)
	cached.Replace([]Pair{{
		Identity:  cachedInvocation.Identity,
		Approvals: []Record{approvedRecord("permanent", cachedInvocation.ToolName, map[string]string{"repo": "acme/*"}, "permanent", time.Now().UTC())},
	}}, `"v1"`)

	tests := []struct {
		name        string
		wantOutcome string
		gate        *Gate
		invocation  Invocation
	}{
		{
			name:        "proceed",
			wantOutcome: "proceed",
			gate:        NewGate(cached, &brokerStub{}),
			invocation:  cachedInvocation,
		},
		{
			name:        "elicit",
			wantOutcome: "elicit",
			gate:        NewGate(NewCache(time.Minute, time.Minute), &brokerStub{etag: `"v1"`, createURL: "https://broker.example/approval"}),
			invocation:  testInvocation(),
		},
		{
			name:        "deny",
			wantOutcome: "deny",
			gate:        NewGate(NewCache(time.Minute, time.Minute), &brokerStub{}),
			invocation:  Invocation{SubjectToken: "subject-token"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome := test.gate.Evaluate(context.Background(), test.invocation)
			switch test.wantOutcome {
			case "proceed":
				assert.True(t, outcome.Proceed)
			case "elicit":
				assert.NotEmpty(t, outcome.URL)
			case "deny":
				assert.NotEmpty(t, outcome.Reason)
			}
		})
	}

	var resourceMetrics metricdata.ResourceMetrics
	require.NoError(t, metricReader.Collect(context.Background(), &resourceMetrics))
	counterOutcomes := make(map[string]int64)
	var counterFound bool
	for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			if metric.Name != "extproc.approval.gate.decisions" {
				continue
			}
			counterFound = true
			sum, ok := metric.Data.(metricdata.Sum[int64])
			require.True(t, ok, "gate decisions must be an Int64 counter")
			for _, point := range sum.DataPoints {
				counterOutcomes[attributeValue(point.Attributes.ToSlice(), "outcome")] += point.Value
			}
		}
	}
	assert.True(t, counterFound, "gate decision counter must be exported with its exact name")
	assert.Equal(t, map[string]int64{"proceed": 1, "elicit": 1, "deny": 1}, counterOutcomes)

	require.NoError(t, tracerProvider.ForceFlush(context.Background()))
	spanOutcomes := make(map[string]int)
	for _, span := range spanExporter.GetSpans() {
		if span.Name == "extproc.approval.gate" {
			spanOutcomes[attributeValue(span.Attributes, "approval.outcome")]++
		}
	}
	assert.Equal(t, map[string]int{"proceed": 1, "elicit": 1, "deny": 1}, spanOutcomes)
}

func attributeValue(attributes []attribute.KeyValue, key string) string {
	for _, attribute := range attributes {
		if string(attribute.Key) == key {
			return attribute.Value.AsString()
		}
	}
	return ""
}
