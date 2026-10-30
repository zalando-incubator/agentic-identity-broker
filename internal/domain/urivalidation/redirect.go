package urivalidation

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// IsValidRedirectURI validates a web redirect URI per RFC 6749 §3.1.2.
// HTTPS requires a non-empty host; HTTP is allowed only for loopback hosts.
// Fragments are forbidden. Native callbacks use IsValidPrivateUseRedirectURI.
func IsValidRedirectURI(uriStr string) bool {
	u, ok := parseRedirectURI(uriStr)
	if !ok {
		return false
	}

	switch u.Scheme {
	case "https":
		return true
	case "http":
		// Allow HTTP only for loopback (development-mode callbacks)
		return isLoopbackRedirectHost(u.Hostname())
	default:
		return false
	}
}

const privateUseRedirectURIChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~:/?@!$&'()+,;="

var deniedPrivateUseSchemes = []string{
	"about", "blob", "data", "file", "filesystem", "ftp",
	"javascript", "vbscript", "view-source", "ws", "wss",
}

// IsValidPrivateUseRedirectURI validates a canonical native-app callback (RFC 8252 §7.1).
func IsValidPrivateUseRedirectURI(uriStr string) bool {
	if uriStr == "" {
		return false
	}
	for i := range len(uriStr) {
		if strings.IndexByte(privateUseRedirectURIChars, uriStr[i]) < 0 {
			return false
		}
	}

	u, err := url.Parse(uriStr)
	if err != nil || u.String() != uriStr || u.Opaque != "" || u.User != nil || u.ForceQuery {
		return false
	}
	if len(u.Scheme) < 2 || u.Scheme == "http" || u.Scheme == "https" || slices.Contains(deniedPrivateUseSchemes, u.Scheme) {
		return false
	}
	if u.Host == "" {
		if !u.OmitHost {
			return false
		}
	} else if u.Host != strings.ToLower(u.Host) || !hasValidRedirectAuthority(u) {
		return false
	}
	for segment := range strings.SplitSeq(u.Path, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// IsRegisteredRedirectURI requires private-use callbacks in both the client and Agent lists.
// Web callbacks use the client list, preserving loopback HTTP port matching.
func IsRegisteredRedirectURI(clientRedirectURIs, agentRedirectURIs []string, incoming string) bool {
	if IsValidPrivateUseRedirectURI(incoming) {
		return slices.Contains(clientRedirectURIs, incoming) && slices.Contains(agentRedirectURIs, incoming)
	}
	return slices.ContainsFunc(clientRedirectURIs, func(registered string) bool {
		return MatchesRedirectURI(registered, incoming)
	})
}

// MatchesRedirectURI matches a private-use registration only byte for byte.
// For loopback web callbacks it ignores the port; other components must match exactly.
func MatchesRedirectURI(registered, incoming string) bool {
	if IsValidPrivateUseRedirectURI(registered) {
		return registered == incoming
	}
	r, ok := parseRedirectURI(registered)
	if !ok {
		return false
	}
	in, ok := parseRedirectURI(incoming)
	if !ok {
		return false
	}
	if h := r.Hostname(); isLoopbackRedirectHost(h) {
		r.Host = normalizeRedirectHost(h)
		in.Host = normalizeRedirectHost(in.Hostname())
	}
	return r.String() == in.String()
}

// ValidateAgentRedirectURIRegistration validates an Agent callback registration,
// including the host-boundary rules for HTTPS wildcard patterns.
func ValidateAgentRedirectURIRegistration(raw string) error {
	if !strings.Contains(raw, "*") {
		if IsValidRedirectURI(raw) || IsValidPrivateUseRedirectURI(raw) {
			return nil
		}
		return errors.New("invalid redirect URI registration")
	}

	u, ok := parseRedirectURI(raw)
	if !ok {
		return errors.New("wildcard redirect URI must be a valid HTTPS URI")
	}
	host := u.Hostname()
	if !strings.Contains(host, "*") {
		if IsValidRedirectURI(raw) || IsValidPrivateUseRedirectURI(raw) {
			return nil
		}
		return errors.New("invalid redirect URI registration")
	}
	if u.Scheme != "https" {
		return errors.New("wildcard redirect URI must be a valid HTTPS URI")
	}
	if host == "" || strings.Count(raw, "*") != strings.Count(host, "*") {
		return errors.New("wildcard is allowed only in the HTTPS hostname")
	}

	patternLabels, err := normalizedWildcardLabels(host, true)
	if err != nil {
		return fmt.Errorf("wildcard redirect URI hostname is invalid: %w", err)
	}
	// Validate the shortest valid pattern expansion separately from the synthetic PSL probe.
	if err := validateWildcardPatternLength(patternLabels); err != nil {
		return fmt.Errorf("wildcard redirect URI hostname is invalid: %w", err)
	}

	marker := wildcardMarker(host)
	probe := strings.ReplaceAll(host, "*", marker)
	// The marker exists only to classify public-suffix boundaries, not to measure DNS label lengths.
	asciiHost, err := idna.Lookup.ToASCII(probe)
	if err != nil {
		return fmt.Errorf("wildcard redirect URI hostname is invalid: %w", err)
	}
	asciiHost = strings.ToLower(asciiHost)
	if _, err := netip.ParseAddr(asciiHost); err == nil {
		return errors.New("wildcard redirect URI cannot use an IP literal hostname")
	}
	publicSuffix, _ := publicsuffix.PublicSuffix(asciiHost)
	if publicSuffix == "" {
		return errors.New("wildcard redirect URI has no classifiable public suffix")
	}
	registrableDomain, err := publicsuffix.EffectiveTLDPlusOne(asciiHost)
	if err != nil {
		return fmt.Errorf("wildcard redirect URI has no classifiable registrable domain: %w", err)
	}
	labels := strings.Split(host, ".")
	registrableLabelCount := len(strings.Split(registrableDomain, "."))
	if registrableLabelCount > len(labels) {
		return errors.New("wildcard redirect URI hostname has an invalid public-suffix boundary")
	}
	for _, label := range labels[len(labels)-registrableLabelCount:] {
		if strings.Contains(label, "*") {
			return errors.New("wildcard cannot replace a registrable-domain or public-suffix label")
		}
	}

	return nil
}

// MatchesAgentRedirectURI matches an incoming callback against an Agent registration.
func MatchesAgentRedirectURI(registered, incoming string) bool {
	registeredURL, registeredOK := parseRedirectURI(registered)
	if !registeredOK {
		return false
	}
	if !strings.Contains(registeredURL.Hostname(), "*") {
		return MatchesRedirectURI(registered, incoming)
	}
	if ValidateAgentRedirectURIRegistration(registered) != nil || strings.Contains(incoming, "*") || !IsValidRedirectURI(incoming) {
		return false
	}

	incomingURL, incomingOK := parseRedirectURI(incoming)
	if !incomingOK || registeredURL.Scheme != "https" || incomingURL.Scheme != "https" {
		return false
	}
	if registeredURL.Port() != incomingURL.Port() {
		return false
	}
	registeredOther := *registeredURL
	incomingOther := *incomingURL
	registeredOther.Host = ""
	incomingOther.Host = ""
	if registeredOther.String() != incomingOther.String() {
		return false
	}

	registeredLabels, err := normalizedWildcardLabels(registeredURL.Hostname(), true)
	if err != nil {
		return false
	}
	incomingLabels, err := normalizedWildcardLabels(incomingURL.Hostname(), false)
	if err != nil || len(registeredLabels) != len(incomingLabels) {
		return false
	}
	// Check the concrete host too: `https://*.*.amazonaws.com` must not match
	// `https://bucket.s3.amazonaws.com`, where `s3.amazonaws.com` is a private suffix.
	if !hasLiteralPublicSuffixBoundary(registeredLabels, incomingLabels, incomingURL.Hostname()) {
		return false
	}
	for index := range registeredLabels {
		// Match one concrete label at a time: `https://preview*.example.com` matches
		// `https://preview-pr.example.com`, but not `https://preview.pr.example.com`.
		if len(incomingLabels[index].parts) != 1 || !matchWildcardLabel(registeredLabels[index], incomingLabels[index].parts[0]) {
			return false
		}
	}
	return true
}

type wildcardLabel struct {
	parts []string
}

func hasLiteralPublicSuffixBoundary(patternLabels, candidateLabels []wildcardLabel, candidateHost string) bool {
	if len(patternLabels) != len(candidateLabels) {
		return false
	}
	asciiHost, err := asciiDNSHostname(candidateHost)
	if err != nil {
		return false
	}
	publicSuffix, _ := publicsuffix.PublicSuffix(asciiHost)
	if publicSuffix == "" {
		return false
	}
	registrableDomain, err := publicsuffix.EffectiveTLDPlusOne(asciiHost)
	if err != nil {
		return false
	}
	boundaryLabelCount := len(strings.Split(registrableDomain, "."))
	if boundaryLabelCount > len(candidateLabels) {
		return false
	}
	for index := len(candidateLabels) - boundaryLabelCount; index < len(candidateLabels); index++ {
		// For `bucket.s3.amazonaws.com`, the private suffix is `s3.amazonaws.com`,
		// so the registrable domain `bucket.s3.amazonaws.com` must remain literal.
		if len(patternLabels[index].parts) != 1 || len(candidateLabels[index].parts) != 1 || patternLabels[index].parts[0] != candidateLabels[index].parts[0] {
			return false
		}
	}
	return true
}

func wildcardMarker(host string) string {
	marker := "aibwildcardmarker"
	for suffix := 0; strings.Contains(host, marker); suffix++ {
		marker = "aibwildcardmarker" + strconv.Itoa(suffix)
	}
	return marker
}

func asciiDNSHostname(host string) (string, error) {
	if host == "" || strings.HasSuffix(host, ".") {
		return "", errors.New("hostname is empty or has a trailing dot")
	}
	asciiHost, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return "", err
	}
	asciiHost = strings.ToLower(asciiHost)
	if len(asciiHost) > 253 {
		return "", errors.New("hostname exceeds the DNS length limit")
	}
	for _, label := range strings.Split(asciiHost, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("hostname contains an invalid DNS label")
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return "", errors.New("hostname contains a non-DNS character")
			}
		}
	}
	return asciiHost, nil
}

