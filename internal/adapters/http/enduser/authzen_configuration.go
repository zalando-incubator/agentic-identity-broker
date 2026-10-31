package enduser

import (
	"encoding/json"
	"net/http"
	"strings"
)

// AuthZENEvaluationPath is the AuthZEN 1.0 default Access Evaluation path.
const AuthZENEvaluationPath = "/access/v1/evaluation"

// AuthZENConfiguration is the AuthZEN Policy Decision Point metadata document. Unsupported
// endpoints are omitted, which tells consumers that the PDP does not offer them.
type AuthZENConfiguration struct {
	PolicyDecisionPoint      string `json:"policy_decision_point"`
	AccessEvaluationEndpoint string `json:"access_evaluation_endpoint"`
}

// AuthZENConfigurationHandler serves GET /.well-known/authzen-configuration.
type AuthZENConfigurationHandler struct {
	configuration AuthZENConfiguration
}

// NewAuthZENConfigurationHandler derives the PDP identifier from the end-user public URL.
func NewAuthZENConfigurationHandler(publicURL string) *AuthZENConfigurationHandler {
	pdp := strings.TrimRight(publicURL, "/")
	return &AuthZENConfigurationHandler{configuration: AuthZENConfiguration{
		PolicyDecisionPoint:      pdp,
		AccessEvaluationEndpoint: pdp + AuthZENEvaluationPath,
	}}
}

func (h *AuthZENConfigurationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	echoRequestID(w, r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=3600")
	_ = json.NewEncoder(w).Encode(h.configuration)
}

// echoRequestID copies a supplied X-Request-ID to the response (AuthZEN request
// identification). A generated audit ID is never echoed.
func echoRequestID(w http.ResponseWriter, r *http.Request) {
	if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
		w.Header().Set("X-Request-ID", requestID)
	}
}
