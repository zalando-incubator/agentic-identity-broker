package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestURLElicitationResponsePreservesParameters(t *testing.T) {
	response := urlElicitationResponse("https://broker.example/approvals/id", "approval required", json.RawMessage(`"request-7"`))
	require.Equal(t, int32(200), int32(response.GetImmediateResponse().Status.Code))
	var body struct {
		ID    string `json:"id"`
		Error struct {
			Code int `json:"code"`
			Data struct {
				Elicitations []struct {
					URL string `json:"url"`
				} `json:"elicitations"`
			} `json:"data"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.GetImmediateResponse().Body, &body))
	require.Equal(t, "request-7", body.ID)
	require.Equal(t, -32042, body.Error.Code)
	require.Len(t, body.Error.Data.Elicitations, 1)
	require.Equal(t, "https://broker.example/approvals/id", body.Error.Data.Elicitations[0].URL)
}
