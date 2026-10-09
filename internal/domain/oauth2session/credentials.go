package oauth2session

import (
	"context"
	"errors"
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

const (
	credentialOperationInitiation = "authorization_initiation"
	credentialOperationExchange   = "code_exchange"
	credentialOperationRefresh    = "refresh"
)

func credentialSourceEligible(entity *model.ThirdpartyOAuth2ProviderEntity) bool {
	return !entity.IsPublicClient() && !entity.IsCIMDConfidentialClient() && entity.Flavor != model.OAuth2FlavorGoogle
}

// selectCredentials acquires credentials for one operation without changing the provider.
// Only initiation may establish a new identity; exchange and refresh must retain theirs.
func (s *OAuth2SessionService) selectCredentials(
	ctx context.Context,
	entity *model.ThirdpartyOAuth2ProviderEntity,
	operation string,
	established *id.ClientID,
) (clientID id.ClientID, clientSecret string, err error) {
	candidate := *entity
	if candidate.CredentialSource == "" && !candidate.CredentialSourceProvided {
		candidate.CredentialSource = model.CredentialSourceStored
	}
	errorOperation := OperationCodeExchange
	if operation == credentialOperationRefresh {
		errorOperation = OperationRefresh
	}
	defer func() {
		if errors.Is(err, model.ErrCredentialSourceUnavailable) {
			err = sessionOperationError(ctx, errorOperation, DetailCredentialSourceUnavailable, err)
			s.auditCredentialSourceFailure(ctx, candidate.ID, operation, err)
		} else if err == nil && candidate.CredentialSource == model.CredentialSourceFilesystem {
			s.logger.InfoContext(ctx, "OAuth2 credential files selected",
				"event", "session.oauth2.credential_files_used",
				"service_id", candidate.ID,
				"operation", operation,
				"outcome", "used")
		}
	}()
	if err := candidate.ValidateOutboundCredentials(); err != nil {
		return "", "", sessionOperationError(ctx, errorOperation, DetailConfiguration, errors.Join(ErrInvalidConfiguration, err))
	}

	clientID = candidate.ClientID
	if candidate.CredentialSource == model.CredentialSourceFilesystem {
		binding, ok := s.config.CredentialFiles[*candidate.CanonicalID]
		if !ok {
			return "", "", model.NewCredentialSourceError(model.CredentialSourceReasonMissingBinding)
		}
		if s.credentialFileReader == nil {
			return "", "", model.NewCredentialSourceError(model.CredentialSourceReasonReadFailed)
		}
		var selectedID string
		if operation == credentialOperationInitiation {
			selectedID, err = s.credentialFileReader.ReadClientID(binding.ClientIDFile)
		} else {
			selectedID, clientSecret, err = s.credentialFileReader.ReadPair(binding.ClientIDFile, binding.ClientSecretFile)
		}
		if err != nil {
			return "", "", err
		}
		clientID = id.ClientID(selectedID)
	} else if operation != credentialOperationInitiation && !candidate.IsPublicClient() && !candidate.IsCIMDConfidentialClient() {
		clientSecret, err = candidate.Secret.GetPlaintext()
		if err != nil {
			return "", "", sessionOperationError(ctx, errorOperation, DetailDecryptionFailed, fmt.Errorf("provider secret not in plaintext state; ensure entity was fetched via ThirdpartyOAuth2ProviderService.Get: %w", err))
		}
	}

	if credentialSourceEligible(&candidate) && operation != credentialOperationInitiation {
		if established == nil {
			if candidate.CredentialSource != model.CredentialSourceStored || candidate.CredentialSourceTransitioned {
				return "", "", model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMissing)
			}
		} else if established.IsZero() {
			return "", "", model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMissing)
		} else if clientID != *established {
			return "", "", model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMismatch)
		}
	}
	return clientID, clientSecret, nil
}

func (s *OAuth2SessionService) auditCredentialSourceFailure(ctx context.Context, serviceID id.ServiceID, operation string, err error) {
	reason := model.CredentialSourceReasonReadFailed
	var sourceError *model.CredentialSourceError
	if errors.As(err, &sourceError) {
		reason = sourceError.Reason
	}
	s.logger.ErrorContext(ctx, "OAuth2 credential source unavailable",
		"event", "session.oauth2.credential_source_failed",
		"service_id", serviceID,
		"operation", operation,
		"outcome", "failed",
		"reason", reason,
		"oauth2_session", sessionFailureMetadata(err))
}

func (s *OAuth2SessionService) auditCredentialProviderRejection(ctx context.Context, serviceID id.ServiceID, operation string) {
	s.logger.WarnContext(ctx, "OAuth2 provider rejected selected credentials",
		"event", "session.oauth2.credential_provider_rejected",
		"service_id", serviceID,
		"operation", operation,
		"outcome", "rejected",
		"reason", "provider_rejected")
}
