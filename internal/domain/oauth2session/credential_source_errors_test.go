package oauth2session_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type credentialSourceAudit struct {
	event   string
	outcome string
	reason  string
}

func assertCredentialSourceAudits(t *testing.T, logs string, serviceID id.ServiceID, operation string, want ...credentialSourceAudit) {
	t.Helper()
	var got []credentialSourceAudit
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		event, _ := record["event"].(string)
		if !strings.HasPrefix(event, "session.oauth2.credential_") {
			continue
		}
		assert.Equal(t, serviceID.String(), record["service_id"])
		assert.Equal(t, operation, record["operation"])
		if event == "session.oauth2.credential_source_failed" {
			metadataOperation := operation
			if operation == "authorization_initiation" {
				metadataOperation = "code_exchange"
			}
			assert.Equal(t, map[string]any{
				"operation":        metadataOperation,
				"failure_detail":   "credential_source_unavailable",
				"error_kind":       "infrastructure",
				"dependency":       "credential_source",
				"http_status_code": float64(0),
				"oauth_error_code": "",
			}, record["oauth2_session"])
		}
		for field := range record {
			switch field {
			case "time", "level", "msg", "event", "service_id", "operation", "outcome", "reason", "oauth2_session":
			default:
				assert.Failf(t, "unsafe credential audit field", "field %q must not be recorded", field)
			}
		}
		outcome, _ := record["outcome"].(string)
		reason, _ := record["reason"].(string)
		if value, present := record["reason"]; present {
			assert.IsType(t, "", value, "a credential audit reason must be a non-secret string")
		}
		got = append(got, credentialSourceAudit{event: event, outcome: outcome, reason: reason})
	}
	assert.Equal(t, want, got, "credential audit sequence must distinguish acquisition from upstream rejection")
}

func assertCredentialSourceNotProviderFailure(t *testing.T, err error, reason model.CredentialSourceReason) {
	t.Helper()
	var failure *oauth2session.OperationError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, oauth2session.DetailCredentialSourceUnavailable, failure.Metadata().Detail())
	assert.Equal(t, oauth2session.KindInfrastructure, failure.Metadata().Kind())
	assert.Equal(t, oauth2session.DependencyCredentialSource, failure.Metadata().Dependency())
	assert.Zero(t, failure.Metadata().StatusCode())
	assert.Empty(t, failure.Metadata().OAuthCode())
	assertCredentialSourceReason(t, err, reason)
	for _, category := range []error{oauth2session.ErrRefreshFailed, oauth2session.ErrRefreshTokenExpired, oauth2session.ErrRefreshRejected, oauth2session.ErrSessionExpired, oauth2session.ErrTokenExchange, oauth2session.ErrInvalidPKCE} {
		assert.NotErrorIs(t, err, category)
	}
	var rejected *oauth2session.RefreshRejectedError
	assert.False(t, errors.As(err, &rejected), "an unavailable source is not an upstream HTTP rejection")
	var pathError *os.PathError
	assert.False(t, errors.As(err, &pathError), "raw filesystem error chains must not escape acquisition")
}

func assertCredentialOperationFails(t *testing.T, h *credentialSessionHarness, principal id.Principal, serviceID id.ServiceID, state, operation, code string) error {
	t.Helper()
	var err error
	switch operation {
	case "callback":
		result, callbackErr := h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: serviceID, Code: code, State: state})
		assert.Nil(t, result)
		err = callbackErr
	case "explicit refresh":
		result, refreshErr := h.service.ForceRefreshSession(context.Background(), principal, serviceID)
		assert.Nil(t, result)
		err = refreshErr
	case "automatic refresh":
		result, token, refreshErr := h.service.GetValidAccessToken(context.Background(), principal, serviceID)
		assert.Nil(t, result)
		assert.Empty(t, token)
		err = refreshErr
	}
	require.Error(t, err)
	return err
}

func assertCredentialDiagnosticsSafe(t *testing.T, err error, logs string, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		require.NotEmpty(t, value)
		assert.NotContains(t, err.Error(), value)
		assert.NotContains(t, logs, value)
	}
}

