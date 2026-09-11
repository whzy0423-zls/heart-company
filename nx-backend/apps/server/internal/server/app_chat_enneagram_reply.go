package server

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type appChatEnneagramReplyPlan struct {
	Enabled             bool
	RequestedTypes      []int
	TypeContracts       []appChatEnneagramTypeContract
	RuntimeInstructions string
	MaxOutputTokens     int
	CompletionTimeout   time.Duration
	SourceLimit         int
	SourceSnippetRunes  int
}

type appChatEnneagramTypeContract struct {
	Number        int
	CanonicalName string
	Dimensions    []appChatEnneagramDimensionContract
}

type appChatEnneagramDimensionContract struct {
	Name        string
	Requirement string
}

var appChatEnneagramCanonicalNames = []string{
	"1号完美型",
	"2号助人型",
	"3号成就型",
	"4号自我型",
	"5号思考型",
	"6号忠诚型",
	"7号活跃型",
	"8号领袖型",
	"9号和平型",
}

var appChatEnneagramDimensionContracts = []appChatEnneagramDimensionContract{
	{Name: "核心欲望", Requirement: "结合可观察的选择或行动，说明该类型持续想获得什么以及背后的动机。"},
	{Name: "核心恐惧", Requirement: "结合容易触发不安的具体情境，说明该类型最害怕失去或面对什么。"},
	{Name: "防御机制", Requirement: "说明该类型在压力或冲突中会自动使用哪种心理防御，以及如何表现。"},
	{Name: "压力表现", Requirement: "用工作、生活或关系中的具体行为，说明该类型承压时会出现什么变化。"},
	{Name: "关系模式", Requirement: "说明该类型如何靠近他人、表达需要并处理边界，点出常见互动循环。"},
	{Name: "成长方向", Requirement: "给出可以立即实践的觉察与行动方向，说明如何从惯性走向更灵活的选择。"},
}

var (
	appChatEnneagramRangePattern     = regexp.MustCompile(`([1-9一二三四五六七八九])\s*(?:到|至|-|—|~)\s*([1-9一二三四五六七八九])`)
	appChatEnneagramNumberPattern    = regexp.MustCompile(`([1-9一二三四五六七八九])\s*号`)
	appChatEnneagramShorthandPattern = regexp.MustCompile(`^\s*([1-9一二三四五六七八九](?:[\s,，、和与及]+[1-9一二三四五六七八九])+)[\s,，、]*这些(?:型|型号|类型)`)
	appChatEnneagramNumericAnchor    = regexp.MustCompile(`[1-9一二三四五六七八九]\s*号\s*(?:人格|性格)|(?:人格|性格)\s*[1-9一二三四五六七八九]\s*号`)
	appChatEnneagramOrdinaryDomain   = regexp.MustCompile(`手机|产品|文件|题|房间|楼|日期`)
	appChatEnneagramShorthandNumbers = regexp.MustCompile(`[1-9一二三四五六七八九]`)
	appChatEnneagramAnchoredNumber   = regexp.MustCompile(`[1-9一二三四五六七八九]`)
)

var appChatEnneagramTypeAliases = []struct {
	name   string
	number int
}{
	{name: "完美型", number: 1},
	{name: "助人型", number: 2},
	{name: "成就型", number: 3},
	{name: "自我型", number: 4},
	{name: "思考型", number: 5},
	{name: "忠诚型", number: 6},
	{name: "活跃型", number: 7},
	{name: "领袖型", number: 8},
	{name: "和平型", number: 9},
}

func buildAppChatEnneagramReplyPlan(question string) appChatEnneagramReplyPlan {
	question = strings.TrimSpace(question)
	if question == "" {
		return appChatEnneagramReplyPlan{}
	}

	canonicalTypes := appChatEnneagramTypesFromCanonicalNames(question)
	shorthandTypes, hasShorthand := appChatEnneagramTypesFromShorthand(question)
	numericTypes := normalizeAppChatEnneagramTypes(appChatEnneagramTypesFromNumericReferences(question))
	hasNineTypesAnchor := strings.Contains(question, "九型")
	hasNumericAnchor := appChatEnneagramNumericAnchor.MatchString(question)
	hasKnowledgeIntent := appChatEnneagramHasKnowledgeIntent(question)
	hasNumericKnowledgeForm := len(numericTypes) == 1 && appChatEnneagramNumberPattern.MatchString(question) && hasKnowledgeIntent
	if !hasNineTypesAnchor && len(canonicalTypes) == 0 && !hasNumericAnchor && !hasNumericKnowledgeForm && !hasShorthand {
		return appChatEnneagramReplyPlan{}
	}
	if !hasKnowledgeIntent && len(canonicalTypes) < 2 && !hasShorthand {
		return appChatEnneagramReplyPlan{}
	}

	if appChatEnneagramOrdinaryDomain.MatchString(question) && !hasNineTypesAnchor && len(canonicalTypes) == 0 {
		return appChatEnneagramReplyPlan{}
	}

	requestedTypes := append([]int(nil), canonicalTypes...)
	requestedTypes = append(requestedTypes, shorthandTypes...)
	requestedTypes = append(requestedTypes, numericTypes...)
	if hasNineTypesAnchor {
		anchorEnd := strings.Index(question, "九型") + len("九型")
		for _, token := range appChatEnneagramAnchoredNumber.FindAllString(question[anchorEnd:], -1) {
			requestedTypes = append(requestedTypes, appChatEnneagramDigit(token))
		}
	}
	requestedTypes = normalizeAppChatEnneagramTypes(requestedTypes)
	if hasNineTypesAnchor && (len(requestedTypes) == 0 || appChatEnneagramRequestsOverview(question)) {
		requestedTypes = []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	}
	if len(requestedTypes) == 0 {
		return appChatEnneagramReplyPlan{}
	}
	return newAppChatEnneagramReplyPlan(requestedTypes)
}

