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
	"slices"
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
	principals            []string
	agents                []id.AgentID
	services              []id.ServiceID
	subjectTokens         []string
	assertion             string
	upstream              *helpers.MockUpstreamOAuth2Server
	profile               *bootstrap.LedgerPerformanceProfile
	permissionSets        []id.PermissionSetID
	permissionSetServices [][]id.ServiceID
	agentPermissions      []domainstorage.AgentPermissionSetEntry
}

func newLedgerPerformanceDataForProfile(upstream *helpers.MockUpstreamOAuth2Server, profile *bootstrap.LedgerPerformanceProfile) *ledgerPerformanceData {
	principals, agents, services := bootstrap.LedgerPerformancePrincipals, bootstrap.LedgerPerformanceAgents, bootstrap.LedgerPerformanceServices
	if profile != nil {
		principals, agents, services = profile.Principals, profile.Agents, profile.Services
	}
	data := &ledgerPerformanceData{
		principals:    make([]string, principals),
		agents:        make([]id.AgentID, agents),
		services:      make([]id.ServiceID, services),
		subjectTokens: make([]string, principals),
		upstream:      upstream,
		profile:       profile,
	}
	data.services[0], data.services[1] = fixtures.PlaceholderServiceID, fixtures.SecondaryPlaceholderServiceID
	for i := 2; i < len(data.services); i++ {
		data.services[i] = id.NewServiceID()
	}
	for i := range data.agents {
		data.agents[i] = id.NewAgentID()
	}
	for i := range data.principals {
		principal := fmt.Sprintf("ledger-perf-%05d@example.test", i)
		if profile != nil {
			local, domain, _ := strings.Cut(principal, "@")
			principal = local + strings.Repeat("x", profile.PrincipalIDBytes-len(principal)) + "@" + domain
		}
		data.principals[i] = principal
	}
	if profile != nil {
		data.permissionSets = make([]id.PermissionSetID, len(profile.PermissionSetServiceCounts))
		data.permissionSetServices = make([][]id.ServiceID, len(profile.PermissionSetServiceCounts))
		data.agentPermissions = make([]domainstorage.AgentPermissionSetEntry, len(profile.PermissionSetServiceCounts))
		start := 0
		for i, count := range profile.PermissionSetServiceCounts {
			permissionID := id.NewPermissionSetID()
			data.permissionSets[i] = permissionID
			scoped := make([]id.ServiceID, count)
			for offset := range scoped {
				scoped[offset] = data.services[(start+offset)%len(data.services)]
			}
			data.permissionSetServices[i] = scoped
			requirement := domainstorage.RequirementTypeOptional
			if i == 0 {
				requirement = domainstorage.RequirementTypeMandatory
			}
			data.agentPermissions[i] = domainstorage.AgentPermissionSetEntry{PermissionSetID: permissionID, RequirementType: requirement}
			start += count
		}
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
	if d.profile != nil {
		return principal * len(d.services) / len(d.principals)
	}
	return (principal / len(d.agents)) % len(d.services)
}

func (d *ledgerPerformanceData) eventType(index int) string {
	if d.profile == nil {
		return ledgerPerformanceEventType(index)
	}
	slot := (index/100 + index%100) % 100
	mix := &d.profile.Mix
	switch {
	case slot < mix.Exchanges:
		return "token-exchanged"
	case slot < mix.Exchanges+mix.Approvals:
		return "approval-approved"
	case slot < mix.Exchanges+mix.Approvals+mix.Grants:
		return "grant-updated"
	case slot < mix.Exchanges+mix.Approvals+mix.Grants+mix.Refreshes:
		return "session-refreshed"
	default:
		return "agent-updated"
	}
}

func (d *ledgerPerformanceData) agentPermissionSets() []domainstorage.AgentPermissionSetEntry {
	if d.profile == nil {
		return fixtures.DefaultPermissionSets()
	}
	return d.agentPermissions
}

func (d *ledgerPerformanceData) grantedPermissionSets(principal int) []domainstorage.GrantedPermissionSetEntry {
	service := d.services[d.serviceIndex(principal)]
	entries := make([]domainstorage.GrantedPermissionSetEntry, len(d.permissionSets))
	for i, permissionID := range d.permissionSets {
		included := []id.ServiceID{d.permissionSetServices[i][0]}
		for _, available := range d.permissionSetServices[i] {
			if available == service && included[0] != service {
				included = append(included, service)
			}
		}
		entries[i] = domainstorage.GrantedPermissionSetEntry{PermissionSetID: permissionID, IncludedServiceIDs: included}
	}
	return entries
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
	if d.profile == nil {
		scopes := make([]domainstorage.ServiceScope, len(d.services))
		for i, serviceID := range d.services {
			scopes[i] = domainstorage.ServiceScope{ServiceID: serviceID, Scopes: []string{"read"}, RequirementType: domainstorage.RequirementTypeOptional}
		}
		if err := store.PermissionSets().Create(ctx, &domainstorage.PermissionSet{ID: fixtures.PlaceholderPermissionSetID, Name: "Ledger performance", ServiceScopes: scopes}); err != nil {
			return err
		}
	} else {
		for i, permissionID := range d.permissionSets {
			scopes := make([]domainstorage.ServiceScope, len(d.permissionSetServices[i]))
			for j, serviceID := range d.permissionSetServices[i] {
				scopes[j] = domainstorage.ServiceScope{ServiceID: serviceID, Scopes: []string{"read"}, RequirementType: domainstorage.RequirementTypeOptional}
			}
			if err := store.PermissionSets().Create(ctx, &domainstorage.PermissionSet{ID: permissionID, Name: fmt.Sprintf("Ledger performance %d", i), ServiceScopes: scopes}); err != nil {
				return fmt.Errorf("seed permission set %d: %w", i, err)
			}
		}
	}
	for i, agentID := range d.agents {
		agent := fixtures.ValidAgent()
		agent.ID = agentID
		clientID := id.ClientID(fmt.Sprintf("perf-agent-%02d", i))
		agent.ClientID = &clientID
		agent.AllowedScopes = []string{"read"}
		if d.profile != nil {
			agent.PermissionSets = d.agentPermissionSets()
		}
		if err := store.Agents().Create(ctx, agent); err != nil {
			return fmt.Errorf("seed agent %d: %w", i, err)
		}
	}
	for i, principal := range d.principals {
		serviceID := d.services[d.serviceIndex(i)]
		grant := fixtures.ActiveGrant(principal, d.agents[i%len(d.agents)].String(), serviceID.String(), []string{"read"})
		if d.profile != nil {
			grant.GrantedPermissionSets = d.grantedPermissionSets(i)
		}
		until := time.Now().Add(12 * time.Hour)
		grant.ValidUntil = &until
		if err := store.UserGrants().Create(ctx, grant); err != nil {
			return fmt.Errorf("seed grant %d: %w", i, err)
		}
		sessionServices := []id.ServiceID{serviceID}
		if d.profile == nil && serviceID != fixtures.PlaceholderServiceID {
			sessionServices = append(sessionServices, fixtures.PlaceholderServiceID)
		} else if d.profile != nil {
			for _, scoped := range d.permissionSetServices {
				if !slices.Contains(sessionServices, scoped[0]) {
					sessionServices = append(sessionServices, scoped[0])
				}
			}
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
	typeName := r.data.eventType(index)
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
		var granted map[string][]string
		if r.data.profile == nil {
			included := []string{service.String()}
			if service != fixtures.PlaceholderServiceID {
				included = append(included, fixtures.PlaceholderServiceID.String())
			}
			granted = map[string][]string{fixtures.PlaceholderPermissionSetID.String(): included}
		} else {
			granted = make(map[string][]string, len(r.data.permissionSets))
			for _, entry := range r.data.grantedPermissionSets(principalIndex) {
				services := make([]string, len(entry.IncludedServiceIDs))
				for i, serviceID := range entry.IncludedServiceIDs {
					services[i] = serviceID.String()
				}
				granted[entry.PermissionSetID.String()] = services
			}
		}
		var marshalErr error
		body, marshalErr = json.Marshal(map[string]any{
			"granted_permission_sets": granted,
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
		permissionSets := r.data.agentPermissionSets()
		body, marshalErr = json.Marshal(map[string]any{
			"display_name":    "Ledger Performance Agent",
			"description":     fmt.Sprintf("Performance mutation %s/%d", phase, index),
			"permission_sets": permissionSets,
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

func (r *ledgerPerformanceRunner) runSettings() (concurrency, actions int, interval time.Duration) {
	if r.data.profile == nil {
		return bootstrap.LedgerPerformanceConcurrency, bootstrap.LedgerPerformanceMeasuredActions, 0
	}
	p := r.data.profile
	return p.Concurrency, p.Actions, time.Second / time.Duration(p.ThroughputPerSecond)
}

func (r *ledgerPerformanceRunner) warm(ctx context.Context, phase string) (int, time.Time, time.Time, error) {
	concurrency, _, interval := r.runSettings()
	var ticker *time.Ticker
	if interval > 0 {
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	}
	started := time.Now()
	end := started.Add(bootstrap.LedgerPerformanceWarmup)
	var next atomic.Uint64
	var workers sync.WaitGroup
	errors := make(chan error, concurrency)
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for time.Now().Before(end) && ctx.Err() == nil {
				if ticker != nil {
					select {
					case <-ticker.C:
					case <-ctx.Done():
						return
					}
					if !time.Now().Before(end) {
						return
					}
				}
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
	concurrency, actions, interval := r.runSettings()
	var ticker *time.Ticker
	if interval > 0 {
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	}
	var next atomic.Uint64
	var workers sync.WaitGroup
	durations := make([]time.Duration, actions)
	errors := make(chan error, concurrency)
	start := time.Now()
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				index := int(next.Add(1) - 1)
				if index >= len(durations) || ctx.Err() != nil {
					return
				}
				if ticker != nil {
					select {
					case <-ticker.C:
					case <-ctx.Done():
						return
					}
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
	if r.data.profile != nil {
		return preflightDeploymentLedgerPerformance(ctx, r, db, phase)
	}
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

func preflightDeploymentLedgerPerformance(ctx context.Context, r *ledgerPerformanceRunner, db *sqlx.DB, phase string) (*model.BusinessEvent, error) {
	const prototypeIndex = 0
	prototypePhase := "prototype-" + phase
	prototypeStart := time.Now()
	if _, err := r.action(ctx, prototypePhase, prototypeIndex); err != nil {
		return nil, err
	}
	var table bool
	if err := db.GetContext(ctx, &table, "SELECT to_regclass('public.business_events') IS NOT NULL"); err != nil {
		return nil, err
	}
	if !table {
		return nil, errors.New("completed profile action but public.business_events is absent")
	}
	if _, _, err := verifyLedgerPerformanceEvents(ctx, db, r.data, prototypePhase, 1, prototypeStart, time.Now()); err != nil {
		return nil, fmt.Errorf("approved history prototype: %w", err)
	}
	started := time.Now()
	for index := range 100 {
		if _, err := r.action(ctx, phase, index); err != nil {
			return nil, err
		}
	}
	if _, _, err := verifyLedgerPerformanceEvents(ctx, db, r.data, phase, 100, started, time.Now()); err != nil {
		return nil, fmt.Errorf("approved workload preflight: %w", err)
	}
	var envelopes [][]byte
	if err := db.SelectContext(ctx, &envelopes, `SELECT envelope FROM public.business_events WHERE envelope->>'trace_id'=$1`, ledgerPerformanceTraceID(prototypePhase, prototypeIndex)); err != nil {
		return nil, err
	}
	if len(envelopes) != 1 {
		return nil, fmt.Errorf("profile prototype retained %d events, expected exactly one", len(envelopes))
	}
	var event model.BusinessEvent
	if err := json.Unmarshal(envelopes[0], &event); err != nil {
		return nil, err
	}
	if event.Type != ledgerPerformanceTypePrefix+r.data.eventType(prototypeIndex) {
		return nil, errors.New("profile prototype retained wrong event type")
	}
	return &event, nil
}

// History is synthetic safe data cloned from a real, previously validated
// production event. It is inserted by the migration owner, never by the broker,
// across 90 days of six-hour partitions. It cannot substitute for the live
// per-action completeness check in verifyLedgerPerformanceEvents.

func seedLedgerPerformanceHistoryCount(ctx context.Context, db *sqlx.DB, databaseURL string, example *model.BusinessEvent, count int, profile *bootstrap.LedgerPerformanceProfile) (int, int, error) {
	// Leave ten minutes inside the 90-day retention boundary during fixture
	// creation; the distributed history still spans every retained age bucket.
	const headroom = 10 * time.Minute
	base := time.Now().UTC().Add(-90*24*time.Hour + headroom).Truncate(time.Microsecond)
	period := (90*24*time.Hour - headroom) / time.Duration(count)
	last := base.Add(time.Duration(count-1) * period).Truncate(6 * time.Hour)
	for partition := base.Truncate(6 * time.Hour); !partition.After(last); partition = partition.Add(6 * time.Hour) {
		if _, err := db.ExecContext(ctx, "SELECT public.business_event_create_partition_pair($1)", partition); err != nil {
			return 0, 0, fmt.Errorf("create historical partition pair: %w", err)
		}
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var subject any
	if example.Subject != nil {
		subject = example.Subject.String()
	}
	values := make([]any, 7)
	index := 0
	source := pgx.CopyFromFunc(func() ([]any, error) {
		if index == count {
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
		return 0, 0, fmt.Errorf("insert safe historical events: %w", err)
	}
	if inserted != int64(count) {
		return 0, 0, fmt.Errorf("inserted %d historical events, want %d", inserted, count)
	}
	var observed int
	if err := db.GetContext(ctx, &observed, "SELECT count(*) FROM public.business_events"); err != nil {
		return 0, 0, err
	}
	if observed != count {
		return 0, 0, fmt.Errorf("historical history count %d, want %d", observed, count)
	}
	if profile == nil {
		return 0, 0, nil
	}
	var minBytes, maxBytes int
	if err := db.QueryRowxContext(ctx, "SELECT min(octet_length(envelope::text)), max(octet_length(envelope::text)) FROM public.business_events").Scan(&minBytes, &maxBytes); err != nil {
		return 0, 0, err
	}
	if minBytes < profile.HistoryEventBytes.Min || maxBytes > profile.HistoryEventBytes.Max {
		return 0, 0, fmt.Errorf("stored history event bytes %d-%d outside approved %d-%d", minBytes, maxBytes, profile.HistoryEventBytes.Min, profile.HistoryEventBytes.Max)
	}
	return minBytes, maxBytes, nil
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
		if d.eventType(index) == "approval-approved" {
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
		wantType := d.eventType(index)
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
		if wantType == "token-exchanged" || wantType == "approval-requested" {
			if event.Actor.Kind != "gateway" || event.Actor.ID == nil || *event.Actor.ID != "approval-gateway-client" {
				return 0, 0, fmt.Errorf("%s %d lost its verified initiating gateway", wantType, index)
			}
			if wantType == "token-exchanged" && (event.Actor.OnBehalfOf == nil || event.Actor.OnBehalfOf.String() != d.principals[principal]) {
				return 0, 0, fmt.Errorf("exchange %d lost established delegation", index)
			}
		}
		if d.profile != nil {
			switch wantType {
			case "token-exchanged":
				if event.ServiceID != d.services[d.serviceIndex(principal)] {
					return 0, 0, fmt.Errorf("exchange %d retained wrong configured service", index)
				}
			case "grant-updated":
				if len(event.PermissionSetIDs) != len(d.permissionSets) {
					return 0, 0, fmt.Errorf("grant %d retained %d/%d configured permission sets", index, len(event.PermissionSetIDs), len(d.permissionSets))
				}
				for _, permissionID := range d.permissionSets {
					if !slices.Contains(event.PermissionSetIDs, permissionID) {
						return 0, 0, fmt.Errorf("grant %d lost configured permission set %s", index, permissionID)
					}
				}
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
			if d.profile != nil {
				bounds := d.profile.EventBytes[wantType]
				if len(raw) < bounds.Min || len(raw) > bounds.Max {
					return 0, 0, fmt.Errorf("%s action %d produced %d event bytes outside approved %d-%d", wantType, index, len(raw), bounds.Min, bounds.Max)
				}
			}
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

func awaitLedgerPerformanceMetricsCount(collector *bootstrap.LedgerPerformanceCollector, start, end time.Time, phase string, actions int, feature, poolEnabled bool) (bootstrap.LedgerPerformanceAllocation, bootstrap.LedgerPerformanceRecordings, *bootstrap.LedgerPerformancePoolSample, error) {
	var traceIDs []string
	if feature {
		traceIDs = make([]string, actions)
		for index := range traceIDs {
			traceIDs[index] = ledgerPerformanceTraceID(phase, index)
		}
	}
	deadline := time.Now().Add(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for time.Now().Before(deadline) {
		allocation, aerr := collector.Allocations(start, end)
		var recordings bootstrap.LedgerPerformanceRecordings
		var serr error
		if feature {
			recordings, serr = collector.RecordingTimes(traceIDs)
		}
		var pool *bootstrap.LedgerPerformancePoolSample
		var perr error
		if poolEnabled {
			var sample bootstrap.LedgerPerformancePoolSample
			sample, perr = collector.PoolWaits(start, end)
			pool = &sample
		}
		if aerr == nil && serr == nil && perr == nil {
			return allocation, recordings, pool, nil
		}
		last = errors.Join(aerr, serr, perr)
		<-ticker.C
	}
	return bootstrap.LedgerPerformanceAllocation{}, bootstrap.LedgerPerformanceRecordings{}, nil, fmt.Errorf("measured allocation, recording stages or pool waits unavailable: %w", last)
}

// Raw nanosecond samples are persisted outside the checkout for T095's
// measured report; no credential-bearing request or event body is written.
type ledgerPerformanceReport struct {
	Profile                           string                                 `json:"profile"`
	ApprovedProfile                   *bootstrap.LedgerPerformanceProfile    `json:"approved_deployment_profile,omitempty"`
	Version                           string                                 `json:"version"`
	Revision                          string                                 `json:"baseline_revision"`
	FeatureSource                     string                                 `json:"feature_revision"`
	Repetition                        int                                    `json:"repetition"`
	PostgresVersion                   string                                 `json:"postgres_version"`
	Hardware                          string                                 `json:"hardware"`
	Principals                        int                                    `json:"principals"`
	Agents                            int                                    `json:"agents"`
	Services                          int                                    `json:"services"`
	Concurrency                       int                                    `json:"concurrency"`
	MeasuredActions                   int                                    `json:"measured_actions"`
	PreparationActions                int                                    `json:"untimed_approval_preparations"`
	VerifiedMeasuredEvents            int                                    `json:"verified_measured_events,omitempty"`
	VerifiedApprovalPreparations      int                                    `json:"verified_approval_preparations,omitempty"`
	VerifiedWarmupEvents              int                                    `json:"verified_warmup_events,omitempty"`
	RetainedEventCanariesAbsent       bool                                   `json:"retained_event_canaries_absent,omitempty"`
	WarmupActions                     int                                    `json:"warmup_actions"`
	RetainedHistory                   int                                    `json:"retained_history"`
	HistoricalEventBytesMin           int                                    `json:"historical_event_bytes_min,omitempty"`
	HistoricalEventBytesMax           int                                    `json:"historical_event_bytes_max,omitempty"`
	WarmupSeconds                     int                                    `json:"warmup_seconds"`
	Mix                               string                                 `json:"mix"`
	Pool                              string                                 `json:"database_pool"`
	Telemetry                         string                                 `json:"telemetry"`
	Started                           time.Time                              `json:"started"`
	Ended                             time.Time                              `json:"ended"`
	TransactionLatencyNanoseconds     []time.Duration                        `json:"transaction_latency_nanoseconds"`
	RecordingSpanNanoseconds          []time.Duration                        `json:"recording_span_nanoseconds,omitempty"` // Validation + append stages only; excludes advisory-gate waits, checkout and commit.
	ValidationSpanNanoseconds         []time.Duration                        `json:"recording_validation_nanoseconds,omitempty"`
	AppendSpanNanoseconds             []time.Duration                        `json:"recording_append_nanoseconds,omitempty"`
	PhysicalTransactionNanoseconds    []time.Duration                        `json:"physical_transaction_nanoseconds,omitempty"`
	PoolWaits                         *bootstrap.LedgerPerformancePoolSample `json:"pool_waits,omitempty"`
	TransactionP50Nanoseconds         time.Duration                          `json:"transaction_p50_nanoseconds"`
	TransactionP95Nanoseconds         time.Duration                          `json:"transaction_p95_nanoseconds"`
	TransactionP99Nanoseconds         time.Duration                          `json:"transaction_p99_nanoseconds"`
	RecordingP99Nanoseconds           time.Duration                          `json:"recording_p99_nanoseconds,omitempty"` // P99 of validation + append stage sums, not full recording overhead.
	PhysicalTransactionP99Nanoseconds time.Duration                          `json:"physical_transaction_p99_nanoseconds,omitempty"`
	EventBytesMin                     int                                    `json:"event_bytes_min"`
	EventBytesMax                     int                                    `json:"event_bytes_max"`
	ThroughputPerSecond               float64                                `json:"throughput_per_second"`
	AllocationBytes                   uint64                                 `json:"allocation_bytes"`
	AllocationObjects                 uint64                                 `json:"allocation_objects"`
	HTTPErrors                        int                                    `json:"http_errors"`
	PostgresRollbacks                 int                                    `json:"postgres_rollbacks"`
}

func retainLedgerPerformanceReport(directory string, report ledgerPerformanceReport) (string, error) {
	prefix := "reference"
	if report.ApprovedProfile != nil {
		prefix = "deployment"
	} else if report.Profile == "reference-delivery-diagnostic" {
		prefix = "delivery-diagnostic"
	}
	path := filepath.Join(directory, fmt.Sprintf("%s-repetition-%d-%s.json", prefix, report.Repetition, report.Version))
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

func runLedgerPerformanceScenario(ctx SpecContext, profile *bootstrap.LedgerPerformanceProfile, deliveryDiagnostic bool) []string {
	profileName := "reference-recording"
	var pg bootstrap.LedgerPerformancePostgres
	if profile == nil {
		if deliveryDiagnostic {
			profileName = "reference-delivery-diagnostic"
		}
		fmt.Fprintf(GinkgoWriter, "%s profile baseline=%s principals=%d agents=%d services=%d history=%d concurrency=%d pool=%d/%d warmup=%s measured_actions=%d repetitions=%d mix=50%%exchange,20%%approval-transition,10%%grant-update,10%%session-refresh,10%%admin-mutation\n", profileName, bootstrap.LedgerPerformanceBaselineRevision, bootstrap.LedgerPerformancePrincipals, bootstrap.LedgerPerformanceAgents, bootstrap.LedgerPerformanceServices, bootstrap.LedgerPerformanceHistory, bootstrap.LedgerPerformanceConcurrency, bootstrap.LedgerPerformanceReferencePoolSize, bootstrap.LedgerPerformanceReferencePoolSize, bootstrap.LedgerPerformanceWarmup, bootstrap.LedgerPerformanceMeasuredActions, bootstrap.LedgerPerformanceRepetitions)
	} else {
		profileName = "deployment"
		Expect(bootstrap.CheckLedgerPerformanceHost(profile)).To(Succeed())
		external, err := bootstrap.NewExternalLedgerPerformancePostgres(ctx, profile)
		Expect(err).NotTo(HaveOccurred())
		pg = external
		DeferCleanup(func() { Expect(external.Close()).To(Succeed()) })
		fmt.Fprintf(GinkgoWriter, "approved deployment owner=%s date=%s reference=%s location=%s actions=%d throughput=%d/s concurrency=%d principals=%d agents=%d services=%d permission_set_sizes=%v history=%d\n", profile.Approval.Owner, profile.Approval.Date, profile.Approval.Reference, profile.Approval.Location, profile.Actions, profile.ThroughputPerSecond, profile.Concurrency, profile.Principals, profile.Agents, profile.Services, profile.PermissionSetServiceCounts, profile.History)
	}
	collector, err := bootstrap.NewLedgerPerformanceCollectorForProfile(ctx, profile)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(collector.Close)
	actions, history, concurrency := bootstrap.LedgerPerformanceMeasuredActions, bootstrap.LedgerPerformanceHistory, bootstrap.LedgerPerformanceConcurrency
	preparations := actions / 5
	if profile != nil {
		actions, history, concurrency = profile.Actions, profile.History, profile.Concurrency
		preparations = (actions / 100) * profile.Mix.Approvals
	}
	root, err := integrationbootstrap.FindProjectRoot()
	Expect(err).NotTo(HaveOccurred())
	temp := GinkgoT().TempDir()
	poolSize := 0 // The approved deployment workload uses unmodified production binaries.
	if profile == nil {
		poolSize = bootstrap.LedgerPerformanceReferencePoolSize
	}
	binaries, err := bootstrap.BuildLedgerPerformanceBinaries(ctx, root, temp, poolSize)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(binaries.Close()).To(Succeed()) })
	fmt.Fprintf(GinkgoWriter, "%s binaries: baseline=%s feature=%s\n", profileName, bootstrap.LedgerPerformanceBaselineRevision, binaries.FeatureSource)
	if profile == nil {
		pg = integrationbootstrap.RequireSharedPostgres(ledgerPerformanceT{GinkgoT()})
	}
	canaries := fixtures.LedgerCredentialCanaries()
	upstream := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse().WithAccessToken(canaries.AccessToken).WithRefreshToken(canaries.RefreshToken)
	DeferCleanup(upstream.Close)
	data := newLedgerPerformanceDataForProfile(upstream, profile)
	Expect(data.prepareTokens()).To(Succeed())
	runID := strings.ReplaceAll(data.agents[0].String(), "-", "")
	featureTemplate := "ledger_perf_feature_" + runID
	baselineTemplate := "ledger_perf_baseline_" + runID
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
	preflightDir := filepath.Join(temp, "preflight")
	Expect(os.Mkdir(preflightDir, 0700)).To(Succeed())
	feature, err := bootstrap.StartLedgerPerformanceBrokerWithProfile(ctx, binaries.Feature, preflightDir, featureRuntimeURL, upstream.URL(), collector.Endpoint(), profile, deliveryDiagnostic)
	Expect(err).NotTo(HaveOccurred())
	var stopPreflight sync.Once
	closeFeature := func() { stopPreflight.Do(func() { Expect(feature.Close()).To(Succeed()) }) }
	DeferCleanup(closeFeature)
	prototype, err := preflightLedgerPerformance(ctx, newLedgerPerformanceRunner(feature, data), featureDB, "preflight")
	Expect(err).NotTo(HaveOccurred())
	closeFeature()
	cleanupPreflight()
	var historicalBytesMin, historicalBytesMax int
	historyTemplate := featureTemplate + "_history"
	seedHistory := func(name string) {
		seed(root)(name)
		url := pg.ConnectionString(name)
		owner, openErr := sqlx.ConnectContext(ctx, "pgx", url)
		Expect(openErr).NotTo(HaveOccurred())
		defer owner.Close()
		var historyErr error
		historicalBytesMin, historicalBytesMax, historyErr = seedLedgerPerformanceHistoryCount(ctx, owner, url, prototype, history, profile)
		Expect(historyErr).NotTo(HaveOccurred())
	}
	// Seeded history is cloned once per feature run before its two-minute warmup.
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
		resultsDirectory = filepath.Join(resultsDirectory, runID)
		err = os.MkdirAll(resultsDirectory, 0700)
	}
	Expect(err).NotTo(HaveOccurred())
	fmt.Fprintf(GinkgoWriter, "%s raw latency/recording samples: %s (nanoseconds)\n", profileName, resultsDirectory)
	var violations []string
	baselineP99s := make([]time.Duration, 0, bootstrap.LedgerPerformanceRepetitions)
	for repetition := range bootstrap.LedgerPerformanceRepetitions {
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
				if profile != nil {
					Expect(bootstrap.CheckLedgerPerformancePostgres(ctx, db, profile)).To(Succeed())
				}
				var initialHistory int
				if enabled {
					Expect(db.GetContext(ctx, &initialHistory, "SELECT count(*) FROM public.business_events")).To(Succeed())
					Expect(initialHistory).To(Equal(history), "feature clone must retain the full 90-day historical dataset")
				}
				if profile == nil {
					fmt.Fprintf(GinkgoWriter, "%s repetition=%d version=%s postgres=%s Go=%s/%s cpu=%d gomaxprocs=%d db_pool_open=%d db_pool_idle=%d prewarmed_backends=%d test_only_overlay=true network=local-HTTP-and-container-port telemetry=grpc-traces-logs-metrics-on ledger_copy=%t initial_history=%d\n", profileName, repetition+1, version, postgresVersion, runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0), binaries.PoolSize, binaries.PoolSize, binaries.PoolSize, deliveryDiagnostic, initialHistory)
				} else {
					fmt.Fprintf(GinkgoWriter, "deployment repetition=%d version=%s postgres=%s Go=%s/%s cpus=%d gomaxprocs=%d database_host=%s receiver=%s logs=%t copy_switch=%t effective_copy=%t initial_history=%d\n", repetition+1, version, postgresVersion, runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0), profile.Postgres.Host, profile.Telemetry.Receiver, profile.Telemetry.Logs, profile.Telemetry.LedgerCopy, enabled && profile.Telemetry.Enabled && profile.Telemetry.Logs && profile.Telemetry.LedgerCopy, initialHistory)
				}
				otlp, openErr := bootstrap.NewLedgerPerformanceCollectorForProfile(ctx, profile)
				Expect(openErr).NotTo(HaveOccurred())
				defer otlp.Close()
				folder := filepath.Join(temp, fmt.Sprintf("%s-%d", version, repetition))
				Expect(os.Mkdir(folder, 0700)).To(Succeed())
				server, openErr := bootstrap.StartLedgerPerformanceBrokerWithProfile(ctx, binary, folder, runtimeURL, upstream.URL(), otlp.Endpoint(), profile, deliveryDiagnostic)
				Expect(openErr).NotTo(HaveOccurred())
				defer func() { Expect(server.Close()).To(Succeed()) }()
				runner := newLedgerPerformanceRunner(server, data)
				phase := fmt.Sprintf("%s-%d", version, repetition)
				warmCount, warmStart, warmEnd, warmErr := runner.warm(ctx, "warm-"+phase)
				Expect(warmErr).NotTo(HaveOccurred())
				fmt.Fprintf(GinkgoWriter, "%s repetition=%d version=%s completed_warmup_actions=%d\n", profileName, repetition+1, version, warmCount)
				var rollbacksBefore int64
				Expect(db.GetContext(ctx, &rollbacksBefore, "SELECT xact_rollback FROM pg_stat_database WHERE datname=current_database()")).To(Succeed())
				durations, started, ended, failures, runErr := runner.measure(ctx, "measure-"+phase)
				Expect(runErr).NotTo(HaveOccurred())
				var minSize, maxSize int
				var verifiedMeasured, verifiedPreparations, verifiedWarmup int
				if enabled {
					var verifyErr error
					minSize, maxSize, verifyErr = verifyLedgerPerformanceEvents(ctx, db, data, "measure-"+phase, actions, started, ended)
					var retained int
					Expect(db.GetContext(ctx, &retained, "SELECT count(*) FROM public.business_events")).To(Succeed())
					Expect(retained).To(BeNumerically(">=", history+actions), "retained history and every measured action")
					Expect(verifyErr).NotTo(HaveOccurred())
					verifiedMeasured = len(durations)
					verifiedPreparations = preparations
					_, _, verifyErr = verifyLedgerPerformanceEvents(ctx, db, data, "warm-"+phase, warmCount, warmStart, warmEnd)
					Expect(verifyErr).NotTo(HaveOccurred(), "every completed warmup and preparation action must also retain its event")
					verifiedWarmup = warmCount
				}
				allocation, recordings, poolWaits, measurementErr := awaitLedgerPerformanceMetricsCount(otlp, started, ended, "measure-"+phase, actions, enabled, binaries.PoolSize > 0)
				Expect(measurementErr).NotTo(HaveOccurred())
				var validationAppendP99, physicalP99 time.Duration
				if enabled {
					validationAppendP99 = bootstrap.SummarizeLedgerPerformance(recordings.Total, ended.Sub(started), bootstrap.LedgerPerformanceAllocation{}, 0, 0).P99
					physicalP99 = bootstrap.SummarizeLedgerPerformance(recordings.Transaction, ended.Sub(started), bootstrap.LedgerPerformanceAllocation{}, 0, 0).P99
					fmt.Fprintf(GinkgoWriter, "%s repetition=%d version=%s event_bytes_min=%d event_bytes_max=%d history=%d recording_validation_stage_p99=%s recording_append_stage_p99=%s validation_append_stages_p99=%s physical_transaction_p99=%s\n", profileName, repetition+1, version, minSize, maxSize, history, bootstrap.SummarizeLedgerPerformance(recordings.Validation, ended.Sub(started), bootstrap.LedgerPerformanceAllocation{}, 0, 0).P99, bootstrap.SummarizeLedgerPerformance(recordings.Append, ended.Sub(started), bootstrap.LedgerPerformanceAllocation{}, 0, 0).P99, validationAppendP99, physicalP99)
				}
				if poolWaits != nil {
					fmt.Fprintf(GinkgoWriter, "%s repetition=%d version=%s db_pool_wait_count=%d db_pool_wait_duration=%s\n", profileName, repetition+1, version, poolWaits.WaitCount, poolWaits.WaitDuration)
				}
				var rollbacksAfter int64
				Expect(db.GetContext(ctx, &rollbacksAfter, "SELECT xact_rollback FROM pg_stat_database WHERE datname=current_database()")).To(Succeed())
				index := 0
				if enabled {
					index = 1
				}
				pair[index] = bootstrap.SummarizeLedgerPerformance(durations, ended.Sub(started), allocation, failures, int(rollbacksAfter-rollbacksBefore))
				fmt.Fprintf(GinkgoWriter, "%s repetition=%d version=%s p50=%s p95=%s p99=%s throughput=%.1f/s alloc_bytes=%d alloc_objects=%d http_errors=%d pg_rollbacks=%d\n", profileName, repetition+1, version, pair[index].P50, pair[index].P95, pair[index].P99, pair[index].Throughput, allocation.Bytes, allocation.Objects, failures, pair[index].Rollbacks)
				if profile != nil && pair[index].Throughput < float64(profile.ThroughputPerSecond)*0.95 {
					violations = append(violations, fmt.Sprintf("%s repetition %d %s throughput %.1f/s did not reach approved %d/s", profileName, repetition+1, version, pair[index].Throughput, profile.ThroughputPerSecond))
				}
				report := ledgerPerformanceReport{
					Profile: profileName, Version: version, Revision: bootstrap.LedgerPerformanceBaselineRevision, FeatureSource: binaries.FeatureSource,
					Repetition: repetition + 1, PostgresVersion: postgresVersion,
					Hardware:   fmt.Sprintf("%s/%s cpus=%d gomaxprocs=%d client/broker=loopback postgres=mapped-container-port", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0)),
					Principals: len(data.principals), Agents: len(data.agents), Services: len(data.services),
					Concurrency: concurrency, MeasuredActions: actions,
					VerifiedMeasuredEvents: verifiedMeasured, VerifiedApprovalPreparations: verifiedPreparations,
					VerifiedWarmupEvents: verifiedWarmup, RetainedEventCanariesAbsent: enabled,
					RetainedHistory: initialHistory, WarmupSeconds: int(bootstrap.LedgerPerformanceWarmup.Seconds()),
					WarmupActions: warmCount, PreparationActions: preparations,
					Mix:  "50% exchanges, 20% approval transitions, 10% grant updates, 10% session refreshes, 10% admin mutations",
					Pool: fmt.Sprintf("controlled test-only Go build overlay in both binaries: %d open/%d idle, prewarmed backends", binaries.PoolSize, binaries.PoolSize), Telemetry: fmt.Sprintf("OTLP gRPC traces/logs/metrics enabled; local acknowledging receiver; telemetry_copy_enabled=%t", deliveryDiagnostic),
					Started: started, Ended: ended, TransactionLatencyNanoseconds: durations,
					RecordingSpanNanoseconds: recordings.Total, ValidationSpanNanoseconds: recordings.Validation, AppendSpanNanoseconds: recordings.Append, PhysicalTransactionNanoseconds: recordings.Transaction, PoolWaits: poolWaits,
					RecordingP99Nanoseconds: validationAppendP99, PhysicalTransactionP99Nanoseconds: physicalP99,
					TransactionP50Nanoseconds: pair[index].P50, TransactionP95Nanoseconds: pair[index].P95, TransactionP99Nanoseconds: pair[index].P99,
					EventBytesMin: minSize, EventBytesMax: maxSize, ThroughputPerSecond: pair[index].Throughput,
					AllocationBytes: allocation.Bytes, AllocationObjects: allocation.Objects, HTTPErrors: failures, PostgresRollbacks: pair[index].Rollbacks,
				}
				if profile != nil {
					report.Pool = "unmodified production adapter: 25 open, 5 idle"
					report.Profile, report.ApprovedProfile = "operator-approved-deployment", profile
					report.Hardware = fmt.Sprintf("%s/%s cpus=%d gomaxprocs=%d attested_memory_bytes=%d client/broker=loopback postgres=%s", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0), profile.Hardware.MemoryBytes, profile.Postgres.Host)
					report.Mix = fmt.Sprintf("%d%% exchanges, %d%% approval transitions, %d%% grant updates, %d%% session refreshes, %d%% admin mutations", profile.Mix.Exchanges, profile.Mix.Approvals, profile.Mix.Grants, profile.Mix.Refreshes, profile.Mix.Admin)
					report.Telemetry = fmt.Sprintf("OTLP gRPC traces/metrics enabled; logs=%t receiver=%s transport=plaintext-via-capture-proxy copy_switch=%t effective_copy=%t", profile.Telemetry.Logs, profile.Telemetry.Receiver, profile.Telemetry.LedgerCopy, enabled && profile.Telemetry.Enabled && profile.Telemetry.Logs && profile.Telemetry.LedgerCopy)
					if enabled {
						report.HistoricalEventBytesMin, report.HistoricalEventBytesMax = historicalBytesMin, historicalBytesMax
					}
				}
				path, reportErr := retainLedgerPerformanceReport(resultsDirectory, report)
				Expect(reportErr).NotTo(HaveOccurred())
				fmt.Fprintf(GinkgoWriter, "%s measured distribution saved: %s\n", profileName, path)
				if profile != nil && enabled && validationAppendP99 > 5*time.Millisecond {
					violations = append(violations, fmt.Sprintf("%s repetition %d validation+append stages p99 %s exceeds 5ms (advisory-gate waits excluded)", profileName, repetition+1, validationAppendP99))
				}
			}()
		}
		baselineP99s = append(baselineP99s, pair[0].P99)
		difference := pair[1].P99 - pair[0].P99
		fmt.Fprintf(GinkgoWriter, "%s repetition=%d independent_transaction_p99_difference=%s (feature p99 minus baseline p99; NOT a paired per-action effect; target=5ms)\n", profileName, repetition+1, difference)
		if profile != nil && difference > 5*time.Millisecond {
			violations = append(violations, fmt.Sprintf("%s repetition %d independent transaction p99 difference %s exceeds 5ms", profileName, repetition+1, difference))
		}
	}
	minP99, maxP99 := slices.Min(baselineP99s), slices.Max(baselineP99s)
	fmt.Fprintf(GinkgoWriter, "%s baseline_p99_repetitions=%v baseline_p99_range=%s..%s baseline_p99_spread=%s\n", profileName, baselineP99s, minP99, maxP99, maxP99-minP99)
	if profile == nil {
		fmt.Fprintln(GinkgoWriter, "Reference 5ms gate: NOT PROVEN, irrespective of numeric comparison. Controlled pool overlay and host variability cannot establish an unmodified deployment result; approved deployment profile remains missing.")
	}
	// Explicitly release the first workload before the second begins. DeferCleanup
	// remains a failure-path safety net for the Ginkgo spec.
	collector.Close()
	upstream.Close()
	Expect(binaries.Close()).To(Succeed())
	if profile != nil {
		Expect(pg.(*bootstrap.ExternalLedgerPerformancePostgres).Close()).To(Succeed())
	}
	return violations
}

var _ = Describe("Business event ledger performance", Label("business-event-ledger", "performance"), func() {
	It("retains reference facts with delivery copying paused", Label("recording-reference"), Serial, func(ctx SpecContext) {
		runLedgerPerformanceScenario(ctx, nil, false)
	})
	It("measures background delivery load separately", Label("delivery-diagnostic"), Serial, func(ctx SpecContext) {
		runLedgerPerformanceScenario(ctx, nil, true)
	})
	// US4-AS4 requires the separately approved deployment workload.
	It("requires approved deployment before release acceptance", Label("release-acceptance"), Serial, func(ctx SpecContext) {
		profile, err := bootstrap.LoadLedgerPerformanceProfile(os.Getenv("AIB_LEDGER_PERFORMANCE_PROFILE"))
		Expect(err).NotTo(HaveOccurred(), "reference measurements alone cannot release")
		deploymentViolations := runLedgerPerformanceScenario(ctx, profile, false)
		runLedgerPerformanceScenario(ctx, nil, false)
		Expect(deploymentViolations).To(BeEmpty(), "approved deployment must meet the measured-stage and independent-transaction numerical gates and throughput")
	})
})
