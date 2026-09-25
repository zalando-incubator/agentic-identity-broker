package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"
)

// ApprovalBrokerRequest records one externally visible ExtProc-to-broker request.
// Tests use it to assert endpoint-specific authentication and long-poll semantics.
type ApprovalBrokerRequest struct {
	Method          string
	Path            string
	Authorization   string
	ClientAssertion string
	IfNoneMatch     string
	LongPollTimeout string
	Query           url.Values
	Body            []byte
}

// ApprovalBrokerPair mirrors the approval-sync response consumed by ExtProc.
type ApprovalBrokerPair struct {
	Principal             string                 `json:"principal"`
	AgentID               string                 `json:"agent_id"`
	Approvals             []ApprovalBrokerRecord `json:"approvals"`
	GrantedPermissionSets map[string]any         `json:"granted_permission_sets"`
}

// ApprovalBrokerRecord mirrors the approval fields ExtProc uses from an approval-sync snapshot.
type ApprovalBrokerRecord struct {
	ID             string            `json:"id"`
	ToolName       string            `json:"tool_name"`
	ArgumentsHash  string            `json:"arguments_hash"`
	ToolPattern    string            `json:"tool_pattern"`
	ParamsPattern  map[string]string `json:"params_pattern"`
	Status         string            `json:"status"`
	Persistence    *string           `json:"persistence,omitempty"`
	Consumed       bool              `json:"consumed"`
	AgentSessionID *string           `json:"agent_session_id,omitempty"`
	ApprovedAt     *time.Time        `json:"approved_at,omitempty"`
}

// ApprovalBrokerSyncResponse controls one pending long-poll response. Abort closes
// the connection without an HTTP response, simulating a network interruption.
type ApprovalBrokerSyncResponse struct {
	Status int
	ETag   string
	Pairs  []ApprovalBrokerPair
	Abort  bool
}

// MockApprovalBroker is a synchronized, broker-shaped fake for approval API E2E tests.
// Bootstrap and targeted reads return the configured full snapshot immediately. A
// long-poll waits until a test replies, so tests never spin on synthetic 304 responses.
type MockApprovalBroker struct {
	server *httptest.Server

	mu                    sync.Mutex
	snapshot              []ApprovalBrokerPair
	etag                  string
	bootstrapStatus       int
	targetedReadStatus    int
	createStatus          int
	createURL             string
	createSequence        int
	consumeStatus         map[string]int
	requests              []ApprovalBrokerRequest
	longPollWaiters       []chan ApprovalBrokerSyncResponse
	queuedLongPollReplies []ApprovalBrokerSyncResponse
}

// NewMockApprovalBroker creates a fake with an empty successful snapshot and a
// successful idempotent approval-creation response.
func NewMockApprovalBroker() *MockApprovalBroker {
	return &MockApprovalBroker{
		etag:               `"v1"`,
		bootstrapStatus:    http.StatusOK,
		targetedReadStatus: http.StatusOK,
		createStatus:       http.StatusCreated,
		consumeStatus:      make(map[string]int),
	}
}

// Start starts the approval endpoint. It is safe to call once per fake.
func (m *MockApprovalBroker) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		return
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.handleApprovals))
}

// Stop closes the fake server and releases any pending long-poll handlers.
func (m *MockApprovalBroker) Stop() {
	m.mu.Lock()
	server := m.server
	m.server = nil
	waiters := m.longPollWaiters
	m.longPollWaiters = nil
	m.mu.Unlock()

	for _, waiter := range waiters {
		waiter <- ApprovalBrokerSyncResponse{Status: http.StatusServiceUnavailable}
	}
	if server != nil {
		server.Close()
	}
}

// URL returns the fake broker base URL, or an empty string before Start.
func (m *MockApprovalBroker) URL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server == nil {
		return ""
	}
	return m.server.URL
}

// SetSnapshot configures the complete approval state returned by successful
// bootstrap and targeted reads. ETag is stored verbatim, including quotes.
func (m *MockApprovalBroker) SetSnapshot(etag string, pairs ...ApprovalBrokerPair) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.etag = etag
	m.snapshot = cloneApprovalBrokerPairs(pairs)
}

// SetBootstrapStatus controls unfiltered non-long-poll GET responses.
func (m *MockApprovalBroker) SetBootstrapStatus(status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bootstrapStatus = status
}

// SetTargetedReadStatus controls GET responses carrying a principal query parameter.
func (m *MockApprovalBroker) SetTargetedReadStatus(status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.targetedReadStatus = status
}

// SetCreateResponse overrides the response returned by POST /api/approvals.
// A non-empty URL models an idempotent pending-approval response.
func (m *MockApprovalBroker) SetCreateResponse(status int, approvalURL string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createStatus = status
	m.createURL = approvalURL
}

// SetConsumeStatus controls a consume response for one approval id.
func (m *MockApprovalBroker) SetConsumeStatus(approvalID string, status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consumeStatus[approvalID] = status
}

