package skillcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const reviewPolicyProductBaselineV1 = "product-baseline-v1"

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type CatalogCommand struct {
	Action       string
	Version      string
	SourceDir    string
	ManifestPath string
}

func (c CatalogCommand) Validate() error {
	action := strings.TrimSpace(c.Action)
	if !semanticVersionPattern.MatchString(strings.TrimSpace(c.Version)) {
		return errors.New("skill catalog: version must be semantic version")
	}
	switch action {
	case "draft", "ready", "publish":
		if strings.TrimSpace(c.SourceDir) == "" {
			return errors.New("skill catalog: source directory is required")
		}
		if strings.TrimSpace(c.ManifestPath) == "" {
			return errors.New("skill catalog: review manifest is required")
		}
	case "retire", "rollback":
		return nil
	default:
		return fmt.Errorf("skill catalog: unsupported action %q", action)
	}
	return nil
}

type ReviewManifest struct {
	SchemaVersion  int                            `json:"schemaVersion"`
	CatalogKey     string                         `json:"catalogKey"`
	CatalogVersion string                         `json:"catalogVersion"`
	ReviewPolicy   string                         `json:"reviewPolicy"`
	DecisionRef    string                         `json:"decisionRef"`
	Skills         map[string]SkillReviewDecision `json:"skills"`
}

type SkillReviewDecision struct {
	Decision     string   `json:"decision"`
	SourceNeeded bool     `json:"sourceNeeded,omitempty"`
	RiskNotices  []string `json:"riskNotices,omitempty"`
}

func LoadReviewManifest(path string) (ReviewManifest, string, error) {
	raw, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return ReviewManifest{}, "", fmt.Errorf("skill catalog: read review manifest: %w", err)
	}
	var manifest ReviewManifest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ReviewManifest{}, "", fmt.Errorf("skill catalog: decode review manifest: %w", err)
	}
	digest := sha256.Sum256(raw)
	return manifest, hex.EncodeToString(digest[:]), nil
}

func (m ReviewManifest) Validate(catalog BuiltinCatalog, version string) error {
	if m.SchemaVersion != 1 || strings.TrimSpace(m.CatalogKey) != catalog.Key || strings.TrimSpace(m.CatalogVersion) != version {
		return errors.New("skill catalog: review manifest identity mismatch")
	}
	if strings.TrimSpace(m.ReviewPolicy) != reviewPolicyProductBaselineV1 || strings.TrimSpace(m.DecisionRef) == "" {
		return errors.New("skill catalog: review manifest policy and decisionRef are required")
	}
	expected := make(map[string]BuiltinSkill, 35)
	for _, category := range catalog.Categories {
		for _, skill := range category.Skills {
			expected[skill.Key] = skill
		}
	}
	published, hidden := 0, 0
	for key, skill := range expected {
		decision, ok := m.Skills[key]
		if !ok {
			return fmt.Errorf("skill catalog: review decision missing for %s", key)
		}
		switch strings.TrimSpace(decision.Decision) {
		case "publish":
			published++
			if skill.SourceNeeded || decision.SourceNeeded {
				return fmt.Errorf("skill catalog: sourceNeeded skill %s cannot publish", key)
			}
			if skill.ConditionalRelease && len(nonEmptyStrings(decision.RiskNotices)) == 0 {
				return fmt.Errorf("skill catalog: conditional skill %s requires risk notice", key)
			}
		case "hide":
			hidden++
			if skill.SourceNeeded && !decision.SourceNeeded {
				return fmt.Errorf("skill catalog: sourceNeeded decision missing for %s", key)
			}
		default:
			return fmt.Errorf("skill catalog: invalid review decision for %s", key)
		}
	}
	for key := range m.Skills {
		if _, ok := expected[key]; !ok {
			return fmt.Errorf("skill catalog: unknown review decision %s", key)
		}
	}
	if len(m.Skills) != len(expected) {
		return fmt.Errorf("skill catalog: review manifest must cover exactly %d skills", len(expected))
	}
	if published != 32 || hidden != 3 {
		return fmt.Errorf("skill catalog: product baseline requires 32 publish and 3 hide decisions, got %d/%d", published, hidden)
	}
	return nil
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
