package knowledgeclient

import (
	"context"
	"net/http"
)

func (c *Client) Retrieve(ctx context.Context, request RetrievalRequest) (RetrievalResponse, error) {
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/internal/v1/retrieve", request)
	if err != nil {
		return RetrievalResponse{}, err
	}
	var response RetrievalResponse
	if err := c.doJSON(req, &response); err != nil {
		return RetrievalResponse{}, err
	}
	return response, nil
}
