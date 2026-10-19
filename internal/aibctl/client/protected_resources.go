package client

import (
	"context"
	"net/http"
)

func protectedResourcesPath(serviceID string) string {
	return servicesPath + "/" + pathSegment(serviceID) + "/protected-resources"
}

// ListProtectedResources returns the set and the service ETag.
func (c *Client) ListProtectedResources(ctx context.Context, serviceID string) ([]string, string, error) {
	var out ProtectedResourceSet
	resp, err := c.getJSON(ctx, protectedResourcesPath(serviceID), nil, &out)
	if err != nil {
		return nil, "", err
	}
	return out.ProtectedResources, resp.header.Get("ETag"), nil
}

// AddProtectedResource idempotently adds uri; created reports whether it was new.
func (c *Client) AddProtectedResource(ctx context.Context, serviceID, uri string) (*ProtectedResourceMutation, bool, error) {
	var out ProtectedResourceMutation
	resp, err := c.sendJSON(ctx, http.MethodPut, protectedResourcesPath(serviceID)+"/"+pathSegment(uri), nil, "", &out)
	if err != nil {
		return nil, false, err
	}
	return &out, resp.status == http.StatusCreated, nil
}

func (c *Client) RemoveProtectedResource(ctx context.Context, serviceID, uri string) (*ProtectedResourceMutation, error) {
	var out ProtectedResourceMutation
	resp, err := c.do(ctx, request{method: http.MethodDelete, path: protectedResourcesPath(serviceID) + "/" + pathSegment(uri)})
	if err != nil {
		return nil, err
	}
	return &out, decode(resp, &out)
}

func (c *Client) RenameProtectedResource(ctx context.Context, serviceID, from, to string) (*ProtectedResourceMutation, error) {
	var out ProtectedResourceMutation
	body := struct {
		To string `json:"to"`
	}{To: to}
	if _, err := c.sendJSON(ctx, http.MethodPatch, protectedResourcesPath(serviceID)+"/"+pathSegment(from), body, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}
