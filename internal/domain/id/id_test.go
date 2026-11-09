package id

import (
	"database/sql/driver"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"
)

// ---- UUID-based ID tests ----

func TestAgentID_NewAndString(t *testing.T) {
	id := NewAgentID()
	if id.IsZero() {
		t.Fatal("NewAgentID() produced zero value")
	}
	s := id.String()
	if _, err := uuid.Parse(s); err != nil {
		t.Fatalf("String() not a valid UUID: %v", err)
	}
}

func TestAgentID_ParseRoundTrip(t *testing.T) {
	const raw = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	id, err := ParseAgentID(raw)
	if err != nil {
		t.Fatalf("ParseAgentID failed: %v", err)
	}
	if id.String() != raw {
		t.Fatalf("String() = %q, want %q", id.String(), raw)
	}
}

func TestAgentID_ParseInvalid(t *testing.T) {
	if _, err := ParseAgentID("not-a-uuid"); err == nil {
		t.Fatal("expected error for invalid UUID")
	}
}

func TestAgentID_MustParse(t *testing.T) {
	const raw = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	id := MustParseAgentID(raw)
	if id.String() != raw {
		t.Fatalf("MustParseAgentID() = %q, want %q", id.String(), raw)
	}
}

func TestAgentID_MustParsePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid UUID")
		}
	}()
	MustParseAgentID("not-a-uuid")
}

func TestAgentID_IsZero(t *testing.T) {
	var zero AgentID
	if !zero.IsZero() {
		t.Fatal("zero value should be IsZero()")
	}
	nonZero := NewAgentID()
	if nonZero.IsZero() {
		t.Fatal("new value should not be IsZero()")
	}
}

func TestAgentID_JSONRoundTrip(t *testing.T) {
	type wrapper struct {
		ID AgentID `json:"id"`
	}
	original := wrapper{ID: MustParseAgentID("f47ac10b-58cc-4372-a567-0e02b2c3d479")}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	// Verify JSON output is a quoted UUID string, not a byte array
	expected := `{"id":"f47ac10b-58cc-4372-a567-0e02b2c3d479"}`
	if string(b) != expected {
		t.Fatalf("JSON = %s, want %s", string(b), expected)
	}

	var decoded wrapper
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.ID != original.ID {
		t.Fatalf("round-trip mismatch: got %v, want %v", decoded.ID, original.ID)
	}
}

func TestAgentID_SQLValueAndScan(t *testing.T) {
	original := MustParseAgentID("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	// Value returns string
	val, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("Value() type = %T, want string", val)
	}
	if s != "f47ac10b-58cc-4372-a567-0e02b2c3d479" {
		t.Fatalf("Value() = %q, want %q", s, "f47ac10b-58cc-4372-a567-0e02b2c3d479")
	}

	// Scan from string
	var scanned AgentID
	if err := scanned.Scan(s); err != nil {
		t.Fatalf("Scan(string) error: %v", err)
	}
	if scanned != original {
		t.Fatalf("Scan(string) = %v, want %v", scanned, original)
	}

	// Scan from []byte
	var scanned2 AgentID
	if err := scanned2.Scan([]byte(s)); err != nil {
		t.Fatalf("Scan([]byte) error: %v", err)
	}
	if scanned2 != original {
		t.Fatalf("Scan([]byte) = %v, want %v", scanned2, original)
	}
}

func TestAgentID_TextMarshalRoundTrip(t *testing.T) {
	original := MustParseAgentID("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	b, err := original.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	if string(b) != "f47ac10b-58cc-4372-a567-0e02b2c3d479" {
		t.Fatalf("MarshalText = %q, want UUID string", string(b))
	}

	var decoded AgentID
	if err := decoded.UnmarshalText(b); err != nil {
		t.Fatalf("UnmarshalText error: %v", err)
	}
	if decoded != original {
		t.Fatalf("round-trip mismatch: got %v, want %v", decoded, original)
	}
}

