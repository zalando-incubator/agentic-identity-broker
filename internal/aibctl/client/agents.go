package client

import (
	"context"
	"net/http"
)

const agentsPath = "/api/agents"

func (c *Client) ListAgents(ctx context.Context) ([]Agent, error) {
	var out []Agent
	if _, err := c.getJSON(ctx, agentsPath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetAgent(ctx context.Context, id string) (*Agent, error) {
	var out Agent
	if _, err := c.getJSON(ctx, agentsPath+"/"+pathSegment(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateAgent(ctx context.Context, body AgentWrite) (*Agent, error) {
	var out Agent
	if _, err := c.sendJSON(ctx, http.MethodPost, agentsPath, body, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateAgent(ctx context.Context, id string, body AgentWrite) (*Agent, error) {
	var out Agent
	if _, err := c.sendJSON(ctx, http.MethodPut, agentsPath+"/"+pathSegment(id), body, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteAgent(ctx context.Context, id string) error {
	return c.delete(ctx, agentsPath+"/"+pathSegment(id))
}
