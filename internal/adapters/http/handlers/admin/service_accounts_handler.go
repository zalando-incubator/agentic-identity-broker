package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
)

// ServiceAccountsHandler serves the service accounts of a third-party service.
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

func (h *ServiceAccountsHandler) notImplemented(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "not implemented"})
}

// Create handles POST /api/services/{service-id}/service-accounts.
func (h *ServiceAccountsHandler) Create(w http.ResponseWriter, _ *http.Request) { h.notImplemented(w) }

// List handles GET /api/services/{service-id}/service-accounts.
func (h *ServiceAccountsHandler) List(w http.ResponseWriter, _ *http.Request) { h.notImplemented(w) }

// Get handles GET /api/services/{service-id}/service-accounts/{service-account-id}.
func (h *ServiceAccountsHandler) Get(w http.ResponseWriter, _ *http.Request) { h.notImplemented(w) }

// Update handles PUT /api/services/{service-id}/service-accounts/{service-account-id}.
func (h *ServiceAccountsHandler) Update(w http.ResponseWriter, _ *http.Request) { h.notImplemented(w) }

// Delete handles DELETE /api/services/{service-id}/service-accounts/{service-account-id}.
func (h *ServiceAccountsHandler) Delete(w http.ResponseWriter, _ *http.Request) { h.notImplemented(w) }

// Status handles GET /api/services/{service-id}/service-accounts/{service-account-id}/status.
func (h *ServiceAccountsHandler) Status(w http.ResponseWriter, _ *http.Request) { h.notImplemented(w) }
