package oauth2session

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestLifetimeSeconds_DecodesNumbersAndNumericStrings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		seconds int64
		present bool
	}{
		{name: "integer", body: `{"v":3600}`, seconds: 3600, present: true},
		{name: "numeric string", body: `{"v":"3600"}`, seconds: 3600, present: true},
		{name: "float", body: `{"v":3599.9}`, seconds: 3599, present: true},
		{name: "zero", body: `{"v":0}`, seconds: 0, present: true},
		{name: "negative clamps to zero", body: `{"v":-5}`, seconds: 0, present: true},
		{name: "null", body: `{"v":null}`},
		{name: "absent", body: `{}`},
		{name: "garbage string is ignored", body: `{"v":"soon"}`},
		{name: "object is ignored", body: `{"v":{"seconds":1}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var decoded struct {
				V lifetimeSeconds `json:"v"`
			}
			require.NoError(t, json.Unmarshal([]byte(tc.body), &decoded))
			assert.Equal(t, tc.present, decoded.V.present)
			assert.Equal(t, tc.seconds, decoded.V.seconds)
		})
	}
}

func TestRefreshTokenExpiry(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		extra map[string]any
		want  *time.Time
	}{
		{name: "keycloak number", extra: map[string]any{"refresh_expires_in": float64(1800)}, want: ptrTime(now.Add(30 * time.Minute))},
		{name: "github string", extra: map[string]any{"refresh_token_expires_in": "15897600"}, want: ptrTime(now.Add(15897600 * time.Second))},
		{name: "keycloak offline zero means no expiry", extra: map[string]any{"refresh_expires_in": float64(0)}},
		{name: "not advertised", extra: map[string]any{}},
		{name: "unparseable", extra: map[string]any{"refresh_expires_in": "later"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := refreshTokenExpiry((&oauth2.Token{AccessToken: "a"}).WithExtra(tc.extra), now)
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.True(t, tc.want.Equal(*got), "got %s want %s", got, tc.want)
		})
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, 2*time.Second, retryAfter("2", now))
	assert.Equal(t, 5*time.Second, retryAfter(now.Add(5*time.Second).Format(http.TimeFormat), now))
	assert.Zero(t, retryAfter("", now))
	assert.Zero(t, retryAfter("-3", now))
	assert.Zero(t, retryAfter("soon", now))
	assert.Zero(t, retryAfter(now.Add(-time.Minute).Format(http.TimeFormat), now))
}

func ptrTime(t time.Time) *time.Time { return &t }
