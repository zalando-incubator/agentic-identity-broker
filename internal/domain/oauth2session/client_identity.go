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

// A lost observation replaces service with fresh metadata, never decrypted credentials.
func (s *OAuth2SessionService) resolveClientIdentity(ctx context.Context, service *model.ThirdpartyOAuth2ProviderEntity) (ports.ClientIdentityState, error) {
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
		if s.credentialFileReader == nil || s.clientIdentityRepo == nil {
			return ports.ClientIdentityState{}, identityReadError(nil)
		}
		value, err := s.credentialFileReader.ReadClientID(binding.ClientIDFile)
		if err != nil {
			return ports.ClientIdentityState{}, identityReadError(err)
		}
		if value == "" {
			return ports.ClientIdentityState{}, model.NewCredentialSourceError(model.CredentialSourceReasonEmpty)
		}
		state, applied, changed, err := s.clientIdentityRepo.RecordObservedClientIdentity(ctx, service.ID, id.ClientID(value), ports.ClientIdentityObservation{ServiceVersion: service.Version, CanonicalID: *service.CanonicalID, Revision: service.ClientIdentityRevision})
		if err != nil {
			return ports.ClientIdentityState{}, identityReadError(err)
		}
		if applied {
			service.ClientIdentity = state.Identity
			service.ClientIdentityRevision = state.Revision
			if changed {
				s.logger.InfoContext(ctx, "oauth2 client identity changed",
					"event", "session.oauth2.client_identity_changed",
					"service_id", service.ID,
					"client_identity_revision", state.Revision,
					"channel", "filesystem_observation")
			}
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

func identityReadError(err error) error {
	reason := model.CredentialSourceReasonReadFailed
	var sourceError *model.CredentialSourceError
	if errors.As(err, &sourceError) {
		reason = sourceError.Reason
	}
	failure := model.NewCredentialSourceError(reason)
	switch {
	case errors.Is(err, context.Canceled):
		return errors.Join(failure, context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return errors.Join(failure, context.DeadlineExceeded)
	default:
		return failure
	}
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

func (s *OAuth2SessionService) RequiresReauthentication(ctx context.Context, sessions []*storage.UserSession, mode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
	result := make(map[id.ServiceID]bool, len(sessions))
	type evaluatedIdentity struct {
		service *model.ThirdpartyOAuth2ProviderEntity
		state   ports.ClientIdentityState
	}
	identities := make(map[id.ServiceID]evaluatedIdentity, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		evaluated, ok := identities[session.ServiceID]
		if !ok {
			service, err := s.providerService.GetForIdentityEvaluation(ctx, session.ServiceID)
			if err != nil {
				failure := identityReadError(err)
				if mode != ports.UsabilityDisplay {
					return nil, failure
				}
				s.auditCredentialSourceFailure(ctx, session.ServiceID, "usability_display", failure)
				result[session.ServiceID] = true
				continue
			}
			state, err := s.resolveClientIdentity(ctx, service)
			if err != nil {
				if mode != ports.UsabilityDisplay {
					return nil, err
				}
				s.auditCredentialSourceFailure(ctx, session.ServiceID, "usability_display", err)
				state = ports.ClientIdentityState{Identity: service.ClientIdentity, Revision: service.ClientIdentityRevision}
				if service.CredentialSource != model.CredentialSourceFilesystem {
					clientID := service.ClientID
					state.Identity = &clientID
				}
			}
			evaluated = evaluatedIdentity{service: service, state: state}
			identities[session.ServiceID] = evaluated
		}
		result[session.ServiceID] = result[session.ServiceID] || session.RequiresReauthentication() || sessionIdentityStatus(session, evaluated.service, evaluated.state) != nil
	}
	return result, nil
}

func (s *OAuth2SessionService) WithClientIdentityRepository(repo ports.ThirdpartyOAuth2ClientIdentityRepository) *OAuth2SessionService {
	s.clientIdentityRepo = repo
	return s
}
