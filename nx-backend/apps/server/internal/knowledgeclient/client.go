package knowledgeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultMaxResponseBytes int64 = 8 << 20

type Config struct {
	BaseURL           string
	Token             string
	HTTPClient        *http.Client
	MaxResponseBytes  int64
	StreamIdleTimeout time.Duration
}

type Client struct {
	baseURL           *url.URL
	token             string
	httpClient        *http.Client
	maxResponseBytes  int64
	streamIdleTimeout time.Duration
}

func New(config Config) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(config.BaseURL, "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid knowledge service URL %q", config.BaseURL)
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	limit := config.MaxResponseBytes
	if limit <= 0 {
		limit = defaultMaxResponseBytes
	}
	return &Client{
		baseURL:           baseURL,
		token:             config.Token,
		httpClient:        client,
		maxResponseBytes:  limit,
		streamIdleTimeout: config.StreamIdleTimeout,
	}, nil
}

func (c *Client) newJSONRequest(ctx context.Context, method, path string, payload any) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL.String()+path, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	return req, nil
}

func (c *Client) doJSON(req *http.Request, destination any) error {
	response, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > c.maxResponseBytes {
		return ErrResponseTooLarge
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &ResponseError{StatusCode: response.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	if destination == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("decode knowledge service response: %w", err)
	}
	return nil
}
