package matchers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/onsi/gomega/types"
	"google.golang.org/protobuf/proto"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

// BusinessEventExpectation leaves nil fields unconstrained. Subject uses the
// production selector so an explicitly absent subject differs from no assertion.
// A supplied actor compares all three fields, including explicit null identities.
type BusinessEventExpectation struct {
	Type             string
	Subject          *model.BusinessEventSubject
	Actor            *model.BusinessEventActor
	AgentID          *id.AgentID
	GatewayClientID  *id.ClientID
	ServiceID        *id.ServiceID
	PermissionSetIDs []id.PermissionSetID
	GrantID          *id.GrantID
	SessionID        *id.SessionID
	ApprovalID       *id.ApprovalID
	MCPSessionID     *string
	AgentSessionID   *string
	Outcome          model.BusinessEventOutcome
	ReasonUser       *string
	ReasonAdmin      *string
	TraceID          *string
	SpanID           *string
	Client           *model.BusinessEventClient
	Data             map[string]any
}

// HaveBusinessEventEnvelope accepts a model.BusinessEvent or its pointer. It
// checks envelope invariants as well as every supplied expectation, without
// rendering the actual envelope (which may contain a credential on failure).
func HaveBusinessEventEnvelope(expected BusinessEventExpectation) types.GomegaMatcher {
	return &businessEventEnvelopeMatcher{expected: expected}
}

type businessEventEnvelopeMatcher struct {
	expected   BusinessEventExpectation
	violations []string
}

