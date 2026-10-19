package client

import (
	"context"
	"net/http"
)

const sessionSweepPath = "/api/sessions/sweep"

func (c *Client) SweepSessions(ctx context.Context, body SessionSweepRequest) (*SessionSweepResult, error) {
	var out SessionSweepResult
	if _, err := c.sendJSON(ctx, http.MethodPost, sessionSweepPath, body, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}
