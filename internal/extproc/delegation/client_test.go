package delegation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSubject   = "subject-token-marker"
	testAssertion = "assertion-marker"
	testResource  = "https://api.example.com/repos"
)

type staticAssertion struct {
	value string
	err   error
}

func (s staticAssertion) ClientAssertion() (string, error) { return s.value, s.err }

const permitContextJSON = `{
	"subject": {"id": "alice@example.com"},
	"agent": {"id": "550e8400-e29b-41d4-a716-446655440000", "canonical_id": "data-assistant", "extra": 1},
	"resource": {"id": "880e8400-e29b-41d4-a716-446655440010", "canonical_id": "repo-api",
		"resource_uri": "https://api.example.com/repos", "scopes": ["read:repo"]},
	"grant": {"id": "990e8400-e29b-41d4-a716-446655440020", "updated_at": "2026-10-26T09:59:00Z", "valid_until": "2026-10-26T10:30:00Z"},
	"permission_sets": [{"id": "770e8400-e29b-41d4-a716-446655440001", "canonical_id": "repo-reader",
		"services": [{"id": "880e8400-e29b-41d4-a716-446655440010", "canonical_id": "repo-api", "scopes": ["read:repo"], "unknown": true}]}],
	"unknown_member": {"secret": "do-not-copy"}
}`

type brokerStub struct {
	status       int
	cacheControl []string
	body         string
	delay        time.Duration
	requests     atomic.Int32
	lastBody     atomic.Value
	lastAuth     atomic.Value
}

func (b *brokerStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.requests.Add(1)
		raw, _ := io.ReadAll(r.Body)
		b.lastBody.Store(string(raw))
		b.lastAuth.Store(r.Header.Get("Authorization"))
		if b.delay > 0 {
			time.Sleep(b.delay)
		}
		for _, value := range b.cacheControl {
			w.Header().Add("Cache-Control", value)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(b.status)
		_, _ = w.Write([]byte(b.body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T, srv *httptest.Server, maxStaleness time.Duration) *Client {
	t.Helper()
	endpoint, err := url.Parse(srv.URL + "/access/v1/evaluation")
	require.NoError(t, err)
	client, err := NewClient(srv.Client(), &Metadata{PolicyDecisionPoint: srv.URL, AccessEvaluationEndpoint: endpoint},
		staticAssertion{value: testAssertion}, maxStaleness)
	require.NoError(t, err)
	return client
}

func permitStub(cacheControl ...string) *brokerStub {
	return &brokerStub{status: http.StatusOK, cacheControl: cacheControl, body: `{"decision": true, "context": ` + permitContextJSON + `}`}
}

func TestClient_PermitSendsExactRequestAndDecodesTypedContext(t *testing.T) {
	t.Parallel()
	broker := permitStub("private, max-age=15")
	client := newTestClient(t, broker.server(t), time.Minute)
	start := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return start }

	result, err := client.Evaluate(context.Background(), testSubject, testResource)

	require.NoError(t, err)
	assert.JSONEq(t, `{"subject":{"type":"urn:ietf:params:oauth:token-type:jwt","id":"`+testSubject+`"},
		"action":{"name":"urn:ietf:params:oauth:grant-type:token-exchange"},
		"resource":{"type":"protected_resource","id":"`+testResource+`"}}`, broker.lastBody.Load().(string))
	assert.Equal(t, "Bearer "+testAssertion, broker.lastAuth.Load())
	assert.Equal(t, start.Add(15*time.Second), result.Deadline)
	assert.Equal(t, Context{
		Principal: "alice@example.com",
		Agent:     EntityRef{ID: "550e8400-e29b-41d4-a716-446655440000", CanonicalID: "data-assistant"},
		Resource: Resource{ID: "880e8400-e29b-41d4-a716-446655440010", CanonicalID: "repo-api",
			ResourceURI: "https://api.example.com/repos", Scopes: []string{"read:repo"}},
		Grant: Grant{ID: "990e8400-e29b-41d4-a716-446655440020", UpdatedAt: "2026-10-26T09:59:00Z", ValidUntil: "2026-10-26T10:30:00Z"},
		PermissionSets: []PermissionSet{{ID: "770e8400-e29b-41d4-a716-446655440001", CanonicalID: "repo-reader",
			Services: []Service{{ID: "880e8400-e29b-41d4-a716-446655440010", CanonicalID: "repo-api", Scopes: []string{"read:repo"}}}}},
	}, result.Context)
}

func TestClient_DeadlineUsesClockReadBeforeSendAndMaxStaleness(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		maxAge       string
		maxStaleness time.Duration
		want         time.Duration
	}{
		{"max-age below staleness", "max-age=15", time.Minute, 15 * time.Second},
		{"staleness below max-age", "max-age=60", 2 * time.Second, 2 * time.Second},
		{"max-age above broker cap is still bounded by staleness", "max-age=3600", time.Minute, time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			broker := permitStub("private, " + tt.maxAge)
			broker.delay = 50 * time.Millisecond
			client := newTestClient(t, broker.server(t), tt.maxStaleness)
			start := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
			var reads atomic.Int32
			client.now = func() time.Time {
				reads.Add(1)
				return start.Add(time.Duration(reads.Load()-1) * time.Hour)
			}

			result, err := client.Evaluate(context.Background(), testSubject, testResource)

			require.NoError(t, err)
			assert.Equal(t, int32(1), reads.Load(), "the clock is read once, before the POST")
			assert.Equal(t, start.Add(tt.want), result.Deadline, "a delayed response must not extend the deadline")
		})
	}
}

