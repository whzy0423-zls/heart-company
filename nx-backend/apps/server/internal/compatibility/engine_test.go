package compatibility

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/quiz"
)

func TestAnalyzeIgnoresLegacyWingValues(t *testing.T) {
	withLegacyWings := Analyze(
		card("甲", 2, 5, `{"summary":"关注关系"}`),
		card("乙", 5, 2, `{"summary":"关注思考"}`),
	)
	withoutLegacyWings := Analyze(
		card("甲", 2, 0, `{"summary":"关注关系"}`),
		card("乙", 5, 0, `{"summary":"关注思考"}`),
	)

	if !reflect.DeepEqual(withLegacyWings, withoutLegacyWings) {
		t.Fatalf("legacy wing values changed compatibility result:\nwith=%+v\nwithout=%+v", withLegacyWings, withoutLegacyWings)
	}
	if withLegacyWings.AlgorithmVersion != "v2" {
		t.Fatalf("algorithm version = %q, want v2", withLegacyWings.AlgorithmVersion)
	}
	raw, err := json.Marshal(withLegacyWings)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, retired := range []string{"wing", "侧翼", "翼型"} {
		if strings.Contains(lower, retired) {
			t.Fatalf("compatibility result contains retired wing content %q: %s", retired, raw)
		}
	}
}

func TestAnalyzeSameTypeProducesStableBond(t *testing.T) {
	result := Analyze(card("阿九", 6, 5, `{"summary":"谨慎可靠"}`), card("小满", 6, 7, `{"summary":"认真守护"}`))

	if result.AlgorithmVersion == "" {
		t.Fatalf("expected algorithm version")
	}
	if result.Level != LevelStable {
		t.Fatalf("expected stable level for same valid type, got %q", result.Level)
	}
	if result.Scores.Resonance < 85 {
		t.Fatalf("expected high resonance for same type, got %d", result.Scores.Resonance)
	}
	if result.Scores.Stability < 78 {
		t.Fatalf("expected strong stability for same type, got %d", result.Scores.Stability)
	}
	if !hasTag(result.ExplainTags, "same_type_resonance") {
		t.Fatalf("expected same_type_resonance tag, got %#v", result.ExplainTags)
	}
	if len(result.Evidence) == 0 || result.Summary == "" || len(result.Highlights) == 0 {
		t.Fatalf("expected evidence and narrative output, got %#v", result)
	}
}

func TestAnalyzeCrossCenterComplementAddsTag(t *testing.T) {
	result := Analyze(card("澄澄", 2, 3, `{}`), card("远山", 5, 6, `{}`))

	if result.Scores.Complement < 80 {
		t.Fatalf("expected cross-center pair to have strong complement, got %d", result.Scores.Complement)
	}
	if !hasTag(result.ExplainTags, "cross_center_complement") {
		t.Fatalf("expected cross_center_complement tag, got %#v", result.ExplainTags)
	}
	if len(result.Highlights) == 0 {
		t.Fatalf("expected highlights")
	}
}

func TestAnalyzeInvalidTypeAndSparseCardFallsBackWithoutPanic(t *testing.T) {
	result := Analyze(quiz.Card{MainType: 42}, quiz.Card{Name: " ", MainType: -3})

	if result.Level == "" {
		t.Fatalf("expected fallback level")
	}
	if !hasTag(result.ExplainTags, "fallback_type") {
		t.Fatalf("expected fallback_type tag, got %#v", result.ExplainTags)
	}
	if !hasTag(result.ExplainTags, "missing_name") {
		t.Fatalf("expected missing_name tag, got %#v", result.ExplainTags)
	}
	if !hasTag(result.ExplainTags, "missing_profile") {
		t.Fatalf("expected missing_profile tag, got %#v", result.ExplainTags)
	}
	for label, score := range map[string]int{
		"resonance":     result.Scores.Resonance,
		"complement":    result.Scores.Complement,
		"communication": result.Scores.Communication,
		"conflictRisk":  result.Scores.ConflictRisk,
		"growth":        result.Scores.Growth,
		"stability":     result.Scores.Stability,
	} {
		if score < 0 || score > 100 {
			t.Fatalf("%s score out of range: %d", label, score)
		}
	}
	if result.Summary == "" || len(result.ConflictPoints) == 0 || len(result.Suggestions) == 0 {
		t.Fatalf("expected safe fallback narrative, got %#v", result)
	}
}

func TestAnalyzeSameInputIsDeterministic(t *testing.T) {
	a := card("一一", 1, 9, `{"keywords":["原则","行动"],"summary":"重视秩序"}`)
	b := card("七七", 7, 6, `{"keywords":["乐观","可能"],"summary":"喜欢探索"}`)

	first := Analyze(a, b)
	for i := 0; i < 10; i++ {
		next := Analyze(a, b)
		if !reflect.DeepEqual(first, next) {
			firstJSON, _ := json.Marshal(first)
			nextJSON, _ := json.Marshal(next)
			t.Fatalf("expected deterministic result\nfirst=%s\nnext=%s", firstJSON, nextJSON)
		}
	}
}

func card(name string, mainType, _ int, profile string) quiz.Card {
	return quiz.Card{
		Name:     name,
		MainType: mainType,
		Profile:  json.RawMessage(profile),
	}
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
