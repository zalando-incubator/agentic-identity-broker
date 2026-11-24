package consent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	consentdomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestServiceIconRequirementSerialization(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg"><text>星 &amp; Atlas</text></svg>`
	serviceID := id.NewServiceID()
	for _, icon := range []*string{&source, nil} {
		projected := toServiceRequirementForUser([]consentdomain.ServiceRequirementStatus{{
			ServiceID: serviceID, DisplayName: "Atlas", IconSVG: icon,
			RequirementType: storage.RequirementTypeMandatory, IsConnected: true,
			RequiredScopes: []consentdomain.ServiceScopeInfo{{Name: "read", Description: "Read"}},
		}})
		require.Len(t, projected, 1)
		wire, err := json.Marshal(projected[0])
		require.NoError(t, err)
		var value map[string]any
		require.NoError(t, json.Unmarshal(wire, &value))
		assert.Equal(t, serviceID.String(), value["serviceId"])
		assert.Equal(t, "Atlas", value["serviceName"])
		assert.Equal(t, "connected", value["connectionStatus"])
		if icon == nil {
			assert.NotContains(t, value, "iconSvg")
		} else {
			assert.Equal(t, source, value["iconSvg"])
		}
		assert.NotContains(t, value, "available_services")
	}
}
