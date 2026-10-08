package tokenexchange

import (
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
)

func sessionDiagnostic(err error) Diagnostic {
	var operationErr *oauth2session.OperationError
	if !errors.As(err, &operationErr) {
		return NewDiagnostic(StageSessionLookup, DetailInternalUnclassified)
	}
	metadata := operationErr.Metadata()
	stage := StageSessionLookup
	switch metadata.Operation() {
	case oauth2session.OperationRefresh:
		stage = StageRefresh
	case oauth2session.OperationScopeValidation:
		stage = StageScopeValidation
	}
	var detail FailureDetail
	switch metadata.Detail() {
	case oauth2session.DetailSessionMissing:
		detail = DetailSessionMissing
	case oauth2session.DetailAccessTokenExpired:
		detail = DetailAccessTokenExpired
	case oauth2session.DetailRefreshTokenExpired:
		detail = DetailRefreshTokenExpired
	case oauth2session.DetailRefreshUnavailable:
		detail = DetailRefreshUnavailable
	case oauth2session.DetailRefreshRejected:
		detail = DetailRefreshRejected
	case oauth2session.DetailProviderClientRejected:
		detail = DetailProviderClientRejected
	case oauth2session.DetailProviderRejected:
		detail = DetailProviderRejected
	case oauth2session.DetailProviderUnavailable:
		detail = DetailProviderUnavailable
	case oauth2session.DetailProviderResponseInvalid:
		detail = DetailProviderResponseInvalid
	case oauth2session.DetailRepositoryUnavailable:
		detail = DetailSessionRepositoryUnavailable
	case oauth2session.DetailDecryptionFailed:
		detail = DetailSessionDecryptionFailed
	case oauth2session.DetailEncryptionFailed:
		detail = DetailSessionEncryptionFailed
	case oauth2session.DetailPersistenceFailed:
		detail = DetailSessionPersistenceFailed
	case oauth2session.DetailConfiguration:
		detail = DetailSessionConfiguration
	case oauth2session.DetailCallerCanceled:
		detail = DetailCallerCanceled
	default:
		detail = DetailInternalUnclassified
	}
	return NewDiagnostic(stage, detail)
}
