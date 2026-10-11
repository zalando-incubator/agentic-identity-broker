package config

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestValidateThirdPartyOAuth2Config_RefreshLeeway(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	for _, tc := range []struct {
		name    string
		leeway  time.Duration
		wantErr bool
	}{
		{name: "unset uses default", leeway: 0},
		{name: "minimum", leeway: time.Second},
		{name: "default", leeway: 30 * time.Second},
		{name: "maximum", leeway: 5 * time.Minute},
		{name: "negative", leeway: -time.Second, wantErr: true},
		{name: "below minimum", leeway: 500 * time.Millisecond, wantErr: true},
		{name: "above maximum", leeway: 6 * time.Minute, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateThirdPartyOAuth2Config(&ports.ThirdPartyOAuth2Config{JWESigningKey: key, RefreshLeeway: tc.leeway})
			if tc.wantErr {
				assert.ErrorContains(t, err, "third_party_oauth2.refresh_leeway")
				return
			}
			assert.NoError(t, err)
		})
	}
}
