package oauth2session

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
)

type operationReadFunc func([]byte) (int, error)

func (f operationReadFunc) Read(p []byte) (int, error) { return f(p) }

func TestRefreshBodyFailureClassifiesDeadlineAndCallerCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		shared bool
	}{
		{"shared dependency deadline", true},
		{"caller canceled while reading", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shared := tc.shared
			service, provider, session := newOperationTestService(t, io.Discard)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cause := context.DeadlineExceeded
			detail, kind := DetailProviderUnavailable, KindInfrastructure
			if !shared {
				cause, detail, kind = context.Canceled, DetailCallerCanceled, KindCanceled
			}
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				body := operationReadFunc(func([]byte) (int, error) {
					if !shared {
						cancel()
					}
					return 0, cause
				})
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body)}, nil
			})
			var err error
			if shared {
				_, _, err = service.GetValidAccessToken(ctx, session.Principal, provider.ID)
			} else {
				_, err = service.RefreshAccessToken(ctx, provider, "refresh")
			}
			metadata := requireOperationError(t, err, OperationRefresh, detail)
			assert.Equal(t, kind, metadata.Kind())
			assert.Equal(t, http.StatusOK, metadata.StatusCode())
			assert.ErrorIs(t, err, cause)
		})
	}
}

func TestRefreshProviderRejectionKeepsCallerCancellationPrecedence(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct caller canceled", true: "shared operation canceled"}[shared], func(t *testing.T) {
			service, provider, _ := newOperationTestService(t, io.Discard)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if shared {
				ctx = context.WithValue(ctx, sharedRefreshContextKey{}, true)
			}
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				cancel()
				return operationTokenResponse(http.StatusBadRequest, `{"error":"invalid_grant"}`), nil
			})
			_, err := service.refreshAccessToken(ctx, provider, "refresh", nil)
			detail, kind := DetailCallerCanceled, KindCanceled
			if shared {
				detail, kind = DetailRefreshRejected, KindProvider
			} else {
				assert.ErrorIs(t, err, context.Canceled)
			}
			metadata := requireOperationError(t, err, OperationRefresh, detail)
			assert.Equal(t, kind, metadata.Kind())
			assert.Equal(t, DependencyProvider, metadata.Dependency())
			assert.Equal(t, http.StatusBadRequest, metadata.StatusCode())
			assert.Equal(t, "invalid_grant", metadata.OAuthCode())
			assert.ErrorIs(t, err, ErrRefreshRejected)
			assert.NotErrorIs(t, err, ErrRefreshTokenExpired)
		})
	}
}

func TestRefreshProviderLookupKeepsConfigurationCauses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing bool
	}{
		{"provider deleted", true},
		{"provider identity mismatch", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			missing := tc.missing
			service, provider, session := newOperationTestService(t, io.Discard)
			repository := operationProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				if missing {
					return nil, ports.ErrNotFound
				}
				wrong := *provider
				wrong.ID = id.NewServiceID()
				return &wrong, nil
			}}
			service.providerService = thirdparty.NewThirdpartyOAuth2ProviderService(repository, operationEncryption{}, operationBranchKeyManager{}, nil, false, service.logger)
			_, _, err := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
			metadata := requireOperationError(t, err, OperationRefresh, DetailConfiguration)
			assert.Equal(t, KindConfiguration, metadata.Kind())
			assert.Equal(t, DependencyProviderRepository, metadata.Dependency())
			if missing {
				assert.ErrorIs(t, err, ports.ErrNotFound)
				assert.ErrorIs(t, err, ErrServiceNotFound)
			} else {
				assert.ErrorIs(t, err, thirdparty.ErrProviderConfiguration)
			}
		})
	}
}
