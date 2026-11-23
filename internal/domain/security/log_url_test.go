package security

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain https", raw: "https://agent.example.com/client", want: "https://agent.example.com/client"},
		{name: "query removed", raw: "https://agent.example.com/client?token=secret", want: "https://agent.example.com/client"},
		{name: "bare query delimiter removed", raw: "https://agent.example.com/client?", want: "https://agent.example.com/client"},
		{name: "fragment removed", raw: "https://agent.example.com/cb#access_token=secret", want: "https://agent.example.com/cb"},
		{name: "userinfo removed", raw: "https://user:pass@agent.example.com:8443/cb?code=abc", want: "https://agent.example.com:8443/cb"},
		{name: "loopback port kept", raw: "http://127.0.0.1:53682/callback?state=xyz", want: "http://127.0.0.1:53682/callback"},
		{name: "private-use scheme", raw: "com.example.app:/oauth/callback?code=abc", want: "com.example.app:/oauth/callback"},
		{name: "private-use scheme with authority", raw: "github-app://oauth/callback?code=abc", want: "github-app://oauth/callback"},
		{name: "opaque value dropped", raw: "mailto:someone@example.com?subject=secret", want: "mailto:"},
		{name: "relative reference", raw: "/sessions?redirect=secret", want: "/sessions"},
		{name: "line break in path escaped", raw: "https://agent.example.com/a b", want: "https://agent.example.com/a%E2%80%A8b"},
		{name: "empty", raw: "", want: ""},
		{name: "unparsable", raw: "https://[::1/cb?secret=1", want: InvalidURLLogValue},
		{name: "control character", raw: "https://agent.example.com/cb\n?secret=1", want: InvalidURLLogValue},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, RedactURL(test.raw))
		})
	}
}

func TestRedactURL_LimitsLength(t *testing.T) {
	t.Parallel()

	raw := "https://agent.example.com/" + strings.Repeat("a", 4*MaxLoggedURLBytes) + "?secret=1"
	got := RedactURL(raw)
	assert.LessOrEqual(t, len(got), MaxLoggedURLBytes)
	assert.True(t, strings.HasPrefix(got, "https://agent.example.com/aaa"))
	assert.NotContains(t, got, "secret")
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestClassifyOutboundError(t *testing.T) {
	t.Parallel()

	urlErr := func(cause error) error {
		return &url.Error{Op: "Get", URL: "https://idp.example.com/jwks?secret=1", Err: cause}
	}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "deadline exceeded", err: fmt.Errorf("fetch: %w", context.DeadlineExceeded), want: OutboundErrorTimeout},
		{name: "url error timeout", err: urlErr(timeoutError{}), want: OutboundErrorTimeout},
		{name: "canceled", err: urlErr(context.Canceled), want: OutboundErrorCanceled},
		{name: "transport failure", err: fmt.Errorf("fetch: %w", urlErr(errors.New("connection refused"))), want: OutboundErrorTransport},
		{name: "other", err: errors.New("unexpected status 500"), want: OutboundErrorUnclassified},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, ClassifyOutboundError(test.err))
		})
	}
}
