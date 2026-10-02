//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	tokenexchange "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	integrationbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const ledgerPerformanceTypePrefix = "agentic-identity-broker."

type ledgerPerformanceT struct {
	integrationbootstrap.PostgresTestHandle
}

func (ledgerPerformanceT) Skip(args ...any)                 { Fail(fmt.Sprint(args...)) }
func (ledgerPerformanceT) Skipf(format string, args ...any) { Fail(fmt.Sprintf(format, args...)) }

type ledgerPerformanceData struct {
	principals    []string
	agents        []id.AgentID
	services      []id.ServiceID
	subjectTokens []string
	assertion     string
	upstream      *helpers.MockUpstreamOAuth2Server
}

func newLedgerPerformanceData(upstream *helpers.MockUpstreamOAuth2Server) *ledgerPerformanceData {
	data := &ledgerPerformanceData{
		principals:    make([]string, bootstrap.LedgerPerformancePrincipals),
		agents:        make([]id.AgentID, bootstrap.LedgerPerformanceAgents),
		services:      make([]id.ServiceID, bootstrap.LedgerPerformanceServices),
		subjectTokens: make([]string, bootstrap.LedgerPerformancePrincipals),
		upstream:      upstream,
	}
	data.services[0], data.services[1] = fixtures.PlaceholderServiceID, fixtures.SecondaryPlaceholderServiceID
	for i := 2; i < len(data.services); i++ {
		data.services[i] = id.NewServiceID()
	}
	for i := range data.agents {
		data.agents[i] = id.NewAgentID()
	}
	for i := range data.principals {
		data.principals[i] = fmt.Sprintf("ledger-perf-%05d@example.test", i)
	}
	return data
}

func (d *ledgerPerformanceData) prepareTokens() error {
	privateKey, issuer := d.upstream.GetPrivateKeyPEM(), d.upstream.URL()
	now := time.Now()
	assertion, err := helpers.SignTestJWT(map[string]any{
		"sub": "approval-gateway-client", "iss": issuer,
		"aud": []string{tokenexchange.DefaultBrokerAudience},
		"iat": now.Unix(), "exp": now.Add(24 * time.Hour).Unix(),
	}, privateKey)
	if err != nil {
		return err
	}
	d.assertion = assertion
	for i, principal := range d.principals {
		agentIndex := i % len(d.agents)
		subject, err := helpers.SignTestJWT(map[string]any{
			"sub": principal, "azp": d.agents[agentIndex].String(), "iss": issuer,
			"aud": []string{tokenexchange.DefaultBrokerAudience},
			"iat": now.Unix(), "exp": now.Add(24 * time.Hour).Unix(),
		}, privateKey)
		if err != nil {
			return err
		}
		d.subjectTokens[i] = subject
	}
	return nil
}

func (d *ledgerPerformanceData) serviceIndex(principal int) int {
	return (principal / len(d.agents)) % len(d.services)
}

// All business-state fixture rows are inserted through the actual storage
// adapter in separate PostgreSQL migration templates. The benchmark's events
// themselves are generated only by the production HTTP binary, never inserted
// through this fixture path.
func seedLedgerPerformanceBusinessState(ctx context.Context, databaseURL string, d *ledgerPerformanceData) error {
	config := &ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: databaseURL}, Timeouts: ports.StorageTimeouts{Read: 10 * time.Second, Write: 10 * time.Second}}
	store, err := storageadapter.NewAdapter(config)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close(context.Background()) }()
	for i, serviceID := range d.services {
		service := fixtures.GitHubService()
		service.ID = serviceID
		service.ClientID = id.ClientID(fmt.Sprintf("perf-service-%02d", i))
		service.DisplayName = fmt.Sprintf("Performance Service %02d", i)
		service.IssuerURI = d.upstream.URL()
		service.Discovery.EnableDiscovery = false
		service.Endpoints.TokenEndpoint = d.upstream.URL() + "/oauth/token"
		service.Endpoints.AuthorizeEndpoint = d.upstream.URL() + "/oauth/authorize"
		service.ProtectedResources = []string{fmt.Sprintf("https://resource-%02d.example.test", i)}
		service.Scopes = []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}}
		service.Secret = fixtures.EncryptedSecret(serviceID.String(), fixtures.LedgerCredentialCanaries().ClientSecret)
		if err := store.Services().Create(ctx, service); err != nil {
			return fmt.Errorf("seed service %d: %w", i, err)
		}
	}
	scopes := make([]domainstorage.ServiceScope, len(d.services))
	for i, serviceID := range d.services {
		scopes[i] = domainstorage.ServiceScope{ServiceID: serviceID, Scopes: []string{"read"}, RequirementType: domainstorage.RequirementTypeOptional}
	}
	if err := store.PermissionSets().Create(ctx, &domainstorage.PermissionSet{ID: fixtures.PlaceholderPermissionSetID, Name: "Ledger performance", ServiceScopes: scopes}); err != nil {
		return err
	}
	for i, agentID := range d.agents {
		agent := fixtures.ValidAgent()
		agent.ID = agentID
		clientID := id.ClientID(fmt.Sprintf("perf-agent-%02d", i))
		agent.ClientID = &clientID
		agent.AllowedScopes = []string{"read"}
		if err := store.Agents().Create(ctx, agent); err != nil {
			return fmt.Errorf("seed agent %d: %w", i, err)
		}
	}
	for i, principal := range d.principals {
		serviceID := d.services[d.serviceIndex(i)]
		grant := fixtures.ActiveGrant(principal, d.agents[i%len(d.agents)].String(), serviceID.String(), []string{"read"})
		until := time.Now().Add(12 * time.Hour)
		grant.ValidUntil = &until
		if err := store.UserGrants().Create(ctx, grant); err != nil {
			return fmt.Errorf("seed grant %d: %w", i, err)
		}
		sessionServices := []id.ServiceID{serviceID}
		if serviceID != fixtures.PlaceholderServiceID {
			sessionServices = append(sessionServices, fixtures.PlaceholderServiceID)
		}
		for _, sessionService := range sessionServices {
			session := fixtures.SessionForService(principal, sessionService.String())
			accessUntil, refreshUntil := time.Now().Add(12*time.Hour), time.Now().Add(24*time.Hour)
			session.AccessTokenExpiresAt, session.RefreshTokenExpiresAt = &accessUntil, &refreshUntil
			if err := store.UserSessions().Create(ctx, session); err != nil {
				return fmt.Errorf("seed session %d: %w", i, err)
			}
		}
	}
	return nil
}