var (
	ledgerTypePattern  = regexp.MustCompile(`^agentic-identity-broker\.[a-z][a-z0-9-]*$`)
	ledgerTracePattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	ledgerSpanPattern  = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

func (m *businessEventEnvelopeMatcher) Match(actual any) (bool, error) {
	m.violations = nil
	var event *model.BusinessEvent
	switch value := actual.(type) {
	case *model.BusinessEvent:
		event = value
	case model.BusinessEvent:
		event = &value
	default:
		return false, fmt.Errorf("HaveBusinessEventEnvelope expects a business event, got %T", actual)
	}
	if event == nil {
		return false, errors.New("HaveBusinessEventEnvelope received a nil event")
	}
	if subject := m.expected.Subject; subject != nil {
		hasPrincipal := !subject.Principal.IsZero()
		if subject.NoSubject == hasPrincipal {
			return false, errors.New("expected subject must select exactly one principal or no subject")
		}
	}
	m.check(uuid.UUID(event.ID).Version() == 7 && uuid.UUID(event.ID).Variant() == uuid.RFC4122, "id must be UUIDv7")
	m.check(ledgerTypePattern.MatchString(event.Type), "type must use the broker namespace")
	m.check(event.Source == "urn:agentic-identity-broker:broker", "source must identify the broker")
	m.check(ledgerUTC(event.OccurredAt), "occurred_at must be a nonzero UTC instant")
	m.check(ledgerUTC(event.RecordedAt), "recorded_at must be a nonzero UTC instant")
	m.check(event.Subject == nil || !event.Subject.IsZero(), "subject must be null or nonempty")
	m.check(event.Actor.ID == nil || *event.Actor.ID != "", "actor.id must be null or nonempty")
	m.check(event.Actor.OnBehalfOf == nil || !event.Actor.OnBehalfOf.IsZero(), "actor.on_behalf_of must be null or nonempty")
	switch event.Actor.Kind {
	case "user", "admin", "agent", "gateway", "system", "policy":
	default:
		m.check(false, "actor.kind must be a known identity kind")
	}
	switch event.Outcome {
	case model.BusinessEventSuccess, model.BusinessEventFailure, model.BusinessEventDenied, model.BusinessEventPending:
	default:
		m.check(false, "outcome must be a known outcome")
	}
	m.check(strings.TrimSpace(event.ReasonUser) != "", "reason_user is required")
	m.check(strings.TrimSpace(event.ReasonAdmin) != "", "reason_admin is required")
	m.check(event.Data != nil, "data must be an object, not null")
	for index, permissionSetID := range event.PermissionSetIDs {
		m.check(!permissionSetID.IsZero(), "permission_set_ids must contain nonzero IDs")
		if index > 0 {
			m.check(event.PermissionSetIDs[index-1].String() < permissionSetID.String(), "permission_set_ids must be sorted and unique")
		}
	}
	if event.TraceID != "" {
		m.check(ledgerTracePattern.MatchString(event.TraceID) && event.TraceID != "00000000000000000000000000000000", "trace_id must be nonzero lowercase hex")
	}
	if event.SpanID != "" {
		m.check(ledgerSpanPattern.MatchString(event.SpanID) && event.SpanID != "0000000000000000", "span_id must be nonzero lowercase hex")
		m.check(event.TraceID != "", "span_id requires trace_id")
	}
	if event.Client != nil {
		m.check(event.Client.IP != "" || event.Client.UserAgent != "", "client must not be an empty object")
		if event.Client.IP != "" {
			address, err := netip.ParseAddr(event.Client.IP)
			m.check(err == nil && address.Zone() == "", "client.ip must be an IP address without a zone")
		}
		switch event.Client.UserAgent {
		case "", "Chrome", "Firefox", "Safari", "Edge", "curl", "Go-http-client":
		default:
			m.check(false, "client.user_agent must be an allowed product name")
		}
	}
	m.matchExpected(event)
	return len(m.violations) == 0, nil
}

func ledgerUTC(value time.Time) bool {
	_, offset := value.Zone()
	return !value.IsZero() && offset == 0
}

func (m *businessEventEnvelopeMatcher) check(valid bool, violation string) {
	if !valid {
		m.violations = append(m.violations, violation)
	}
}

func (m *businessEventEnvelopeMatcher) matchExpected(event *model.BusinessEvent) {
	expected := m.expected
	if expected.Type != "" {
		m.check(event.Type == expected.Type, "type differs")
	}
	if expected.Subject != nil {
		if expected.Subject.NoSubject {
			m.check(event.Subject == nil, "subject must be null")
		} else {
			m.check(event.Subject != nil && *event.Subject == expected.Subject.Principal, "subject differs")
		}
	}
	if expected.Outcome != "" {
		m.check(event.Outcome == expected.Outcome, "outcome differs")
	}
	if expected.PermissionSetIDs != nil {
		m.check(reflect.DeepEqual(event.PermissionSetIDs, expected.PermissionSetIDs), "permission_set_ids differ")
	}
	if expected.Data != nil {
		m.check(reflect.DeepEqual(event.Data, expected.Data), "data differs")
	}
	for _, field := range []struct {
		name     string
		actual   any
		expected any
	}{
		{"actor", event.Actor, expected.Actor},
		{"agent_id", event.AgentID, expected.AgentID},
		{"gateway_client_id", event.GatewayClientID, expected.GatewayClientID},
		{"service_id", event.ServiceID, expected.ServiceID},
		{"grant_id", event.GrantID, expected.GrantID},
		{"session_id", event.SessionID, expected.SessionID},
		{"approval_id", event.ApprovalID, expected.ApprovalID},
		{"mcp_session_id", event.MCPSessionID, expected.MCPSessionID},
		{"agent_session_id", event.AgentSessionID, expected.AgentSessionID},
		{"reason_user", event.ReasonUser, expected.ReasonUser},
		{"reason_admin", event.ReasonAdmin, expected.ReasonAdmin},
		{"trace_id", event.TraceID, expected.TraceID},
		{"span_id", event.SpanID, expected.SpanID},
	} {
		value := reflect.ValueOf(field.expected)
		if !value.IsNil() {
			m.check(reflect.DeepEqual(field.actual, value.Elem().Interface()), field.name+" differs")
		}
	}
	if expected.Client != nil {
		m.check(reflect.DeepEqual(event.Client, expected.Client), "client differs")
	}
}

func (m *businessEventEnvelopeMatcher) FailureMessage(_ any) string {
	return "Expected a valid business event envelope matching the supplied fields; " + strings.Join(m.violations, "; ")
}

func (m *businessEventEnvelopeMatcher) NegatedFailureMessage(_ any) string {
	return "Expected the business event envelope not to match the supplied fields (values withheld)"
}

// BeFreeOfLedgerCredentials scans the complete input, including map keys,
// nested values, raw bytes, protobuf unknown fields, resource and scope fields.
// It accepts events, receiver snapshots, OTLP protobufs, and collections of them.
// All seven canaries are required; diagnostics name classes, never values.
func BeFreeOfLedgerCredentials(canaries fixtures.CredentialCanaries) types.GomegaMatcher {
	values := [...]string{
		canaries.AccessToken, canaries.RefreshToken, canaries.ClientSecret,
		canaries.ClientAssertion, canaries.RawJWT, canaries.AuthorizationCode, canaries.PKCEVerifier,
	}
	matcher := &ledgerCredentialMatcher{}
	for index, value := range values {
		if value == "" {
			matcher.incomplete = true
			continue
		}
		// JSON encodes byte slices as base64; retain reflection scanning too so
		// a canary embedded inside a larger byte slice is not missed.
		for _, pattern := range []string{
			value,
			base64.StdEncoding.EncodeToString([]byte(value)),
			base64.RawStdEncoding.EncodeToString([]byte(value)),
			base64.URLEncoding.EncodeToString([]byte(value)),
			base64.RawURLEncoding.EncodeToString([]byte(value)),
		} {
			matcher.patterns = append(matcher.patterns, credentialPattern{class: index, text: pattern, bytes: []byte(pattern)})
		}
	}
	return matcher
}

type credentialPattern struct {
	class int
	text  string
	bytes []byte
}

type ledgerCredentialMatcher struct {
	patterns   []credentialPattern
	found      [7]bool
	incomplete bool
}

var ledgerCredentialClasses = [...]string{
	"access token", "refresh token", "client secret", "client assertion", "raw JWT", "authorization code", "PKCE verifier",
}

func (m *ledgerCredentialMatcher) Match(actual any) (bool, error) {
	m.found = [7]bool{}
	if m.incomplete {
		return false, errors.New("credential exclusion requires all seven nonempty canaries")
	}
	if actual == nil {
		return false, errors.New("credential exclusion requires a non-nil input")
	}
	value := reflect.ValueOf(actual)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return false, errors.New("credential exclusion requires a non-nil input")
	}
	serialized, err := json.Marshal(actual)
	if err != nil {
		return false, errors.New("credential exclusion could not serialize input; values withheld")
	}
	m.scanBytes(serialized)
	if err := m.scanValue(value, 0); err != nil {
		return false, err
	}
	for _, found := range m.found {
		if found {
			return false, nil
		}
	}
	return true, nil
}

