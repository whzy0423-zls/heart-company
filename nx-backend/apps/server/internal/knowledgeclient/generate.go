package knowledgeclient

import (
	"context"
	"net/http"
)

func (c *Client) Generate(ctx context.Context, request AnswerRequest) (AnswerResponse, error) {
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/internal/v1/answer", request)
	if err != nil {
		return AnswerResponse{}, err
	}
	var response AnswerResponse
	if err := c.doJSON(req, &response); err != nil {
		return AnswerResponse{}, err
	}
	return response, nil
}
