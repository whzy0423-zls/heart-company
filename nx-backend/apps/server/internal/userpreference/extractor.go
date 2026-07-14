package userpreference

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Extraction separates instructions that must affect the current answer from
// durable preference changes that should be stored for later conversations.
type Extraction struct {
	CurrentDirectives []string
	Mutations         []Mutation
}

type extractionCandidate struct {
	position    int
	slot        string
	directive   string
	preference  *Preference
	deleteSlot  string
	currentOnly bool
}

type deterministicRule struct {
	pattern     *regexp.Regexp
	slot        string
	category    string
	instruction string
	build       func([]string) string
}

var deterministicRules = []deterministicRule{
	{
		pattern:     regexp.MustCompile(`(?:不要|别|不许)(?:再)?(?:叫我|称呼我(?:为)?|喊我)[“"']?亲爱的`),
		slot:        "addressing.avoid_dear",
		category:    "addressing",
		instruction: "不要使用“亲爱的”等亲昵称呼",
	},
	{
		pattern:  regexp.MustCompile(`(?:以后|今后|之后|这次|本次|这一条|这轮)?(?:都)?(?:叫我|称呼我(?:为)?|喊我)([\p{Han}A-Za-z0-9_-]{1,16})`),
		slot:     "addressing.preferred_name",
		category: "addressing",
		build: func(matches []string) string {
			name := strings.TrimSuffix(strings.TrimSuffix(matches[1], "吧"), "就好")
			if utf8.RuneCountInString(name) == 0 || utf8.RuneCountInString(name) > 12 || name == "亲爱的" {
				return ""
			}
			return "称呼用户为" + name
		},
	},
	{
		pattern:     regexp.MustCompile(`(?:回答|回复)(?:得|时)?(?:再)?(?:短|简短|精简)(?:一点|一些)?|不要(?:再)?长篇大论|少(?:写|说)(?:一点|一些)`),
		slot:        "length.detail_level",
		category:    "length",
		instruction: "回答简短，避免长篇大论",
	},
	{
		pattern:     regexp.MustCompile(`(?:以后|今后|之后)?(?:回答|回复)?(?:再)?(?:详细|展开)(?:一点|一些|说)?|(?:回答|回复)(?:得|时)?更详细`),
		slot:        "length.detail_level",
		category:    "length",
		instruction: "回答更详细",
	},
	{
		pattern:     regexp.MustCompile(`少说教|(?:回答|回复|表达|说话)(?:再)?直接(?:一点|一些)?|直接(?:一点|一些)`),
		slot:        "tone.direct",
		category:    "tone",
		instruction: "表达直接，少说教",
	},
	{
		pattern:     regexp.MustCompile(`(?:不要|别|无需|不用)(?:再)?(?:使用|用)?(?:列表|列清单|分点)|(?i:\bno\s+(?:bullet(?:ed)?\s+)?lists?\b)`),
		slot:        "format.no_lists",
		category:    "format",
		instruction: "不要使用列表",
	},
	{
		pattern:     regexp.MustCompile(`先(?:说|给|写)?结论|结论(?:放|写)在前面`),
		slot:        "format.conclusion_first",
		category:    "format",
		instruction: "先给结论",
	},
	{
		pattern:     regexp.MustCompile(`(?:不要|别|无需|不用)(?:再)?反问(?:我)?|少追问|(?:不要|别|无需|不用)(?:再)?追问`),
		slot:        "interaction.no_followup",
		category:    "interaction",
		instruction: "不要反问或追问",
	},
}

var currentConclusionOnlyPattern = regexp.MustCompile(`(?:这次|本次|这一条|这轮)(?:回答)?只(?:说|给)?结论`)
var oneTurnPattern = regexp.MustCompile(`这次|本次|这一条|这轮|这回`)
var durablePattern = regexp.MustCompile(`以后|今后|之后|每次|一直|长期|都`)
var cancellationPattern = regexp.MustCompile(`取消|忘掉|删除|去掉|恢复默认|不再遵守`)

var cancellationRules = []struct {
	pattern *regexp.Regexp
	slots   []string
}{
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:回答)?(?:简短|详细|长篇|长度)`), []string{"length.detail_level"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:不要|使用)?(?:列表|清单|分点)`), []string{"format.no_lists"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:先给结论|结论优先)`), []string{"format.conclusion_first"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:直接|说教)`), []string{"tone.direct"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:反问|追问)`), []string{"interaction.no_followup"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:亲爱的|亲昵称呼)`), []string{"addressing.avoid_dear"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:叫我|称呼我的名字|称呼要求)`), []string{"addressing.preferred_name"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:所有)?称呼(?:方面)?(?:的)?要求`), []string{"addressing.avoid_dear", "addressing.preferred_name"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:所有)?语气(?:方面)?(?:的)?要求`), []string{"tone.direct", "tone.formality", "tone.warmth"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:所有)?格式(?:方面)?(?:的)?要求`), []string{"format.no_lists", "format.conclusion_first"}},
	{regexp.MustCompile(`(?:取消|忘掉|删除|去掉|恢复默认|不再遵守).{0,20}?(?:所有)?互动(?:方面)?(?:的)?要求`), []string{"interaction.no_followup"}},
}