func (m *ledgerCredentialMatcher) scanBytes(value []byte) {
	for _, pattern := range m.patterns {
		if bytes.Contains(value, pattern.bytes) {
			m.found[pattern.class] = true
		}
	}
}

func (m *ledgerCredentialMatcher) scanValue(value reflect.Value, depth int) error {
	if !value.IsValid() {
		return nil
	}
	if depth > 128 {
		return errors.New("credential exclusion input is cyclic or exceeds the nesting limit; values withheld")
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		if value.CanInterface() {
			if message, ok := value.Interface().(proto.Message); ok {
				encoded, err := proto.Marshal(message)
				if err != nil {
					return errors.New("credential exclusion could not inspect protobuf input; values withheld")
				}
				m.scanBytes(encoded)
				return nil
			}
		}
		return m.scanValue(value.Elem(), depth+1)
	}
	switch value.Kind() {
	case reflect.String:
		for _, pattern := range m.patterns {
			if strings.Contains(value.String(), pattern.text) {
				m.found[pattern.class] = true
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if err := m.scanValue(iterator.Key(), depth+1); err != nil {
				return err
			}
			if err := m.scanValue(iterator.Value(), depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			m.scanBytes(value.Bytes())
			return nil
		}
		fallthrough
	case reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			m.scanByteArray(value)
			return nil
		}
		for index := range value.Len() {
			if err := m.scanValue(value.Index(index), depth+1); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for index := range value.NumField() {
			if value.Type().Field(index).IsExported() {
				if err := m.scanValue(value.Field(index), depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m *ledgerCredentialMatcher) scanByteArray(value reflect.Value) {
	for _, pattern := range m.patterns {
		for start := 0; start+len(pattern.bytes) <= value.Len(); start++ {
			matches := true
			for offset, expected := range pattern.bytes {
				if value.Index(start+offset).Uint() != uint64(expected) {
					matches = false
					break
				}
			}
			if matches {
				m.found[pattern.class] = true
				break
			}
		}
	}
}

func (m *ledgerCredentialMatcher) FailureMessage(_ any) string {
	var classes []string
	for index, found := range m.found {
		if found {
			classes = append(classes, ledgerCredentialClasses[index])
		}
	}
	return "Expected no ledger credential canaries; detected classes: " + strings.Join(classes, ", ") + " (values withheld)"
}

func (m *ledgerCredentialMatcher) NegatedFailureMessage(_ any) string {
	return "Expected a ledger credential canary, but none of the seven classes was found (values withheld)"
}
