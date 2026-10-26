package helpers

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	. "github.com/onsi/gomega" //nolint:staticcheck
)

// ExpectReauthElicitationBody checks the JSON-RPC recovery error independently of field order.
func ExpectReauthElicitationBody(body []byte, recoveryURL, description string) {
	var envelope struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Elicitations []struct {
					Mode          string `json:"mode"`
					ElicitationID string `json:"elicitationId"`
					URL           string `json:"url"`
					Message       string `json:"message"`
				} `json:"elicitations"`
			} `json:"data"`
		} `json:"error"`
	}
	Expect(json.Unmarshal(body, &envelope)).To(Succeed())
	Expect(envelope.JSONRPC).To(Equal("2.0"))
	Expect(envelope.ID).To(BeNil(), "the headers-phase response must have a null JSON-RPC id")
	Expect(envelope.Error.Code).To(Equal(mcp.URL_ELICITATION_REQUIRED))
	Expect(envelope.Error.Message).To(Equal(description))
	Expect(envelope.Error.Data.Elicitations).To(HaveLen(1))
	elicitation := envelope.Error.Data.Elicitations[0]
	Expect(elicitation.Mode).To(Equal(mcp.ElicitationModeURL))
	Expect(elicitation.URL).To(Equal(recoveryURL))
	Expect(elicitation.Message).To(Equal(description))
	Expect(elicitation.ElicitationID).NotTo(BeEmpty())
}