func normalizedWildcardLabels(host string, wildcard bool) ([]wildcardLabel, error) {
	if !wildcard {
		// Normalize concrete hosts through IDNA before comparing their labels with the pattern.
		asciiHost, err := asciiDNSHostname(host)
		if err != nil {
			return nil, err
		}
		unicodeHost, err := idna.Lookup.ToUnicode(asciiHost)
		if err != nil {
			return nil, err
		}
		labels := strings.Split(strings.ToLower(unicodeHost), ".")
		result := make([]wildcardLabel, len(labels))
		for index, label := range labels {
			result[index] = wildcardLabel{parts: []string{label}}
		}
		return result, nil
	}

	// Keep wildcards structural: IDNA lowercasing `AIBWILDCARDMARKER` could turn
	// `*.AIBWILDCARDMARKER.preview.example.com` into `*.*.preview.example.com`.
	labels := []wildcardLabel{{parts: []string{""}}}
	parts := strings.Split(host, "*")
	for index, literal := range parts {
		if literal != "" {
			var err error
			labels, err = appendNormalizedWildcardLiteral(labels, literal, index > 0, index < len(parts)-1)
			if err != nil {
				return nil, err
			}
		}
		if index < len(parts)-1 {
			lastLabel := len(labels) - 1
			labels[lastLabel].parts = append(labels[lastLabel].parts, "")
		}
	}
	return labels, nil
}

