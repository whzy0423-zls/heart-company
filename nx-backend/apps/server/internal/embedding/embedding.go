// Package embedding 封装文本向量化（用于 RAG 语义检索）。
// Provider 为空时 Enabled()=false，调用方应回退到关键词检索。
// 兼容 OpenAI 风格的 /v1/embeddings 接口（OpenAI、通义、MiniMax 等多数兼容）。
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Provider  string
	APIBase   string
	APIKey    string
	Model     string
	Dimension int
}

type Client struct {
	cfg     Config
	http    *http.Client
	enabled bool
}

func NewClient(cfg Config) *Client {
	enabled := strings.TrimSpace(cfg.Provider) != "" && cfg.APIKey != "" && cfg.APIBase != "" && cfg.Model != ""
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: 15 * time.Second},
		enabled: enabled,
	}
}

// Enabled 是否已配置可用的向量化能力。
func (c *Client) Enabled() bool { return c.enabled }

// Dimension 向量维度（与 schema 中 vector(N) 必须一致）。
func (c *Client) Dimension() int { return c.cfg.Dimension }

// ModelName 当前 embedding 模型标识（写入 rag_documents.embedding_model）。
func (c *Client) ModelName() string { return c.cfg.Model }

type embedRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed 把单段文本向量化。
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("empty embedding result")
	}
	return vectors[0], nil
}

// EmbedBatch 批量向量化。
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if !c.enabled {
		return nil, errors.New("embedding not configured")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(embedRequest{Input: texts, Model: c.cfg.Model})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(c.cfg.APIBase, "/") + "/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding api status %d: %s", resp.StatusCode, string(body))
	}
	var parsed embedResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, errors.New(parsed.Error.Message)
	}
	out := make([][]float32, 0, len(parsed.Data))
	for _, d := range parsed.Data {
		out = append(out, d.Embedding)
	}
	return out, nil
}
