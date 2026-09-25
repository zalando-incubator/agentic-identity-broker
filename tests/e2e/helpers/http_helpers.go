// Package helpers provides HTTP utilities for E2E testing.
// These utilities are HTTP contract focused and stable across refactoring.
package helpers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var boundedHTTPClient = &http.Client{Timeout: 10 * time.Second}

// HTTPClient returns the shared bounded client for E2E requests.
func HTTPClient() *http.Client {
	return boundedHTTPClient
}

// ExtractRedirectURL extracts the Location header from an HTTP response.
// Returns an error if the response doesn't have a Location header.
// This is stable because it only depends on HTTP response contract.
func ExtractRedirectURL(resp *http.Response) (*url.URL, error) {
	if resp == nil {
		return nil, fmt.Errorf("response is nil")
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return nil, fmt.Errorf("response has no Location header (status: %d)", resp.StatusCode)
	}

	redirectURL, err := url.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Location header: %w", err)
	}

	return redirectURL, nil
}

// ParseOAuth2ErrorFromRedirect extracts error parameters from a redirect URL.
// Returns empty strings if no error parameters are present.
// This is stable because it only depends on URL query parameter parsing.
func ParseOAuth2ErrorFromRedirect(redirectURL *url.URL) (error string, errorDescription string) {
	if redirectURL == nil {
		return "", ""
	}

	query := redirectURL.Query()
	errorCode := query.Get("error")
	errorDescription = query.Get("error_description")

	return errorCode, errorDescription
}

// CreateAuthorizationRequest builds a standard OAuth2 authorization request URL.
// This is stable because it only constructs standard OAuth2 parameters.
// clientID: the OAuth2 client ID (e.g., agent ID)
// redirectURI: the callback URI where authorization code is sent
// state: CSRF protection state token
// baseURL: the authorization endpoint base URL (e.g., http://localhost:8000/oauth2/authorize)
func CreateAuthorizationRequest(baseURL, clientID, redirectURI, state string) string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("state", state)

	return baseURL + "?" + params.Encode()
}

// ReadResponseBody reads and returns the entire response body.
// Caller should call resp.Body.Close() when done if needed.
// This is stable because it only reads HTTP response body.
func ReadResponseBody(resp *http.Response) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("response is nil")
	}

	if resp.Body == nil {
		return "", nil
	}

	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(bodyBytes), nil
}

// ExtractQueryParam extracts a single query parameter from a URL.
// Returns empty string if parameter not found.
// This is stable because it only depends on URL query parameter parsing.
func ExtractQueryParam(u *url.URL, paramName string) string {
	if u == nil {
		return ""
	}
	return u.Query().Get(paramName)
}

// ExtractAllQueryParams extracts all query parameters from a URL into a map.
// This is stable because it only depends on URL query parameter parsing.
func ExtractAllQueryParams(u *url.URL) map[string]string {
	if u == nil {
		return make(map[string]string)
	}

	result := make(map[string]string)
	for key, values := range u.Query() {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

// IsHTTPRedirect checks if response is a redirect (3xx status code).
// This is stable because it only depends on HTTP status code.
func IsHTTPRedirect(statusCode int) bool {
	return statusCode >= 300 && statusCode < 400
}

// IsHTTPClientError checks if response is a client error (4xx status code).
// This is stable because it only depends on HTTP status code.
func IsHTTPClientError(statusCode int) bool {
	return statusCode >= 400 && statusCode < 500
}

// IsHTTPServerError checks if response is a server error (5xx status code).
// This is stable because it only depends on HTTP status code.
func IsHTTPServerError(statusCode int) bool {
	return statusCode >= 500 && statusCode < 600
}

// ParseJSONError parses a JSON error response body.
// Expects format: {"error": "...", "error_description": "..."}
// This is stable because it only depends on standard OAuth2 error format.
func ParseJSONError(body string) (errorCode string, errorDescription string) {
	// Simple parsing without external dependencies - enough for testing
	// A production implementation would use json.Unmarshal

	// Extract error field
	// Look for "error":"value" or "error": "value"
	errorPattern := `"error":"` // Most common in json.Marshal
	if idx := strings.Index(body, errorPattern); idx != -1 {
		startIdx := idx + len(errorPattern)
		endIdx := strings.Index(body[startIdx:], "\"")
		if endIdx != -1 {
			errorCode = body[startIdx : startIdx+endIdx]
		}
	} else {
		// Try alternative pattern: "error": "value"
		errorPattern = `"error": "`
		if idx := strings.Index(body, errorPattern); idx != -1 {
			startIdx := idx + len(errorPattern)
			endIdx := strings.Index(body[startIdx:], "\"")
			if endIdx != -1 {
				errorCode = body[startIdx : startIdx+endIdx]
			}
		}
	}

	// Extract error_description field
	descPattern := `"error_description":"`
	if idx := strings.Index(body, descPattern); idx != -1 {
		startIdx := idx + len(descPattern)
		endIdx := strings.Index(body[startIdx:], "\"")
		if endIdx != -1 {
			errorDescription = body[startIdx : startIdx+endIdx]
		}
	} else {
		// Try alternative pattern with space
		descPattern = `"error_description": "`
		if idx := strings.Index(body, descPattern); idx != -1 {
			startIdx := idx + len(descPattern)
			endIdx := strings.Index(body[startIdx:], "\"")
			if endIdx != -1 {
				errorDescription = body[startIdx : startIdx+endIdx]
			}
		}
	}

	return errorCode, errorDescription
}
