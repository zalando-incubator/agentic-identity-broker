package server

import (
	"strings"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

func reauthChallengeResponse(recoveryURI, description string) *extprocv3.ProcessingResponse {
	return immediateResponseWithHeaders(httpv3.StatusCode_Forbidden,
		urlElicitationBody(recoveryURI, description, nil),
		map[string]string{"www-authenticate": insufficientScopeChallenge(description, recoveryURI)})
}

func insufficientScopeChallenge(description, errorURI string) string {
	const base = `Bearer error="insufficient_scope"`
	const descriptionAttr = `, error_description="`
	const uriAttr = `, error_uri="`
	includeDescription := description != "" && validChallengeDescription(description)
	includeURI := errorURI != "" && validChallengeURI(errorURI)
	capacity := len(base)
	if includeDescription {
		capacity += len(descriptionAttr) + len(description) + 1
	}
	if includeURI {
		capacity += len(uriAttr) + len(errorURI) + 1
	}
	var challenge strings.Builder
	challenge.Grow(capacity)
	challenge.WriteString(base)
	if includeDescription {
		challenge.WriteString(descriptionAttr)
		challenge.WriteString(description)
		challenge.WriteByte('"')
	}
	if includeURI {
		challenge.WriteString(uriAttr)
		challenge.WriteString(errorURI)
		challenge.WriteByte('"')
	}
	return challenge.String()
}

func validChallengeDescription(value string) bool {
	for i := range len(value) {
		b := value[i]
		if b < 0x20 || b > 0x7e || b == '"' || b == '\\' {
			return false
		}
	}
	return true
}

func validChallengeURI(value string) bool {
	for i := range len(value) {
		b := value[i]
		if b < 0x21 || b > 0x7e || b == '"' || b == '\\' {
			return false
		}
	}
	return true
}
