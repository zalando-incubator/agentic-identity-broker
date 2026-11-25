package oauth2session

import (
	"context"
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.SessionUsabilityEvaluator = (*OAuth2SessionService)(nil)

const maxIdentityObservationAttempts = 8

type identityChangeChannel string

const identityChangeFilesystem identityChangeChannel = "filesystem_observation"

// A lost observation replaces service with fresh metadata, never decrypted credentials.
func (s *OAuth2SessionService) resolveClientIdentity(ctx context.Context, service *model.ThirdpartyOAuth2ProviderEntity) (ports.ClientIdentityState, error) {
	return s.resolveIdentity(ctx, service, true)
}

func (s *OAuth2SessionService) resolveIdentity(ctx context.Context, service *model.ThirdpartyOAuth2ProviderEntity, record bool) (ports.ClientIdentityState, error) {
	for range maxIdentityObservationAttempts {
		if err := ctx.Err(); err != nil {
			return ports.ClientIdentityState{}, identityReadError(err)
		}
		if service.ClientIdentityRevision < 0 || (service.CredentialSource != "" && service.CredentialSource != model.CredentialSourceStored && service.CredentialSource != model.CredentialSourceFilesystem) {
			return ports.ClientIdentityState{}, identityReadError(nil)
		}
		if service.CredentialSource != model.CredentialSourceFilesystem {
			if service.ClientID.IsZero() {
				return ports.ClientIdentityState{}, identityReadError(nil)
			}
			clientID := service.ClientID
			return ports.ClientIdentityState{Identity: &clientID, Revision: service.ClientIdentityRevision}, nil
		}
		if service.CanonicalID == nil {
			return ports.ClientIdentityState{}, model.NewCredentialSourceError(model.CredentialSourceReasonMissingBinding)
		}
		binding, ok := s.config.CredentialFiles[*service.CanonicalID]
		if !ok {
			return ports.ClientIdentityState{}, model.NewCredentialSourceError(model.CredentialSourceReasonMissingBinding)
		}
		if s.credentialFileReader == nil || record && s.clientIdentityRepo == nil {
			return ports.ClientIdentityState{}, identityReadError(nil)
		}
		value, err := s.credentialFileReader.ReadClientID(binding.ClientIDFile)
		if err != nil {
			return ports.ClientIdentityState{}, identityReadError(err)
		}
		if value == "" {
			return ports.ClientIdentityState{}, model.NewCredentialSourceError(model.CredentialSourceReasonEmpty)
		}
		if !record {
			identity := id.ClientID(value)
			revision := service.ClientIdentityRevision
			if service.ClientIdentity != nil && *service.ClientIdentity != identity {
				revision++
			}
			return ports.ClientIdentityState{Identity: &identity, Revision: revision}, nil
		}
		state, applied, err := s.recordClientIdentityObservation(ctx, service, id.ClientID(value))
		if err != nil {
			return ports.ClientIdentityState{}, err
		}
		if applied {
			return state, nil
		}
		fresh, err := s.providerService.GetForIdentityEvaluation(ctx, service.ID)
		if err != nil {
			return ports.ClientIdentityState{}, identityReadError(err)
		}
		// The rejected file value belongs to the old configuration generation.
		*service = *fresh
	}
	return ports.ClientIdentityState{}, model.NewCredentialSourceError(model.CredentialSourceReasonGenerationChanged)
}

func (s *OAuth2SessionService) recordClientIdentityObservation(ctx context.Context, service *model.ThirdpartyOAuth2ProviderEntity, observed id.ClientID) (ports.ClientIdentityState, bool, error) {
	if observed.IsZero() {
		return ports.ClientIdentityState{}, false, model.NewCredentialSourceError(model.CredentialSourceReasonEmpty)
	}
	if s.clientIdentityRepo == nil {
		return ports.ClientIdentityState{}, false, identityReadError(nil)
	}
	state, applied, changed, err := s.clientIdentityRepo.RecordObservedClientIdentity(ctx, service.ID, observed, ports.ClientIdentityObservation{ServiceVersion: service.Version, CanonicalID: *service.CanonicalID, Revision: service.ClientIdentityRevision})
	if err != nil {
		return ports.ClientIdentityState{}, false, identityReadError(err)
	}
	if applied {
		service.ClientIdentity = state.Identity
		service.ClientIdentityRevision = state.Revision
		if changed {
			s.logger.InfoContext(ctx, "oauth2 client identity changed",
				"event", "session.oauth2.client_identity_changed",
				"service_id", service.ID,
				"client_identity_revision", state.Revision,
				"channel", identityChangeFilesystem)
		}
	}
	return state, applied, nil
}

func identityReadError(err error) error {
	reason := model.CredentialSourceReasonReadFailed
	var sourceError *model.CredentialSourceError
	if errors.As(err, &sourceError) {
		reason = sourceError.Reason
	}
	failure := model.NewCredentialSourceError(reason)
	var repositoryError *storage.StorageError
	if ports.IsNotFoundErr(err) {
		err = errors.Join(ErrServiceNotFound, err)
	}
	if errors.As(err, &repositoryError) || errors.Is(err, ErrServiceNotFound) {
		// Keep repository causes inspectable behind a credential-free error message.
		return NewOperationError(NewErrorMetadata(OperationUnknown, DetailCredentialSourceUnavailable), errors.Join(failure, err))
	}
	switch {
	case errors.Is(err, context.Canceled):
		return errors.Join(failure, context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return errors.Join(failure, context.DeadlineExceeded)
	default:
		return failure
	}
}

func identityOperationError(ctx context.Context, operation Operation, err error) *OperationError {
	cause := identityReadError(err)
	var repositoryError *storage.StorageError
	if errors.As(cause, &repositoryError) && !ports.IsNotFoundErr(cause) {
		return tokenAcquisitionError(ctx, operation, cause)
	}
	return sessionOperationError(ctx, operation, DetailCredentialSourceUnavailable, cause)
}

func sessionIdentityStatus(session *storage.UserSession, service *model.ThirdpartyOAuth2ProviderEntity, current ports.ClientIdentityState) error {
	if session.UpstreamClientID == nil {
		if (service.CredentialSource == "" || service.CredentialSource == model.CredentialSourceStored) && !service.CredentialSourceTransitioned && current.Revision == 0 {
			return nil
		}
		return model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMissing)
	}
	if session.UpstreamClientID.IsZero() {
		return model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMissing)
	}
	if current.Identity == nil || *session.UpstreamClientID != *current.Identity {
		return model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMismatch)
	}
	if session.UpstreamClientRevision != current.Revision {
		return model.NewCredentialSourceError(model.CredentialSourceReasonIdentitySuperseded)
	}
	return nil
}

