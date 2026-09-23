package model

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCIMDClientID(t *testing.T) {
	serviceID := id.NewServiceID()

	tests := []struct {
		name      string
		publicURL string
		serviceID id.ServiceID
		want      string
		wantError string
	}{
		{
			name:      "derives identifier from HTTPS origin and path",
			publicURL: "https://broker.example/base/",
			serviceID: serviceID,
			want:      "https://broker.example/base/.well-known/oauth-client/" + serviceID.String(),
		},
		{name: "rejects empty service ID", publicURL: "https://broker.example", wantError: "service ID is required"},
		{name: "rejects HTTP", publicURL: "http://broker.example", serviceID: serviceID, wantError: "public HTTPS URL"},
		{name: "rejects credentials", publicURL: "https://user@broker.example", serviceID: serviceID, wantError: "public HTTPS URL"},
		{name: "rejects empty hostname", publicURL: "https://:443", serviceID: serviceID, wantError: "public HTTPS URL"},
		{name: "rejects force-empty query", publicURL: "https://broker.example?", serviceID: serviceID, wantError: "public HTTPS URL"},
		{name: "rejects query", publicURL: "https://broker.example?x=1", serviceID: serviceID, wantError: "public HTTPS URL"},
		{name: "rejects fragment", publicURL: "https://broker.example#metadata", serviceID: serviceID, wantError: "public HTTPS URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CIMDClientID(tt.publicURL, tt.serviceID)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, id.ClientID(tt.want), got)
		})
	}
}