// ReplyToNextLongPoll resolves the oldest pending long-poll. If no poll is
// pending, the response is delivered to the next long-poll request.
func (m *MockApprovalBroker) ReplyToNextLongPoll(response ApprovalBrokerSyncResponse) {
	m.mu.Lock()
	if len(m.longPollWaiters) == 0 {
		m.queuedLongPollReplies = append(m.queuedLongPollReplies, cloneApprovalBrokerResponse(response))
		m.mu.Unlock()
		return
	}
	waiter := m.longPollWaiters[0]
	m.longPollWaiters = m.longPollWaiters[1:]
	m.mu.Unlock()
	waiter <- cloneApprovalBrokerResponse(response)
}

// PublishSnapshot updates the configured complete state and delivers it to one
// long-poll request, modeling the broker wake-up after an approval mutation.
func (m *MockApprovalBroker) PublishSnapshot(etag string, pairs ...ApprovalBrokerPair) {
	m.SetSnapshot(etag, pairs...)
	m.ReplyToNextLongPoll(ApprovalBrokerSyncResponse{Status: http.StatusOK, ETag: etag, Pairs: pairs})
}

// Requests returns a defensive copy of the complete broker request history.
func (m *MockApprovalBroker) Requests() []ApprovalBrokerRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]ApprovalBrokerRequest, len(m.requests))
	for i, request := range m.requests {
		requests[i] = cloneApprovalBrokerRequest(request)
	}
	return requests
}

// BootstrapRequestCount returns unfiltered GET requests without an ETag.
func (m *MockApprovalBroker) BootstrapRequestCount() int {
	return m.requestCount(func(request ApprovalBrokerRequest) bool {
		return request.Method == http.MethodGet && request.Query.Get("principal") == "" && request.IfNoneMatch == ""
	})
}

// LongPollRequestCount returns GET requests carrying an If-None-Match header.
func (m *MockApprovalBroker) LongPollRequestCount() int {
	return m.requestCount(func(request ApprovalBrokerRequest) bool {
		return request.Method == http.MethodGet && request.IfNoneMatch != ""
	})
}

// TargetedReadCount returns GET requests scoped by a principal query parameter.
func (m *MockApprovalBroker) TargetedReadCount() int {
	return m.requestCount(func(request ApprovalBrokerRequest) bool {
		return request.Method == http.MethodGet && request.Query.Get("principal") != ""
	})
}

// CreateCount returns POST /api/approvals requests.
func (m *MockApprovalBroker) CreateCount() int {
	return m.requestCount(func(request ApprovalBrokerRequest) bool {
		return request.Method == http.MethodPost && request.Path == "/api/approvals"
	})
}

// ConsumeCount returns POST consume requests.
func (m *MockApprovalBroker) ConsumeCount() int {
	return m.requestCount(func(request ApprovalBrokerRequest) bool {
		return request.Method == http.MethodPost && len(request.Path) > len("/api/approvals/") && request.Path[len(request.Path)-len("/consume"):] == "/consume"
	})
}

func (m *MockApprovalBroker) requestCount(matches func(ApprovalBrokerRequest) bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, request := range m.requests {
		if matches(request) {
			count++
		}
	}
	return count
}

func (m *MockApprovalBroker) handleApprovals(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}
	request := ApprovalBrokerRequest{
		Method:          r.Method,
		Path:            r.URL.Path,
		Authorization:   r.Header.Get("Authorization"),
		ClientAssertion: r.Header.Get("X-Client-Assertion"),
		IfNoneMatch:     r.Header.Get("If-None-Match"),
		LongPollTimeout: r.Header.Get("X-Long-Poll-Timeout"),
		Query:           r.URL.Query(),
		Body:            append([]byte(nil), body...),
	}

	m.mu.Lock()
	m.requests = append(m.requests, cloneApprovalBrokerRequest(request))

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/approvals":
		m.handleSyncRequestLocked(w, r, request)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/api/approvals":
		status := m.createStatus
		m.createSequence++
		approvalID := fmt.Sprintf("approval-%d", m.createSequence)
		approvalURL := m.createURL
		if approvalURL == "" {
			approvalURL = "https://broker.example/approvals/" + approvalID
		}
		m.mu.Unlock()
		m.writeCreateResponse(w, status, approvalID, approvalURL)
		return
	case r.Method == http.MethodPost:
		status := m.consumeStatus[r.URL.Path]
		if status == 0 {
			status = http.StatusOK
		}
		if status == http.StatusOK {
			m.markConsumedLocked(approvalIDFromConsumePath(r.URL.Path))
		}
		m.mu.Unlock()
		w.WriteHeader(status)
		return
	default:
		m.mu.Unlock()
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func (m *MockApprovalBroker) handleSyncRequestLocked(w http.ResponseWriter, r *http.Request, request ApprovalBrokerRequest) {
	if request.Query.Get("principal") != "" {
		status := m.targetedReadStatus
		etag := m.etag
		pairs := cloneApprovalBrokerPairs(m.snapshot)
		m.mu.Unlock()
		m.writeSyncResponse(w, status, etag, pairs)
		return
	}
	if request.IfNoneMatch == "" {
		status := m.bootstrapStatus
		etag := m.etag
		pairs := cloneApprovalBrokerPairs(m.snapshot)
		m.mu.Unlock()
		m.writeSyncResponse(w, status, etag, pairs)
		return
	}

	waiter := make(chan ApprovalBrokerSyncResponse, 1)
	if len(m.queuedLongPollReplies) > 0 {
		response := m.queuedLongPollReplies[0]
		m.queuedLongPollReplies = m.queuedLongPollReplies[1:]
		m.mu.Unlock()
		m.writeLongPollResponse(w, response)
		return
	}
	m.longPollWaiters = append(m.longPollWaiters, waiter)
	m.mu.Unlock()

	select {
	case response := <-waiter:
		m.writeLongPollResponse(w, response)
	case <-r.Context().Done():
	}
}

