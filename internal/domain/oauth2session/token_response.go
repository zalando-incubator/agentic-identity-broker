package oauth2session

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Providers advertise refresh-token lifetime under different names: Keycloak uses
// refresh_expires_in, GitHub Apps, Auth0 and Microsoft use refresh_token_expires_in.
var refreshLifetimeFields = []string{"refresh_expires_in", "refresh_token_expires_in"}

// lifetimeSeconds decodes a token-endpoint lifetime sent as a JSON number or numeric string.
// A value it cannot interpret is treated as absent so that a malformed lifetime never discards
// an otherwise valid token.
type lifetimeSeconds struct {
	seconds int64
	present bool
}

func (l *lifetimeSeconds) UnmarshalJSON(data []byte) error {
	var number json.Number
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) || json.Unmarshal(data, &number) != nil {
		return nil
	}
	if seconds, ok := numberSeconds(number.String()); ok {
		*l = lifetimeSeconds{seconds: seconds, present: true}
	}
	return nil
}

func numberSeconds(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return max(seconds, 0), true
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds > math.MaxInt32 {
		return 0, false
	}
	return int64(max(seconds, 0)), true
}

// extraSeconds reads a lifetime from oauth2.Token extras, which hold float64 for JSON
// responses, strings for form-encoded responses, and int64 for responses built by this package.
func extraSeconds(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return max(v, 0), true
	case float64:
		return numberSeconds(strconv.FormatFloat(v, 'f', -1, 64))
	case json.Number:
		return numberSeconds(v.String())
	case string:
		return numberSeconds(v)
	default:
		return 0, false
	}
}

// refreshTokenExpiry returns the refresh-token expiry advertised in a token response.
// A missing field or a lifetime of zero, which Keycloak uses for non-expiring offline
// tokens, yields nil.
func refreshTokenExpiry(token *oauth2.Token, now time.Time) *time.Time {
	for _, field := range refreshLifetimeFields {
		seconds, ok := extraSeconds(token.Extra(field))
		if !ok {
			continue
		}
		if seconds == 0 {
			return nil
		}
		expiresAt := now.Add(time.Duration(seconds) * time.Second)
		return &expiresAt
	}
	return nil
}

const maxRetryAfter = 10 * time.Second

// retryAfter parses a Retry-After header given in seconds or as an HTTP date.
func retryAfter(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(max(seconds, 0)) * time.Second
	}
	if at, err := http.ParseTime(header); err == nil {
		return max(at.Sub(now), 0)
	}
	return 0
}
