package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// serviceAccountMaxBodyBytes fits the largest valid annotation map sent as \u00XX escapes
// beside a 16 KiB JWT.
const serviceAccountMaxBodyBytes = 512 << 10

// ServiceAccountsHandler serves the service accounts of a third-party service. It parses input
// and maps results; every rule lives in thirdparty.ServiceAccountService.
type ServiceAccountsHandler struct {
	accounts *thirdparty.ServiceAccountService
	services *thirdparty.ThirdpartyOAuth2ProviderService
	logger   *slog.Logger
}

// NewServiceAccountsHandler creates the service-account handler.
func NewServiceAccountsHandler(accounts *thirdparty.ServiceAccountService, services *thirdparty.ThirdpartyOAuth2ProviderService, logger *slog.Logger) *ServiceAccountsHandler {
	if logger == nil {
		panic("admin.NewServiceAccountsHandler: logger must not be nil")
	}
	return &ServiceAccountsHandler{accounts: accounts, services: services, logger: logger}
}

type serviceAccountRequest struct {
	CanonicalID json.RawMessage `json:"canonical_id"`
	Kind        json.RawMessage `json:"kind"`
	Scopes      json.RawMessage `json:"scopes"`
	Credential  json.RawMessage `json:"credential"`
	Metadata    json.RawMessage `json:"metadata"`
}

type serviceAccountCredentialRequest struct {
	Format json.RawMessage `json:"format"`
	Use    json.RawMessage `json:"use"`
	JWT    json.RawMessage `json:"jwt"`
}

type serviceAccountResponse struct {
	ID                string                  `json:"id"`
	ServiceID         string                  `json:"service_id"`
	CanonicalID       string                  `json:"canonical_id"`
	Kind              string                  `json:"kind"`
	Scopes            []string                `json:"scopes"`
	Narrowing         string                  `json:"narrowing"`
	CredentialSummary model.CredentialSummary `json:"credential_summary"`
	CreatedAt         string                  `json:"created_at"`
	UpdatedAt         string                  `json:"updated_at"`
	Metadata          *ObjectMetadataResponse `json:"metadata,omitempty"`
}

type serviceAccountStatusResponse struct {
	Status              string  `json:"status"`
	FailureReason       *string `json:"failure_reason"`
	CredentialExpiresAt *string `json:"credential_expires_at"`
}

// errInvalidBody marks a body that is not a JSON object of the known members.
var errInvalidBody = errors.New("invalid request body")

// errNullMember carries the constant message for a member stated as null.
type errNullMember struct{ member string }

func (e errNullMember) Error() string { return e.member + " must not be null" }

// Create handles POST /api/services/{service-id}/service-accounts.
func (h *ServiceAccountsHandler) Create(w http.ResponseWriter, r *http.Request) {
	operator, ok := h.operator(w, r)
	if !ok {
		return
	}
	serviceID, ok := h.service(w, r)
	if !ok {
		return
	}
	body, request, credential, ok := h.decode(w, r)
	if !ok {
		return
	}
	create := thirdparty.ServiceAccountCreate{Credential: credential}
	if !h.assign(w, request.CanonicalID, "canonical_id", &create.CanonicalID) ||
		!h.assign(w, request.Kind, "kind", &create.Kind) ||
		!h.assign(w, request.Scopes, "scopes", &create.Scopes) {
		return
	}
	metadata, _, ok := h.requestMetadata(w, body)
	if !ok {
		return
	}
	create.Metadata = metadata
	view, err := h.accounts.Create(r.Context(), operator, serviceID, create)
	if err != nil {
		h.domainError(w, err, true)
		return
	}
	w.Header().Set("Location", "/api/services/"+serviceID.String()+"/service-accounts/"+view.Account.ID.String())
	h.writeAccount(w, http.StatusCreated, view)
}

