package fixtures

import (
	"context"
	"slices"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// PlaceholderPermissionSetID is the stable permission set ID used by grant fixtures
// that do not require specific permission set resolution. Call SeedPlaceholderGrantData
// on the test storage before exercising paths that resolve permission set scopes.
var PlaceholderPermissionSetID = id.MustParsePermissionSetID("00000000-0000-0000-0000-000000000001")

// PlaceholderServiceID is the stable service ID paired with PlaceholderPermissionSetID
// in grant fixtures. Call SeedPlaceholderGrantData to seed the backing rows.
var PlaceholderServiceID = id.MustParseServiceID("00000000-0000-0000-0000-000000000002")

// SecondaryPlaceholderPermissionSetID is a second stable permission set ID used by
// multi-service grant fixtures. Call SeedPlaceholderGrantData to seed backing rows.
var SecondaryPlaceholderPermissionSetID = id.MustParsePermissionSetID("00000000-0000-0000-0000-000000000003")

// SecondaryPlaceholderServiceID is the stable service ID paired with SecondaryPlaceholderPermissionSetID.
var SecondaryPlaceholderServiceID = id.MustParseServiceID("00000000-0000-0000-0000-000000000004")

// placeholderEntry returns a GrantedPermissionSetEntry using placeholder IDs plus
// any distinct services explicitly covered by the calling fixture.
func placeholderEntry(extra ...id.ServiceID) storagedomain.GrantedPermissionSetEntry {
	includedServiceIDs := []id.ServiceID{PlaceholderServiceID}
	for _, serviceID := range extra {
		if !slices.Contains(includedServiceIDs, serviceID) {
			includedServiceIDs = append(includedServiceIDs, serviceID)
		}
	}

	return storagedomain.GrantedPermissionSetEntry{
		PermissionSetID:    PlaceholderPermissionSetID,
		IncludedServiceIDs: includedServiceIDs,
	}
}

func placeholderEntryForService(serviceID string) storagedomain.GrantedPermissionSetEntry {
	parsedServiceID, err := id.ParseServiceID(serviceID)
	if err != nil {
		return placeholderEntry()
	}
	return placeholderEntry(parsedServiceID)
}

// SeedPlaceholderGrantData inserts the placeholder service, permission set, and
// their FK relationship into storage. Covered services must already exist because
// permission_set_service_scopes.service_id references thirdparty_oauth2_services.
// Call this in BeforeEach blocks for tests that exercise paths that resolve permission
// set scopes (e.g. token exchange, consent info).
func SeedPlaceholderGrantData(ctx context.Context, store *storageadapter.Adapter, covered ...id.ServiceID) error {
	svc := ServiceWithID(PlaceholderServiceID.String())
	// Override scopes to match what the placeholder permission set declares.
	svc.Scopes = []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}}
	svc.ProtectedResources = nil
	if err := store.Services().Create(ctx, svc); err != nil {
		return err
	}

	serviceScopes := []storagedomain.ServiceScope{
		{ServiceID: PlaceholderServiceID, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
	}
	seenServiceIDs := map[id.ServiceID]bool{PlaceholderServiceID: true}
	for _, serviceID := range covered {
		if seenServiceIDs[serviceID] {
			continue
		}
		seenServiceIDs[serviceID] = true
		serviceScopes = append(serviceScopes, storagedomain.ServiceScope{
			ServiceID:       serviceID,
			RequirementType: storagedomain.RequirementTypeOptional,
		})
	}

	ps := &storagedomain.PermissionSet{
		ID:            PlaceholderPermissionSetID,
		Name:          "Placeholder Permission Set",
		Description:   "Test fixture permission set — seeded by SeedPlaceholderGrantData",
		ServiceScopes: serviceScopes,
	}
	if err := store.PermissionSets().Create(ctx, ps); err != nil {
		return err
	}

	// Seed secondary service+PS for multi-service fixtures (GrantWithMultipleServices).
	svc2 := ServiceWithID(SecondaryPlaceholderServiceID.String())
	svc2.Scopes = []model.OAuthScope{{ScopeValue: "write", Description: "Write access"}}
	svc2.ProtectedResources = nil
	if err := store.Services().Create(ctx, svc2); err != nil {
		return err
	}

	ps2 := &storagedomain.PermissionSet{
		ID:          SecondaryPlaceholderPermissionSetID,
		Name:        "Secondary Placeholder Permission Set",
		Description: "Secondary test fixture permission set — seeded by SeedPlaceholderGrantData",
		ServiceScopes: []storagedomain.ServiceScope{
			{ServiceID: SecondaryPlaceholderServiceID, Scopes: []string{"write"}, RequirementType: storagedomain.RequirementTypeOptional},
		},
	}
	return store.PermissionSets().Create(ctx, ps2)
}

