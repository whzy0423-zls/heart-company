package knowledgeclient

import (
	"context"
	"net/http"
)

func (c *Client) Ready(ctx context.Context) error {
	req, err := c.newJSONRequest(ctx, http.MethodGet, "/health/ready", nil)
	if err != nil {
		return err
	}
	return c.doJSON(req, nil)
}