// List handles GET /api/services/{service-id}/service-accounts.
func (h *ServiceAccountsHandler) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.operator(w, r); !ok {
		return
	}
	serviceID, ok := h.service(w, r)
	if !ok {
		return
	}
	views, err := h.accounts.List(r.Context(), serviceID)
	if err != nil {
		h.readError(w, r, err, true)
		return
	}
	responses := make([]serviceAccountResponse, len(views))
	for i := range views {
		responses[i] = toServiceAccountResponse(&views[i])
	}
	h.writeJSON(w, http.StatusOK, responses)
}

// Get handles GET /api/services/{service-id}/service-accounts/{service-account-id}.
func (h *ServiceAccountsHandler) Get(w http.ResponseWriter, r *http.Request) {
	serviceID, accountID, ok := h.address(w, r)
	if !ok {
		return
	}
	view, err := h.accounts.Get(r.Context(), serviceID, accountID)
	if err != nil {
		h.readError(w, r, err, false)
		return
	}
	h.writeAccount(w, http.StatusOK, view)
}

// Update handles PUT /api/services/{service-id}/service-accounts/{service-account-id}.
func (h *ServiceAccountsHandler) Update(w http.ResponseWriter, r *http.Request) {
	operator, ok := h.operator(w, r)
	if !ok {
		return
	}
	serviceID, accountID, ok := h.resolve(w, r)
	if !ok {
		return
	}
	expectedVersion, ok := h.ifMatch(w, r)
	if !ok {
		return
	}
	body, request, credential, ok := h.decode(w, r)
	if !ok {
		return
	}
	update := thirdparty.ServiceAccountUpdate{Credential: credential, ExpectedVersion: expectedVersion}
	if request.CanonicalID != nil {
		update.CanonicalID = new(string)
		if !h.assign(w, request.CanonicalID, "canonical_id", update.CanonicalID) {
			return
		}
	}
	if request.Kind != nil {
		update.Kind = new(model.ServiceAccountKind)
		if !h.assign(w, request.Kind, "kind", update.Kind) {
			return
		}
	}
	if !h.assign(w, request.Scopes, "scopes", &update.Scopes) {
		return
	}
	metadata, present, ok := h.requestMetadata(w, body)
	if !ok {
		return
	}
	if present {
		update.Metadata = &metadata
	}
	view, _, err := h.accounts.Update(r.Context(), operator, serviceID, accountID, update)
	if err != nil {
		h.domainError(w, err, false)
		return
	}
	h.writeAccount(w, http.StatusOK, view)
}

