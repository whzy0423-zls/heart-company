package videoproject

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/video"
)

type DiagnosticsHashInput struct {
	ShotContent       map[string]any            `json:"shotContent"`
	References        video.CanonicalReferences `json:"references"`
	CapabilityVersion string                    `json:"capabilityVersion"`
	CompilerVersion   string                    `json:"compilerVersion"`
}

func ShotRequestHash(request video.GenerateRequest) string {
	request.RequestKey = ""
	request.References = append([]video.Reference{}, request.References...)
	return hashWorkflowValue(request)
}

func DiagnosticsHash(input DiagnosticsHashInput) string {
	input.CapabilityVersion = strings.TrimSpace(input.CapabilityVersion)
	input.CompilerVersion = strings.TrimSpace(input.CompilerVersion)
	input.References.References = append([]video.CanonicalReference{}, input.References.References...)
	return hashWorkflowValue(input)
}

func SelectionAckHash(currentRequestHash, generationID string) string {
	return hashWorkflowValue(struct {
		CurrentRequestHash string `json:"currentRequestHash"`
		GenerationID       string `json:"generationId"`
	}{
		CurrentRequestHash: strings.TrimSpace(currentRequestHash),
		GenerationID:       strings.TrimSpace(generationID),
	})
}

func ComposeInputHash(orderedSelectedGenerationIDs []string, settings ComposeProjectInput) string {
	selections := make([]string, len(orderedSelectedGenerationIDs))
	for index, generationID := range orderedSelectedGenerationIDs {
		selections[index] = strings.TrimSpace(generationID)
	}
	settings.Transition = strings.TrimSpace(settings.Transition)
	settings.MusicURL = strings.TrimSpace(settings.MusicURL)
	return hashWorkflowValue(struct {
		SelectedGenerationIDs []string            `json:"selectedGenerationIds"`
		Settings              ComposeProjectInput `json:"settings"`
	}{SelectedGenerationIDs: selections, Settings: settings})
}

func hashWorkflowValue(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