// Extract applies conservative local rules for common, explicit communication
// instructions. Ambiguous facts, quoted speech, and third-person discussions are
// deliberately left unresolved.
func Extract(message string) Extraction {
	message = strings.TrimSpace(message)
	if message == "" || isFalsePositiveContext(message) {
		return Extraction{}
	}

	candidates := make([]extractionCandidate, 0, 8)
	source := truncateRunes(message, MaxSourceTextRunes)
	oneTurnOnly := oneTurnPattern.MatchString(message) && !durablePattern.MatchString(message)

	for _, rule := range deterministicRules {
		indexes := rule.pattern.FindAllStringSubmatchIndex(message, -1)
		for _, index := range indexes {
			matches := submatches(message, index)
			instruction := rule.instruction
			if rule.build != nil {
				instruction = rule.build(matches)
			}
			if instruction == "" {
				continue
			}
			preference := &Preference{
				Category:    rule.category,
				Slot:        rule.slot,
				Instruction: instruction,
				SourceText:  source,
			}
			candidates = append(candidates, extractionCandidate{
				position:    index[0],
				slot:        rule.slot,
				directive:   instruction,
				preference:  preference,
				currentOnly: oneTurnOnly,
			})
		}
	}

	for _, index := range currentConclusionOnlyPattern.FindAllStringIndex(message, -1) {
		candidates = append(candidates, extractionCandidate{
			position:    index[0],
			slot:        "format.conclusion_first",
			directive:   "只给结论",
			currentOnly: true,
		})
	}

	if cancellationPattern.MatchString(message) {
		for _, rule := range cancellationRules {
			for _, index := range rule.pattern.FindAllStringIndex(message, -1) {
				for _, slot := range rule.slots {
					candidates = append(candidates, extractionCandidate{
						position:   index[1],
						slot:       slot,
						deleteSlot: slot,
					})
				}
			}
		}
	}

	return coalesceCandidates(candidates)
}

func coalesceCandidates(candidates []extractionCandidate) Extraction {
	if len(candidates) == 0 {
		return Extraction{}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].position == candidates[j].position {
			return candidates[i].slot < candidates[j].slot
		}
		return candidates[i].position < candidates[j].position
	})

	latest := make(map[string]extractionCandidate, len(candidates))
	for _, candidate := range candidates {
		latest[candidate.slot] = candidate
	}
	final := make([]extractionCandidate, 0, len(latest))
	for _, candidate := range latest {
		final = append(final, candidate)
	}
	sort.Slice(final, func(i, j int) bool {
		if final[i].position == final[j].position {
			return final[i].slot < final[j].slot
		}
		return final[i].position < final[j].position
	})

	result := Extraction{
		CurrentDirectives: make([]string, 0, len(final)),
		Mutations:         make([]Mutation, 0, len(final)),
	}
	for _, candidate := range final {
		if candidate.deleteSlot != "" {
			result.Mutations = append(result.Mutations, Mutation{DeleteSlot: candidate.deleteSlot})
			continue
		}
		if candidate.directive != "" {
			result.CurrentDirectives = append(result.CurrentDirectives, candidate.directive)
		}
		if !candidate.currentOnly && candidate.preference != nil {
			preference := *candidate.preference
			result.Mutations = append(result.Mutations, Mutation{Upsert: &preference})
		}
	}
	return result
}

func submatches(message string, indexes []int) []string {
	matches := make([]string, len(indexes)/2)
	for i := 0; i < len(indexes); i += 2 {
		if indexes[i] < 0 {
			continue
		}
		matches[i/2] = message[indexes[i]:indexes[i+1]]
	}
	return matches
}

func isFalsePositiveContext(message string) bool {
	lower := strings.ToLower(message)
	if strings.Contains(message, "另一个人") || strings.Contains(message, "别人") ||
		strings.Contains(message, "怎么让他") || strings.Contains(message, "怎么让她") ||
		strings.Contains(message, "让另") {
		return true
	}
	if (strings.Contains(message, "他说") || strings.Contains(message, "她说") ||
		strings.Contains(message, "这句话") || strings.Contains(message, "什么意思")) &&
		containsQuotedSpan(message) {
		return true
	}
	if strings.Contains(lower, "what does") && containsQuotedSpan(message) {
		return true
	}
	return false
}

func containsQuotedSpan(message string) bool {
	quotePairs := [][2]string{{"“", "”"}, {"‘", "’"}, {`"`, `"`}, {"「", "」"}, {"『", "』"}}
	for _, pair := range quotePairs {
		start := strings.Index(message, pair[0])
		if start < 0 {
			continue
		}
		if strings.Index(message[start+len(pair[0]):], pair[1]) >= 0 {
			return true
		}
	}
	return false
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
