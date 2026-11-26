package opa

import (
	"context"
	"errors"
	"net/http"

	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/plugins/rest"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/telemetryhttp"
)

// errTokenUnavailable fails a bundle request whose token cannot be read; OPA retries and the
// active revision stays.
var errTokenUnavailable = errors.New("bundle token unavailable")

// tokenPluginFactory registers the aib_bearer_token_file credential plugin with the OPA SDK.
type tokenPluginFactory struct {
	cfg       tokenFileConfig
	telemetry *telemetry
}

func (f *tokenPluginFactory) Validate(*plugins.Manager, []byte) (any, error) {
	return f.cfg, nil
}

func (f *tokenPluginFactory) New(manager *plugins.Manager, _ any) plugins.Plugin {
	return &tokenPlugin{manager: manager, cfg: f.cfg, telemetry: f.telemetry}
}

// tokenPlugin is a rest.HTTPAuthPlugin that re-reads the token file before every bundle request,
// so a rotated Secret applies without a restart and a replaced file cannot leave its directory.
type tokenPlugin struct {
	manager   *plugins.Manager
	cfg       tokenFileConfig
	telemetry *telemetry
}

var (
	_ plugins.Plugin      = (*tokenPlugin)(nil)
	_ rest.HTTPAuthPlugin = (*tokenPlugin)(nil)
)

func (p *tokenPlugin) Start(context.Context) error {
	p.manager.UpdatePluginStatus(tokenPluginName, &plugins.Status{State: plugins.StateOK})
	return nil
}

func (p *tokenPlugin) Stop(context.Context) {}

func (p *tokenPlugin) Reconfigure(context.Context, any) {}

// NewClient builds OPA's default client for the service and traces it with telemetryhttp.
func (p *tokenPlugin) NewClient(c rest.Config) (*http.Client, error) {
	tlsConfig, err := rest.DefaultTLSConfig(c)
	if err != nil {
		return nil, err
	}
	var timeout int64
	if c.ResponseHeaderTimeoutSeconds != nil {
		timeout = *c.ResponseHeaderTimeoutSeconds
	}
	client := rest.DefaultRoundTripperClient(tlsConfig, timeout)
	client.Transport = telemetryhttp.NewTransport(client.Transport)
	return client, nil
}

// Prepare sets the Authorization header from the current token file contents.
func (p *tokenPlugin) Prepare(req *http.Request) error {
	token, err := readToken(p.cfg.path)
	if err != nil {
		reason := reasonUnreadable
		var cfgErr *configError
		if errors.As(err, &cfgErr) {
			switch cfgErr.reason {
			case reasonFileTooLarge:
				reason = reasonFileTooLarge
			case reasonTokenInvalid:
				reason = "empty"
			}
		}
		p.telemetry.tokenUnavailable(reason)
		return errTokenUnavailable
	}
	req.Header.Set("Authorization", p.cfg.scheme+" "+token)
	return nil
}