func (m *MockApprovalBroker) writeLongPollResponse(w http.ResponseWriter, response ApprovalBrokerSyncResponse) {
	if response.Abort {
		panic(http.ErrAbortHandler)
	}
	if response.Status == 0 {
		response.Status = http.StatusOK
	}
	if response.Status == http.StatusOK && response.ETag == "" {
		m.mu.Lock()
		response.ETag = m.etag
		response.Pairs = cloneApprovalBrokerPairs(m.snapshot)
		m.mu.Unlock()
	}
	m.writeSyncResponse(w, response.Status, response.ETag, response.Pairs)
}

func (m *MockApprovalBroker) writeSyncResponse(w http.ResponseWriter, status int, etag string, pairs []ApprovalBrokerPair) {
	if status == 0 {
		status = http.StatusOK
	}
	if status != http.StatusOK {
		http.Error(w, "approval broker unavailable", status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	_ = json.NewEncoder(w).Encode(struct {
		Data struct {
			Pairs []ApprovalBrokerPair `json:"pairs"`
		} `json:"data"`
	}{Data: struct {
		Pairs []ApprovalBrokerPair `json:"pairs"`
	}{Pairs: pairs}})
}

func (m *MockApprovalBroker) writeCreateResponse(w http.ResponseWriter, status int, approvalID, approvalURL string) {
	if status != http.StatusCreated && status != http.StatusOK {
		http.Error(w, "approval broker unavailable", status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Data struct {
			ID          string `json:"id"`
			ApprovalURL string `json:"approval_url"`
		} `json:"data"`
	}{Data: struct {
		ID          string `json:"id"`
		ApprovalURL string `json:"approval_url"`
	}{ID: approvalID, ApprovalURL: approvalURL}})
}

func (m *MockApprovalBroker) markConsumedLocked(approvalID string) {
	for pairIndex := range m.snapshot {
		for approvalIndex := range m.snapshot[pairIndex].Approvals {
			if m.snapshot[pairIndex].Approvals[approvalIndex].ID == approvalID {
				m.snapshot[pairIndex].Approvals[approvalIndex].Consumed = true
			}
		}
	}
}

func approvalIDFromConsumePath(path string) string {
	const prefix = "/api/approvals/"
	const suffix = "/consume"
	if len(path) <= len(prefix)+len(suffix) {
		return ""
	}
	return path[len(prefix) : len(path)-len(suffix)]
}

func cloneApprovalBrokerResponse(response ApprovalBrokerSyncResponse) ApprovalBrokerSyncResponse {
	response.Pairs = cloneApprovalBrokerPairs(response.Pairs)
	return response
}

func cloneApprovalBrokerRequest(request ApprovalBrokerRequest) ApprovalBrokerRequest {
	query := make(url.Values, len(request.Query))
	for key, values := range request.Query {
		query[key] = append([]string(nil), values...)
	}
	request.Query = query
	request.Body = append([]byte(nil), request.Body...)
	return request
}

func cloneApprovalBrokerPairs(pairs []ApprovalBrokerPair) []ApprovalBrokerPair {
	cloned := make([]ApprovalBrokerPair, len(pairs))
	for pairIndex, pair := range pairs {
		cloned[pairIndex] = pair
		cloned[pairIndex].GrantedPermissionSets = cloneStringAnyMap(pair.GrantedPermissionSets)
		cloned[pairIndex].Approvals = make([]ApprovalBrokerRecord, len(pair.Approvals))
		for approvalIndex, approval := range pair.Approvals {
			cloned[pairIndex].Approvals[approvalIndex] = approval
			cloned[pairIndex].Approvals[approvalIndex].ParamsPattern = cloneStringMap(approval.ParamsPattern)
			if approval.Persistence != nil {
				persistence := *approval.Persistence
				cloned[pairIndex].Approvals[approvalIndex].Persistence = &persistence
			}
			if approval.AgentSessionID != nil {
				sessionID := *approval.AgentSessionID
				cloned[pairIndex].Approvals[approvalIndex].AgentSessionID = &sessionID
			}
			if approval.ApprovedAt != nil {
				approvedAt := *approval.ApprovedAt
				cloned[pairIndex].Approvals[approvalIndex].ApprovedAt = &approvedAt
			}
		}
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneStringAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