// Delete handles DELETE /api/services/{service-id}/service-accounts/{service-account-id}.
func (h *ServiceAccountsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	operator, ok := h.operator(w, r)
	if !ok {
		return
	}
	serviceID, accountID, ok := h.resolve(w, r)
	if !ok {
		return
	}
	expectedVersion, ok := h.ifMatch(w, r)
	if !ok {
		return
	}
	if err := h.accounts.Delete(r.Context(), operator, serviceID, accountID, expectedVersion); err != nil {
		h.domainError(w, err, false)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Status handles GET /api/services/{service-id}/service-accounts/{service-account-id}/status.
func (h *ServiceAccountsHandler) Status(w http.ResponseWriter, r *http.Request) {
	serviceID, accountID, ok := h.address(w, r)
	if !ok {
		return
	}
	view, err := h.accounts.Status(r.Context(), serviceID, accountID)
	if err != nil {
		h.readError(w, r, err, false)
		return
	}
	response := serviceAccountStatusResponse{Status: string(view.Status)}
	if view.FailureReason != nil {
		response.FailureReason = new(string(*view.FailureReason))
	}
	if expiry := view.Account.CredentialExpiresAt; expiry != nil {
		response.CredentialExpiresAt = new(expiry.UTC().Format(time.RFC3339))
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *ServiceAccountsHandler) operator(w http.ResponseWriter, r *http.Request) (id.Principal, bool) {
	operator, ok := operatorPrincipalFromContext(r.Context(), h.logger)
	if !ok {
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	}
	return operator, ok
}

func (h *ServiceAccountsHandler) service(w http.ResponseWriter, r *http.Request) (id.ServiceID, bool) {
	serviceID, err := h.services.ResolveID(r.Context(), chi.URLParam(r, "service-id"))
	if err != nil {
		h.readError(w, r, err, true)
		return id.ServiceID{}, false
	}
	return serviceID, true
}

// address authenticates the operator and resolves the service and the account.
func (h *ServiceAccountsHandler) address(w http.ResponseWriter, r *http.Request) (id.ServiceID, id.ServiceAccountID, bool) {
	if _, ok := h.operator(w, r); !ok {
		return id.ServiceID{}, id.ServiceAccountID{}, false
	}
	return h.resolve(w, r)
}

func (h *ServiceAccountsHandler) resolve(w http.ResponseWriter, r *http.Request) (id.ServiceID, id.ServiceAccountID, bool) {
	serviceID, ok := h.service(w, r)
	if !ok {
		return id.ServiceID{}, id.ServiceAccountID{}, false
	}
	accountID, err := h.accounts.Resolve(r.Context(), serviceID, chi.URLParam(r, "service-account-id"))
	if err != nil {
		h.readError(w, r, err, false)
		return id.ServiceID{}, id.ServiceAccountID{}, false
	}
	return serviceID, accountID, true
}

func (h *ServiceAccountsHandler) ifMatch(w http.ResponseWriter, r *http.Request) (*int64, bool) {
	values := r.Header.Values("If-Match")
	if len(values) == 0 {
		return nil, true
	}
	if len(values) == 1 {
		if version, err := parseStrongETag(values[0]); err == nil {
			return &version, true
		}
	}
	h.writeError(w, http.StatusBadRequest, "invalid If-Match", "If-Match must be one strong ETag")
	return nil, false
}

// decode reads the request body strictly: at most 512 KiB, one JSON object, known members only.
// It returns the body so that metadata can be decoded from the same bytes.
func (h *ServiceAccountsHandler) decode(w http.ResponseWriter, r *http.Request) ([]byte, serviceAccountRequest, *model.ServiceAccountCredentialInput, bool) {
	var request serviceAccountRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, serviceAccountMaxBodyBytes))
	if err == nil {
		err = strictUnmarshal(body, &request)
	}
	if err != nil {
		h.writeError(w, http.StatusBadRequest, errInvalidBody.Error(), invalidServiceAccountBody)
		return nil, request, nil, false
	}
	if request.Credential == nil {
		return body, request, nil, true
	}
	if isJSONNull(request.Credential) {
		h.writeError(w, http.StatusBadRequest, "validation failed", errNullMember{"credential"}.Error())
		return nil, request, nil, false
	}
	var credential serviceAccountCredentialRequest
	if err := strictUnmarshal(request.Credential, &credential); err != nil {
		h.writeError(w, http.StatusBadRequest, errInvalidBody.Error(), invalidServiceAccountBody)
		return nil, request, nil, false
	}
	input := &model.ServiceAccountCredentialInput{}
	var jwt string
	if !h.assign(w, credential.Format, "credential.format", &input.Format) ||
		!h.assign(w, credential.Use, "credential.use", &input.Use) ||
		!h.assign(w, credential.JWT, "credential.jwt", &jwt) {
		return nil, request, nil, false
	}
	input.JWT = model.NewPlaintextSecret(jwt)
	return body, request, input, true
}

const invalidServiceAccountBody = "request body must be one JSON object of at most 512 KiB with known members"

// requestMetadata decodes the metadata member. Its errors are fixed messages that never carry
// request content, and the request log already records the refusal.
func (h *ServiceAccountsHandler) requestMetadata(w http.ResponseWriter, body []byte) (model.ObjectMetadata, bool, bool) {
	metadata, present, err := decodeRequestMetadata(body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return model.ObjectMetadata{}, false, false
	}
	return metadata, present, true
}

func strictUnmarshal(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errInvalidBody
	}
	if decoder.More() {
		return errInvalidBody
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errInvalidBody
	}
	return nil
}