func migrateLedgerPerformanceDatabase(ctx context.Context, databaseURL, checkout string) error {
	migrationFiles, err := filepath.Glob(filepath.Join(checkout, "migrations", "*.up.sql"))
	if err != nil {
		return err
	}
	if len(migrationFiles) == 0 {
		return errors.New("no baseline migrations in worktree")
	}
	sort.Strings(migrationFiles)
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint PRIMARY KEY, dirty boolean NOT NULL DEFAULT false)`); err != nil {
		return err
	}
	for _, migration := range migrationFiles {
		filename := filepath.Base(migration)
		prefix, _, ok := strings.Cut(filename, "_")
		if !ok {
			return fmt.Errorf("migration without version: %s", filename)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return fmt.Errorf("migration version %s: %w", filename, err)
		}
		content, err := os.ReadFile(migration)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, string(content), pgx.QueryExecModeSimpleProtocol); err != nil {
			return fmt.Errorf("apply %s: %w", filename, err)
		}
		if _, err := conn.Exec(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, false) ON CONFLICT DO NOTHING`, version); err != nil {
			return err
		}
	}
	return nil
}

func ledgerPerformanceTraceID(phase string, index int) string {
	value := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", phase, index)))
	return hex.EncodeToString(value[:16])
}

func ledgerPerformanceTraceparent(phase string, index int) string {
	value := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", phase, index)))
	return "00-" + hex.EncodeToString(value[:16]) + "-" + hex.EncodeToString(value[16:24]) + "-01"
}

func ledgerPerformanceEventType(index int) string {
	slot := (index/bootstrap.LedgerPerformancePrincipals + index%bootstrap.LedgerPerformancePrincipals) % 10
	switch slot {
	case 0, 1, 2, 3, 4:
		return "token-exchanged"
	case 5, 6:
		return "approval-approved"
	case 7:
		return "grant-updated"
	case 8:
		return "session-refreshed"
	default:
		return "agent-updated"
	}
}

type ledgerPerformanceRunner struct {
	broker *bootstrap.LedgerPerformanceBroker
	data   *ledgerPerformanceData
	client *http.Client
}

func newLedgerPerformanceRunner(broker *bootstrap.LedgerPerformanceBroker, data *ledgerPerformanceData) *ledgerPerformanceRunner {
	return &ledgerPerformanceRunner{broker: broker, data: data, client: &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 64}}}
}

