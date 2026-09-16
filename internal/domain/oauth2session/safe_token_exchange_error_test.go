package oauth2session

import (
	"errors"
	"net/url"
	"testing"

	"golang.org/x/oauth2"

	"github.com/stretchr/testify/assert"
)

func TestSafeTokenExchangeError_PreservesTransportCause(t *testing.T) {
	cause := errors.New("connection refused")
	err := safeTokenExchangeError(&url.Error{
		Op:  "Post",
		URL: "https://token.example.com",
		Err: cause,
	})

	assert.ErrorIs(t, err, ErrTokenExchange)
	assert.ErrorIs(t, err, cause)
	assert.NotContains(t, err.Error(), "https://token.example.com")
}

func TestSafeTokenExchangeError_RedactsUnknownOAuthErrorFields(t *testing.T) {
	const sentinel = "sentinel-upstream-oauth-error"

	err := safeTokenExchangeError(&oauth2.RetrieveError{
		ErrorCode:        sentinel,
		ErrorDescription: sentinel,
	})

	assert.ErrorIs(t, err, ErrTokenExchange)
	assert.NotContains(t, err.Error(), sentinel)
	assert.Contains(t, err.Error(), "upstream token request failed")
}
