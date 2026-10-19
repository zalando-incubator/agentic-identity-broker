package client

import (
	"context"
	"net/http"
	"net/url"
)

const permissionSetsPath = "/api/permission-sets"

type permissionSetList struct {
	Items []PermissionSet `json:"items"`
}

// ListPermissionSets lists permission sets, optionally filtered by service
// (UUID or canonical ID).
func (c *Client) ListPermissionSets(ctx context.Context, serviceID string) ([]PermissionSet, error) {
	var query url.Values
	if serviceID != "" {
		query = url.Values{"service_id": {serviceID}}
	}
	var out permissionSetList
	if _, err := c.getJSON(ctx, permissionSetsPath, query, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *Client) GetPermissionSet(ctx context.Context, id string) (*PermissionSet, error) {
	var out PermissionSet
	if _, err := c.getJSON(ctx, permissionSetsPath+"/"+pathSegment(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreatePermissionSet(ctx context.Context, body PermissionSetWrite) (*PermissionSet, error) {
	var out PermissionSet
	if _, err := c.sendJSON(ctx, http.MethodPost, permissionSetsPath, body, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdatePermissionSet(ctx context.Context, id string, body PermissionSetWrite) (*PermissionSet, error) {
	var out PermissionSet
	if _, err := c.sendJSON(ctx, http.MethodPut, permissionSetsPath+"/"+pathSegment(id), body, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeletePermissionSet(ctx context.Context, id string) error {
	return c.delete(ctx, permissionSetsPath+"/"+pathSegment(id))
}