func (r *ledgerPerformanceRunner) request(ctx context.Context, method, path, principal, contentType, phase string, index int, body []byte, headers map[string]string) (*http.Response, error) {
	base := r.broker.EndUserURL
	if strings.HasPrefix(path, "/api/agents/") {
		base = r.broker.AdminURL
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if principal != "" {
		req.Header.Set("X-Remote-User", principal)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Traceparent", ledgerPerformanceTraceparent(phase, index))
	req.Header.Set("User-Agent", "ledger-perf "+fixtures.LedgerCredentialCanaries().AccessToken)
	return r.client.Do(req)
}

func (r *ledgerPerformanceRunner) action(ctx context.Context, phase string, index int) (time.Duration, error) {
	principalIndex := index % len(r.data.principals)
	principal := r.data.principals[principalIndex]
	agent := r.data.agents[principalIndex%len(r.data.agents)]
	service := r.data.services[r.data.serviceIndex(principalIndex)]
	typeName := ledgerPerformanceEventType(index)
	var method, path, contentType string
	var body []byte
	var headers map[string]string
	var status int
	switch typeName {
	case "token-exchanged":
		method, path, contentType, status = http.MethodPost, "/oauth2/token", "application/x-www-form-urlencoded", http.StatusOK
		principal = "" // Public token exchange authenticates only the signed subject and client assertion.
		body = []byte(url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:token-exchange"}, "subject_token": {r.data.subjectTokens[principalIndex]}, "subject_token_type": {"urn:ietf:params:oauth:token-type:access_token"}, "requested_token_type": {"urn:ietf:params:oauth:token-type:access_token"}, "resource": {fmt.Sprintf("https://resource-%02d.example.test", r.data.serviceIndex(principalIndex))}, "client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"}, "client_assertion": {r.data.assertion}}.Encode())
	case "approval-approved":
		requestBody := []byte(`{"metadata":{"description":"Performance approval"},"tool_name":"benchmark_tool","arguments":{"marker":"safe"}}`)
		create, err := r.request(ctx, http.MethodPost, "/api/approvals", "", "application/json", "prepare-"+phase, index, requestBody, helpers.ApprovalCreateHeaders(r.data.subjectTokens[principalIndex], r.data.assertion, agent))
		if err != nil {
			return 0, err
		}
		if create.StatusCode != http.StatusCreated && create.StatusCode != http.StatusOK {
			_ = create.Body.Close()
			return 0, fmt.Errorf("prepare approval: HTTP %d", create.StatusCode)
		}
		var created helpers.CreateApprovalResponse
		if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
			_ = create.Body.Close()
			return 0, err
		}
		_ = create.Body.Close()
		if created.Data.ID == "" {
			return 0, errors.New("approval preparation returned no id")
		}
		method, path, contentType, status = http.MethodPost, "/api/approvals/"+created.Data.ID+"/approve", "application/json", http.StatusOK
		body = []byte(`{"persistence":"once"}`)
	case "grant-updated":
		method, path, contentType, status = http.MethodPost, "/api/consent/agents/"+agent.String()+"/grants", "application/json", http.StatusCreated
		included := []string{service.String()}
		if service != fixtures.PlaceholderServiceID {
			included = append(included, fixtures.PlaceholderServiceID.String())
		}
		var marshalErr error
		body, marshalErr = json.Marshal(map[string]any{
			"granted_permission_sets": map[string][]string{fixtures.PlaceholderPermissionSetID.String(): included},
			"valid_until":             time.Now().Add(12 * time.Hour).UTC().Format(time.RFC3339Nano),
		})
		if marshalErr != nil {
			return 0, marshalErr
		}
	case "session-refreshed":
		method, path, contentType, status = http.MethodPost, "/api/third-party/"+service.String()+"/session/refresh", "application/json", http.StatusOK
	case "agent-updated":
		method, path, contentType, status = http.MethodPut, "/api/agents/"+agent.String(), "application/json", http.StatusOK
		var marshalErr error
		body, marshalErr = json.Marshal(map[string]any{
			"display_name":    "Ledger Performance Agent",
			"description":     fmt.Sprintf("Performance mutation %s/%d", phase, index),
			"permission_sets": fixtures.DefaultPermissionSets(),
			"allowed_scopes":  []string{"read"},
			"redirect_uris":   []string{"https://client.example.com/cb"},
			"client_id":       fmt.Sprintf("perf-agent-%02d", principalIndex%len(r.data.agents)),
		})
		if marshalErr != nil {
			return 0, marshalErr
		}
	}
	start := time.Now()
	response, err := r.request(ctx, method, path, principal, contentType, phase, index, body, headers)
	if err != nil {
		return 0, err
	}
	if response.StatusCode != status {
		_ = response.Body.Close()
		return 0, fmt.Errorf("%s completed with HTTP %d, expected %d", typeName, response.StatusCode, status)
	}
	if err := bootstrap.DrainLedgerPerformanceResponse(response); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (r *ledgerPerformanceRunner) warm(ctx context.Context, phase string) (int, time.Time, time.Time, error) {
	started := time.Now()
	end := started.Add(bootstrap.LedgerPerformanceWarmup)
	var next atomic.Uint64
	var workers sync.WaitGroup
	errors := make(chan error, bootstrap.LedgerPerformanceConcurrency)
	for range bootstrap.LedgerPerformanceConcurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for time.Now().Before(end) && ctx.Err() == nil {
				index := int(next.Add(1) - 1)
				// Rotating category by principal interleaves the exact reference
				// mix even when a two-minute warmup ends mid-cycle.
				_, err := r.action(ctx, phase, index)
				if err != nil {
					errors <- err
					return
				}
			}
		}()
	}
	workers.Wait()
	finished := time.Now()
	close(errors)
	for err := range errors {
		if err != nil {
			return 0, started, finished, err
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, started, finished, err
	}
	return int(next.Load()), started, finished, nil
}

func (r *ledgerPerformanceRunner) measure(ctx context.Context, phase string) ([]time.Duration, time.Time, time.Time, int, error) {
	var next atomic.Uint64
	var workers sync.WaitGroup
	durations := make([]time.Duration, bootstrap.LedgerPerformanceMeasuredActions)
	errors := make(chan error, bootstrap.LedgerPerformanceConcurrency)
	start := time.Now()
	for range bootstrap.LedgerPerformanceConcurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				index := int(next.Add(1) - 1)
				if index >= len(durations) || ctx.Err() != nil {
					return
				}
				duration, err := r.action(ctx, phase, index)
				if err != nil {
					errors <- err
					return
				}
				durations[index] = duration
			}
		}()
	}
	workers.Wait()
	end := time.Now()
	close(errors)
	failures := 0
	var first error
	for err := range errors {
		failures++
		if first == nil {
			first = err
		}
	}
	if first != nil {
		return nil, start, end, failures, first
	}
	if ctx.Err() != nil {
		return nil, start, end, failures, ctx.Err()
	}
	for index, duration := range durations {
		if duration <= 0 {
			return nil, start, end, failures, fmt.Errorf("action %d did not complete", index)
		}
	}
	return durations, start, end, failures, nil
}

