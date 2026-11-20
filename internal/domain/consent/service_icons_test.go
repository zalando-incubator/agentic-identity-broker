package consent

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestServiceIconUsesCurrentRequirementMetadata(t *testing.T) {
	first := `<svg xmlns="http://www.w3.org/2000/svg"><text>Atlas 星</text></svg>`
	second := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="8"/></svg>`
	serviceID, unrelatedID := id.NewServiceID(), id.NewServiceID()
	provider := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, DisplayName: "Atlas", IconSVG: &first, Secret: model.NewAbsentSecret()}
	repo := &mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{
		serviceID: provider, unrelatedID: {ID: unrelatedID, DisplayName: "Not a requirement", IconSVG: &second, Secret: model.NewAbsentSecret()},
	}}
	svc := &Service{providerService: newTestProviderService(repo), logger: slog.New(slog.DiscardHandler)}
	agent := &storage.Agent{ID: id.NewAgentID(), ServiceRequirements: []storage.ServiceRequirement{{ServiceID: serviceID, RequirementType: storage.RequirementTypeMandatory}}}
	for _, icon := range []*string{&first, &second, nil} {
		provider.IconSVG = icon
		result, err := svc.resolveServiceRequirements(context.Background(), agent, nil, []id.ServiceID{serviceID})
		require.NoError(t, err)
		require.Len(t, result, 1, "do not broaden existing response scope")
		assert.Equal(t, serviceID, result[0].ServiceID)
		assert.Equal(t, "Atlas", result[0].DisplayName)
		assert.True(t, result[0].IsConnected)
		assert.Equal(t, icon, result[0].IconSVG)
	}
}
