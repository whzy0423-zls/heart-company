package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/netguard"
)

// Growth has its own finite output budget; other admin/chat consumers retain their defaults.
func callGrowthModelJSON(ctx context.Context, cfg modelconfig.AdminModelConfig, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		return "", errors.New("growth model output limit must be positive")
	}
	return callGrowthModelJSONWithClient(ctx, newGrowthModelHTTPClient(cfg), cfg, systemPrompt, userPrompt, maxTokens)
}

func newGrowthModelHTTPClient(cfg modelconfig.AdminModelConfig) *http.Client {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	// JSON completions may not send headers until generation finishes. Preserve
	// the model timeout instead of the generic 15-second response-header default.
	// The worker's parent context still caps the entire pass at four minutes.
	return netguard.NewGuardedClientWithOptions(timeout, netguard.TransportOptions{
		ResponseHeaderTimeout: timeout,
	})
}

func callGrowthModelJSONWithClient(ctx context.Context, client *http.Client, cfg modelconfig.AdminModelConfig, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		return "", errors.New("growth model output limit must be positive")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "anthropic", "anthropic-compatible":
		return callAnthropicJSONWithLimit(ctx, client, cfg, systemPrompt, userPrompt, maxTokens)
	case "openai", "openai-compatible", "newapi":
		return callOpenAICompatibleJSONWithLimit(ctx, client, cfg, systemPrompt, userPrompt, maxTokens)
	default:
		return callMiniMaxJSONWithLimit(ctx, client, cfg, systemPrompt, userPrompt, maxTokens)
	}
}