func TestAgentID_Comparable(t *testing.T) {
	// Verify UUID-backed IDs are comparable (usable as map keys)
	id1 := MustParseAgentID("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	id2 := MustParseAgentID("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	id3 := NewAgentID()

	if id1 != id2 {
		t.Fatal("identical UUIDs should be equal")
	}
	if id1 == id3 {
		t.Fatal("different UUIDs should not be equal")
	}

	// Map key usage
	m := map[AgentID]string{id1: "test"}
	if m[id2] != "test" {
		t.Fatal("map lookup with equal key failed")
	}
}

// Compile-time type safety: AgentID ≠ ServiceID
// Uncomment these to verify compile failure:
// func TestCompileTimeTypeSafety(t *testing.T) {
// 	var a AgentID
// 	var s ServiceID
// 	_ = a == s // compile error: mismatched types
// }

func TestServiceID_RoundTrip(t *testing.T) {
	const raw = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	id := MustParseServiceID(raw)
	if id.String() != raw {
		t.Fatalf("ServiceID round-trip failed: got %q", id.String())
	}
	if id.IsZero() {
		t.Fatal("non-zero ServiceID reports IsZero()")
	}
}

func TestGrantID_RoundTrip(t *testing.T) {
	id := NewGrantID()
	s := id.String()
	parsed, err := ParseGrantID(s)
	if err != nil {
		t.Fatalf("ParseGrantID failed: %v", err)
	}
	if parsed != id {
		t.Fatal("GrantID round-trip mismatch")
	}
}

func TestSessionID_RoundTrip(t *testing.T) {
	id := NewSessionID()
	s := id.String()
	parsed, err := ParseSessionID(s)
	if err != nil {
		t.Fatalf("ParseSessionID failed: %v", err)
	}
	if parsed != id {
		t.Fatal("SessionID round-trip mismatch")
	}
}

func TestUserID_RoundTrip(t *testing.T) {
	id := NewUserID()
	s := id.String()
	parsed, err := ParseUserID(s)
	if err != nil {
		t.Fatalf("ParseUserID failed: %v", err)
	}
	if parsed != id {
		t.Fatal("UserID round-trip mismatch")
	}
}

// ---- UUID types implement driver.Valuer ----

func TestUUIDTypes_ImplementDriverValuer(t *testing.T) {
	var _ driver.Valuer = AgentID{}
	var _ driver.Valuer = ServiceID{}
	var _ driver.Valuer = GrantID{}
	var _ driver.Valuer = SessionID{}
	var _ driver.Valuer = UserID{}
}

// ---- String-based ID tests ----

func TestClientID_Basic(t *testing.T) {
	id := NewClientID("my-oauth-client")
	if id.String() != "my-oauth-client" {
		t.Fatalf("ClientID String() = %q, want %q", id.String(), "my-oauth-client")
	}
	if id.IsZero() {
		t.Fatal("non-empty ClientID should not be IsZero()")
	}

	var zero ClientID
	if !zero.IsZero() {
		t.Fatal("zero ClientID should be IsZero()")
	}
}

func TestExternalID_Basic(t *testing.T) {
	id := NewExternalID("ext-123")
	if id.String() != "ext-123" {
		t.Fatalf("ExternalID String() = %q, want %q", id.String(), "ext-123")
	}
	if id.IsZero() {
		t.Fatal("non-empty ExternalID should not be IsZero()")
	}

	var zero ExternalID
	if !zero.IsZero() {
		t.Fatal("zero ExternalID should be IsZero()")
	}
}

func TestPrincipal_Basic(t *testing.T) {
	p := NewPrincipal("user@example.com")
	if p.String() != "user@example.com" {
		t.Fatalf("Principal String() = %q, want %q", p.String(), "user@example.com")
	}
	if p.IsZero() {
		t.Fatal("non-empty Principal should not be IsZero()")
	}

	var zero Principal
	if !zero.IsZero() {
		t.Fatal("zero Principal should be IsZero()")
	}
}

// ---- Golden test: JSON output identical to string-based approach ----

func TestGolden_JSONOutputIdenticalToStringBased(t *testing.T) {
	// Struct with typed IDs
	type TypedStruct struct {
		AgentID   AgentID   `json:"agent_id"`
		ServiceID ServiceID `json:"service_id"`
	}

	// Equivalent struct with plain strings
	type StringStruct struct {
		AgentID   string `json:"agent_id"`
		ServiceID string `json:"service_id"`
	}

	const (
		agentUUID   = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
		serviceUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	)

	typed := TypedStruct{
		AgentID:   MustParseAgentID(agentUUID),
		ServiceID: MustParseServiceID(serviceUUID),
	}
	stringBased := StringStruct{
		AgentID:   agentUUID,
		ServiceID: serviceUUID,
	}

	typedJSON, err := json.Marshal(typed)
	if err != nil {
		t.Fatalf("Marshal typed failed: %v", err)
	}
	stringJSON, err := json.Marshal(stringBased)
	if err != nil {
		t.Fatalf("Marshal string failed: %v", err)
	}

	if string(typedJSON) != string(stringJSON) {
		t.Fatalf("JSON output mismatch:\n  typed:  %s\n  string: %s", typedJSON, stringJSON)
	}
}

func TestPrincipal_LogValue(t *testing.T) {
	var p slog.LogValuer = NewPrincipal("user@example.com")
	v := p.LogValue()
	if v.Kind() != slog.KindString || v.String() != "user@example.com" {
		t.Fatalf("Principal LogValue() = %v (%s), want string %q", v, v.Kind(), "user@example.com")
	}
}
