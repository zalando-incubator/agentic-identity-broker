package enduser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const maxProxyTokenResponseBytes = 1 << 20

type oauth2TokenProxy struct {
	endpoint string
	client   *http.Client
}

func NewOAuth2TokenProxy(endpoint string, client *http.Client) ports.OAuth2TokenProxy {
	return &oauth2TokenProxy{endpoint: endpoint, client: client}
}

func (p *oauth2TokenProxy) Exchange(ctx context.Context, encodedForm, contentType string) (ports.TokenProxyResponse, ports.TokenProxyFailure, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, strings.NewReader(encodedForm))
	if err != nil {
		return nil, ports.TokenProxyRequestCreationFailed, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	client := p.client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, ports.TokenProxyTransportFailed, err
	}
	return (*oauth2TokenProxyResponse)(response), ports.TokenProxyNoFailure, nil
}

type oauth2TokenProxyResponse http.Response

func (r *oauth2TokenProxyResponse) ResponseStatus() int {
	return r.StatusCode
}

func (r *oauth2TokenProxyResponse) HeaderValues(name string) []string {
	return r.Header.Values(name)
}

func (r *oauth2TokenProxyResponse) ReadBody() ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxProxyTokenResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxProxyTokenResponseBytes {
		return nil, fmt.Errorf("upstream token response exceeds %d byte limit", maxProxyTokenResponseBytes)
	}
	return body, nil
}

func (r *oauth2TokenProxyResponse) StreamBody(dst io.Writer) (int64, error) {
	return io.Copy(dst, r.Body)
}

func (r *oauth2TokenProxyResponse) CloseBody() error {
	return r.Body.Close()
}