func TestClient_UnusableMaxAgeFailsClosed(t *testing.T) {
	t.Parallel()
	for name, values := range map[string][]string{
		"missing":         nil,
		"no max-age":      {"private"},
		"zero":            {"private, max-age=0"},
		"negative":        {"max-age=-5"},
		"malformed":       {"max-age=abc"},
		"non-integer":     {"max-age=1.5"},
		"quoted":          {`max-age="15"`},
		"empty":           {"max-age="},
		"duplicate":       {"max-age=10, max-age=10"},
		"duplicate split": {"max-age=10", "private, max-age=20"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := newTestClient(t, permitStub(values...).server(t), time.Minute)
			_, err := client.Evaluate(context.Background(), testSubject, testResource)
			var resolveErr *Error
			require.ErrorAs(t, err, &resolveErr)
			assert.Equal(t, CategoryInvalidResponse, resolveErr.Category)
		})
	}
}

func TestClient_TypedContextValidation(t *testing.T) {
	t.Parallel()
	mutate := func(path string, value any) string {
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(permitContextJSON), &doc))
		parts := strings.Split(path, ".")
		cursor := doc
		for i, part := range parts {
			if i == len(parts)-1 {
				if value == "<delete>" {
					delete(cursor, part)
				} else {
					cursor[part] = value
				}
				break
			}
			switch next := cursor[part].(type) {
			case map[string]any:
				cursor = next
			case []any:
				cursor = next[0].(map[string]any)
			}
		}
		raw, err := json.Marshal(doc)
		require.NoError(t, err)
		return string(raw)
	}
	invalid := map[string]string{
		"missing subject":                mutate("subject", "<delete>"),
		"empty subject id":               mutate("subject.id", ""),
		"null agent id":                  mutate("agent.id", nil),
		"null agent canonical id":        mutate("agent.canonical_id", nil),
		"empty agent canonical id":       mutate("agent.canonical_id", ""),
		"empty resource canonical id":    mutate("resource.canonical_id", ""),
		"missing resource uri":           mutate("resource.resource_uri", "<delete>"),
		"null scopes":                    mutate("resource.scopes", nil),
		"string scopes":                  mutate("resource.scopes", "read:repo"),
		"bad updated_at":                 mutate("grant.updated_at", "yesterday"),
		"null valid_until":               mutate("grant.valid_until", nil),
		"empty valid_until":              mutate("grant.valid_until", ""),
		"empty permission sets":          mutate("permission_sets", []any{}),
		"null permission sets":           mutate("permission_sets", nil),
		"empty permission set canonical": mutate("permission_sets.canonical_id", ""),
		"missing services":               mutate("permission_sets.services", "<delete>"),
		"empty service canonical": `{"subject":{"id":"a"},"agent":{"id":"x"},"resource":{"id":"r","resource_uri":"https://a","scopes":[]},
			"grant":{"id":"g","updated_at":"2026-10-26T09:59:00Z"},"permission_sets":[{"id":"p","services":[{"id":"s","canonical_id":"","scopes":[]}]}]}`,
		"null service scopes": `{"subject":{"id":"a"},"agent":{"id":"x"},"resource":{"id":"r","resource_uri":"https://a","scopes":[]},
			"grant":{"id":"g","updated_at":"2026-10-26T09:59:00Z"},"permission_sets":[{"id":"p","services":[{"id":"s","scopes":null}]}]}`,
		"context not object": `"permit"`,
	}
	for name, ctxJSON := range invalid {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			broker := &brokerStub{status: http.StatusOK, cacheControl: []string{"max-age=30"}, body: `{"decision":true,"context":` + ctxJSON + `}`}
			_, err := newTestClient(t, broker.server(t), time.Minute).Evaluate(context.Background(), testSubject, testResource)
			var resolveErr *Error
			require.ErrorAs(t, err, &resolveErr)
			assert.Equal(t, CategoryInvalidResponse, resolveErr.Category)
		})
	}

	t.Run("absent optional members and empty arrays are accepted", func(t *testing.T) {
		t.Parallel()
		minimal := `{"subject":{"id":"a"},"agent":{"id":"x"},"resource":{"id":"r","resource_uri":"https://a","scopes":[]},
			"grant":{"id":"g","updated_at":"2026-10-26T09:59:00Z"},"permission_sets":[{"id":"p","services":[]}]}`
		broker := &brokerStub{status: http.StatusOK, cacheControl: []string{"max-age=30"}, body: `{"decision":true,"context":` + minimal + `}`}
		result, err := newTestClient(t, broker.server(t), time.Minute).Evaluate(context.Background(), testSubject, testResource)
		require.NoError(t, err)
		assert.Empty(t, result.Context.Agent.CanonicalID)
		assert.Empty(t, result.Context.Grant.ValidUntil)
		assert.NotNil(t, result.Context.Resource.Scopes)
		assert.NotNil(t, result.Context.PermissionSets[0].Services)
	})
}

