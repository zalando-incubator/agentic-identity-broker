package tokenexchange

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

func TestExchange_RejectsUndeclaredPermissionSets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		mixedGrant        bool
		emptyDeclarations bool
		otherServiceOnly  bool
	}{
		{name: "unrelated set without service requirements"},
		{name: "mixed declared and undeclared sets for a declared service", mixedGrant: true},
		{name: "declared service A with undeclared set covering only B", mixedGrant: true, otherServiceOnly: true},
		{name: "empty agent declarations", emptyDeclarations: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			privateKey, keySet := generateTestRSAKeySet(t)
			agentID := id.NewAgentID()
			coveredServiceID := id.NewServiceID()
			requestedServiceID := id.NewServiceID()
			declaredSetID := id.NewPermissionSetID()
			undeclaredSetID := id.NewPermissionSetID()
			now := time.Now()
			futureTime := now.Add(time.Hour)
			agent := &storagedomain.Agent{
				ID:          agentID,
				ClientID:    ptr.To(id.ClientID("test-agent-client")),
				DisplayName: "Test Agent",
				PermissionSets: []storagedomain.AgentPermissionSetEntry{
					{PermissionSetID: declaredSetID, RequirementType: storagedomain.RequirementTypeOptional},
				},
			}
			// Seed a stored grant independently of consent validation.
			grant := &storagedomain.UserGrant{
				ID:         id.NewGrantID(),
				AgentID:    agentID,
				Principal:  id.Principal("user@example.com"),
				ValidUntil: &futureTime,
				GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
					{PermissionSetID: undeclaredSetID, IncludedServiceIDs: []id.ServiceID{requestedServiceID}},
				},
				CreatedAt: now,
				UpdatedAt: now,
			}
			agentRepo := &singleAgentRepo{agentID: agentID, agent: agent}
			grantRepo := &MockGrantRepository{grant: grant}
			psRepo := &MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
				declaredSetID: {
					ID:   declaredSetID,
					Name: "Declared A",
					ServiceScopes: []storagedomain.ServiceScope{
						{ServiceID: coveredServiceID, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
					},
				},
				undeclaredSetID: {
					ID:   undeclaredSetID,
					Name: "Unrelated B",
					ServiceScopes: []storagedomain.ServiceScope{
						{ServiceID: coveredServiceID, Scopes: []string{"read", "write"}, RequirementType: storagedomain.RequirementTypeOptional},
						{ServiceID: requestedServiceID, Scopes: []string{"read", "write"}, RequirementType: storagedomain.RequirementTypeOptional},
					},
				},
			}}
			if tt.mixedGrant {
				otherServiceID := requestedServiceID
				requestedServiceID = coveredServiceID
				agent.ServiceRequirements = []storagedomain.ServiceRequirement{
					{ServiceID: coveredServiceID, RequiredScopes: []string{"read", "write"}, RequirementType: storagedomain.RequirementTypeOptional},
				}
				undeclaredServiceID := coveredServiceID
				if tt.otherServiceOnly {
					undeclaredServiceID = otherServiceID
					psRepo.psMap[undeclaredSetID].ServiceScopes = []storagedomain.ServiceScope{{
						ServiceID: otherServiceID, Scopes: []string{"read", "write"}, RequirementType: storagedomain.RequirementTypeOptional,
					}}
				}
				grant.GrantedPermissionSets = []storagedomain.GrantedPermissionSetEntry{
					{PermissionSetID: declaredSetID, IncludedServiceIDs: []id.ServiceID{coveredServiceID}},
					{PermissionSetID: undeclaredSetID, IncludedServiceIDs: []id.ServiceID{undeclaredServiceID}},
				}
			}
			if tt.emptyDeclarations {
				agent.PermissionSets = nil
			}
			psService := permissionset.NewPermissionSetService(psRepo, grantRepo, slog.Default())
			t.Cleanup(psService.Close)
			consentSvc := consent.NewService(
				agentRepo,
				newTestProviderService(&MockServiceRepository{}),
				grantRepo,
				nil,
				nil,
				slog.Default(),
			)
			sessionRepo := &MockSessionRepository{
				session: &storagedomain.UserSession{
					ID:                   id.NewSessionID(),
					Principal:            id.Principal("user@example.com"),
					ServiceID:            requestedServiceID,
					EncryptedAccessToken: []byte("access-token"),
					TokenType:            "Bearer",
					AccessTokenExpiresAt: &futureTime,
					Scope:                []string{"read", "write"},
				},
			}
			oauth2SessionService := oauth2session.NewOAuth2SessionService(
				nil,
				sessionRepo,
				nil,
				nil,
				&MockEncryption{},
				nil,
				nil,
				oauth2session.Config{CallbackBaseURL: "https://broker.example.com/"},
				slog.Default(),
			)
			jwtValidator, err := NewJWTValidator(
				&MockJWKSProvider{keySet: keySet},
				"https://auth.example.com",
				"agentic-identity-broker",
				60,
			)
			require.NoError(t, err)
			celEvaluator, err := NewCELEvaluator(CELEvaluatorConfig{
				PrincipalExpression:     "subject_token.sub",
				AgentIDExpression:       "subject_token.azp",
				AuthorizationExpression: "true",
				EvaluationTimeout:       100 * time.Millisecond,
			})
			require.NoError(t, err)
			svc := &TokenExchangeService{
				jwtValidator: jwtValidator,
				celEvaluator: celEvaluator,
				providerService: newTestProviderService(&MockServiceRepository{
					service: &model.ThirdpartyOAuth2ProviderEntity{
						ID:                 requestedServiceID,
						DisplayName:        "Requested Service",
						ClientID:           id.ClientID("service-client"),
						Secret:             model.NewEncryptedSecret([]byte("placeholder")),
						ProtectedResources: []string{"https://api.example.com/requested"},
					},
				}),
				oauth2SessionService: oauth2SessionService,
				consentService:       consentSvc,
				permissionSetService: psService,
				agentRepository:      agentRepo,
				config: &ports.TokenExchangeConfig{
					ClaimExtraction: ports.ClaimExtractionConfig{PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.azp"},
					Authorization:   ports.AuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{Expression: "true"}},
				},
			}
			claims := map[string]interface{}{
				"iss": "https://auth.example.com",
				"aud": "agentic-identity-broker",
				"sub": "user@example.com",
				"azp": agentID.String(),
				"exp": futureTime.Unix(),
				"iat": now.Unix(),
			}
			req := NewTokenExchangeRequest(
				TokenExchangeGrantType,
				signServiceTestJWT(t, privateKey, claims),
				AccessTokenType,
				signServiceTestJWT(t, privateKey, claims),
				JWTBearerType,
				"https://api.example.com/requested",
				"",
			)

			response, err := svc.Exchange(context.Background(), req)
			assert.Nil(t, response)
			var tokenErr *TokenExchangeError
			require.ErrorAs(t, err, &tokenErr)
			assert.Equal(t, "invalid_grant", tokenErr.Code())
			assert.Equal(t, "https://broker.example.com/agents/"+agentID.String(), tokenErr.ErrorURI())
			assert.Zero(t, sessionRepo.findByPrincipalAndServiceCalls, "undeclared permission sets must be rejected before token-vault lookup")
		})
	}
}

