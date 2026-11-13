// Package requestbody enforces JSON request boundaries for the HTTP adapter.
package requestbody

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Read limits the complete body, including whitespace and trailing documents.
func Read(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	if r.ContentLength > limit {
		return nil, &http.MaxBytesError{Limit: limit}
	}
	if r.Body == nil {
		return nil, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	return io.ReadAll(r.Body)
}

// Unmarshal accepts one JSON value and rejects unknown struct fields.
// Map keys remain open; custom unmarshalers must enforce their own field policy.
func Unmarshal(data []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if len(bytes.Trim(data[decoder.InputOffset():], " \t\r\n")) != 0 {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

// Decode bounds the body before parsing so oversized requests always return 413,
// even when an invalid field or trailing document occurs before the size limit.
func Decode(w http.ResponseWriter, r *http.Request, limit int64, dst any) error {
	data, err := Read(w, r, limit)
	if err != nil {
		return err
	}
	return Unmarshal(data, dst)
}

// Status maps JSON request errors to their HTTP rejection status.
func Status(err error) int {
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