func TestCredentialSourceErrors_InitiationRetainsSourceCategoryAndSafeEvent(t *testing.T) {
	for _, failure := range []struct {
		name   string
		reason model.CredentialSourceReason
	}{
		{name: "missing binding", reason: model.CredentialSourceReasonMissingBinding},
		{name: "client ID read error", reason: model.CredentialSourceReasonReadFailed},
	} {
		t.Run(failure.name, func(t *testing.T) {
			binding := credentialPair(t, "initiation-file-client-value", "initiation-file-secret-value")
			bindings := map[string]ports.CredentialFileBinding{"initiation-source": binding}
			if failure.reason == model.CredentialSourceReasonMissingBinding {
				delete(bindings, "initiation-source")
			} else {
				require.NoError(t, os.Remove(binding.ClientIDFile))
				require.NoError(t, os.Symlink(binding.ClientIDFile, binding.ClientIDFile))
			}
			logs := new(strings.Builder)
			h := newCredentialSessionHarness(t, bindings, slog.New(slog.NewJSONHandler(logs, nil)))
			provider := h.register(t, model.CredentialSourceFilesystem, "initiation-source", "", "")
			logs.Reset()
			flow, err := h.service.InitiateOAuth2Flow(context.Background(), id.Principal("initiation-source@example.com"), provider.ID, "https://broker.example.com/sessions")
			assert.Nil(t, flow, "source failure must not yield a provider redirect or state")
			assertCredentialSourceNotProviderFailure(t, err, failure.reason)
			assertOperationMetadata(t, err, oauth2session.OperationCodeExchange, oauth2session.DetailCredentialSourceUnavailable)
			assert.Empty(t, h.endpoint.captured(t))
			ids, pairs := h.reader.counts()
			if failure.reason == model.CredentialSourceReasonMissingBinding {
				assert.Zero(t, ids)
			} else {
				assert.Equal(t, 1, ids)
			}
			assert.Zero(t, pairs, "initiation must never acquire the secret")
			assertCredentialSourceAudits(t, logs.String(), provider.ID, "authorization_initiation", credentialSourceAudit{event: "session.oauth2.credential_source_failed", outcome: "failed", reason: string(failure.reason)})
			assertCredentialDiagnosticsSafe(t, err, logs.String(), binding.ClientIDFile, binding.ClientSecretFile, "initiation-file-client-value", "initiation-file-secret-value", "too many levels of symbolic links")
		})
	}
}

func TestCredentialSourceErrors_CallbackAndRefreshBypassProviderAndPKCECategories(t *testing.T) {
	for _, failure := range []struct {
		name   string
		reason model.CredentialSourceReason
	}{
		{name: "client ID read error", reason: model.CredentialSourceReasonReadFailed},
		{name: "secret read error", reason: model.CredentialSourceReasonReadFailed},
		{name: "identity mismatch", reason: model.CredentialSourceReasonIdentityMismatch},
	} {
		for _, operation := range []string{"callback", "explicit refresh", "automatic refresh"} {
			t.Run(failure.name+"/"+operation, func(t *testing.T) {
				binding := credentialPair(t, "established-file-client-value", "source-file-secret-value")
				bindings := map[string]ports.CredentialFileBinding{"source-propagation": binding}
				logs := new(strings.Builder)
				h := newCredentialSessionHarness(t, bindings, slog.New(slog.NewJSONHandler(logs, nil)))
				provider := h.register(t, model.CredentialSourceFilesystem, "source-propagation", "", "")
				principal := id.Principal("source-propagation@example.com")
				flow := h.initiate(t, principal, provider.ID)
				claims, err := h.service.ValidateStateToken(flow.StateToken, principal, provider.ID)
				require.NoError(t, err)
				h.callback(t, principal, provider.ID, flow.StateToken)
				h.expire(t, principal, provider.ID)
				before, err := h.sessions.FindByPrincipalAndService(context.Background(), principal, provider.ID)
				require.NoError(t, err)
				require.NotNil(t, before)
				requestsBefore := h.endpoint.captured(t)
				require.Len(t, requestsBefore, 1)
				idsBefore, pairsBefore := h.reader.counts()
				require.False(t, before.HasValidAccessToken(), "automatic refresh must exercise acquisition, not a cached access token")
				require.True(t, before.CanRefresh())
				providerBefore, err := h.records.Get(context.Background(), provider.ID)
				require.NoError(t, err)
				switch failure.name {
				case "client ID read error", "secret read error":
					path := binding.ClientIDFile
					if failure.name == "secret read error" {
						path = binding.ClientSecretFile
					}
					require.NoError(t, os.Remove(path))
					require.NoError(t, os.Symlink(path, path))
				case "identity mismatch":
					require.NoError(t, publishCredential(binding.ClientIDFile, "mismatched-file-client-value"))
				}
				logs.Reset()
				err = assertCredentialOperationFails(t, h, principal, provider.ID, flow.StateToken, operation, "source-authorization-code-value")
				assertCredentialSourceNotProviderFailure(t, err, failure.reason)
				assert.Equal(t, requestsBefore, h.endpoint.captured(t), "the failed attempt must send zero requests, even after a usable pair previously reached the provider")
				ids, pairs := h.reader.counts()
				assert.Equal(t, idsBefore, ids)
				assert.Equal(t, pairsBefore+1, pairs, "source failures are not provider retry candidates")
				after, findErr := h.sessions.Get(context.Background(), before.ID)
				require.NoError(t, findErr)
				assert.Equal(t, before, after, "failure must preserve encrypted tokens, recorded identity, and session metadata")
				providerAfter, getErr := h.records.Get(context.Background(), provider.ID)
				require.NoError(t, getErr)
				assert.Equal(t, providerBefore, providerAfter)
				eventOperation := "refresh"
				if operation == "callback" {
					eventOperation = "code_exchange"
				}
				assertOperationMetadata(t, err, oauth2session.Operation(eventOperation), oauth2session.DetailCredentialSourceUnavailable)
				assertCredentialSourceAudits(t, logs.String(), provider.ID, eventOperation, credentialSourceAudit{event: "session.oauth2.credential_source_failed", outcome: "failed", reason: string(failure.reason)})
				assert.NotContains(t, logs.String(), `"event":"session.oauth2.pkce_validation_failed"`)
				assert.NotContains(t, logs.String(), `"event":"session.oauth2.refresh_failed"`)
				assertCredentialDiagnosticsSafe(t, err, logs.String(), binding.ClientIDFile, binding.ClientSecretFile, "established-file-client-value", "mismatched-file-client-value", "source-file-secret-value", "source-authorization-code-value", claims.PKCEVerifier, "credential-access", "credential-refresh", "too many levels of symbolic links")
			})
		}
	}
}

