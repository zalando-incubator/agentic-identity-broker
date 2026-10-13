package oauth2session

import (
	"errors"
	"net/http"
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
	var operationErr *OperationError
	if assert.ErrorAs(t, err, &operationErr) {
		assert.Equal(t, DetailProviderRejected, operationErr.Metadata().Detail())
		assert.Equal(t, "unknown", operationErr.Metadata().OAuthCode())
	}
}

func TestSafeTokenExchangeErrorPreservesProviderCauseAndPrioritizesStatus(t *testing.T) {
	for _, code := range []string{"invalid_grant", "invalid_client", "unauthorized_client"} {
		for _, status := range []int{400, 429, 503} {
			cause := &oauth2.RetrieveError{Response: &http.Response{StatusCode: status}, ErrorCode: code, ErrorDescription: "sentinel-secret"}
			err := safeTokenExchangeError(cause)
			var preserved *oauth2.RetrieveError
			if assert.ErrorAs(t, err, &preserved) {
				assert.Same(t, cause, preserved)
			}
			metadata := sessionFailureMetadata(err)
			if status == 429 || status == 503 {
				assert.Equal(t, DetailProviderUnavailable, metadata.Detail())
				assert.Equal(t, KindInfrastructure, metadata.Kind())
			} else if code == "invalid_grant" {
				assert.Equal(t, DetailProviderRejected, metadata.Detail())
			} else {
				assert.Equal(t, DetailProviderClientRejected, metadata.Detail())
				assert.Equal(t, KindConfiguration, metadata.Kind())
			}
			assert.Equal(t, code, metadata.OAuthCode())
			assert.Equal(t, status, metadata.StatusCode())
			assert.NotContains(t, err.Error(), "sentinel-secret")
		}
	}
}
