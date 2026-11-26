package impersonation

import (
	"mime"
	"net/url"
	"sort"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// policyInputType is the input.type of every impersonation decision, next to ExtProc's values.
const policyInputType = "impersonation"

// excludedHeaders never reach the policy input. Pseudo-headers are excluded as well.
var excludedHeaders = map[string]bool{"authorization": true, "proxy-authorization": true, "cookie": true}

// excludedFormParameters carry credentials and never reach parsed_body.
var excludedFormParameters = []string{
	"client_assertion", "actor_token", "subject_token", "client_secret", "refresh_token", "code", "code_verifier",
}

// AgentContext is the broker-resolved target Agent in ExtProc's Grant Context agent shape plus
// the literal External ID. Unset optional identifiers are omitted.
type AgentContext struct {
	ID          string
	CanonicalID string
	ExternalID  string
}

func agentContext(agent *storage.Agent) *AgentContext {
	ctx := &AgentContext{ID: agent.ID.String()}
	if agent.CanonicalID != nil {
		ctx.CanonicalID = *agent.CanonicalID
	}
	if agent.ExternalID != nil {
		ctx.ExternalID = agent.ExternalID.String()
	}
	return ctx
}

func (a *AgentContext) toMap() map[string]any {
	agent := map[string]any{"id": a.ID}
	if a.CanonicalID != "" {
		agent["canonical_id"] = a.CanonicalID
	}
	if a.ExternalID != "" {
		agent["external_id"] = a.ExternalID
	}
	return agent
}

// buildPolicyInput builds the Envoy-compatible OPA input. context is built only from validated
// values, so caller headers and form fields cannot populate or overwrite it. host and scheme come
// from the configured public URL, not the request.
func buildPolicyInput(in policyInputs, publicURL *url.URL) ports.ImpersonationPolicyInput {
	http := map[string]any{
		"method":   in.http.Method,
		"path":     in.http.Path,
		"protocol": in.http.Protocol,
		"headers":  policyHeaders(in.http.Headers),
	}
	if publicURL != nil {
		http["host"] = publicURL.Host
		http["scheme"] = publicURL.Scheme
	}
	context := map[string]any{
		celClientAssertion:   orEmptyMap(in.clientClaims),
		celActorToken:        orEmptyMap(in.actorClaims),
		celSubjectToken:      orEmptyMap(in.subjectClaims),
		celSubjectUnverified: in.subjectUnverified,
		celRequest:           in.request,
	}
	if in.agent != nil {
		context["agent"] = in.agent.toMap()
	}
	return ports.ImpersonationPolicyInput{
		"type":           policyInputType,
		"version":        map[string]any{"ext_authz": "v3", "encoding": "protojson"},
		"attributes":     map[string]any{"request": map[string]any{"http": http}},
		"parsed_path":    parsedPath(in.http.Path),
		"parsed_query":   parsedQuery(in.http.RawQuery),
		"parsed_body":    parsedBody(in.http.ContentType, in.http.Form),
		"truncated_body": false,
		"context":        context,
	}
}

// policyHeaders lower-cases names, joins repeated values with ", ", and drops credential and
// pseudo-headers.
func policyHeaders(headers map[string][]string) map[string]any {
	out := make(map[string]any, len(headers))
	for name, values := range headers {
		lower := strings.ToLower(name)
		if excludedHeaders[lower] || strings.HasPrefix(lower, ":") {
			continue
		}
		if existing, ok := out[lower].(string); ok {
			out[lower] = existing + ", " + strings.Join(values, ", ")
			continue
		}
		out[lower] = strings.Join(values, ", ")
	}
	return out
}

// parsedPath splits the path into segments as the Envoy plugin does.
func parsedPath(path string) []any {
	segments := strings.Split(strings.TrimLeft(path, "/"), "/")
	out := make([]any, 0, len(segments))
	for _, segment := range segments {
		if unescaped, err := url.PathUnescape(segment); err == nil {
			segment = unescaped
		}
		out = append(out, segment)
	}
	return out
}

func parsedQuery(raw string) map[string]any {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return map[string]any{}
	}
	return stringArrays(values, nil)
}

// parsedBody returns the form fields as string arrays without credential-bearing parameters, or
// nil (null) for any content type other than a form.
func parsedBody(contentType string, form map[string][]string) any {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		return nil
	}
	return stringArrays(form, excludedFormParameters)
}

func stringArrays(values map[string][]string, excluded []string) map[string]any {
	skip := make(map[string]bool, len(excluded))
	for _, name := range excluded {
		skip[name] = true
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if !skip[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		items := make([]any, 0, len(values[key]))
		for _, value := range values[key] {
			items = append(items, value)
		}
		out[key] = items
	}
	return out
}