// Synthetic history must not substitute for recording a completed business action.
func preflightLedgerPerformance(ctx context.Context, r *ledgerPerformanceRunner, db *sqlx.DB, phase string) (*model.BusinessEvent, error) {
	const index = 9 * bootstrap.LedgerPerformancePrincipals
	if _, err := r.action(ctx, phase, index); err != nil {
		return nil, err
	}
	var table bool
	if err := db.GetContext(ctx, &table, "SELECT to_regclass('public.business_events') IS NOT NULL"); err != nil {
		return nil, err
	}
	if !table {
		return nil, errors.New("completed agent-updated but no retained event: public.business_events is absent")
	}
	started := time.Now()
	for action := range 10 {
		if _, err := r.action(ctx, phase, action); err != nil {
			return nil, err
		}
	}
	if _, _, err := verifyLedgerPerformanceEvents(ctx, db, r.data, phase, 10, started, time.Now()); err != nil {
		return nil, fmt.Errorf("workflow preflight: %w", err)
	}
	var envelopes [][]byte
	if err := db.SelectContext(ctx, &envelopes, `SELECT envelope FROM public.business_events WHERE envelope->>'trace_id'=$1`, ledgerPerformanceTraceID(phase, index)); err != nil {
		return nil, err
	}
	if len(envelopes) != 1 {
		return nil, fmt.Errorf("completed agent-updated retained %d expected events, want exactly one", len(envelopes))
	}
	if bytes.Contains(envelopes[0], []byte(fixtures.LedgerCredentialCanaries().AccessToken)) {
		return nil, errors.New("credential canary in preflight ledger envelope")
	}
	var event model.BusinessEvent
	if err := json.Unmarshal(envelopes[0], &event); err != nil {
		return nil, err
	}
	if event.Type != ledgerPerformanceTypePrefix+"agent-updated" || event.Subject != nil {
		return nil, errors.New("preflight retained wrong event type or subject")
	}
	return &event, nil
}

// History is synthetic safe data cloned from a real, previously validated
// production event. It is inserted by the migration owner, never by the broker,
// across 90 days of six-hour partitions. It cannot substitute for the live
// per-action completeness check in verifyLedgerPerformanceEvents.
func seedLedgerPerformanceHistory(ctx context.Context, db *sqlx.DB, databaseURL string, example *model.BusinessEvent) error {
	// Leave ten minutes inside the 90-day retention boundary during fixture
	// creation; the distributed history still spans every retained age bucket.
	const headroom = 10 * time.Minute
	base := time.Now().UTC().Add(-90*24*time.Hour + headroom).Truncate(time.Microsecond)
	period := (90*24*time.Hour - headroom) / bootstrap.LedgerPerformanceHistory
	last := base.Add(time.Duration(bootstrap.LedgerPerformanceHistory-1) * period).Truncate(6 * time.Hour)
	for partition := base.Truncate(6 * time.Hour); !partition.After(last); partition = partition.Add(6 * time.Hour) {
		if _, err := db.ExecContext(ctx, "SELECT public.business_event_create_partition_pair($1)", partition); err != nil {
			return fmt.Errorf("create historical partition pair: %w", err)
		}
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var subject any
	if example.Subject != nil {
		subject = example.Subject.String()
	}
	values := make([]any, 7)
	index := 0
	source := pgx.CopyFromFunc(func() ([]any, error) {
		if index == bootstrap.LedgerPerformanceHistory {
			return nil, nil
		}
		at := base.Add(time.Duration(index) * period).Truncate(time.Microsecond)
		seed := *example
		seed.ID, seed.RecordedAt, seed.OccurredAt = id.NewBusinessEventID(), at, at
		seed.TraceID, seed.SpanID = "", ""
		serialized, err := json.Marshal(&seed)
		if err != nil {
			return nil, err
		}
		values[0], values[1], values[2] = at, seed.ID.String(), at
		values[3], values[4], values[5], values[6] = seed.Type, string(seed.Outcome), subject, string(serialized)
		index++
		return values, nil
	})
	inserted, err := conn.CopyFrom(ctx, pgx.Identifier{"public", "business_events"}, []string{"recorded_at", "id", "occurred_at", "type", "outcome", "subject", "envelope"}, source)
	if err != nil {
		return fmt.Errorf("insert safe historical events: %w", err)
	}
	if inserted != bootstrap.LedgerPerformanceHistory {
		return fmt.Errorf("inserted %d historical events, want %d", inserted, bootstrap.LedgerPerformanceHistory)
	}
	var count int
	if err := db.GetContext(ctx, &count, "SELECT count(*) FROM public.business_events"); err != nil {
		return err
	}
	if count != bootstrap.LedgerPerformanceHistory {
		return fmt.Errorf("historical history count %d, want %d", count, bootstrap.LedgerPerformanceHistory)
	}
	return nil
}

func verifyLedgerPerformanceEvents(ctx context.Context, db *sqlx.DB, d *ledgerPerformanceData, phase string, actions int, start, end time.Time) (int, int, error) {
	rows, err := db.QueryxContext(ctx, `SELECT envelope FROM public.business_events WHERE recorded_at >= $1 AND recorded_at <= $2`, start.UTC().Add(-time.Second), end.UTC().Add(time.Second))
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	type expectedAction struct {
		index       int
		preparation bool
	}
	seen, seenPreparations := make([]bool, actions), make([]bool, actions)
	indexForTrace := make(map[string]expectedAction, actions+actions/5)
	preparationCount := 0
	for index := range seen {
		indexForTrace[ledgerPerformanceTraceID(phase, index)] = expectedAction{index: index}
		if ledgerPerformanceEventType(index) == "approval-approved" {
			indexForTrace[ledgerPerformanceTraceID("prepare-"+phase, index)] = expectedAction{index: index, preparation: true}
			preparationCount++
		}
	}
	canaries := fixtures.LedgerCredentialCanaries()
	fixedCanaries := []string{canaries.AccessToken, canaries.RefreshToken, canaries.ClientSecret, canaries.ClientAssertion, canaries.RawJWT, canaries.AuthorizationCode, canaries.PKCEVerifier}
	minSize, maxSize := int(^uint(0)>>1), 0
	count, prepared := 0, 0
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, 0, err
		}
		var event model.BusinessEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return 0, 0, err
		}
		expected, ok := indexForTrace[event.TraceID]
		if !ok {
			continue
		} // Other phases have independent trace IDs.
		index := expected.index
		if expected.preparation && seenPreparations[index] || !expected.preparation && seen[index] {
			return 0, 0, fmt.Errorf("duplicate retained event for completed catalogue action %d", index)
		}
		wantType := ledgerPerformanceEventType(index)
		if expected.preparation {
			wantType = "approval-requested"
		}
		if event.Type != ledgerPerformanceTypePrefix+wantType {
			return 0, 0, fmt.Errorf("action %d retained wrong type %s", index, event.Type)
		}
		principal := index % len(d.principals)
		if wantType == "agent-updated" {
			if event.Subject != nil {
				return 0, 0, fmt.Errorf("action %d retained unexpected subject", index)
			}
		} else if event.Subject == nil || event.Subject.String() != d.principals[principal] {
			return 0, 0, fmt.Errorf("action %d retained wrong principal", index)
		}
		wantAgent := d.agents[principal%len(d.agents)]
		if wantType == "session-refreshed" {
			wantAgent = id.AgentID{}
			if event.ServiceID != d.services[d.serviceIndex(principal)] || event.SessionID.IsZero() {
				return 0, 0, fmt.Errorf("refresh %d lost its service or session", index)
			}
			if event.Actor.Kind != "user" || event.Actor.ID == nil || *event.Actor.ID != d.principals[principal] || event.Actor.OnBehalfOf != nil {
				return 0, 0, fmt.Errorf("refresh %d retained wrong initiating user", index)
			}
		}
		if event.AgentID != wantAgent {
			return 0, 0, fmt.Errorf("action %d retained wrong agent", index)
		}
		if wantType == "token-exchanged" {
			if event.Actor.ID == nil || *event.Actor.ID != "approval-gateway-client" {
				return 0, 0, fmt.Errorf("exchange %d lost its verified initiating gateway", index)
			}
			if event.Actor.OnBehalfOf == nil || event.Actor.OnBehalfOf.String() != d.principals[principal] {
				return 0, 0, fmt.Errorf("exchange %d lost established delegation", index)
			}
		}
		for _, secret := range fixedCanaries {
			if bytes.Contains(raw, []byte(secret)) {
				return 0, 0, fmt.Errorf("action %d retained credential canary", index)
			}
		}
		if bytes.Contains(raw, []byte(d.subjectTokens[principal])) || bytes.Contains(raw, []byte(d.assertion)) {
			return 0, 0, fmt.Errorf("action %d retained signed credential", index)
		}
		if expected.preparation {
			seenPreparations[index], prepared = true, prepared+1
		} else {
			if len(raw) < minSize {
				minSize = len(raw)
			}
			if len(raw) > maxSize {
				maxSize = len(raw)
			}
			seen[index], count = true, count+1
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	if count != actions || prepared != preparationCount {
		return 0, 0, fmt.Errorf("retained %d/%d measured and %d/%d preparation catalogue events", count, actions, prepared, preparationCount)
	}
	return minSize, maxSize, nil
}

