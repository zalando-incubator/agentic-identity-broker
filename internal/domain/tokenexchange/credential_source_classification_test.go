package tokenexchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/credentialfile"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExchange_AutomaticRefreshCredentialSourceClassification(t *testing.T) {
	const (
		fileClientID       = "file-client-selection-sentinel"
		fileSecret         = "file-secret-authentication-sentinel"
		establishedClient  = "previous-client-identity-sentinel"
		expiredAccessToken = "expired-access-token-sentinel"
		refreshToken       = "refresh-token-input-sentinel"
		refreshedAccess    = "refreshed-access-token-sentinel"
		providerSentinel   = "raw-provider-description-sentinel"
	)
	privateKey, keySet := generateTestRSAKeySet(t)
	for _, tc := range []struct {
		name               string
		sourceReason       model.CredentialSourceReason
		fileProblem        string
		missingIdentity    bool
		mismatchedIdentity bool
		providerStatus     int
		providerError      string
		malformedResponse  bool
		transportFailure   bool
		code               string
		detail             FailureDetail
	}{
		{name: "missing binding", sourceReason: model.CredentialSourceReasonMissingBinding, code: "server_error"},
		{name: "missing client ID file", sourceReason: model.CredentialSourceReasonNotFound, fileProblem: "client_id_missing", code: "server_error"},
		{name: "missing secret file", sourceReason: model.CredentialSourceReasonNotFound, fileProblem: "secret_missing", code: "server_error"},
		{name: "empty client ID file", sourceReason: model.CredentialSourceReasonEmpty, fileProblem: "client_id_empty", code: "server_error"},
		{name: "empty secret file", sourceReason: model.CredentialSourceReasonEmpty, fileProblem: "secret_empty", code: "server_error"},
		{name: "secret is a directory", sourceReason: model.CredentialSourceReasonNotRegular, fileProblem: "secret_directory", code: "server_error"},
		{name: "missing established identity", sourceReason: model.CredentialSourceReasonIdentityMissing, missingIdentity: true, code: "server_error"},
		{name: "established identity mismatch", sourceReason: model.CredentialSourceReasonIdentityMismatch, mismatchedIdentity: true, code: "server_error"},
		{name: "usable pair succeeds", providerStatus: http.StatusOK},
		{name: "usable pair rejected as invalid_client", providerStatus: http.StatusUnauthorized, providerError: "invalid_client", code: "server_error", detail: DetailProviderClientRejected},
		{name: "usable pair rejected as invalid_grant", providerStatus: http.StatusBadRequest, providerError: "invalid_grant", code: "invalid_grant", detail: DetailRefreshRejected},
		{name: "usable pair rejected with non-JSON body", providerStatus: http.StatusServiceUnavailable, code: "server_error", detail: DetailProviderUnavailable},
		{name: "usable pair transport failure is not rejection", transportFailure: true, code: "server_error", detail: DetailProviderUnavailable},
		{name: "usable pair decoding failure is not rejection", providerStatus: http.StatusOK, malformedResponse: true, code: "server_error", detail: DetailProviderResponseInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			directory := t.TempDir()
			binding := ports.CredentialFileBinding{
				ClientIDFile:     filepath.Join(directory, "identity-input-sentinel"),
				ClientSecretFile: filepath.Join(directory, "authentication-input-sentinel"),
			}
			rawClientID, rawSecret := "\n "+fileClientID+" \n", "\n "+fileSecret+" \n"
			require.NoError(t, os.WriteFile(binding.ClientIDFile, []byte(rawClientID), 0600))
			require.NoError(t, os.WriteFile(binding.ClientSecretFile, []byte(rawSecret), 0600))
			switch tc.fileProblem {
			case "client_id_missing":
				require.NoError(t, os.Remove(binding.ClientIDFile))
			case "secret_missing":
				require.NoError(t, os.Remove(binding.ClientSecretFile))
			case "client_id_empty":
				require.NoError(t, os.WriteFile(binding.ClientIDFile, []byte(" \n\t"), 0600))
			case "secret_empty":
				require.NoError(t, os.WriteFile(binding.ClientSecretFile, []byte(" \n\t"), 0600))
			case "secret_directory":
				require.NoError(t, os.Remove(binding.ClientSecretFile))
				require.NoError(t, os.Mkdir(binding.ClientSecretFile, 0700))
			}

			providerPayload := strings.Join([]string{
				providerSentinel, fileClientID, fileSecret, binding.ClientIDFile, binding.ClientSecretFile,
				"client_secret=" + fileSecret, "refresh_token=" + refreshToken, "client_assertion=provider-echo-assertion",
			}, " ")
			var requestMu sync.Mutex
			var requests []struct {
				form   url.Values
				header http.Header
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if err := request.ParseForm(); err != nil {
					t.Errorf("synthetic provider cannot parse request: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				requestMu.Lock()
				requests = append(requests, struct {
					form   url.Values
					header http.Header
				}{form: request.PostForm, header: request.Header.Clone()})
				requestMu.Unlock()
				if tc.providerStatus == 0 {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.providerStatus)
				switch {
				case tc.malformedResponse:
					_, _ = fmt.Fprint(w, "not-json "+providerPayload)
				case tc.providerError != "":
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error": tc.providerError, "error_description": providerPayload,
						"error_uri": "https://provider.example.com/" + providerSentinel,
					})
				case tc.providerStatus != http.StatusOK:
					_, _ = fmt.Fprint(w, "<html>"+providerPayload+"</html>")
				default:
					_ = json.NewEncoder(w).Encode(map[string]any{
						"access_token": refreshedAccess, "refresh_token": "rotated-" + refreshToken,
						"token_type": "Bearer", "expires_in": 3600, "scope": "repo",
					})
				}
			}))
			t.Cleanup(upstream.Close)
			if tc.transportFailure {
				upstream.Close()
			}

			agentID, svcID, psID := id.NewAgentID(), id.NewServiceID(), id.NewPermissionSetID()
			canonicalID := "filesystem-provider"
			repo := &resolvingServiceRepository{MockServiceRepository{service: &model.ThirdpartyOAuth2ProviderEntity{
				ID: svcID, CanonicalID: &canonicalID, DisplayName: "Example Service",
				CredentialSource: model.CredentialSourceFilesystem, Secret: model.NewAbsentSecret(),
				ProtectedResources: []string{"https://api.example.com/resource"},
				Endpoints:          model.OAuth2Endpoints{TokenEndpoint: upstream.URL},
			}}}
			agentRepo := &singleAgentRepo{agentID: agentID, agent: &storagedomain.Agent{
				ID: agentID, DisplayName: "Test Agent", PermissionSets: []storagedomain.AgentPermissionSetEntry{
					{PermissionSetID: psID, RequirementType: storagedomain.RequirementTypeOptional},
				},
			}}
			now := time.Now()
			future, past := now.Add(time.Hour), now.Add(-time.Hour)
			grantRepo := &MockGrantRepository{grant: &storagedomain.UserGrant{
				ID: id.NewGrantID(), AgentID: agentID, Principal: "user@example.com", ValidUntil: &future,
				GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}}},
			}}
			psRepo := &MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{psID: {
				ID: psID, Name: "Test Permission Set", ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcID, Scopes: []string{"repo"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			}}}
			establishedID := id.ClientID(fileClientID)
			if tc.mismatchedIdentity {
				establishedID = id.ClientID(establishedClient)
			}
			sessionRepo := &MockSessionRepository{session: &storagedomain.UserSession{
				ID: id.NewSessionID(), Principal: "user@example.com", ServiceID: svcID, UpstreamClientID: &establishedID,
				EncryptedAccessToken: []byte(expiredAccessToken), EncryptedRefreshToken: []byte(refreshToken),
				TokenType: "Bearer", Scope: []string{"repo"}, AccessTokenExpiresAt: &past, RefreshTokenExpiresAt: &future,
				EncryptionContext: storagedomain.EncryptionContext{ServiceID: svcID},
			}}
			if tc.missingIdentity {
				sessionRepo.session.UpstreamClientID = nil
			}
			config := oauth2session.DefaultConfig()
			config.MaxRetries = 1
			config.CredentialFiles = map[string]ports.CredentialFileBinding{canonicalID: binding}
			if tc.sourceReason == model.CredentialSourceReasonMissingBinding {
				config.CredentialFiles = nil
			}
			var logs strings.Builder
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			thirdpartyService := newTestProviderService(repo)
			sessionService := oauth2session.NewOAuth2SessionService(thirdpartyService, sessionRepo, sessionRepo, nil, nil,
				&MockEncryption{}, &http.Client{Timeout: time.Second}, nil, config, logger).WithCredentialFileReader(credentialfile.New())
			jwtValidator, err := NewJWTValidator(&MockJWKSProvider{keySet: keySet}, "https://auth.example.com", "agentic-identity-broker", 60)
			require.NoError(t, err)
			celEvaluator, err := NewCELEvaluator(CELEvaluatorConfig{
				PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.azp",
				AuthorizationExpression: "true", EvaluationTimeout: 100 * time.Millisecond,
			})
			require.NoError(t, err)
			svc := &TokenExchangeService{
				jwtValidator: jwtValidator, celEvaluator: celEvaluator, providerService: thirdpartyService,
				oauth2SessionService: sessionService, agentRepository: agentRepo,
				consentService:       consent.NewService(agentRepo, thirdpartyService, grantRepo, nil, nil, logger),
				permissionSetService: permissionset.NewPermissionSetService(psRepo, grantRepo, logger),
			}
			claims := map[string]interface{}{
				"iss": "https://auth.example.com", "aud": "agentic-identity-broker", "sub": "user@example.com",
				"azp": agentID.String(), "exp": future.Unix(), "iat": now.Unix(),
			}
			signedJWT := signServiceTestJWT(t, privateKey, claims)
			req := NewTokenExchangeRequest(TokenExchangeGrantType, signedJWT, AccessTokenType,
				signedJWT, JWTBearerType, "https://api.example.com/resource", "")
			response, err := svc.Exchange(ctx, req)
			serviceRef := ServiceRef{ID: svcID}
			var printableErrors strings.Builder
			if tc.code == "" {
				require.NoError(t, err)
				require.NotNil(t, response)
				assert.Equal(t, refreshedAccess, response.AccessToken, "exchange must return the refreshed token, not the expired token")
				assert.Equal(t, serviceRef, response.Service)
			} else {
				require.Error(t, err)
				assert.Nil(t, response)
				var tokenErr *TokenExchangeError
				require.ErrorAs(t, err, &tokenErr)
				assert.Equal(t, tc.code, tokenErr.Code())
				assert.Equal(t, serviceRef, tokenErr.Service())
				if tc.code == "server_error" {
					assert.Equal(t, http.StatusInternalServerError, tokenErr.HTTPStatus())
					assert.Equal(t, "failed to get valid access token", tokenErr.Description())
					assert.Empty(t, tokenErr.ErrorURI())
				} else {
					assert.Equal(t, http.StatusBadRequest, tokenErr.HTTPStatus())
					assert.Equal(t, sessionService.SessionRecoveryURL(), tokenErr.ErrorURI())
				}
				wantDetail := tc.detail
				if tc.sourceReason != "" {
					wantDetail = DetailCredentialSourceUnavailable
				}
				wantOutcome, wantAction, wantTarget := OutcomeInfrastructureError, RecoveryRetry, TargetNone
				switch wantDetail {
				case DetailProviderClientRejected:
					wantOutcome, wantAction, wantTarget = OutcomeConfigurationError, RecoveryFixConfiguration, TargetBrokerConfiguration
				case DetailRefreshRejected:
					wantOutcome, wantAction, wantTarget = OutcomeReauthRequired, RecoveryReauthenticate, TargetProviderSession
					assert.ErrorIs(t, err, oauth2session.ErrRefreshRejected)
					assert.NotErrorIs(t, err, oauth2session.ErrRefreshTokenExpired)
				}
				diagnostic := tokenErr.Diagnostic()
				assert.Equal(t, wantDetail, diagnostic.Detail())
				assert.Equal(t, StageRefresh, diagnostic.Stage())
				assert.Equal(t, wantOutcome, diagnostic.Outcome())
				assert.Equal(t, wantAction, diagnostic.RecoveryAction())
				assert.Equal(t, wantTarget, diagnostic.RecoveryTarget())
				assert.Equal(t, ExchangeThirdParty, diagnostic.ExchangeKind())
				if tc.sourceReason != "" {
					assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
					assert.NotErrorIs(t, err, oauth2session.ErrRefreshFailed)
					assert.NotErrorIs(t, err, oauth2session.ErrRefreshTokenExpired)
					var sourceErr *model.CredentialSourceError
					if assert.ErrorAs(t, err, &sourceErr) {
						assert.Equal(t, tc.sourceReason, sourceErr.Reason)
					}
					var operationErr *oauth2session.OperationError
					require.ErrorAs(t, err, &operationErr)
					assert.Equal(t, oauth2session.OperationRefresh, operationErr.Metadata().Operation())
					assert.Equal(t, oauth2session.DetailCredentialSourceUnavailable, operationErr.Metadata().Detail())
					assert.Equal(t, oauth2session.KindInfrastructure, operationErr.Metadata().Kind())
					assert.Equal(t, oauth2session.DependencyCredentialSource, operationErr.Metadata().Dependency())
					var rejected *oauth2session.RefreshRejectedError
					assert.False(t, errors.As(err, &rejected), "source failures must not become provider rejection")
				} else {
					assert.NotErrorIs(t, err, model.ErrCredentialSourceUnavailable)
				}
				for cause := err; cause != nil; cause = errors.Unwrap(cause) {
					_, _ = fmt.Fprintln(&printableErrors, cause)
				}
				_, _ = fmt.Fprintln(&printableErrors, tokenErr.Description(), tokenErr.Diagnostic(), tokenErr.ErrorURI())
			}

			requestMu.Lock()
			captured := requests
			requestMu.Unlock()
			wantRequests := 1
			if tc.sourceReason != "" || tc.transportFailure {
				wantRequests = 0
			}
			assert.Len(t, captured, wantRequests)
			for _, request := range captured {
				assert.Equal(t, "refresh_token", request.form.Get("grant_type"))
				assert.Equal(t, refreshToken, request.form.Get("refresh_token"))
				assert.Equal(t, fileClientID, request.form.Get("client_id"))
				assert.Equal(t, fileSecret, request.form.Get("client_secret"))
				assert.Empty(t, request.form.Get("client_assertion"))
				assert.Empty(t, request.header.Get("Authorization"))
				assert.NotContains(t, logs.String(), request.form.Encode(), "secret-bearing request forms must not be logged")
				assert.NotContains(t, printableErrors.String(), request.form.Encode())
			}

			wantEvents := []string{"session.oauth2.credential_files_used"}
			if tc.sourceReason != "" {
				wantEvents = []string{"session.oauth2.credential_source_failed"}
			} else if tc.providerStatus >= http.StatusBadRequest {
				wantEvents = append(wantEvents, "session.oauth2.credential_provider_rejected")
			}
			var eventNames []string
			for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
				if line == "" {
					continue
				}
				var entry map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &entry))
				event, _ := entry["event"].(string)
				if !strings.HasPrefix(event, "session.oauth2.credential_") {
					continue
				}
				eventNames = append(eventNames, event)
				assert.Equal(t, svcID.String(), entry["service_id"])
				assert.Equal(t, "refresh", entry["operation"])
				switch event {
				case "session.oauth2.credential_files_used":
					assert.Equal(t, "used", entry["outcome"])
				case "session.oauth2.credential_source_failed":
					assert.Equal(t, "failed", entry["outcome"])
					assert.Equal(t, string(tc.sourceReason), entry["reason"])
				case "session.oauth2.credential_provider_rejected":
					assert.Equal(t, "rejected", entry["outcome"])
					assert.Equal(t, "provider_rejected", entry["reason"])
				}
				for _, forbiddenKey := range []string{"client_id", "client_secret", "client_id_file", "client_secret_file", "path", "form", "authorization", "client_assertion", "error_description"} {
					assert.NotContains(t, entry, forbiddenKey)
				}
			}
			assert.Equal(t, wantEvents, eventNames, "selection, source failure, and provider rejection are separate ordered outcomes")
			for _, sensitive := range []string{
				directory, binding.ClientIDFile, binding.ClientSecretFile,
				filepath.Base(binding.ClientIDFile), filepath.Base(binding.ClientSecretFile),
				fileClientID, fileSecret, rawClientID, rawSecret, establishedClient,
				expiredAccessToken, refreshToken, refreshedAccess, signedJWT, providerSentinel, providerPayload,
				"provider-echo-assertion", "client_secret=", "refresh_token=", "client_assertion=",
				"no such file or directory", "permission denied", "is a directory", "PathError",
			} {
				assert.NotContains(t, logs.String(), sensitive, "logs must not disclose credential or provider material")
				assert.NotContains(t, printableErrors.String(), sensitive, "printable error chains must not disclose credential or provider material")
			}
		})
	}
}
