package oauth2session

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorMetadataBoundsAndClassification(t *testing.T) {
	for _, tc := range []struct {
		detail     ErrorDetail
		kind       ErrorKind
		dependency Dependency
	}{
		{DetailSessionMissing, KindSession, DependencySessionRepository},
		{DetailAccessTokenExpired, KindSession, DependencySessionRepository},
		{DetailRefreshTokenExpired, KindSession, DependencySessionRepository},
		{DetailRefreshUnavailable, KindSession, DependencySessionRepository},
		{DetailRefreshRejected, KindProvider, DependencyProvider},
		{DetailProviderClientRejected, KindConfiguration, DependencyProvider},
		{DetailProviderRejected, KindProvider, DependencyProvider},
		{DetailProviderUnavailable, KindInfrastructure, DependencyProvider},
		{DetailProviderResponseInvalid, KindProvider, DependencyProvider},
		{DetailCredentialSourceUnavailable, KindInfrastructure, DependencyCredentialSource},
		{DetailRepositoryUnavailable, KindInfrastructure, DependencySessionRepository},
		{DetailDecryptionFailed, KindInfrastructure, DependencyEncryption},
		{DetailEncryptionFailed, KindInfrastructure, DependencyEncryption},
		{DetailPersistenceFailed, KindInfrastructure, DependencySessionRepository},
		{DetailConfiguration, KindConfiguration, DependencyNone},
		{DetailCallerCanceled, KindCanceled, DependencyNone},
		{DetailInternalUnclassified, KindInternal, DependencyNone},
	} {
		t.Run(string(tc.detail), func(t *testing.T) {
			metadata := NewErrorMetadata(OperationRefresh, tc.detail)
			assert.Equal(t, OperationRefresh, metadata.Operation())
			assert.Equal(t, tc.detail, metadata.Detail())
			assert.Equal(t, tc.kind, metadata.Kind())
			assert.Equal(t, tc.dependency, metadata.Dependency())
		})
	}
	metadata := NewErrorMetadata(Operation("secret-operation"), ErrorDetail("secret-detail"))
	assert.Equal(t, OperationUnknown, metadata.Operation())
	assert.Equal(t, DetailInternalUnclassified, metadata.Detail())
	assert.Equal(t, DependencyNone, metadata.WithDependency(Dependency("secret-dependency")).Dependency())
	assert.Equal(t, DependencyCredentialSource, metadata.WithDependency(DependencyCredentialSource).Dependency())
	for _, status := range []int{-1, 99, 600, 1000} {
		assert.Zero(t, metadata.WithProviderResponse(status, "secret-code").StatusCode())
	}
	bounded := metadata.WithProviderResponse(http.StatusBadRequest, "invalid_grant")
	assert.Equal(t, http.StatusBadRequest, bounded.StatusCode())
	assert.Equal(t, "invalid_grant", bounded.OAuthCode())
	assert.Equal(t, "unknown", metadata.WithProviderResponse(400, "secret-code").OAuthCode())
	assert.Empty(t, metadata.OAuthCode())
	assert.Zero(t, metadata.StatusCode())
}

func TestOperationErrorPreservesCausesWithoutSerializingThem(t *testing.T) {
	cause := errors.New("sentinel-token-url-header-body")
	rejected := &RefreshRejectedError{StatusCode: 400, OAuthError: "invalid_grant"}
	err := NewOperationError(NewErrorMetadata(OperationRefresh, DetailRefreshRejected), errors.Join(cause, rejected))
	assert.ErrorIs(t, err, cause)
	assert.ErrorIs(t, err, ErrRefreshRejected)
	assert.NotErrorIs(t, err, ErrRefreshTokenExpired)
	var found *RefreshRejectedError
	require.ErrorAs(t, err, &found)
	assert.Same(t, rejected, found)
	assert.NotContains(t, err.Error(), cause.Error())
	assert.NotContains(t, fmt.Sprintf("%+v", err), cause.Error())
}

func TestOperationErrorEnrichmentIsConcurrentAndImmutable(t *testing.T) {
	cause := errors.New("sentinel-cause")
	original := NewOperationError(NewErrorMetadata(OperationRefresh, DetailProviderUnavailable), cause)
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				metadata := original.Metadata().WithProviderResponse(503, "invalid_grant").WithDependency(DependencySigning)
				enriched := original.WithMetadata(metadata)
				assert.Equal(t, 503, enriched.Metadata().StatusCode())
				assert.Equal(t, DependencySigning, enriched.Metadata().Dependency())
				assert.ErrorIs(t, enriched, cause)
				assert.Zero(t, original.Metadata().StatusCode())
				assert.Equal(t, DependencyProvider, original.Metadata().Dependency())
			}
		}()
	}
	workers.Wait()
}

func TestRefreshRejectedErrorDistinguishesTransientStatus(t *testing.T) {
	for _, status := range []int{200, 302, 400, 401, 429, 500, 503, 600} {
		err := &RefreshRejectedError{StatusCode: status, OAuthError: "invalid_grant"}
		if status >= 400 && status <= 499 && status != 429 {
			assert.ErrorIs(t, err, ErrRefreshRejected)
		} else {
			assert.NotErrorIs(t, err, ErrRefreshRejected)
		}
		assert.NotErrorIs(t, err, ErrRefreshTokenExpired)
	}
}