func (s *OAuth2SessionService) checkSessionIdentity(ctx context.Context, session *storage.UserSession, operation Operation) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	service, err := s.providerService.GetForIdentityEvaluation(ctx, session.ServiceID)
	if err == nil {
		var state ports.ClientIdentityState
		state, err = s.resolveClientIdentity(ctx, service)
		if err == nil {
			err = sessionIdentityStatus(session, service, state)
		}
	}
	if err != nil {
		return nil, identityOperationError(ctx, operation, err)
	}
	return service, nil
}

func (s *OAuth2SessionService) providerForEstablishedIdentity(ctx context.Context, service *model.ThirdpartyOAuth2ProviderEntity, session *storage.UserSession, operation Operation) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if service.CredentialSource == model.CredentialSourceFilesystem || service.IsPublicClient() || service.IsCIMDConfidentialClient() {
		return service, nil
	}
	service, err := s.providerService.GetForTokenAcquisition(ctx, session.ServiceID)
	if err != nil {
		return nil, tokenAcquisitionError(ctx, operation, err)
	}
	if operation == OperationRefresh && service.Discovery.ClientMethod == model.ClientBootstrapDCR && service.ID != session.ServiceID {
		return nil, errors.New("DCR token identity does not match the requested service")
	}
	identity, err := s.resolveClientIdentity(ctx, service)
	if err == nil {
		err = sessionIdentityStatus(session, service, identity)
	}
	if err != nil {
		return service, sessionOperationError(ctx, operation, DetailCredentialSourceUnavailable, identityReadError(err))
	}
	return service, nil
}

func (s *OAuth2SessionService) checkCallbackIdentity(ctx context.Context, claims *OAuth2StateTokenClaims) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if claims.UpstreamClientID == nil || claims.UpstreamClientID.IsZero() || claims.UpstreamClientRevision == nil || *claims.UpstreamClientRevision < 0 {
		return nil, sessionOperationError(ctx, OperationCodeExchange, DetailCredentialSourceUnavailable, model.NewCredentialSourceError(model.CredentialSourceReasonIdentityMissing))
	}
	return s.checkSessionIdentity(ctx, &storage.UserSession{ServiceID: claims.ServiceID, UpstreamClientID: claims.UpstreamClientID, UpstreamClientRevision: *claims.UpstreamClientRevision}, OperationCodeExchange)
}

func (s *OAuth2SessionService) checkDryRunIdentity(ctx context.Context, session *storage.UserSession) error {
	service, err := s.providerService.GetForIdentityEvaluation(ctx, session.ServiceID)
	if err == nil {
		var state ports.ClientIdentityState
		state, err = s.resolveIdentity(ctx, service, false)
		if err == nil {
			err = sessionIdentityStatus(session, service, state)
		}
	}
	if err != nil {
		return identityOperationError(ctx, OperationRefresh, err)
	}
	return nil
}

func (s *OAuth2SessionService) RequiresReauthentication(ctx context.Context, sessions []*storage.UserSession, mode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
	result := make(map[id.ServiceID]bool, len(sessions))
	identities := make(map[id.ServiceID]summaryIdentityEvaluation, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		evaluated, ok := identities[session.ServiceID]
		if ok && evaluated.service == nil {
			result[session.ServiceID] = true
			continue
		}
		if !ok {
			service, err := s.providerService.GetForIdentityEvaluation(ctx, session.ServiceID)
			if mode == ports.UsabilityDisplay {
				evaluated = s.evaluateSummaryIdentity(ctx, session.ServiceID, service, err)
			} else {
				if err != nil {
					return nil, &ports.SessionUsabilityError{ServiceID: session.ServiceID, Err: identityReadError(err)}
				}
				state, err := s.resolveClientIdentity(ctx, service)
				if err != nil {
					return nil, &ports.SessionUsabilityError{ServiceID: session.ServiceID, Err: err}
				}
				evaluated = summaryIdentityEvaluation{service: service, state: state}
			}
			identities[session.ServiceID] = evaluated
			if evaluated.service == nil {
				result[session.ServiceID] = true
				continue
			}
		}
		result[session.ServiceID] = result[session.ServiceID] || session.RequiresReauthentication() || sessionIdentityStatus(session, evaluated.service, evaluated.state) != nil
	}
	return result, nil
}

func (s *OAuth2SessionService) WithClientIdentityRepository(repo ports.ThirdpartyOAuth2ClientIdentityRepository) *OAuth2SessionService {
	s.clientIdentityRepo = repo
	return s
}