func awaitLedgerPerformanceMetrics(collector *bootstrap.LedgerPerformanceCollector, start, end time.Time, phase string) (bootstrap.LedgerPerformanceAllocation, []time.Duration, error) {
	traceIDs := make([]string, bootstrap.LedgerPerformanceMeasuredActions)
	for index := range traceIDs {
		traceIDs[index] = ledgerPerformanceTraceID(phase, index)
	}
	deadline := time.Now().Add(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for time.Now().Before(deadline) {
		allocation, aerr := collector.Allocations(start, end)
		spans, serr := collector.RecordingTimes(traceIDs)
		if aerr == nil && serr == nil {
			return allocation, spans, nil
		}
		last = errors.Join(aerr, serr)
		<-ticker.C
	}
	return bootstrap.LedgerPerformanceAllocation{}, nil, fmt.Errorf("measured allocation or ledger.record span unavailable: %w", last)
}

// Raw nanosecond samples are persisted outside the checkout for T095's
// measured report; no credential-bearing request or event body is written.
type ledgerPerformanceReport struct {
	Profile                       string          `json:"profile"`
	Version                       string          `json:"version"`
	Revision                      string          `json:"baseline_revision"`
	FeatureSource                 string          `json:"feature_revision"`
	Repetition                    int             `json:"repetition"`
	PostgresVersion               string          `json:"postgres_version"`
	Hardware                      string          `json:"hardware"`
	Principals                    int             `json:"principals"`
	Agents                        int             `json:"agents"`
	Services                      int             `json:"services"`
	Concurrency                   int             `json:"concurrency"`
	MeasuredActions               int             `json:"measured_actions"`
	PreparationActions            int             `json:"untimed_approval_preparations"`
	VerifiedMeasuredEvents        int             `json:"verified_measured_events,omitempty"`
	VerifiedApprovalPreparations  int             `json:"verified_approval_preparations,omitempty"`
	VerifiedWarmupEvents          int             `json:"verified_warmup_events,omitempty"`
	RetainedEventCanariesAbsent   bool            `json:"retained_event_canaries_absent,omitempty"`
	WarmupActions                 int             `json:"warmup_actions"`
	RetainedHistory               int             `json:"retained_history"`
	WarmupSeconds                 int             `json:"warmup_seconds"`
	Mix                           string          `json:"mix"`
	Pool                          string          `json:"database_pool"`
	Telemetry                     string          `json:"telemetry"`
	Started                       time.Time       `json:"started"`
	Ended                         time.Time       `json:"ended"`
	TransactionLatencyNanoseconds []time.Duration `json:"transaction_latency_nanoseconds"`
	RecordingSpanNanoseconds      []time.Duration `json:"recording_span_nanoseconds,omitempty"`
	TransactionP50Nanoseconds     time.Duration   `json:"transaction_p50_nanoseconds"`
	TransactionP95Nanoseconds     time.Duration   `json:"transaction_p95_nanoseconds"`
	TransactionP99Nanoseconds     time.Duration   `json:"transaction_p99_nanoseconds"`
	RecordingP99Nanoseconds       time.Duration   `json:"recording_p99_nanoseconds,omitempty"`
	EventBytesMin                 int             `json:"event_bytes_min"`
	EventBytesMax                 int             `json:"event_bytes_max"`
	ThroughputPerSecond           float64         `json:"throughput_per_second"`
	AllocationBytes               uint64          `json:"allocation_bytes"`
	AllocationObjects             uint64          `json:"allocation_objects"`
	HTTPErrors                    int             `json:"http_errors"`
	PostgresRollbacks             int             `json:"postgres_rollbacks"`
}

func retainLedgerPerformanceReport(directory string, report ledgerPerformanceReport) (string, error) {
	path := filepath.Join(directory, fmt.Sprintf("reference-repetition-%d-%s.json", report.Repetition, report.Version))
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	writeErr := json.NewEncoder(output).Encode(report)
	closeErr := output.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

var _ = Describe("Business event ledger performance", Label("business-event-ledger", "performance"), func() {
	Context("under the fixed reference workload with PostgreSQL and a pinned pre-feature binary", func() {
		// US4-AS4 from specs/048-business-event-ledger/spec.md.
		It("retains every completed action and limits paired recording overhead", Serial, func(ctx SpecContext) {
			fmt.Fprintf(GinkgoWriter, "reference profile revision=%s principals=%d agents=%d services=%d history=%d concurrency=%d warmup=%s measured_actions=%d repetitions=%d mix=50%%exchange,20%%approval-transition,10%%grant-update,10%%session-refresh,10%%admin-mutation\n", bootstrap.LedgerPerformanceBaselineRevision, bootstrap.LedgerPerformancePrincipals, bootstrap.LedgerPerformanceAgents, bootstrap.LedgerPerformanceServices, bootstrap.LedgerPerformanceHistory, bootstrap.LedgerPerformanceConcurrency, bootstrap.LedgerPerformanceWarmup, bootstrap.LedgerPerformanceMeasuredActions, bootstrap.LedgerPerformanceRepetitions)
			root, err := integrationbootstrap.FindProjectRoot()
			Expect(err).NotTo(HaveOccurred())
			temp := GinkgoT().TempDir()
			binaries, err := bootstrap.BuildLedgerPerformanceBinaries(ctx, root, temp)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(binaries.Close()).To(Succeed()) })
			fmt.Fprintf(GinkgoWriter, "reference binaries: baseline=%s feature=%s\n", bootstrap.LedgerPerformanceBaselineRevision, binaries.FeatureSource)
			pg := integrationbootstrap.RequireSharedPostgres(ledgerPerformanceT{GinkgoT()})
			canaries := fixtures.LedgerCredentialCanaries()
			upstream := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse().WithAccessToken(canaries.AccessToken).WithRefreshToken(canaries.RefreshToken)
			DeferCleanup(upstream.Close)
			data := newLedgerPerformanceData(upstream)
			Expect(data.prepareTokens()).To(Succeed())
			featureTemplate := fmt.Sprintf("ledger_perf_feature_%s", bootstrap.LedgerPerformanceBaselineRevision[:8])
			baselineTemplate := fmt.Sprintf("ledger_perf_baseline_%s", bootstrap.LedgerPerformanceBaselineRevision[:8])
			seed := func(migrationRoot string) func(string) {
				return func(name string) {
					url := pg.ConnectionString(name)
					Expect(migrateLedgerPerformanceDatabase(ctx, url, migrationRoot)).To(Succeed())
					Expect(seedLedgerPerformanceBusinessState(ctx, url, data)).To(Succeed())
				}
			}
			featureURL, featureRuntimeURL, featureDrop, err := bootstrap.OpenLedgerPerformanceDatabase(ctx, ledgerPerformanceT{GinkgoT()}, pg, featureTemplate, seed(root))
			Expect(err).NotTo(HaveOccurred())
			featureDB, err := sqlx.ConnectContext(ctx, "pgx", featureURL)
			Expect(err).NotTo(HaveOccurred())
			var closePreflight sync.Once
			cleanupPreflight := func() { closePreflight.Do(func() { _ = featureDB.Close(); featureDrop() }) }
			DeferCleanup(cleanupPreflight)
			collector, err := bootstrap.NewLedgerPerformanceCollector()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(collector.Close)
			preflightDir := filepath.Join(temp, "preflight")
			Expect(os.Mkdir(preflightDir, 0700)).To(Succeed())
			feature, err := bootstrap.StartLedgerPerformanceBroker(ctx, binaries.Feature, preflightDir, featureRuntimeURL, upstream.URL(), collector.Endpoint())
			Expect(err).NotTo(HaveOccurred())
			var stopPreflight sync.Once
			closeFeature := func() { stopPreflight.Do(func() { Expect(feature.Close()).To(Succeed()) }) }
			DeferCleanup(closeFeature)
			prototype, err := preflightLedgerPerformance(ctx, newLedgerPerformanceRunner(feature, data), featureDB, "preflight")
			Expect(err).NotTo(HaveOccurred())
			closeFeature()
			cleanupPreflight()
			historyTemplate := featureTemplate + "_history"
			seedHistory := func(name string) {
				seed(root)(name)
				url := pg.ConnectionString(name)
				owner, openErr := sqlx.ConnectContext(ctx, "pgx", url)
				Expect(openErr).NotTo(HaveOccurred())
				defer owner.Close()
				Expect(seedLedgerPerformanceHistory(ctx, owner, url, prototype)).To(Succeed())
			}
			// The feature template's million real-table rows are prepared once;
			// each run clones that history before its identical two-minute warmup.
			baselineURL, _, baselineDrop, err := bootstrap.OpenLedgerPerformanceDatabase(ctx, ledgerPerformanceT{GinkgoT()}, pg, baselineTemplate, seed(binaries.BaselineCheckout()))
			Expect(err).NotTo(HaveOccurred())
			var closeBaseline sync.Once
			cleanupBaseline := func() { closeBaseline.Do(baselineDrop) }
			DeferCleanup(cleanupBaseline)
			baselineDB, err := sqlx.ConnectContext(ctx, "pgx", baselineURL)
			Expect(err).NotTo(HaveOccurred())
			var baselineHasEvents bool
			Expect(baselineDB.GetContext(ctx, &baselineHasEvents, "SELECT to_regclass('public.business_events') IS NOT NULL")).To(Succeed())
			Expect(baselineHasEvents).To(BeFalse(), "baseline must not have the ledger table")
			Expect(baselineDB.Close()).To(Succeed())
			cleanupBaseline()
			resultsDirectory := os.Getenv("AIB_LEDGER_PERFORMANCE_RESULTS_DIR")
			if resultsDirectory == "" {
				resultsDirectory, err = os.MkdirTemp("", "aib-ledger-performance-")
			} else {
				err = os.MkdirAll(resultsDirectory, 0700)
			}
			Expect(err).NotTo(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "reference raw latency/recording samples: %s (nanoseconds)\n", resultsDirectory)
			for repetition := 0; repetition < bootstrap.LedgerPerformanceRepetitions; repetition++ {
				var pair [2]bootstrap.LedgerPerformanceStats
				order := []bool{false, true}
				if repetition%2 == 1 {
					order[0], order[1] = true, false
				}
				for _, enabled := range order {
					version, template, binary, provision := "baseline", baselineTemplate, binaries.Baseline, seed(binaries.BaselineCheckout())
					if enabled {
						version, template, binary, provision = "feature", historyTemplate, binaries.Feature, seedHistory
					}
					ownerURL, runtimeURL, drop, openErr := bootstrap.OpenLedgerPerformanceDatabase(ctx, ledgerPerformanceT{GinkgoT()}, pg, template, provision)
					Expect(openErr).NotTo(HaveOccurred())
					func() {
						defer drop()
						db, openErr := sqlx.ConnectContext(ctx, "pgx", ownerURL)
						Expect(openErr).NotTo(HaveOccurred())
						defer db.Close()
						var postgresVersion string
						Expect(db.GetContext(ctx, &postgresVersion, "SHOW server_version")).To(Succeed())
						var initialHistory int
						if enabled {
							Expect(db.GetContext(ctx, &initialHistory, "SELECT count(*) FROM public.business_events")).To(Succeed())
							Expect(initialHistory).To(Equal(bootstrap.LedgerPerformanceHistory), "feature clone must retain the full 90-day historical dataset")
						}
						fmt.Fprintf(GinkgoWriter, "reference repetition=%d version=%s postgres=%s Go=%s/%s cpu=%d gomaxprocs=%d db_pool_open=25 db_pool_idle=5 network=local-HTTP-and-container-port telemetry=grpc-traces-logs-metrics-on ledger_copy=on initial_history=%d\n", repetition+1, version, postgresVersion, runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0), initialHistory)
						otlp, openErr := bootstrap.NewLedgerPerformanceCollector()
						Expect(openErr).NotTo(HaveOccurred())
						defer otlp.Close()
						folder := filepath.Join(temp, fmt.Sprintf("%s-%d", version, repetition))
						Expect(os.Mkdir(folder, 0700)).To(Succeed())
						server, openErr := bootstrap.StartLedgerPerformanceBroker(ctx, binary, folder, runtimeURL, upstream.URL(), otlp.Endpoint())
						Expect(openErr).NotTo(HaveOccurred())
						defer func() { Expect(server.Close()).To(Succeed()) }()
						runner := newLedgerPerformanceRunner(server, data)
						phase := fmt.Sprintf("%s-%d", version, repetition)
						warmCount, warmStart, warmEnd, warmErr := runner.warm(ctx, "warm-"+phase)
						Expect(warmErr).NotTo(HaveOccurred())
						fmt.Fprintf(GinkgoWriter, "reference repetition=%d version=%s completed_warmup_actions=%d\n", repetition+1, version, warmCount)
						var rollbacksBefore int64
						Expect(db.GetContext(ctx, &rollbacksBefore, "SELECT xact_rollback FROM pg_stat_database WHERE datname=current_database()")).To(Succeed())
						durations, started, ended, failures, runErr := runner.measure(ctx, "measure-"+phase)
						Expect(runErr).NotTo(HaveOccurred())
						var allocation bootstrap.LedgerPerformanceAllocation
						var recordings []time.Duration
						var minSize, maxSize int
						var verifiedMeasured, verifiedPreparations, verifiedWarmup int
						var recordingP99 time.Duration
						if enabled {
							var verifyErr error
							minSize, maxSize, verifyErr = verifyLedgerPerformanceEvents(ctx, db, data, "measure-"+phase, bootstrap.LedgerPerformanceMeasuredActions, started, ended)
							var retained int
							Expect(db.GetContext(ctx, &retained, "SELECT count(*) FROM public.business_events")).To(Succeed())
							Expect(retained).To(BeNumerically(">=", bootstrap.LedgerPerformanceHistory+bootstrap.LedgerPerformanceMeasuredActions), "retained history and every measured action")
							Expect(verifyErr).NotTo(HaveOccurred())
							verifiedMeasured = len(durations)
							verifiedPreparations = bootstrap.LedgerPerformanceMeasuredActions / 5
							_, _, verifyErr = verifyLedgerPerformanceEvents(ctx, db, data, "warm-"+phase, warmCount, warmStart, warmEnd)
							Expect(verifyErr).NotTo(HaveOccurred(), "every completed warmup and preparation action must also retain its event")
							verifiedWarmup = warmCount
							allocation, recordings, verifyErr = awaitLedgerPerformanceMetrics(otlp, started, ended, "measure-"+phase)
							Expect(verifyErr).NotTo(HaveOccurred())
							span := bootstrap.SummarizeLedgerPerformance(recordings, ended.Sub(started), bootstrap.LedgerPerformanceAllocation{}, 0, 0)
							recordingP99 = span.P99
							fmt.Fprintf(GinkgoWriter, "reference repetition=%d version=%s event_bytes_min=%d event_bytes_max=%d history=%d recording_p99=%s\n", repetition+1, version, minSize, maxSize, bootstrap.LedgerPerformanceHistory, span.P99)
						} else {
							var verifyErr error
							allocation, verifyErr = func() (bootstrap.LedgerPerformanceAllocation, error) {
								deadline := time.Now().Add(15 * time.Second)
								ticker := time.NewTicker(100 * time.Millisecond)
								defer ticker.Stop()
								for time.Now().Before(deadline) {
									if value, err := otlp.Allocations(started, ended); err == nil {
										return value, nil
									}
									<-ticker.C
								}
								return bootstrap.LedgerPerformanceAllocation{}, errors.New("baseline allocation samples unavailable")
							}()
							Expect(verifyErr).NotTo(HaveOccurred())
						}
						var rollbacksAfter int64
						Expect(db.GetContext(ctx, &rollbacksAfter, "SELECT xact_rollback FROM pg_stat_database WHERE datname=current_database()")).To(Succeed())
						index := 0
						if enabled {
							index = 1
						}
						pair[index] = bootstrap.SummarizeLedgerPerformance(durations, ended.Sub(started), allocation, failures, int(rollbacksAfter-rollbacksBefore))
						fmt.Fprintf(GinkgoWriter, "reference repetition=%d version=%s p50=%s p95=%s p99=%s throughput=%.1f/s alloc_bytes=%d alloc_objects=%d http_errors=%d pg_rollbacks=%d\n", repetition+1, version, pair[index].P50, pair[index].P95, pair[index].P99, pair[index].Throughput, allocation.Bytes, allocation.Objects, failures, pair[index].Rollbacks)
						report := ledgerPerformanceReport{
							Profile: "fixed-reference", Version: version, Revision: bootstrap.LedgerPerformanceBaselineRevision, FeatureSource: binaries.FeatureSource,
							Repetition: repetition + 1, PostgresVersion: postgresVersion,
							Hardware:   fmt.Sprintf("%s/%s cpus=%d gomaxprocs=%d client/broker=loopback postgres=mapped-container-port", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0)),
							Principals: bootstrap.LedgerPerformancePrincipals, Agents: bootstrap.LedgerPerformanceAgents, Services: bootstrap.LedgerPerformanceServices,
							Concurrency: bootstrap.LedgerPerformanceConcurrency, MeasuredActions: bootstrap.LedgerPerformanceMeasuredActions,
							VerifiedMeasuredEvents: verifiedMeasured, VerifiedApprovalPreparations: verifiedPreparations,
							VerifiedWarmupEvents: verifiedWarmup, RetainedEventCanariesAbsent: enabled,
							RetainedHistory: initialHistory, WarmupSeconds: int(bootstrap.LedgerPerformanceWarmup.Seconds()),
							WarmupActions: warmCount, PreparationActions: bootstrap.LedgerPerformanceMeasuredActions / 5,
							Mix:  "50% exchanges, 20% approval transitions, 10% grant updates, 10% session refreshes, 10% admin mutations",
							Pool: "production adapter: 25 open, 5 idle", Telemetry: "OTLP gRPC traces/logs/metrics enabled; local acknowledging receiver; copy enabled",
							Started: started, Ended: ended, TransactionLatencyNanoseconds: durations,
							RecordingSpanNanoseconds: recordings, RecordingP99Nanoseconds: recordingP99,
							TransactionP50Nanoseconds: pair[index].P50, TransactionP95Nanoseconds: pair[index].P95, TransactionP99Nanoseconds: pair[index].P99,
							EventBytesMin: minSize, EventBytesMax: maxSize, ThroughputPerSecond: pair[index].Throughput,
							AllocationBytes: allocation.Bytes, AllocationObjects: allocation.Objects, HTTPErrors: failures, PostgresRollbacks: pair[index].Rollbacks,
						}
						path, reportErr := retainLedgerPerformanceReport(resultsDirectory, report)
						Expect(reportErr).NotTo(HaveOccurred())
						fmt.Fprintf(GinkgoWriter, "reference measured distribution saved: %s\n", path)
						if enabled {
							Expect(recordingP99).To(BeNumerically("<=", 5*time.Millisecond), "recording span p99")
						}
					}()
				}
				overhead := pair[1].P99 - pair[0].P99 // Difference of distribution percentiles; not a paired per-request percentile.
				fmt.Fprintf(GinkgoWriter, "reference repetition=%d transaction_p99_added=%s\n", repetition+1, overhead)
				Expect(overhead).To(BeNumerically("<=", 5*time.Millisecond))
			}
			// An unavailable approved current-load profile is an explicit release
			// blocker. This reference run cannot satisfy T095 in its place.
			fmt.Fprintln(GinkgoWriter, "reference profile only; T095 release acceptance separately requires the approved deployment profile and its measured run")
		})
	})
})