func TestResolveEffectiveScopes_RejectsRemovedDeclaration(t *testing.T) {
	t.Parallel()

	serviceID := id.NewServiceID()
	retainedSetID := id.NewPermissionSetID()
	removedSetID := id.NewPermissionSetID()
	psRepo := &MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
		retainedSetID: {
			ID:   retainedSetID,
			Name: "Retained A",
			ServiceScopes: []storagedomain.ServiceScope{
				{ServiceID: serviceID, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
			},
		},
		removedSetID: {
			ID:   removedSetID,
			Name: "Removed B",
			ServiceScopes: []storagedomain.ServiceScope{
				{ServiceID: serviceID, Scopes: []string{"read", "write"}, RequirementType: storagedomain.RequirementTypeOptional},
			},
		},
	}}
	psService := permissionset.NewPermissionSetService(psRepo, &MockGrantRepository{}, slog.Default())
	t.Cleanup(psService.Close)
	svc := &TokenExchangeService{
		permissionSetService: psService,
		oauth2SessionService: oauth2session.NewOAuth2SessionService(
			nil, nil, nil, nil, nil, nil, nil,
			oauth2session.Config{CallbackBaseURL: "https://broker.example.com/"}, nil,
		),
	}
	agent := &storagedomain.Agent{
		ID: id.NewAgentID(),
		PermissionSets: []storagedomain.AgentPermissionSetEntry{
			{PermissionSetID: retainedSetID, RequirementType: storagedomain.RequirementTypeOptional},
			{PermissionSetID: removedSetID, RequirementType: storagedomain.RequirementTypeOptional},
		},
	}
	grant := &storagedomain.UserGrant{
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: removedSetID, IncludedServiceIDs: []id.ServiceID{serviceID}},
		},
	}

	scopes, err := svc.resolveEffectiveScopes(context.Background(), grant, agent)
	require.NoError(t, err)
	require.Equal(t, map[id.ServiceID][]string{serviceID: {"read", "write"}}, scopes)

	// Keep the definitions and cached service; only the agent declaration changes.
	agent.PermissionSets = agent.PermissionSets[:1]
	scopes, err = svc.resolveEffectiveScopes(context.Background(), grant, agent)
	assert.Nil(t, scopes)
	var tokenErr *TokenExchangeError
	if assert.ErrorAs(t, err, &tokenErr) {
		assert.Equal(t, "invalid_grant", tokenErr.Code())
		assert.Equal(t, "https://broker.example.com/agents/"+agent.ID.String(), tokenErr.ErrorURI())
	}
}
