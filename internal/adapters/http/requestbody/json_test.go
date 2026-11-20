package requestbody

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeBoundaries(t *testing.T) {
	const limit = 64
	for _, tt := range []struct {
		name   string
		body   string
		status int
	}{
		{"exact limit", `{"name":"ok"}` + strings.Repeat(" ", limit-len(`{"name":"ok"}`)), 0},
		{"oversized whitespace", `{"name":"ok"}` + strings.Repeat(" ", limit), 413},
		{"oversized value", `{"name":"` + strings.Repeat("x", limit) + `"}`, 413},
		{"oversized malformed body", `!` + strings.Repeat("x", limit), 413},
		{"unknown field", `{"unexpected":true}`, 400},
		{"nested unknown field", `{"child":{"unexpected":true}}`, 400},
		{"trailing object", `{"name":"ok"} {}`, 400},
		{"non JSON whitespace", "{\"name\":\"ok\"}\u00a0", 400},
		{"trailing null", `{"name":"ok"} null`, 400},
		{"trailing junk", `{"name":"ok"} !`, 400},
		{"truncated", `{"name":`, 400},
		{"empty", ``, 400},
		{"open map", `{"arguments":{"custom":{"nested":true}}}`, 0},
	} {
		for _, chunked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/chunked=%t", tt.name, chunked), func(t *testing.T) {
				r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
				if chunked {
					r.ContentLength = -1
				}
				var dst struct {
					Name  string `json:"name"`
					Child struct {
						Name string `json:"name"`
					} `json:"child"`
					Arguments map[string]any `json:"arguments"`
				}
				err := Decode(httptest.NewRecorder(), r, limit, &dst)
				if tt.status == 0 {
					require.NoError(t, err)
					if tt.name == "exact limit" {
						assert.Equal(t, "ok", dst.Name)
					}
					if tt.name == "open map" {
						assert.Equal(t, map[string]any{"custom": map[string]any{"nested": true}}, dst.Arguments)
					}
					return
				}
				require.Error(t, err)
				assert.Equal(t, tt.status, Status(err))
			})
		}
	}
}

func TestDecodeReadFailureAndOptionalBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Body = io.NopCloser(iotest.ErrReader(errors.New("body read failed")))
	var dst map[string]any
	err := Decode(httptest.NewRecorder(), r, 64, &dst)
	require.ErrorContains(t, err, "body read failed")
	assert.Equal(t, http.StatusBadRequest, Status(err))
	r.Body = nil
	require.ErrorIs(t, Decode(httptest.NewRecorder(), r, 64, &dst), io.EOF)
}