func TestCredentialSourceErrors_UsablePairPreservesProviderRejectionClassification(t *testing.T) {
	for _, oauthError := range []string{"invalid_client", "invalid_grant"} {
		for _, operation := range []string{"callback", "explicit refresh", "automatic refresh"} {
			t.Run(oauthError+"/"+operation, func(t *testing.T) {
				binding := credentialPair(t, "rejected-file-client-value", "rejected-file-secret-value")
				logs := new(strings.Builder)
				h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"provider-rejection": binding}, slog.New(slog.NewJSONHandler(logs, nil)))
				provider := h.register(t, model.CredentialSourceFilesystem, "provider-rejection", "", "")
				principal := id.Principal("provider-rejection@example.com")
				flow := h.initiate(t, principal, provider.ID)
				claims, err := h.service.ValidateStateToken(flow.StateToken, principal, provider.ID)
				require.NoError(t, err)
				before := h.seedSession(t, principal, provider.ID, claims.UpstreamClientID)
				require.False(t, before.HasValidAccessToken())
				require.True(t, before.CanRefresh())
				payload, err := json.Marshal(map[string]string{"error": oauthError, "error_description": strings.Join([]string{"raw-provider-description-value", "rejected-file-client-value", "rejected-file-secret-value", binding.ClientIDFile, binding.ClientSecretFile, "rejected-authorization-code-value", claims.PKCEVerifier, "legacy-access", "legacy-refresh"}, " ")})
				require.NoError(t, err)
				h.endpoint.responseBody = string(payload)
				h.endpoint.onRequest = func(credentialTokenRequest) (int, error) { return http.StatusUnauthorized, nil }
				logs.Reset()
				err = assertCredentialOperationFails(t, h, principal, provider.ID, flow.StateToken, operation, "rejected-authorization-code-value")
				assert.NotErrorIs(t, err, model.ErrCredentialSourceUnavailable)
				assert.NotErrorIs(t, err, oauth2session.ErrInvalidPKCE)
				eventOperation := "refresh"
				if operation == "callback" {
					eventOperation = "code_exchange"
					assert.ErrorIs(t, err, oauth2session.ErrTokenExchange)
					assert.NotErrorIs(t, err, oauth2session.ErrRefreshFailed)
				} else {
					assert.ErrorIs(t, err, oauth2session.ErrRefreshFailed)
					var rejected *oauth2session.RefreshRejectedError
					require.ErrorAs(t, err, &rejected)
					assert.Equal(t, http.StatusUnauthorized, rejected.StatusCode)
					assert.Equal(t, oauthError, rejected.OAuthError)
					if oauthError == "invalid_grant" {
						assert.ErrorIs(t, err, oauth2session.ErrRefreshRejected)
					} else {
						assert.NotErrorIs(t, err, oauth2session.ErrRefreshRejected)
					}
					assert.NotErrorIs(t, err, oauth2session.ErrRefreshTokenExpired)
				}
				requests := h.endpoint.captured(t)
				require.NotEmpty(t, requests, "provider rejection requires actual authentication with a usable acquired pair")
				for _, request := range requests {
					assert.Equal(t, "rejected-file-client-value", request.clientID)
					assert.Equal(t, "rejected-file-secret-value", request.secret)
					if operation == "callback" {
						assert.Equal(t, "authorization_code", request.form.Get("grant_type"))
						assert.Equal(t, "rejected-authorization-code-value", request.form.Get("code"))
						assert.Equal(t, claims.PKCEVerifier, request.form.Get("code_verifier"))
					} else {
						assert.Equal(t, "refresh_token", request.form.Get("grant_type"))
						assert.Equal(t, "legacy-refresh", request.form.Get("refresh_token"))
					}
					assertCredentialDiagnosticsSafe(t, err, logs.String(), request.form.Encode())
					if authorization := request.header.Get("Authorization"); authorization != "" {
						assertCredentialDiagnosticsSafe(t, err, logs.String(), authorization)
					}
				}
				_, pairs := h.reader.counts()
				assert.Equal(t, 1, pairs, "permanent provider rejection is not a fresh acquisition retry")
				after, findErr := h.sessions.Get(context.Background(), before.ID)
				require.NoError(t, findErr)
				assert.Equal(t, before, after)
				assertCredentialSourceAudits(t, logs.String(), provider.ID, eventOperation,
					credentialSourceAudit{event: "session.oauth2.credential_files_used", outcome: "used"},
					credentialSourceAudit{event: "session.oauth2.credential_provider_rejected", outcome: "rejected", reason: "provider_rejected"})
				assertCredentialDiagnosticsSafe(t, err, logs.String(), binding.ClientIDFile, binding.ClientSecretFile, "rejected-file-client-value", "rejected-file-secret-value", "rejected-authorization-code-value", claims.PKCEVerifier, "legacy-access", "legacy-refresh", "raw-provider-description-value", string(payload))
			})
		}
	}
}

