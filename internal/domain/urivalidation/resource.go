package urivalidation

import (
	"net/url"
	"strings"
)

// NormalizeResourceURI strips trailing slashes from a resource URI path.
// Query strings and fragments are preserved unchanged.
func NormalizeResourceURI(uri string) string {
	if uri == "" {
		return ""
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return uri
	}

	escapedPath := parsed.EscapedPath()
	trimmedEscapedPath := strings.TrimRight(escapedPath, "/")
	if trimmedEscapedPath == escapedPath {
		return uri
	}

	if trimmedEscapedPath == "" {
		parsed.Path = ""
		parsed.RawPath = ""
	} else {
		trimmedPath, err := url.PathUnescape(trimmedEscapedPath)
		if err != nil {
			return uri
		}

		parsed.Path = trimmedPath
		if trimmedEscapedPath != trimmedPath {
			parsed.RawPath = trimmedEscapedPath
		} else {
			parsed.RawPath = ""
		}
	}

	normalized := parsed.String()
	if _, err := url.Parse(normalized); err != nil {
		return uri
	}

	return normalized
}