func newAppChatEnneagramReplyPlan(requestedTypes []int) appChatEnneagramReplyPlan {
	contracts := make([]appChatEnneagramTypeContract, 0, len(requestedTypes))
	for _, number := range requestedTypes {
		dimensions := append([]appChatEnneagramDimensionContract(nil), appChatEnneagramDimensionContracts...)
		contracts = append(contracts, appChatEnneagramTypeContract{
			Number:        number,
			CanonicalName: appChatEnneagramCanonicalNames[number-1],
			Dimensions:    dimensions,
		})
	}

	maxOutputTokens := 600 + 320*len(requestedTypes)
	if maxOutputTokens > 3600 {
		maxOutputTokens = 3600
	}
	sourceLimit := 5 + len(requestedTypes)
	if sourceLimit > 14 {
		sourceLimit = 14
	}
	plan := appChatEnneagramReplyPlan{
		Enabled:            true,
		RequestedTypes:     append([]int(nil), requestedTypes...),
		TypeContracts:      contracts,
		MaxOutputTokens:    maxOutputTokens,
		CompletionTimeout:  70 * time.Second,
		SourceLimit:        sourceLimit,
		SourceSnippetRunes: 360,
	}
	plan.RuntimeInstructions = buildAppChatEnneagramRuntimeInstructions(plan.TypeContracts)
	return plan
}

func buildAppChatEnneagramRuntimeInstructions(contracts []appChatEnneagramTypeContract) string {
	var builder strings.Builder
	builder.WriteString("你正在回答 App 主会话中的九型人格知识问题。严格只覆盖以下请求型号，按数字升序展开；不要增加未点名的型号。\n")
	builder.WriteString("每个型号使用给定的完整名称作为标题，并逐项回答六个维度。每个维度只写一条约20至60个中文字符的具体句子，必须落到可观察的情境、动机或行为，不能只写抽象标签。\n")
	for _, contract := range contracts {
		fmt.Fprintf(&builder, "\n## %s\n", contract.CanonicalName)
		for _, dimension := range contract.Dimensions {
			fmt.Fprintf(&builder, "- %s：%s\n", dimension.Name, dimension.Requirement)
		}
	}
	builder.WriteString("\n画像只能用于个性化示例，不能替代任何请求型号或维度。")
	builder.WriteString("即使某个型号知识库资料缺失，仍须保留该型号及全部六个维度，可基于审慎的通用九型知识回答。")
	builder.WriteString("不得推断用户尚未测定的类型，不得将九型内容表述为临床诊断。")
	return builder.String()
}

func appChatEnneagramHasKnowledgeIntent(question string) bool {
	for _, phrase := range []string{
		"是什么", "什么是", "是什么样", "为什么", "如何", "怎么", "有什么区别", "如何表现",
		"核心欲望", "核心恐惧", "防御机制", "压力表现", "关系模式", "成长方向",
		"特点", "解释", "对比", "反馈", "区别", "分别", "哪些", "哪几",
	} {
		if strings.Contains(question, phrase) {
			return true
		}
	}
	return false
}

func appChatEnneagramRequestsOverview(question string) bool {
	compact := strings.ReplaceAll(question, " ", "")
	for _, phrase := range []string{"什么是九型", "九型是什么", "所有类型", "全部类型", "九种类型", "九个类型"} {
		if strings.Contains(compact, phrase) {
			return true
		}
	}
	return false
}

func appChatEnneagramTypesFromCanonicalNames(question string) []int {
	var types []int
	for _, alias := range appChatEnneagramTypeAliases {
		if strings.Contains(question, alias.name) {
			types = append(types, alias.number)
		}
	}
	return types
}

func appChatEnneagramTypesFromShorthand(question string) ([]int, bool) {
	match := appChatEnneagramShorthandPattern.FindStringSubmatch(question)
	if len(match) != 2 {
		return nil, false
	}
	var types []int
	for _, token := range appChatEnneagramShorthandNumbers.FindAllString(match[1], -1) {
		if number := appChatEnneagramDigit(token); number != 0 {
			types = append(types, number)
		}
	}
	return normalizeAppChatEnneagramTypes(types), len(types) > 1
}

func appChatEnneagramTypesFromNumericReferences(question string) []int {
	var types []int
	for _, match := range appChatEnneagramRangePattern.FindAllStringSubmatch(question, -1) {
		start, end := appChatEnneagramDigit(match[1]), appChatEnneagramDigit(match[2])
		if start == 0 || end == 0 {
			continue
		}
		if start > end {
			start, end = end, start
		}
		for number := start; number <= end; number++ {
			types = append(types, number)
		}
	}
	for _, match := range appChatEnneagramNumberPattern.FindAllStringSubmatch(question, -1) {
		if number := appChatEnneagramDigit(match[1]); number != 0 {
			types = append(types, number)
		}
	}
	return types
}

func normalizeAppChatEnneagramTypes(types []int) []int {
	seen := make(map[int]bool, len(types))
	result := make([]int, 0, len(types))
	for _, number := range types {
		if number < 1 || number > 9 || seen[number] {
			continue
		}
		seen[number] = true
		result = append(result, number)
	}
	sort.Ints(result)
	if len(result) == 0 {
		return nil
	}
	return result
}

func appChatEnneagramDigit(value string) int {
	switch value {
	case "1", "一":
		return 1
	case "2", "二":
		return 2
	case "3", "三":
		return 3
	case "4", "四":
		return 4
	case "5", "五":
		return 5
	case "6", "六":
		return 6
	case "7", "七":
		return 7
	case "8", "八":
		return 8
	case "9", "九":
		return 9
	default:
		return 0
	}
}
