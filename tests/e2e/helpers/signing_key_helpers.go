package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

// ProvisionSigningKey ensures a current signing key exists and is active.
// When startup already auto-generated a usable key, this is a no-op. Otherwise
// it promotes the current key (or creates one) so local token issuance works
// immediately in tests that are not validating startup readiness.
func ProvisionSigningKey(adminBaseURL string) error {
	resp, err := doAdminRequest(http.MethodGet, adminBaseURL+"/api/oauth2-server/signing-keys", nil, "")
	if err != nil {
		return fmt.Errorf("failed to list signing keys: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("list signing keys returned %d", resp.StatusCode)
	}

	var list struct {
		Items []struct {
			KID         string `json:"kid"`
			IsCurrent   bool   `json:"is_current"`
			ActivatesAt string `json:"activates_at"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return fmt.Errorf("failed to decode signing key list: %w", err)
	}

	var promotableKID string
	for _, item := range list.Items {
		if promotableKID == "" {
			promotableKID = item.KID
		}
		if !item.IsCurrent {
			continue
		}
		activatesAt, err := time.Parse(time.RFC3339, item.ActivatesAt)
		if err != nil {
			return fmt.Errorf("failed to parse activates_at %q: %w", item.ActivatesAt, err)
		}
		if !activatesAt.After(time.Now()) {
			return nil
		}
		return promoteSigningKey(adminBaseURL, item.KID)
	}

	if promotableKID != "" {
		return promoteSigningKey(adminBaseURL, promotableKID)
	}

	createResp, err := doAdminRequest(http.MethodPost, adminBaseURL+"/api/oauth2-server/signing-keys", nil, "application/json")
	if err != nil {
		return fmt.Errorf("failed to provision signing key: %w", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	if createResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("provision signing key returned %d", createResp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(createResp.Body).Decode(&body); err != nil {
		return fmt.Errorf("failed to decode signing key response: %w", err)
	}
	kid, ok := body["kid"].(string)
	if !ok || kid == "" {
		return fmt.Errorf("signing key response missing kid")
	}

	return promoteSigningKey(adminBaseURL, kid)
}

func promoteSigningKey(adminBaseURL, kid string) error {
	promResp, err := doAdminRequest(
		http.MethodPut,
		adminBaseURL+"/api/oauth2-server/signing-keys/"+kid+"/current",
		strings.NewReader(""),
		"application/json",
	)
	if err != nil {
		return fmt.Errorf("failed to promote signing key: %w", err)
	}
	defer func() { _ = promResp.Body.Close() }()
	if promResp.StatusCode != http.StatusOK {
		return fmt.Errorf("promote signing key returned %d", promResp.StatusCode)
	}
	return nil
}

func doAdminRequest(method, url string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Remote-User", fixtures.AdminPrincipal().String())
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return HTTPClient().Do(req)
}
