package server

import (
	"net/url"
	"strings"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/structpb"
)

const maxResourceMetadataURLBytes = 2048

func reauthChallengeResponse(recoveryURI, description, scope, resourceMetadata string) *extprocv3.ProcessingResponse {
	return immediateResponseWithHeaders(httpv3.StatusCode_Forbidden,
		urlElicitationBody(recoveryURI, description, nil),
		map[string]string{"www-authenticate": serviceScopeChallenge(description, recoveryURI, scope, resourceMetadata)})
}

// serviceScopeChallenge appends the broker's service scope and the route's protected-resource
// metadata URL, in that fixed order, to the insufficient_scope challenge. Each attribute is
// omitted unless it passes validation; the challenge itself stays valid.
func serviceScopeChallenge(description, errorURI, scope, resourceMetadata string) string {
	challenge := insufficientScopeChallenge(description, errorURI)
	if validChallengeScope(scope) {
		challenge += `, scope="` + scope + `"`
	}
	if resourceMetadata != "" && validChallengeURI(resourceMetadata) {
		challenge += `, resource_metadata="` + resourceMetadata + `"`
	}
	return challenge
}

// validChallengeScope accepts exactly one absolute http(s) scope URL with a host.
func validChallengeScope(scope string) bool {
	if scope == "" || !validChallengeURI(scope) {
		return false
	}
	parsed, err := url.Parse(scope)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// resourceMetadataURL validates the optional aib.tokenexchange.resource_metadata_url route
// metadata. present reports whether the field was supplied at all; an invalid value returns "".
func resourceMetadataURL(value *structpb.Value) (metadataURL string, present bool) {
	if value == nil {
		return "", false
	}
	text, ok := value.GetKind().(*structpb.Value_StringValue)
	if !ok {
		return "", true
	}
	raw := text.StringValue
	if len(raw) > maxResourceMetadataURLBytes || !validChallengeURI(raw) {
		return "", true
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || strings.Contains(raw, "#") {
		return "", true
	}
	return raw, true
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