// Measure literal length with empty wildcard matches; fill a wildcard only when needed for a valid label.
func validateWildcardPatternLength(labels []wildcardLabel) error {
	probeLabels := make([]string, len(labels))
	for index, label := range labels {
		parts := slices.Clone(label.parts)
		literal := strings.Join(parts, "")
		if literal == "" && len(parts) > 1 {
			parts[0] = "a"
		} else {
			if parts[0] == "" && strings.HasPrefix(literal, "-") {
				parts[0] = "a"
			}
			lastPart := len(parts) - 1
			if parts[lastPart] == "" && strings.HasSuffix(literal, "-") {
				parts[lastPart] = "a"
			}
		}
		probeLabels[index] = strings.Join(parts, "")
	}
	_, err := asciiDNSHostname(strings.Join(probeLabels, "."))
	return err
}

func appendNormalizedWildcardLiteral(labels []wildcardLabel, literal string, leftWildcard, rightWildcard bool) ([]wildcardLabel, error) {
	literalParts := strings.Split(literal, ".")
	for index, part := range literalParts {
		if part != "" {
			completeLabel := (index > 0 || !leftWildcard) && (index < len(literalParts)-1 || !rightWildcard)
			idnaPart := part
			if !completeLabel {
				idnaPart = "x" + part + "x"
			}
			asciiPart, err := idna.Lookup.ToASCII(idnaPart)
			if err != nil {
				return nil, err
			}
			unicodePart, err := idna.Lookup.ToUnicode(asciiPart)
			if err != nil {
				return nil, err
			}
			// IDNA can map Unicode separators to dots; preserve those as label boundaries.
			normalizedLabels := strings.Split(strings.ToLower(unicodePart), ".")
			if !completeLabel {
				normalizedLabels[0] = strings.TrimPrefix(normalizedLabels[0], "x")
				lastNormalizedLabel := len(normalizedLabels) - 1
				normalizedLabels[lastNormalizedLabel] = strings.TrimSuffix(normalizedLabels[lastNormalizedLabel], "x")
			}
			for labelIndex, normalizedPart := range normalizedLabels {
				lastLabel := len(labels) - 1
				lastPart := len(labels[lastLabel].parts) - 1
				labels[lastLabel].parts[lastPart] += normalizedPart
				if labelIndex < len(normalizedLabels)-1 {
					labels = append(labels, wildcardLabel{parts: []string{""}})
				}
			}
		}
		if index < len(literalParts)-1 {
			labels = append(labels, wildcardLabel{parts: []string{""}})
		}
	}
	return labels, nil
}

