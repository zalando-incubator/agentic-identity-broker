package helpers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// AuthZEN request values used by the broker's Access Evaluation endpoint.
const (
	AuthZENSubjectTypeJWT        = "urn:ietf:params:oauth:token-type:jwt" // #nosec G101 -- RFC-defined token type URI, not a credential.
	AuthZENActionTokenExchange   = "urn:ietf:params:oauth:grant-type:token-exchange"
	AuthZENResourceTypeProtected = "protected_resource"
	AuthZENEvaluationPath        = "/access/v1/evaluation"
	AuthZENConfigurationPath     = "/.well-known/authzen-configuration"
)

// AuthZENEvaluationBody builds an Access Evaluation request. A nil context omits the member.
func AuthZENEvaluationBody(subjectToken, resourceURI string, context map[string]any) []byte {
	body := map[string]any{
		"subject":  map[string]any{"type": AuthZENSubjectTypeJWT, "id": subjectToken},
		"action":   map[string]any{"name": AuthZENActionTokenExchange},
		"resource": map[string]any{"type": AuthZENResourceTypeProtected, "id": resourceURI},
	}
	if context != nil {
		body["context"] = context
	}
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return raw
}

// AuthZENResponse is a decoded evaluation or discovery response.
type AuthZENResponse struct {
	StatusCode int
	Header     http.Header
	Raw        string
	Body       map[string]any
}

// DoAuthZENEvaluation posts body with the bearer assertion and optional request ID.
func DoAuthZENEvaluation(client *http.Client, baseURL, assertion, requestID string, body []byte) (*AuthZENResponse, error) {
	req, err := http.NewRequest(http.MethodPost, baseURL+AuthZENEvaluationPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if assertion != "" {
		req.Header.Set("Authorization", "Bearer "+assertion)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	return doAuthZEN(client, req)
}

// GetAuthZENConfiguration reads the PDP metadata document.
func GetAuthZENConfiguration(client *http.Client, baseURL string) (*AuthZENResponse, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+AuthZENConfigurationPath, nil)
	if err != nil {
		return nil, err
	}
	return doAuthZEN(client, req)
}

func doAuthZEN(client *http.Client, req *http.Request) (*AuthZENResponse, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	result := &AuthZENResponse{StatusCode: resp.StatusCode, Header: resp.Header, Raw: string(raw)}
	_ = json.Unmarshal(raw, &result.Body)
	return result, nil
}