func TestCredentialSourceErrors_TransportAndDecodingDoNotEmitProviderRejection(t *testing.T) {
	for _, failure := range []string{"transport", "response decoding"} {
		for _, operation := range []string{"callback", "explicit refresh", "automatic refresh"} {
			t.Run(failure+"/"+operation, func(t *testing.T) {
				binding := credentialPair(t, "nonrejection-file-client-value", "nonrejection-file-secret-value")
				logs := new(strings.Builder)
				h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"nonrejection": binding}, slog.New(slog.NewJSONHandler(logs, nil)))
				provider := h.register(t, model.CredentialSourceFilesystem, "nonrejection", "", "")
				principal := id.Principal("nonrejection@example.com")
				flow := h.initiate(t, principal, provider.ID)
				claims, err := h.service.ValidateStateToken(flow.StateToken, principal, provider.ID)
				require.NoError(t, err)
				before := h.seedSession(t, principal, provider.ID, claims.UpstreamClientID)
				require.False(t, before.HasValidAccessToken())
				require.True(t, before.CanRefresh())
				if failure == "transport" {
					h.endpoint.server.Close()
				} else {
					h.endpoint.responseBody = `{"access_token":"nonrejection-file-secret-value","expires_in":false,"raw_provider_payload":"decoding-provider-payload-value"}`
				}
				logs.Reset()
				err = assertCredentialOperationFails(t, h, principal, provider.ID, flow.StateToken, operation, "nonrejection-authorization-code-value")
				assert.NotErrorIs(t, err, model.ErrCredentialSourceUnavailable)
				assert.NotErrorIs(t, err, oauth2session.ErrRefreshTokenExpired)
				var rejected *oauth2session.RefreshRejectedError
				assert.False(t, errors.As(err, &rejected))
				eventOperation, attempts := "refresh", 1
				if operation == "callback" {
					eventOperation = "code_exchange"
					assert.ErrorIs(t, err, oauth2session.ErrTokenExchange)
					if failure == "transport" {
						attempts = 3
					}
				} else {
					assert.ErrorIs(t, err, oauth2session.ErrRefreshFailed)
				}
				_, pairs := h.reader.counts()
				assert.Equal(t, attempts, pairs)
				want := make([]credentialSourceAudit, attempts)
				for index := range want {
					want[index] = credentialSourceAudit{event: "session.oauth2.credential_files_used", outcome: "used"}
				}
				assertCredentialSourceAudits(t, logs.String(), provider.ID, eventOperation, want...)
				requests := h.endpoint.captured(t)
				if failure == "transport" {
					assert.Empty(t, requests)
				} else {
					require.NotEmpty(t, requests)
					for _, request := range requests {
						assert.Equal(t, "nonrejection-file-client-value", request.clientID)
						assert.Equal(t, "nonrejection-file-secret-value", request.secret)
						assertCredentialDiagnosticsSafe(t, err, logs.String(), request.form.Encode())
					}
				}
				after, findErr := h.sessions.Get(context.Background(), before.ID)
				require.NoError(t, findErr)
				assert.Equal(t, before, after)
				assertCredentialDiagnosticsSafe(t, err, logs.String(), binding.ClientIDFile, binding.ClientSecretFile, "nonrejection-file-client-value", "nonrejection-file-secret-value", "nonrejection-authorization-code-value", claims.PKCEVerifier, "legacy-access", "legacy-refresh", "decoding-provider-payload-value")
			})
		}
	}
}