// SeedDefaultConsentData inserts the shared default permission set and an active
// opaque session for the supplied principal. It avoids encryption because consent
// flows only need session presence, not token decryption.
func SeedDefaultConsentData(ctx context.Context, store *storageadapter.Adapter, principal id.Principal) error {
	if err := store.PermissionSets().Create(ctx, &storagedomain.PermissionSet{
		ID:          PlaceholderPermissionSetID,
		Name:        "Default Test Permission Set",
		Description: "Permission set for authorization-code test flows",
		ServiceScopes: []storagedomain.ServiceScope{{
			ServiceID:       PlaceholderServiceID,
			RequirementType: storagedomain.RequirementTypeMandatory,
		}},
	}); err != nil {
		return err
	}

	return store.UserSessions().Create(ctx, &storagedomain.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            principal,
		ServiceID:            PlaceholderServiceID,
		EncryptedAccessToken: []byte("opaque-test-token"),
		TokenType:            "Bearer",
		EncryptionContext:    storagedomain.EncryptionContext{ServiceID: PlaceholderServiceID},
	})
}

// ActiveGrant returns a user grant that is currently active (not expired).
// Principal and agentID must be provided by caller.
// ValidUntil is set to 1 hour in the future.
// Call SeedPlaceholderGrantData with the referenced service ID when the grant will
// be resolved through paths that look up permission set scopes (e.g. token exchange).
func ActiveGrant(principal, agentID, serviceID string, scopes []string) *storagedomain.UserGrant {
	now := time.Now()
	validUntil := now.Add(1 * time.Hour)

	return &storagedomain.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal(principal),
		AgentID:               id.MustParseAgentID(agentID),
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{placeholderEntryForService(serviceID)},
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

// ExpiredGrant returns a user grant that has already expired.
// ValidUntil is set to 1 hour in the past.
func ExpiredGrant(principal, agentID, serviceID string, scopes []string) *storagedomain.UserGrant {
	now := time.Now()
	validUntil := now.Add(-1 * time.Hour)

	return &storagedomain.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal(principal),
		AgentID:               id.MustParseAgentID(agentID),
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{placeholderEntryForService(serviceID)},
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

// GrantExpiringIn returns a user grant that expires in the specified duration.
// Useful for testing edge cases like grants expiring soon.
func GrantExpiringIn(principal, agentID, serviceID string, scopes []string, duration time.Duration) *storagedomain.UserGrant {
	now := time.Now()
	validUntil := now.Add(duration)

	return &storagedomain.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal(principal),
		AgentID:               id.MustParseAgentID(agentID),
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{placeholderEntryForService(serviceID)},
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

// IndefiniteGrant returns a user grant that never expires (ValidUntil is nil).
// Indefinite grants remain active until explicitly revoked.
func IndefiniteGrant(principal, agentID, serviceID string, scopes []string) *storagedomain.UserGrant {
	now := time.Now()

	return &storagedomain.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal(principal),
		AgentID:               id.MustParseAgentID(agentID),
		ValidUntil:            nil,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{placeholderEntryForService(serviceID)},
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

// GrantWithMultipleServices returns a user grant referencing two distinct permission sets,
// each covering a different service. Requires SeedPlaceholderGrantData to be called first
// to ensure both permission sets and their backing services exist in storage.
func GrantWithMultipleServices(principal, agentID string) *storagedomain.UserGrant {
	now := time.Now()
	validUntil := now.Add(24 * time.Hour)

	return &storagedomain.UserGrant{
		ID:         id.NewGrantID(),
		Principal:  id.Principal(principal),
		AgentID:    id.MustParseAgentID(agentID),
		ValidUntil: &validUntil,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{
			placeholderEntry(),
			{
				PermissionSetID:    SecondaryPlaceholderPermissionSetID,
				IncludedServiceIDs: []id.ServiceID{SecondaryPlaceholderServiceID},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// GrantWithService returns a user grant covering the supplied service ID.
func GrantWithService(principal, agentID, serviceID string, scopes []string) *storagedomain.UserGrant {
	now := time.Now()
	validUntil := now.Add(30 * 24 * time.Hour) // 30 days

	return &storagedomain.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             id.Principal(principal),
		AgentID:               id.MustParseAgentID(agentID),
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{placeholderEntryForService(serviceID)},
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}
