package urivalidation

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
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