// assign decodes one present member into target. An absent member leaves target unchanged; a
// null member is refused.
func (h *ServiceAccountsHandler) assign(w http.ResponseWriter, raw json.RawMessage, member string, target any) bool {
	if raw == nil {
		return true
	}
	if isJSONNull(raw) {
		h.writeError(w, http.StatusBadRequest, "validation failed", errNullMember{member}.Error())
		return false
	}
	if err := json.Unmarshal(raw, target); err != nil {
		h.writeError(w, http.StatusBadRequest, errInvalidBody.Error(), invalidServiceAccountBody)
		return false
	}
	return true
}

// readError answers a failed read. The domain does not log reads, so an internal failure is
// logged here, by storage error kind only: a driver cause can carry stored values.
func (h *ServiceAccountsHandler) readError(w http.ResponseWriter, r *http.Request, err error, serviceScope bool) {
	if h.domainError(w, err, serviceScope) {
		kind := storage.ErrorKindUnknown
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) {
			kind = storageErr.Kind
		}
		h.logger.ErrorContext(r.Context(), "service account read failed", "error_kind", string(kind))
	}
}

// domainError maps a domain result to the error envelope and reports whether it answered 500.
// Messages are constants or the domain's constant validation messages; none contains request
// content. The domain logs write failures; the request log carries the status of every other
// failure.
func (h *ServiceAccountsHandler) domainError(w http.ResponseWriter, err error, serviceScope bool) (internal bool) {
	var credentialErr *model.CredentialValidationError
	var entityErr *model.ServiceAccountValidationError
	switch {
	case errors.As(err, &credentialErr), errors.As(err, &entityErr):
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
	case errors.Is(err, ports.ErrServiceAccountVersionMismatch):
		h.writeError(w, http.StatusPreconditionFailed, "precondition failed", "service account was changed; read it again and retry")
	case errors.Is(err, ports.ErrServiceAccountCanonicalIDConflict):
		h.writeError(w, http.StatusConflict, "conflict", ports.ErrServiceAccountCanonicalIDConflict.Error())
	case errors.Is(err, thirdparty.ErrServiceAccountBound):
		h.writeError(w, http.StatusConflict, "conflict", thirdparty.ErrServiceAccountBound.Error())
	case ports.IsNotFoundErr(err) && serviceScope:
		h.writeError(w, http.StatusNotFound, "service not found", "service not found")
	case ports.IsNotFoundErr(err):
		h.writeError(w, http.StatusNotFound, "service account not found", "service account not found")
	default:
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return true
	}
	return false
}

func toServiceAccountResponse(view *thirdparty.ServiceAccountView) serviceAccountResponse {
	account := view.Account
	response := serviceAccountResponse{
		ID: account.ID.String(), ServiceID: account.ServiceID.String(), CanonicalID: account.CanonicalID,
		Kind: string(account.Kind), Scopes: account.Scopes, Narrowing: string(view.Narrowing),
		CredentialSummary: account.Summary,
		CreatedAt:         account.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         account.UpdatedAt.UTC().Format(time.RFC3339),
		Metadata:          objectMetadataResponse(account.ObjectMetadata),
	}
	return response
}

func (h *ServiceAccountsHandler) writeAccount(w http.ResponseWriter, status int, view *thirdparty.ServiceAccountView) {
	w.Header().Set("ETag", strongETag(view.Account.Version))
	h.writeJSON(w, status, toServiceAccountResponse(view))
}

func (h *ServiceAccountsHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *ServiceAccountsHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, ErrorResponse{Error: code, Message: message})
}
