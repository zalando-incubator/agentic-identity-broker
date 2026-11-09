package model

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/netpolicy"
)

var publicURLBlocklist = func() netpolicy.SSRFBlocklist {
	blocklist, err := netpolicy.NewSSRFBlocklist(nil)
	if err != nil {
		panic("model: invalid built-in SSRF blocklist")
	}
	return blocklist
}()

// ValidatePublicHTTPSURL checks an untrusted discovery location before network I/O.
// The outbound transport must separately check DNS results at dial time.
func ValidatePublicHTTPSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.Host == "" || u.User != nil || strings.Contains(raw, "#") {
		return errors.New("must be a public HTTPS URL without userinfo or fragment")
	}
	host := u.Hostname()
	if host == "" || strings.Contains(host, "%") || strings.HasSuffix(u.Host, ":") {
		return errors.New("must have a public HTTPS host")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("must have a valid HTTPS port")
		}
	}
	if ip := net.ParseIP(strings.TrimSuffix(host, ".")); ip != nil {
		if publicURLBlocklist.Contains(ip) {
			return errors.New("must not target a blocked IP address")
		}
		return nil
	}
	if strings.HasPrefix(u.Host, "[") {
		return errors.New("must have a valid HTTPS host")
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return errors.New("must have a public HTTPS host")
	}
	for label := range strings.SplitSeq(host, ".") {
		if label == "" || !isPublicDNSAlnum(label[0]) || !isPublicDNSAlnum(label[len(label)-1]) {
			return errors.New("must have a valid HTTPS host")
		}
		for i := 1; i < len(label)-1; i++ {
			if !isPublicDNSAlnum(label[i]) && label[i] != '-' {
				return errors.New("must have a valid HTTPS host")
			}
		}
	}
	return nil
}

func isPublicDNSAlnum(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

// ErrUnsafeDiscoveryResourceURL identifies a rejected outbound discovery target.
var ErrUnsafeDiscoveryResourceURL = errors.New("discovery.resource_url must be a public HTTPS URL without userinfo or fragment")

// validateResourceSource rejects ambiguous protected-resource discovery input.
// Direct authorization-server metadata has no ResourceURL and retains its old rules.
func (d DiscoveryConfig) validateResourceSource() error {
	if d.ResourceURL == nil {
		if d.ClientMethod != "" {
			return errors.New("client_method requires discovery.resource_url")
		}
		return nil
	}
	if !d.EnableDiscovery {
		return errors.New("discovery.resource_url requires enable_discovery")
	}
	if d.MetadataURL != nil {
		return errors.New("discovery.metadata_url cannot be combined with resource_url")
	}
	if err := ValidatePublicHTTPSURL(*d.ResourceURL); err != nil {
		return ErrUnsafeDiscoveryResourceURL
	}
	return nil
}

// ClientBootstrapMethod identifies the selected source of an OAuth2 client identity.
type ClientBootstrapMethod string

const (
	ClientBootstrapCIMD ClientBootstrapMethod = "cimd"
	ClientBootstrapDCR  ClientBootstrapMethod = "dcr"
)

// DiscoveryConfig distinguishes direct authorization-server metadata from
// protected-resource discovery. ClientMethod is chosen by the broker.
type DiscoveryConfig struct {
	EnableDiscovery bool
	MetadataURL     *string
	ResourceURL     *string
	ClientMethod    ClientBootstrapMethod
}

// OAuth2Endpoints holds explicit OAuth2 endpoint URLs for a provider.
// These are used when EnableDiscovery is false in DiscoveryConfig.
type OAuth2Endpoints struct {
	TokenEndpoint     string
	AuthorizeEndpoint string
	JWKsURI           string
}
