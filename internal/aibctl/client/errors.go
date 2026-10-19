package client

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

const maxErrorBody = 512

// APIError is any non-2xx response from the admin API or the proxy in front of it.
type APIError struct {
	Status  int
	Code    string
	Message string
	Hint    string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	var b strings.Builder
	b.WriteString(msg)
	switch {
	case e.Code != "" && e.Code != msg:
		_, _ = fmt.Fprintf(&b, " (%s, HTTP %d)", e.Code, e.Status)
	default:
		_, _ = fmt.Fprintf(&b, " (HTTP %d)", e.Status)
	}
	if e.Hint != "" {
		b.WriteString("\n")
		b.WriteString(e.Hint)
	}
	return b.String()
}

func IsStatus(err error, status int) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.Status == status
}

func newAPIError(resp *http.Response) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody*4))
	apiErr := &APIError{Status: resp.StatusCode}

	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "application/json" {
		var parsed struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &parsed) == nil && (parsed.Error != "" || parsed.Message != "") {
			apiErr.Code = parsed.Error
			apiErr.Message = parsed.Message
			if apiErr.Message == "" {
				apiErr.Message = parsed.Error
			}
			return apiErr
		}
	}

	text := strings.TrimSpace(string(body))
	if len(text) > maxErrorBody {
		text = text[:maxErrorBody] + "..."
	}
	apiErr.Message = text
	switch resp.StatusCode {
	case http.StatusForbidden:
		apiErr.Hint = "hint: the broker rejects requests whose Host differs from server.admin.public_url; set host_header in the context or pass --host-header"
	case http.StatusUnsupportedMediaType:
		apiErr.Hint = "hint: the admin API requires Content-Type: application/json; this is an aibctl bug, please report it"
	}
	return apiErr
}
