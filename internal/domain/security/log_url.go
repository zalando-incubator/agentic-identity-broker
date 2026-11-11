package security

import (
	"context"
	"errors"
	"net"
	"net/url"
)

const (
	// InvalidURLLogValue replaces a URL that does not parse, so the raw input is never echoed.
	InvalidURLLogValue = "invalid_url"
	MaxLoggedURLBytes  = 512
)

// RedactURL reduces raw to scheme, host, and path for logging (coding-standards/LOGGING.md
// section 6.2). It drops userinfo, query, fragment, and opaque data. The result is
// percent-encoded ASCII, so it cannot split a log line.
func RedactURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return InvalidURLLogValue
	}
	reduced := url.URL{
		Scheme:   parsed.Scheme,
		Host:     parsed.Host,
		Path:     parsed.Path,
		RawPath:  parsed.RawPath,
		OmitHost: parsed.OmitHost,
	}
	logged := reduced.String()
	if parsed.Opaque != "" {
		logged = parsed.Scheme + ":"
	}
	if len(logged) > MaxLoggedURLBytes {
		logged = logged[:MaxLoggedURLBytes]
	}
	return logged
}

// Outbound error kinds classify a failed call to an external system without its cause,
// which can carry the full request URL (*url.Error) or a response body.
const (
	OutboundErrorTimeout      = "timeout"
	OutboundErrorCanceled     = "canceled"
	OutboundErrorTransport    = "transport"
	OutboundErrorUnclassified = "unclassified"
)

// ClassifyOutboundError returns the error_kind for a failed outbound call, or "" for nil.
func ClassifyOutboundError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return OutboundErrorCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return OutboundErrorTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return OutboundErrorTimeout
		}
		return OutboundErrorTransport
	}
	return OutboundErrorUnclassified
}
