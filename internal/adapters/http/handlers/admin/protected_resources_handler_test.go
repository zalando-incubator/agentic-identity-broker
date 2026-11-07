package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func newProtectedResourcesHandler(repo *MockProviderRepository, logger *slog.Logger) *ProtectedResourcesHandler {
	service := thirdparty.NewThirdpartyOAuth2ProviderService(repo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, logger)
	return NewProtectedResourcesHandler(service, logger)
}

func protectedResourceRequest(method, serviceID, escapedResource, body string) *http.Request {
	path := "/api/services/" + serviceID + "/protected-resources/" + escapedResource
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("service-id", serviceID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req.WithContext(principal.WithPrincipal(req.Context(), "admin@example.com"))
}

func protectedResourceCollectionRequest(method, serviceID, body string) *http.Request {
	return protectedResourceRequest(method, serviceID, "", body)
}

func TestProtectedResourcesHandler_MemberResource_DecodesEscapedURIExactlyOnce(t *testing.T) {
	serviceID := id.NewServiceID().String()
	resource := "https://api.example.com/path/to%2Fencoded?query=one%25#fragment"

	decoded, err := (&ProtectedResourcesHandler{}).memberResource(protectedResourceRequest(http.MethodGet, serviceID, url.PathEscape(resource), ""))

	require.NoError(t, err)
	assert.Equal(t, resource, decoded)
}

func TestProtectedResourcesHandler_Add_EncodedMemberAddressingAndAudit(t *testing.T) {
	serviceID := id.NewServiceID().String()
	resource := "https://api.example.com/path/to?query=one#fragment%25"
	escaped := url.PathEscape(resource)

	repo := new(MockProviderRepository)
	repo.On("AddProtectedResource", mock.Anything, mock.Anything, resource).Return(ports.ProtectedResourceMutationResult{
		Resource: resource, ProtectedResources: []string{resource}, Version: 7, Changed: true,
	}, nil)
	var audit bytes.Buffer
	handler := newProtectedResourcesHandler(repo, slog.New(slog.NewJSONHandler(&audit, nil)))
	w := httptest.NewRecorder()
	handler.Add(w, protectedResourceRequest(http.MethodPut, serviceID, escaped, ""))

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, `"7"`, w.Header().Get("ETag"))
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.NotContains(t, w.Body.String(), "client_secret")
	var response protectedResourceMutationResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	assert.Equal(t, resource, response.Resource)
	assert.Equal(t, []string{resource}, response.ProtectedResources)
	assert.Contains(t, audit.String(), `"event":"service.protected_resource.added"`)
	assert.Contains(t, audit.String(), `"resource_uri":"`+resource+`"`)
	assert.NotContains(t, audit.String(), escaped)
	repo.AssertExpectations(t)
}