func matchWildcardLabel(pattern wildcardLabel, candidate string) bool {
	parts := pattern.parts
	if len(parts) == 1 {
		return parts[0] == candidate
	}
	if !strings.HasPrefix(candidate, parts[0]) {
		return false
	}
	position := len(parts[0])
	for _, part := range parts[1 : len(parts)-1] {
		if part == "" {
			continue
		}
		relativePosition := strings.Index(candidate[position:], part)
		if relativePosition < 0 {
			return false
		}
		position += relativePosition + len(part)
	}
	return strings.HasSuffix(candidate[position:], parts[len(parts)-1])
}

func parseRedirectURI(uriStr string) (*url.URL, bool) {
	// Reject raw fragment or whitespace before parsing — url.ParseRequestURI
	// percent-encodes these instead of erroring.
	if strings.ContainsAny(uriStr, "# \t\n\r") {
		return nil, false
	}

	u, err := url.ParseRequestURI(uriStr)
	if err != nil {
		return nil, false
	}
	if !hasValidRedirectAuthority(u) {
		return nil, false
	}
	return u, true
}

func hasValidRedirectAuthority(u *url.URL) bool {
	if u.Host == "" || u.Hostname() == "" {
		return false
	}
	if strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}

func isLoopbackRedirectHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func normalizeRedirectHost(host string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}
