package oauth2session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"golang.org/x/oauth2"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

type credentialOperation string

const (
	credentialOperationInitiation  credentialOperation = "authorization_initiation"
	credentialOperationExchange    credentialOperation = "code_exchange"
	credentialOperationRefresh     credentialOperation = "refresh"
	credentialOperationAcquisition credentialOperation = "token_acquisition" // #nosec G101 -- closed audit operation code, not a credential.
	credentialOperationDisplay     credentialOperation = "usability_display"
)

type credentialOutcome string

const (
	credentialOutcomeFailed   credentialOutcome = "failed"
	credentialOutcomeUsed     credentialOutcome = "used"
	credentialOutcomeRejected credentialOutcome = "rejected"
)

// selectCredentials binds a coherent credential pair to its provider snapshot.
func (s *OAuth2SessionService) selectCredentials(
	ctx context.Context,
	entity *model.ThirdpartyOAuth2ProviderEntity,
	operation credentialOperation,
	established *id.ClientID,
) (clientID id.ClientID, clientSecret string, provider *model.ThirdpartyOAuth2ProviderEntity, err error) {
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
		} else if err == nil && candidate.CredentialSource == model.CredentialSourceFilesystem {
			s.logger.InfoContext(ctx, "oauth2 credential files selected",
				"event", "session.oauth2.credential_files_used",
				"service_id", candidate.ID,
				"operation", operation,
				"outcome", credentialOutcomeUsed)
		}
	}()
	for attempt := 0; ; attempt++ {
		if attempt == maxIdentityObservationAttempts {
			return "", "", nil, model.NewCredentialSourceError(model.CredentialSourceReasonGenerationChanged)
		}
		if err := candidate.ValidateOutboundCredentials(); err != nil {
			return "", "", nil, sessionOperationError(ctx, errorOperation, DetailConfiguration, errors.Join(ErrInvalidConfiguration, err))
		}
		clientID = candidate.ClientID
		if candidate.CredentialSource == model.CredentialSourceFilesystem {
			binding, ok := s.config.CredentialFiles[*candidate.CanonicalID]
			if !ok {
				return "", "", nil, model.NewCredentialSourceError(model.CredentialSourceReasonMissingBinding)
			}
			if s.credentialFileReader == nil || s.clientIdentityRepo == nil {
				return "", "", nil, model.NewCredentialSourceError(model.CredentialSourceReasonReadFailed)
			}
			if err := ctx.Err(); err != nil {
				return "", "", nil, identityReadError(err)
			}
			var selectedID string
			selectedID, clientSecret, err = s.credentialFileReader.ReadPair(binding.ClientIDFile, binding.ClientSecretFile)
			if err != nil {
				return "", "", nil, err
			}
			clientID = id.ClientID(selectedID)
			_, applied, err := s.recordClientIdentityObservation(ctx, &candidate, clientID)
			if err != nil {
				return "", "", nil, err
			}
			if !applied {
				clientID, clientSecret = "", ""
				fresh, err := s.providerService.GetForTokenAcquisition(ctx, candidate.ID)
				if err != nil {
					return "", "", nil, identityReadError(err)
				}
				candidate = *fresh
				continue
			}
		} else if !candidate.IsPublicClient() && !candidate.IsCIMDConfidentialClient() {
			clientSecret, err = candidate.Secret.GetPlaintext()
			if err != nil {
				return "", "", nil, sessionOperationError(ctx, errorOperation, DetailDecryptionFailed, fmt.Errorf("provider secret not in plaintext state; ensure entity was fetched via ThirdpartyOAuth2ProviderService.Get: %w", err))
			}
		}
		break
	}

	if established == nil {
		if candidate.CredentialSource != model.CredentialSourceStored || candidate.CredentialSourceTransitioned || candidate.ClientIdentityRevision != 0 {
			return "", "", nil, model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMissing)
		}
	} else if established.IsZero() {
		return "", "", nil, model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMissing)
	} else if clientID != *established {
		return "", "", nil, model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMismatch)
	}
	if candidate.ClientIdentityRevision != entity.ClientIdentityRevision {
		return "", "", nil, model.NewCredentialSourceError(model.CredentialSourceReasonIdentitySuperseded)
	}
	return clientID, clientSecret, &candidate, nil
}

func (s *OAuth2SessionService) auditCredentialSourceFailure(ctx context.Context, serviceID id.ServiceID, operation credentialOperation, err error) {
	if !errors.Is(err, model.ErrCredentialSourceUnavailable) || ctx.Err() != nil || sessionFailureMetadata(err).Kind() == KindCanceled {
		return
	}
	reason := model.CredentialSourceReasonReadFailed
	var sourceError *model.CredentialSourceError
	if errors.As(err, &sourceError) {
		reason = model.NewCredentialSourceError(sourceError.Reason).Reason
	}
	metadataOperation := OperationSessionLookup
	switch operation {
	case credentialOperationRefresh:
		metadataOperation = OperationRefresh
	case credentialOperationInitiation, credentialOperationExchange:
		metadataOperation = OperationCodeExchange
	}
	level := slog.LevelError
	if reason.IsIdentity() {
		level = slog.LevelWarn
	}
	s.logger.Log(ctx, level, "oauth2 credential source unavailable",
		"event", "session.oauth2.credential_source_failed",
		"service_id", serviceID,
		"operation", operation,
		"outcome", credentialOutcomeFailed,
		"reason", reason,
		"oauth2_session", NewErrorMetadata(metadataOperation, DetailCredentialSourceUnavailable))
}

func (s *OAuth2SessionService) auditCredentialProviderRejection(ctx context.Context, serviceID id.ServiceID, operation credentialOperation) {
	s.logger.WarnContext(ctx, "oauth2 provider rejected selected credentials",
		"event", "session.oauth2.credential_provider_rejected",
		"service_id", serviceID,
		"operation", operation,
		"outcome", credentialOutcomeRejected,
		"reason", "provider_rejected")
}

func credentialProviderRejected(err error) bool {
	var responseError *oauth2.RetrieveError
	if !errors.As(err, &responseError) || responseError.Response == nil {
		return false
	}
	status := responseError.Response.StatusCode
	return status >= 400 && status < 500 && status != http.StatusTooManyRequests && (status == http.StatusUnauthorized ||
		responseError.ErrorCode == "invalid_client" || responseError.ErrorCode == "unauthorized_client")
}
