package encryption

import (
	"fmt"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// BranchKeySubjectKind identifies the type of data namespace routed to a branch key.
type BranchKeySubjectKind string

const (
	BranchKeySubjectKindService                     BranchKeySubjectKind = "service"
	BranchKeySubjectKindSigningKey                  BranchKeySubjectKind = "signing_key"
	BranchKeySubjectKindCIMDClientAuthenticationKey BranchKeySubjectKind = "cimd_client_authentication_key"
	ContextKeyServiceID                                                  = "service_id"
	ContextKeyKID                                                        = "kid"
)

// CIMDClientAuthenticationKeyIDPrefix distinguishes CIMD key AAD values without adding a second AAD key.
const CIMDClientAuthenticationKeyIDPrefix = "cimd-"

// BranchKeySubject identifies the logical namespace that should map to a branch key.
//
// Third-party OAuth2 services keep using ServiceID-backed subjects so their existing
// branch key IDs remain unchanged. Signing keys use a dedicated `signing_key` subject
// keyed by the JWT `kid` term and therefore no longer masquerade as services.
type BranchKeySubject struct {
	kind                          BranchKeySubjectKind
	serviceID                     id.ServiceID
	signingKeyID                  id.KeyID
	cimdClientAuthenticationKeyID id.KeyID
}

func NewServiceBranchKeySubject(serviceID id.ServiceID) BranchKeySubject {
	return BranchKeySubject{
		kind:      BranchKeySubjectKindService,
		serviceID: serviceID,
	}
}

func NewSigningKeyBranchKeySubject(signingKeyID id.KeyID) BranchKeySubject {
	return BranchKeySubject{
		kind:         BranchKeySubjectKindSigningKey,
		signingKeyID: signingKeyID,
	}
}

// NewCIMDClientAuthenticationKeyBranchKeySubject creates the CIMD key namespace.
func NewCIMDClientAuthenticationKeyBranchKeySubject(keyID id.KeyID) BranchKeySubject {
	return BranchKeySubject{
		kind:                          BranchKeySubjectKindCIMDClientAuthenticationKey,
		cimdClientAuthenticationKeyID: keyID,
	}
}

func BranchKeySubjectFromEncryptionContext(encryptionContext map[string]string) (BranchKeySubject, error) {
	serviceID, hasServiceID := encryptionContext[ContextKeyServiceID]
	signingKeyID, hasSigningKeyID := encryptionContext[ContextKeyKID]

	hasServiceID = hasServiceID && serviceID != ""
	hasSigningKeyID = hasSigningKeyID && signingKeyID != ""

	switch {
	case hasServiceID && hasSigningKeyID:
		return BranchKeySubject{}, fmt.Errorf("encryption context must include exactly one branch key subject")
	case hasServiceID:
		parsed, err := id.ParseServiceID(serviceID)
		if err != nil {
			return BranchKeySubject{}, fmt.Errorf("invalid service_id %q: %w", serviceID, err)
		}
		return NewServiceBranchKeySubject(parsed), nil
	case hasSigningKeyID:
		keyID := id.NewKeyID(signingKeyID)
		if strings.HasPrefix(signingKeyID, CIMDClientAuthenticationKeyIDPrefix) {
			return NewCIMDClientAuthenticationKeyBranchKeySubject(keyID), nil
		}
		return NewSigningKeyBranchKeySubject(keyID), nil
	default:
		return BranchKeySubject{}, fmt.Errorf("encryption context missing branch key subject")
	}
}

func (s BranchKeySubject) Kind() BranchKeySubjectKind {
	return s.kind
}

func (s BranchKeySubject) ServiceID() (id.ServiceID, bool) {
	if s.kind != BranchKeySubjectKindService {
		return id.ServiceID{}, false
	}
	return s.serviceID, true
}

func (s BranchKeySubject) KeyID() (id.KeyID, bool) {
	switch s.kind {
	case BranchKeySubjectKindSigningKey:
		return s.signingKeyID, true
	case BranchKeySubjectKindCIMDClientAuthenticationKey:
		return s.cimdClientAuthenticationKeyID, true
	default:
		return id.KeyID(""), false
	}
}

func (s BranchKeySubject) Identifier() string {
	switch s.kind {
	case BranchKeySubjectKindService:
		return s.serviceID.String()
	case BranchKeySubjectKindSigningKey:
		return s.signingKeyID.String()
	case BranchKeySubjectKindCIMDClientAuthenticationKey:
		return s.cimdClientAuthenticationKeyID.String()
	default:
		return "<unknown>"
	}
}

func (s BranchKeySubject) EncryptionContext() map[string]string {
	switch s.kind {
	case BranchKeySubjectKindService:
		return map[string]string{ContextKeyServiceID: s.serviceID.String()}
	case BranchKeySubjectKindSigningKey:
		return map[string]string{ContextKeyKID: s.signingKeyID.String()}
	case BranchKeySubjectKindCIMDClientAuthenticationKey:
		return map[string]string{ContextKeyKID: s.cimdClientAuthenticationKeyID.String()}
	default:
		return map[string]string{}
	}
}

func (s BranchKeySubject) Validate() error {
	switch s.kind {
	case BranchKeySubjectKindService:
		if s.serviceID.IsZero() {
			return fmt.Errorf("service_id is required")
		}
	case BranchKeySubjectKindSigningKey:
		if s.signingKeyID.IsZero() {
			return fmt.Errorf("kid is required")
		}
	case BranchKeySubjectKindCIMDClientAuthenticationKey:
		if s.cimdClientAuthenticationKeyID.IsZero() {
			return fmt.Errorf("kid is required")
		}
		if !strings.HasPrefix(s.cimdClientAuthenticationKeyID.String(), CIMDClientAuthenticationKeyIDPrefix) {
			return fmt.Errorf("CIMD client-authentication kid must use %q prefix", CIMDClientAuthenticationKeyIDPrefix)
		}
	default:
		return fmt.Errorf("branch key subject kind is required")
	}
	return nil
}
