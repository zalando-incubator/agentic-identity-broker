package oauth2session

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type summaryIdentityEvaluation struct {
	service *model.ThirdpartyOAuth2ProviderEntity
	state   ports.ClientIdentityState
}

func (s *OAuth2SessionService) evaluateSummaryIdentity(ctx context.Context, serviceID id.ServiceID, service *model.ThirdpartyOAuth2ProviderEntity, err error) summaryIdentityEvaluation {
	if err != nil || service == nil {
		if err == nil {
			err = ErrServiceNotFound
		}
		s.auditCredentialSourceFailure(ctx, serviceID, credentialOperationDisplay, identityReadError(err))
		return summaryIdentityEvaluation{}
	}
	state, err := s.resolveClientIdentity(ctx, service)
	if err != nil {
		s.auditCredentialSourceFailure(ctx, serviceID, credentialOperationDisplay, err)
		state = ports.ClientIdentityState{Identity: service.ClientIdentity, Revision: service.ClientIdentityRevision}
		if service.CredentialSource != model.CredentialSourceFilesystem {
			clientID := service.ClientID
			state.Identity = &clientID
		}
	}
	return summaryIdentityEvaluation{service: service, state: state}
}

func (evaluated summaryIdentityEvaluation) apply(summary *storage.UserSessionSummary) {
	if evaluated.service == nil {
		summary.SetRequiresReauthentication(true)
		return
	}
	summary.ServiceDisplayName = evaluated.service.DisplayName
	summary.SetRequiresReauthentication(summary.RequiresReauthentication || summaryIdentityStatus(summary, evaluated.service, evaluated.state) != nil)
}

func summaryIdentityStatus(summary *storage.UserSessionSummary, service *model.ThirdpartyOAuth2ProviderEntity, current ports.ClientIdentityState) error {
	return sessionIdentityStatus(&storage.UserSession{
		UpstreamClientID:       summary.UpstreamClientID,
		UpstreamClientRevision: summary.UpstreamClientRevision,
	}, service, current)
}