func TestClient_DenialAndFailures(t *testing.T) {
	t.Parallel()
	consent := "https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"
	tests := []struct {
		name     string
		broker   *brokerStub
		denied   *DeniedError
		category string
	}{
		{name: "allowlisted denial", broker: &brokerStub{status: 200, body: `{"decision":false,"context":{"reason_admin":{"code":"grant_missing"},"reason_user":{"consent_url":"` + consent + `"}}}`},
			denied: &DeniedError{Code: "grant_missing", ConsentURL: consent}},
		{name: "unknown denial code is generalized", broker: &brokerStub{status: 200, body: `{"decision":false,"context":{"reason_admin":{"code":"<script>"},"reason_user":{"consent_url":"` + consent + `"}}}`},
			denied: &DeniedError{Code: "delegation_denied", ConsentURL: consent}},
		{name: "relative consent URL", broker: &brokerStub{status: 200, body: `{"decision":false,"context":{"reason_admin":{"code":"grant_missing"},"reason_user":{"consent_url":"/agents/x"}}}`},
			category: CategoryInvalidResponse},
		{name: "javascript consent URL", broker: &brokerStub{status: 200, body: `{"decision":false,"context":{"reason_admin":{"code":"grant_missing"},"reason_user":{"consent_url":"javascript:alert(1)"}}}`},
			category: CategoryInvalidResponse},
		{name: "denial without context", broker: &brokerStub{status: 200, body: `{"decision":false}`}, category: CategoryInvalidResponse},
		{name: "non-boolean decision", broker: &brokerStub{status: 200, body: `{"decision":"true"}`}, category: CategoryInvalidResponse},
		{name: "malformed JSON", broker: &brokerStub{status: 200, body: `{`}, category: CategoryInvalidResponse},
		{name: "server error", broker: &brokerStub{status: 500, body: `{"error":"server_error"}`}, category: CategoryRejected},
		{name: "unauthorized", broker: &brokerStub{status: 401, body: `{}`}, category: CategoryRejected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := newTestClient(t, tt.broker.server(t), time.Minute).Evaluate(context.Background(), testSubject, testResource)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), testSubject)
			assert.NotContains(t, err.Error(), testAssertion)
			if tt.denied != nil {
				var denied *DeniedError
				require.ErrorAs(t, err, &denied)
				assert.Equal(t, tt.denied, denied)
				return
			}
			var resolveErr *Error
			require.ErrorAs(t, err, &resolveErr)
			assert.Equal(t, tt.category, resolveErr.Category)
		})
	}
}

func TestClient_TimeoutAndAssertionFailure(t *testing.T) {
	t.Parallel()
	broker := permitStub("max-age=30")
	broker.delay = 200 * time.Millisecond
	srv := broker.server(t)
	client := newTestClient(t, srv, time.Minute)
	client.httpClient = &http.Client{Timeout: 20 * time.Millisecond}
	_, err := client.Evaluate(context.Background(), testSubject, testResource)
	var resolveErr *Error
	require.ErrorAs(t, err, &resolveErr)
	assert.Equal(t, CategoryTimeout, resolveErr.Category)

	client = newTestClient(t, srv, time.Minute)
	client.assertions = staticAssertion{err: errors.New("assertion expired")}
	_, err = client.Evaluate(context.Background(), testSubject, testResource)
	require.ErrorAs(t, err, &resolveErr)
	assert.Equal(t, CategoryUnavailable, resolveErr.Category)
}