func TestProtectedResourcesHandler_Create_StatusValidationAndAudit(t *testing.T) {
	serviceID := id.NewServiceID().String()
	resource := "https://api.example.com/resource"
	tests := []struct {
		name       string
		body       string
		configure  func(*MockProviderRepository)
		wantStatus int
		wantETag   string
		wantEvent  string
		wantAudit  string
	}{
		{
			name: "creates normalized body resource",
			body: `{"resource_uri":"https://api.example.com/resource/"}`,
			configure: func(repo *MockProviderRepository) {
				repo.On("AddProtectedResource", mock.Anything, mock.Anything, resource).Return(ports.ProtectedResourceMutationResult{Resource: resource, ProtectedResources: []string{resource}, Version: 8, Changed: true}, nil)
			},
			wantStatus: http.StatusCreated,
			wantETag:   `"8"`,
			wantAudit:  `"resource_uri":"` + resource + `"`,
			wantEvent:  "service.protected_resource.added",
		},
		{
			name: "replay is OK with existing ETag",
			body: `{"resource_uri":"https://api.example.com/resource"}`,
			configure: func(repo *MockProviderRepository) {
				repo.On("AddProtectedResource", mock.Anything, mock.Anything, resource).Return(ports.ProtectedResourceMutationResult{Resource: resource, ProtectedResources: []string{resource}, Version: 8, Changed: false}, nil)
			},
			wantStatus: http.StatusOK,
			wantETag:   `"8"`,
		},
		{
			name:       "rejects malformed JSON body",
			body:       `{"resource_uri":`,
			configure:  func(*MockProviderRepository) {},
			wantStatus: http.StatusBadRequest,
			wantAudit:  `"resource_uri":null`,
			wantEvent:  "service.protected_resource.add_rejected",
		},
		{
			name: "maps cross-service resource ownership conflict",
			body: `{"resource_uri":"https://api.example.com/resource"}`,
			configure: func(repo *MockProviderRepository) {
				repo.On("AddProtectedResource", mock.Anything, mock.Anything, resource).Return(ports.ProtectedResourceMutationResult{}, storage.NewStorageError("add", storage.ErrorKindConflict, errors.New("owned"), "owned"))
			},
			wantStatus: http.StatusConflict,
			wantAudit:  `"resource_uri":"` + resource + `"`,
			wantEvent:  "service.protected_resource.add_rejected",
		},
		{
			name:       "rejects missing body",
			body:       "",
			configure:  func(*MockProviderRepository) {},
			wantStatus: http.StatusBadRequest,
			wantAudit:  `"resource_uri":null`,
			wantEvent:  "service.protected_resource.add_rejected",
		},
		{
			name:       "rejects malformed URI without leaking input",
			body:       `{"resource_uri":"://raw-secret-input"}`,
			configure:  func(*MockProviderRepository) {},
			wantStatus: http.StatusBadRequest,
			wantAudit:  `"resource_uri":null`,
			wantEvent:  "service.protected_resource.add_rejected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			tt.configure(repo)
			var audit bytes.Buffer
			handler := newProtectedResourcesHandler(repo, slog.New(slog.NewJSONHandler(&audit, nil)))
			w := httptest.NewRecorder()
			handler.Create(w, protectedResourceCollectionRequest(http.MethodPost, serviceID, tt.body))

			require.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantETag, w.Header().Get("ETag"))
			if tt.wantAudit == "" {
				assert.Empty(t, audit.String(), "an add that changed nothing must not emit a success audit")
			} else {
				assert.Contains(t, audit.String(), tt.wantAudit)
			}
			if tt.wantEvent != "" {
				assert.Contains(t, audit.String(), `"event":"`+tt.wantEvent+`"`)
			}
			if tt.wantStatus == http.StatusBadRequest {
				assert.NotContains(t, audit.String(), "raw-secret-input")
				repo.AssertNotCalled(t, "AddProtectedResource", mock.Anything, mock.Anything, mock.Anything)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestProtectedResourcesHandler_Remove_EncodedRoutesErrorsAndAudit(t *testing.T) {
	serviceID := id.NewServiceID().String()
	resource := "https://api.example.com/path/to?query=one#fragment%25"
	escaped := url.PathEscape(resource)
	notFound := storage.NewStorageError("remove", storage.ErrorKindNotFound, errors.New("missing"), "missing")

	tests := []struct {
		name       string
		escaped    string
		configure  func(*MockProviderRepository)
		wantStatus int
		wantETag   string
		wantEvent  string
		wantAudit  string
	}{
		{
			name:    "removes decoded resource with mutation body and ETag",
			escaped: escaped,
			configure: func(repo *MockProviderRepository) {
				repo.On("RemoveProtectedResource", mock.Anything, mock.Anything, resource).Return(ports.ProtectedResourceMutationResult{
					Resource: resource, ProtectedResources: []string{}, Version: 11, Changed: true,
				}, nil)
			},
			wantStatus: http.StatusOK,
			wantETag:   `"11"`,
			wantAudit:  `"resource_uri":"` + resource + `"`,
			wantEvent:  "service.protected_resource.removed",
		},
		{
			name:    "maps absent resource to not found",
			escaped: escaped,
			configure: func(repo *MockProviderRepository) {
				repo.On("RemoveProtectedResource", mock.Anything, mock.Anything, resource).Return(ports.ProtectedResourceMutationResult{}, notFound)
			},
			wantStatus: http.StatusNotFound,
			wantAudit:  `"resource_uri":"` + resource + `"`,
			wantEvent:  "service.protected_resource.remove_rejected",
		},
		{
			name:       "rejects malformed escaped resource without raw audit input",
			escaped:    "%25ZZ",
			configure:  func(*MockProviderRepository) {},
			wantStatus: http.StatusBadRequest,
			wantAudit:  `"resource_uri":null`,
			wantEvent:  "service.protected_resource.remove_rejected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			tt.configure(repo)
			var audit bytes.Buffer
			handler := newProtectedResourcesHandler(repo, slog.New(slog.NewJSONHandler(&audit, nil)))
			w := httptest.NewRecorder()
			handler.Remove(w, protectedResourceRequest(http.MethodDelete, serviceID, tt.escaped, ""))

			require.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantETag, w.Header().Get("ETag"))
			assert.Contains(t, audit.String(), tt.wantAudit)
			assert.Contains(t, audit.String(), `"event":"`+tt.wantEvent+`"`)
			if tt.wantStatus == http.StatusOK {
				var response protectedResourceMutationResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
				assert.Equal(t, resource, response.Resource)
				assert.Equal(t, []string{}, response.ProtectedResources)
			}
			if tt.wantStatus == http.StatusBadRequest {
				assert.NotContains(t, audit.String(), "%ZZ")
				repo.AssertNotCalled(t, "RemoveProtectedResource", mock.Anything, mock.Anything, mock.Anything)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestProtectedResourcesHandler_Rename_EncodedRoutesMappingsAndAudit(t *testing.T) {
	serviceID := id.NewServiceID().String()
	from := "https://api.example.com/path/to?query=one#fragment%25"
	to := "https://api.example.com/renamed"
	escaped := url.PathEscape(from)
	conflict := storage.NewStorageError("rename", storage.ErrorKindConflict, errors.New("owned"), "owned")

	tests := []struct {
		name       string
		escaped    string
		body       string
		configure  func(*MockProviderRepository)
		wantStatus int
		wantETag   string
		wantEvent  string
		wantSource string
		wantTarget string
	}{
		{
			name:    "renames decoded source and normalized target",
			escaped: escaped,
			body:    `{"to":"https://api.example.com/renamed/"}`,
			configure: func(repo *MockProviderRepository) {
				repo.On("RenameProtectedResource", mock.Anything, mock.Anything, from, to).Return(ports.ProtectedResourceMutationResult{Resource: to, ProtectedResources: []string{to}, Version: 12, Changed: true}, nil)
			},
			wantStatus: http.StatusOK,
			wantETag:   `"12"`,
			wantSource: `"source_resource_uri":"` + from + `"`,
			wantTarget: `"target_resource_uri":"` + to + `"`,
			wantEvent:  "service.protected_resource.renamed",
		},
		{
			name:    "unchanged rename emits no success audit",
			escaped: url.PathEscape(to),
			body:    `{"to":"https://api.example.com/renamed"}`,
			configure: func(repo *MockProviderRepository) {
				repo.On("RenameProtectedResource", mock.Anything, mock.Anything, to, to).Return(ports.ProtectedResourceMutationResult{Resource: to, ProtectedResources: []string{to}, Version: 12, Changed: false}, nil)
			},
			wantStatus: http.StatusOK,
			wantETag:   `"12"`,
		},
		{
			name:    "maps target conflict",
			escaped: escaped,
			body:    `{"to":"https://api.example.com/renamed"}`,
			configure: func(repo *MockProviderRepository) {
				repo.On("RenameProtectedResource", mock.Anything, mock.Anything, from, to).Return(ports.ProtectedResourceMutationResult{}, conflict)
			},
			wantStatus: http.StatusConflict,
			wantSource: `"source_resource_uri":"` + from + `"`,
			wantTarget: `"target_resource_uri":"` + to + `"`,
			wantEvent:  "service.protected_resource.rename_rejected",
		},
		{
			name:    "maps missing source to not found",
			escaped: escaped,
			body:    `{"to":"https://api.example.com/renamed"}`,
			configure: func(repo *MockProviderRepository) {
				repo.On("RenameProtectedResource", mock.Anything, mock.Anything, from, to).Return(ports.ProtectedResourceMutationResult{}, storage.NewStorageError("rename", storage.ErrorKindNotFound, errors.New("missing"), "missing"))
			},
			wantStatus: http.StatusNotFound,
			wantSource: `"source_resource_uri":"` + from + `"`,
			wantTarget: `"target_resource_uri":"` + to + `"`,
			wantEvent:  "service.protected_resource.rename_rejected",
		},
		{
			name:       "rejects malformed JSON with independently normalized source",
			escaped:    escaped,
			body:       `{"to":`,
			configure:  func(*MockProviderRepository) {},
			wantStatus: http.StatusBadRequest,
			wantSource: `"source_resource_uri":"` + from + `"`,
			wantTarget: `"target_resource_uri":null`,
			wantEvent:  "service.protected_resource.rename_rejected",
		},
		{
			name:       "rejects malformed escaped source without raw audit input",
			escaped:    "%25ZZ",
			body:       `{"to":"https://api.example.com/renamed"}`,
			configure:  func(*MockProviderRepository) {},
			wantStatus: http.StatusBadRequest,
			wantSource: `"source_resource_uri":null`,
			wantTarget: `"target_resource_uri":"` + to + `"`,
			wantEvent:  "service.protected_resource.rename_rejected",
		},
		{
			name:       "rejects malformed target without raw audit input",
			escaped:    escaped,
			body:       `{"to":"://raw-target-input"}`,
			configure:  func(*MockProviderRepository) {},
			wantStatus: http.StatusBadRequest,
			wantSource: `"source_resource_uri":"` + from + `"`,
			wantTarget: `"target_resource_uri":null`,
			wantEvent:  "service.protected_resource.rename_rejected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			tt.configure(repo)
			var audit bytes.Buffer
			handler := newProtectedResourcesHandler(repo, slog.New(slog.NewJSONHandler(&audit, nil)))
			w := httptest.NewRecorder()
			handler.Rename(w, protectedResourceRequest(http.MethodPatch, serviceID, tt.escaped, tt.body))

			require.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantETag, w.Header().Get("ETag"))
			if tt.wantSource == "" && tt.wantTarget == "" {
				assert.Empty(t, audit.String(), "a rename that changed nothing must not emit a success audit")
			} else {
				assert.Contains(t, audit.String(), tt.wantSource)
				assert.Contains(t, audit.String(), tt.wantTarget)
			}
			if tt.wantEvent != "" {
				assert.Contains(t, audit.String(), `"event":"`+tt.wantEvent+`"`)
			}
			if tt.wantStatus == http.StatusBadRequest {
				assert.NotContains(t, audit.String(), "raw-target-input")
				assert.NotContains(t, audit.String(), "%ZZ")
				repo.AssertNotCalled(t, "RenameProtectedResource", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestProtectedResourcesHandler_RejectedMutationsAuditRawRoutedServiceID(t *testing.T) {
	const unresolvedServiceID = "unresolved-service-id"
	validServiceID := id.NewServiceID().String()
	resource := "https://api.example.com/resource"
	escaped := url.PathEscape(resource)
	notFound := storage.NewStorageError("GetByCanonicalID", storage.ErrorKindNotFound, errors.New("missing"), "missing")
	tests := []struct {
		name             string
		handle           func(*ProtectedResourcesHandler, http.ResponseWriter, *http.Request)
		request          *http.Request
		resolveServiceID bool
		wantStatus       int
		wantAudit        []string
		mustNotLog       []string
	}{
		{
			name:       "add malformed JSON includes routed service ID",
			handle:     (*ProtectedResourcesHandler).Create,
			request:    protectedResourceCollectionRequest(http.MethodPost, unresolvedServiceID, `{"resource_uri":`),
			wantStatus: http.StatusBadRequest,
			wantAudit: []string{
				`"event":"service.protected_resource.add_rejected"`, `"operation":"add"`, `"service_id":"` + unresolvedServiceID + `"`, `"outcome":"rejected"`, `"resource_uri":null`,
			},
		},
		{
			name:             "add unresolved service ID retains only normalized resource",
			handle:           (*ProtectedResourcesHandler).Add,
			request:          protectedResourceRequest(http.MethodPut, unresolvedServiceID, escaped, ""),
			resolveServiceID: true,
			wantStatus:       http.StatusNotFound,
			wantAudit: []string{
				`"event":"service.protected_resource.add_rejected"`, `"operation":"add"`, `"service_id":"` + unresolvedServiceID + `"`, `"resource_uri":"` + resource + `"`,
			},
		},
		{
			name:             "remove unresolved service ID retains only normalized resource",
			handle:           (*ProtectedResourcesHandler).Remove,
			request:          protectedResourceRequest(http.MethodDelete, unresolvedServiceID, escaped, ""),
			resolveServiceID: true,
			wantStatus:       http.StatusNotFound,
			wantAudit: []string{
				`"event":"service.protected_resource.remove_rejected"`, `"operation":"remove"`, `"service_id":"` + unresolvedServiceID + `"`, `"resource_uri":"` + resource + `"`,
			},
		},
		{
			name:             "rename unresolved service ID retains independently normalized URIs",
			handle:           (*ProtectedResourcesHandler).Rename,
			request:          protectedResourceRequest(http.MethodPatch, unresolvedServiceID, escaped, `{"to":"https://api.example.com/renamed/"}`),
			resolveServiceID: true,
			wantStatus:       http.StatusNotFound,
			wantAudit: []string{
				`"event":"service.protected_resource.rename_rejected"`, `"operation":"rename"`, `"service_id":"` + unresolvedServiceID + `"`, `"source_resource_uri":"` + resource + `"`, `"target_resource_uri":"https://api.example.com/renamed"`,
			},
		},
		{
			name:       "rename malformed target does not leak raw URI",
			handle:     (*ProtectedResourcesHandler).Rename,
			request:    protectedResourceRequest(http.MethodPatch, validServiceID, escaped, `{"to":"://raw-secret-target"}`),
			wantStatus: http.StatusBadRequest,
			wantAudit: []string{
				`"event":"service.protected_resource.rename_rejected"`, `"operation":"rename"`, `"service_id":"` + validServiceID + `"`, `"source_resource_uri":"` + resource + `"`, `"target_resource_uri":null`,
			},
			mustNotLog: []string{"raw-secret-target"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var audit bytes.Buffer
			repo := new(MockProviderRepository)
			if tt.resolveServiceID {
				repo.On("GetByCanonicalID", mock.Anything, unresolvedServiceID).Return(nil, notFound)
			}
			handler := newProtectedResourcesHandler(repo, slog.New(slog.NewJSONHandler(&audit, nil)))
			w := httptest.NewRecorder()
			tt.handle(handler, w, tt.request)

			require.Equal(t, tt.wantStatus, w.Code)
			for _, expected := range tt.wantAudit {
				assert.Contains(t, audit.String(), expected)
			}
			for _, forbidden := range tt.mustNotLog {
				assert.NotContains(t, audit.String(), forbidden)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestProtectedResourcesHandler_List_ResponseAndMissingService(t *testing.T) {
	serviceID := id.NewServiceID().String()
	notFound := storage.NewStorageError("list", storage.ErrorKindNotFound, errors.New("missing"), "missing")
	tests := []struct {
		name       string
		configure  func(*MockProviderRepository)
		wantStatus int
		wantETag   string
	}{
		{
			name: "returns normalized resource set and version",
			configure: func(repo *MockProviderRepository) {
				repo.On("ListProtectedResources", mock.Anything, mock.Anything).Return([]string{"https://api.example.com/a", "https://api.example.com/b"}, int64(13), nil)
			},
			wantStatus: http.StatusOK,
			wantETag:   `"13"`,
		},
		{
			name: "maps missing service",
			configure: func(repo *MockProviderRepository) {
				repo.On("ListProtectedResources", mock.Anything, mock.Anything).Return([]string(nil), int64(0), notFound)
			},
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			tt.configure(repo)
			handler := newProtectedResourcesHandler(repo, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
			w := httptest.NewRecorder()
			handler.List(w, protectedResourceCollectionRequest(http.MethodGet, serviceID, ""))

			require.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantETag, w.Header().Get("ETag"))
			if tt.wantStatus == http.StatusOK {
				var response protectedResourceSetResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
				assert.Equal(t, []string{"https://api.example.com/a", "https://api.example.com/b"}, response.ProtectedResources)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestProtectedResourcesHandler_ResolvesCanonicalServiceIDForAllOperations(t *testing.T) {
	canonicalID := "canonical-service"
	serviceID := id.NewServiceID()
	entity := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID}
	resource := "https://api.example.com/resource"
	escapedResource := url.PathEscape(resource)

	tests := []struct {
		name       string
		wantStatus int
		handle     func(*ProtectedResourcesHandler, http.ResponseWriter, *http.Request)
		request    func() *http.Request
		configure  func(*MockProviderRepository)
	}{
		{
			name:       "lists protected resources",
			wantStatus: http.StatusOK,
			handle:     (*ProtectedResourcesHandler).List,
			request: func() *http.Request {
				return protectedResourceCollectionRequest(http.MethodGet, canonicalID, "")
			},
			configure: func(repo *MockProviderRepository) {
				repo.On("ListProtectedResources", mock.Anything, serviceID).Return([]string{resource}, int64(1), nil)
			},
		},
		{
			name:       "creates a protected resource",
			wantStatus: http.StatusCreated,
			handle:     (*ProtectedResourcesHandler).Create,
			request: func() *http.Request {
				return protectedResourceCollectionRequest(http.MethodPost, canonicalID, `{"resource_uri":"`+resource+`"}`)
			},
			configure: func(repo *MockProviderRepository) {
				repo.On("AddProtectedResource", mock.Anything, serviceID, resource).Return(ports.ProtectedResourceMutationResult{Resource: resource, Version: 1, Changed: true}, nil)
			},
		},
		{
			name:       "adds a protected resource",
			wantStatus: http.StatusCreated,
			handle:     (*ProtectedResourcesHandler).Add,
			request: func() *http.Request {
				return protectedResourceRequest(http.MethodPut, canonicalID, escapedResource, "")
			},
			configure: func(repo *MockProviderRepository) {
				repo.On("AddProtectedResource", mock.Anything, serviceID, resource).Return(ports.ProtectedResourceMutationResult{Resource: resource, Version: 1, Changed: true}, nil)
			},
		},
		{
			name:       "renames a protected resource",
			wantStatus: http.StatusOK,
			handle:     (*ProtectedResourcesHandler).Rename,
			request: func() *http.Request {
				return protectedResourceRequest(http.MethodPatch, canonicalID, escapedResource, `{"to":"https://api.example.com/renamed"}`)
			},
			configure: func(repo *MockProviderRepository) {
				repo.On("RenameProtectedResource", mock.Anything, serviceID, resource, "https://api.example.com/renamed").Return(ports.ProtectedResourceMutationResult{Resource: "https://api.example.com/renamed", Version: 1, Changed: true}, nil)
			},
		},
		{
			name:       "removes a protected resource",
			wantStatus: http.StatusOK,
			handle:     (*ProtectedResourcesHandler).Remove,
			request: func() *http.Request {
				return protectedResourceRequest(http.MethodDelete, canonicalID, escapedResource, "")
			},
			configure: func(repo *MockProviderRepository) {
				repo.On("RemoveProtectedResource", mock.Anything, serviceID, resource).Return(ports.ProtectedResourceMutationResult{Resource: resource, Version: 1, Changed: true}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockProviderRepository)
			repo.On("GetByCanonicalID", mock.Anything, canonicalID).Return(entity, nil).Once()
			tt.configure(repo)
			handler := newProtectedResourcesHandler(repo, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
			w := httptest.NewRecorder()

			tt.handle(handler, w, tt.request())

			require.Equal(t, tt.wantStatus, w.Code)
			repo.AssertExpectations(t)
		})
	}
}

func TestProtectedResourcesHandler_RejectsUnresolvedCanonicalServiceID(t *testing.T) {
	const canonicalID = "missing-service"
	repo := new(MockProviderRepository)
	repo.On("GetByCanonicalID", mock.Anything, canonicalID).Return(nil, storage.NewStorageError("GetByCanonicalID", storage.ErrorKindNotFound, errors.New("missing"), "missing")).Once()
	handler := newProtectedResourcesHandler(repo, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	w := httptest.NewRecorder()

	handler.List(w, protectedResourceCollectionRequest(http.MethodGet, canonicalID, ""))

	require.Equal(t, http.StatusNotFound, w.Code)
	repo.AssertExpectations(t)
}
