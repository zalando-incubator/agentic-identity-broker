package branchkey

import (
	"fmt"
	"strings"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

const (
	servicePrefix                     = "service_"
	signingKeyPrefix                  = "key_"
	cimdClientAuthenticationKeyPrefix = "cimd_key_"
	suffix                            = "_branch_key"

	serviceIDFormat                     = servicePrefix + "%s" + suffix
	signingKeyIDFormat                  = signingKeyPrefix + "%s" + suffix
	cimdClientAuthenticationKeyIDFormat = cimdClientAuthenticationKeyPrefix + "%s" + suffix
)

func generateServiceBranchKeyID(serviceID string) string {
	return fmt.Sprintf(serviceIDFormat, serviceID)
}

func generateSigningKeyBranchKeyID(signingKeyID string) string {
	return fmt.Sprintf(signingKeyIDFormat, signingKeyID)
}

func generateCIMDClientAuthenticationBranchKeyID(keyID string) string {
	return fmt.Sprintf(cimdClientAuthenticationKeyIDFormat, keyID)
}

// GenerateBranchKeyId generates a deterministic branch key ID from a typed branch key subject.
func GenerateBranchKeyId(subject domainencryption.BranchKeySubject) (string, error) {
	if err := subject.Validate(); err != nil {
		return "", fmt.Errorf("invalid branch key subject: %w", err)
	}

	switch subject.Kind() {
	case domainencryption.BranchKeySubjectKindService:
		serviceID, _ := subject.ServiceID()
		return generateServiceBranchKeyID(serviceID.String()), nil
	case domainencryption.BranchKeySubjectKindSigningKey:
		keyID, _ := subject.KeyID()
		return generateSigningKeyBranchKeyID(keyID.String()), nil
	case domainencryption.BranchKeySubjectKindCIMDClientAuthenticationKey:
		keyID, _ := subject.KeyID()
		return generateCIMDClientAuthenticationBranchKeyID(keyID.String()), nil
	default:
		return "", fmt.Errorf("unsupported branch key subject kind: %q", subject.Kind())
	}
}

func extractServiceID(branchKeyID string) (string, error) {
	return extractIdentifier(branchKeyID, servicePrefix, serviceIDFormat, "service ID")
}

func extractSigningKeyID(branchKeyID string) (string, error) {
	return extractIdentifier(branchKeyID, signingKeyPrefix, signingKeyIDFormat, "signing key ID")
}

func extractCIMDClientAuthenticationKeyID(branchKeyID string) (string, error) {
	return extractIdentifier(branchKeyID, cimdClientAuthenticationKeyPrefix, cimdClientAuthenticationKeyIDFormat, "CIMD client-authentication key ID")
}

// ExtractSubject parses a branch key ID back into its typed branch key subject.
func ExtractSubject(branchKeyID string) (domainencryption.BranchKeySubject, error) {
	switch {
	case strings.HasPrefix(branchKeyID, servicePrefix):
		serviceID, err := extractServiceID(branchKeyID)
		if err != nil {
			return domainencryption.BranchKeySubject{}, err
		}
		parsed, err := id.ParseServiceID(serviceID)
		if err != nil {
			return domainencryption.BranchKeySubject{}, fmt.Errorf("invalid service subject in branch key ID %q: %w", branchKeyID, err)
		}
		return domainencryption.NewServiceBranchKeySubject(parsed), nil
	case strings.HasPrefix(branchKeyID, cimdClientAuthenticationKeyPrefix):
		keyID, err := extractCIMDClientAuthenticationKeyID(branchKeyID)
		if err != nil {
			return domainencryption.BranchKeySubject{}, err
		}
		return domainencryption.NewCIMDClientAuthenticationKeyBranchKeySubject(id.NewKeyID(keyID)), nil
	case strings.HasPrefix(branchKeyID, signingKeyPrefix):
		signingKeyID, err := extractSigningKeyID(branchKeyID)
		if err != nil {
			return domainencryption.BranchKeySubject{}, err
		}
		return domainencryption.NewSigningKeyBranchKeySubject(id.NewKeyID(signingKeyID)), nil
	default:
		return domainencryption.BranchKeySubject{}, fmt.Errorf(
			"invalid branch key ID format: %q (expected format: %s, %s, or %s)",
			branchKeyID,
			serviceIDFormat,
			signingKeyIDFormat,
			cimdClientAuthenticationKeyIDFormat,
		)
	}
}

func extractIdentifier(branchKeyID, prefix, format, identifierName string) (string, error) {
	if len(branchKeyID) > len(prefix)+len(suffix) &&
		strings.HasPrefix(branchKeyID, prefix) &&
		strings.HasSuffix(branchKeyID, suffix) {
		identifier := branchKeyID[len(prefix) : len(branchKeyID)-len(suffix)]
		if identifier == "" {
			return "", fmt.Errorf("invalid branch key ID: empty %s in %q", identifierName, branchKeyID)
		}
		return identifier, nil
	}

	if branchKeyID == fmt.Sprintf(format, "") {
		return "", fmt.Errorf("invalid branch key ID: empty %s in %q", identifierName, branchKeyID)
	}

	return "", fmt.Errorf("invalid branch key ID format: %q (expected format: %s)", branchKeyID, format)
}
