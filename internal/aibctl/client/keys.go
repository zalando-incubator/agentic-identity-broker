package client

import (
	"context"
	"net/http"
)

const (
	SigningKeysPath    = "/api/oauth2-server/signing-keys"
	CIMDClientKeysPath = "/api/cimd-client-keys"
	clientCredsSuffix  = "/client-credentials"
)

type keyList struct {
	Items []Key `json:"items"`
}

type keyCreateRequest struct {
	Algorithm string `json:"algorithm,omitempty"`
}

// ListKeys lists signing or CIMD client keys depending on basePath.
func (c *Client) ListKeys(ctx context.Context, basePath string) ([]Key, error) {
	var out keyList
	if _, err := c.getJSON(ctx, basePath, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *Client) CreateKey(ctx context.Context, basePath, algorithm string) (*Key, error) {
	var body any
	if algorithm != "" {
		body = keyCreateRequest{Algorithm: algorithm}
	}
	var out Key
	if _, err := c.sendJSON(ctx, http.MethodPost, basePath, body, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PromoteKey(ctx context.Context, basePath, kid string) (*Key, error) {
	var out Key
	if _, err := c.sendJSON(ctx, http.MethodPut, basePath+"/"+pathSegment(kid)+"/current", nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteKey(ctx context.Context, basePath, kid string) error {
	return c.delete(ctx, basePath+"/"+pathSegment(kid))
}

func clientCredentialsPath(agentID string) string {
	return agentsPath + "/" + pathSegment(agentID) + clientCredsSuffix
}

// GenerateClientCredentials creates or rotates broker credentials; rotated
// reports whether existing credentials were replaced.
func (c *Client) GenerateClientCredentials(ctx context.Context, agentID string) (*ClientCredentials, bool, error) {
	var out ClientCredentials
	resp, err := c.sendJSON(ctx, http.MethodPost, clientCredentialsPath(agentID), nil, "", &out)
	if err != nil {
		return nil, false, err
	}
	return &out, resp.status == http.StatusOK, nil
}

func (c *Client) GetClientCredentials(ctx context.Context, agentID string) (*ClientCredentialsInfo, error) {
	var out ClientCredentialsInfo
	if _, err := c.getJSON(ctx, clientCredentialsPath(agentID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RevokeClientCredentials(ctx context.Context, agentID string) error {
	return c.delete(ctx, clientCredentialsPath(agentID))
}
