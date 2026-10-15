package client

import (
	"context"
	"net/http"
)

const servicesPath = "/api/services"

func (c *Client) ListServices(ctx context.Context) ([]Service, error) {
	var out []Service
	if _, err := c.getJSON(ctx, servicesPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetService returns the service and its ETag for conditional updates.
func (c *Client) GetService(ctx context.Context, id string) (*Service, string, error) {
	var out Service
	resp, err := c.getJSON(ctx, servicesPath+"/"+pathSegment(id), nil, &out)
	if err != nil {
		return nil, "", err
	}
	return &out, resp.header.Get("ETag"), nil
}

func (c *Client) CreateService(ctx context.Context, body ServiceWrite) (*Service, error) {
	var out Service
	if _, err := c.sendJSON(ctx, http.MethodPost, servicesPath, body, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateService replaces the service. ifMatch is required by the API whenever
// the body carries protected_resources.
func (c *Client) UpdateService(ctx context.Context, id string, body ServiceWrite, ifMatch string) (*Service, string, error) {
	var out Service
	resp, err := c.sendJSON(ctx, http.MethodPut, servicesPath+"/"+pathSegment(id), body, ifMatch, &out)
	if err != nil {
		return nil, "", err
	}
	return &out, resp.header.Get("ETag"), nil
}

func (c *Client) DeleteService(ctx context.Context, id string) error {
	return c.delete(ctx, servicesPath+"/"+pathSegment(id))
}
