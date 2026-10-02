package branchkey

import (
	"strings"
	"testing"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestGenerateBranchKeyId_ServiceSubject(t *testing.T) {
	tests := []struct {
		name     string
		subject  domainencryption.BranchKeySubject
		expected string
	}{
		{
			name:     "service subject",
			subject:  domainencryption.NewServiceBranchKeySubject(id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440001")),
			expected: "service_550e8400-e29b-41d4-a716-446655440001_branch_key",
		},
		{
			name:    "zero service subject returns error",
			subject: domainencryption.BranchKeySubject{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GenerateBranchKeyId(tt.subject)
			if tt.expected == "" {
				if err == nil {
					t.Fatalf("GenerateBranchKeyId(%q) expected error but got none", tt.subject.Identifier())
				}
				return
			}
			if err != nil {
				t.Fatalf("GenerateBranchKeyId(%q) unexpected error: %v", tt.subject.Identifier(), err)
			}
			if result != tt.expected {
				t.Errorf("GenerateBranchKeyId(%q) = %q, expected %q", tt.subject.Identifier(), result, tt.expected)
			}
		})
	}
}

func TestExtractSubject_ServiceSubject(t *testing.T) {
	tests := []struct {
		name          string
		branchKeyID   string
		expectedID    string
		expectedError bool
		errorContains string
	}{
		{
			name:        "valid service subject",
			branchKeyID: "service_550e8400-e29b-41d4-a716-446655440021_branch_key",
			expectedID:  "550e8400-e29b-41d4-a716-446655440021",
		},
		{
			name:          "empty service ID in key",
			branchKeyID:   "service__branch_key",
			expectedError: true,
			errorContains: "empty service ID",
		},
		{
			name:          "invalid service subject",
			branchKeyID:   "service_not-a-uuid_branch_key",
			expectedError: true,
			errorContains: "invalid service subject",
		},
		{
			name:          "missing prefix",
			branchKeyID:   "550e8400-e29b-41d4-a716-446655440021_branch_key",
			expectedError: true,
			errorContains: "invalid branch key ID format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, err := ExtractSubject(tt.branchKeyID)

			if tt.expectedError {
				if err == nil {
					t.Errorf("ExtractSubject(%q) expected error but got none", tt.branchKeyID)
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("ExtractSubject(%q) error %q does not contain %q", tt.branchKeyID, err.Error(), tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("ExtractSubject(%q) unexpected error: %v", tt.branchKeyID, err)
				return
			}

			if subject.Kind() != domainencryption.BranchKeySubjectKindService {
				t.Errorf("ExtractSubject(%q) kind = %q, expected %q", tt.branchKeyID, subject.Kind(), domainencryption.BranchKeySubjectKindService)
			}
			if subject.Identifier() != tt.expectedID {
				t.Errorf("ExtractSubject(%q) identifier = %q, expected %q", tt.branchKeyID, subject.Identifier(), tt.expectedID)
			}
		})
	}
}

func TestRefreshSessionBranchKeyID(t *testing.T) {
	const sessionUUID = "550e8400-e29b-41d4-a716-446655440025"
	sessionID := id.MustParseRefreshSessionID(sessionUUID)

	branchKeyID, err := GenerateBranchKeyId(domainencryption.NewRefreshSessionBranchKeySubject(sessionID))
	if err != nil {
		t.Fatalf("GenerateBranchKeyId(refresh session) failed: %v", err)
	}
	if want := "refresh_550e8400-e29b-41d4-a716-446655440025_branch_key"; branchKeyID != want {
		t.Fatalf("GenerateBranchKeyId(refresh session) = %q, want %q", branchKeyID, want)
	}

	extracted, err := ExtractSubject(branchKeyID)
	if err != nil {
		t.Fatalf("ExtractSubject(%q) failed: %v", branchKeyID, err)
	}
	if extracted.Kind() != domainencryption.BranchKeySubjectKindRefreshSession {
		t.Errorf("ExtractSubject(%q) kind = %q, want refresh session", branchKeyID, extracted.Kind())
	}
	extractedID, ok := extracted.RefreshSessionID()
	if !ok {
		t.Fatal("ExtractSubject did not restore a typed refresh session ID")
	}
	if extractedID != sessionID {
		t.Errorf("ExtractSubject(%q) refresh session ID = %s, want %s", branchKeyID, extractedID, sessionID)
	}
}

func TestRefreshSessionBranchKeyIDRejectsInvalidValues(t *testing.T) {
	if _, err := GenerateBranchKeyId(domainencryption.NewRefreshSessionBranchKeySubject(id.RefreshSessionID{})); err == nil {
		t.Error("GenerateBranchKeyId accepted a zero refresh session ID")
	}

	const sessionUUID = "550e8400-e29b-41d4-a716-446655440025"
	for _, tt := range []struct {
		name        string
		branchKeyID string
	}{
		{name: "empty ID", branchKeyID: "refresh__branch_key"},
		{name: "malformed UUID", branchKeyID: "refresh_not-a-uuid_branch_key"},
		{name: "zero UUID", branchKeyID: "refresh_00000000-0000-0000-0000-000000000000_branch_key"},
		{name: "wrong suffix", branchKeyID: "refresh_" + sessionUUID + "_branch"},
		{name: "trailing suffix", branchKeyID: "refresh_" + sessionUUID + "_branch_key_extra"},
		{name: "wrong prefix", branchKeyID: "refreshed_" + sessionUUID + "_branch_key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ExtractSubject(tt.branchKeyID); err == nil {
				t.Errorf("ExtractSubject(%q) accepted an invalid refresh session branch key ID", tt.branchKeyID)
			}
		})
	}
}

// TestSymmetricOperations verifies that GenerateBranchKeyId and ExtractSubject are inverse operations.
func TestSymmetricOperations(t *testing.T) {
	testServiceIDs := []struct {
		name string
		uuid string
	}{
		{name: "oauth2", uuid: "550e8400-e29b-41d4-a716-446655440011"},
		{name: "github", uuid: "550e8400-e29b-41d4-a716-446655440012"},
	}

	for _, tt := range testServiceIDs {
		t.Run(tt.name, func(t *testing.T) {
			subject := domainencryption.NewServiceBranchKeySubject(id.MustParseServiceID(tt.uuid))
			branchKeyID, err := GenerateBranchKeyId(subject)
			if err != nil {
				t.Fatalf("GenerateBranchKeyId failed for %q: %v", tt.uuid, err)
			}
			extractedSubject, err := ExtractSubject(branchKeyID)

			if err != nil {
				t.Errorf("ExtractSubject failed for generated ID: %v", err)
				return
			}

			if extractedSubject.Kind() != subject.Kind() {
				t.Errorf("Symmetric operation failed: expected kind %q, got %q", subject.Kind(), extractedSubject.Kind())
			}
			if extractedSubject.Identifier() != subject.Identifier() {
				t.Errorf("Symmetric operation failed: %q -> %q -> %q", tt.uuid, branchKeyID, extractedSubject.Identifier())
			}

			if _, err := ExtractSubject(branchKeyID); err != nil {
				t.Errorf("ExtractSubject failed validation for generated ID %q: %v", branchKeyID, err)
			}
		})
	}
}
