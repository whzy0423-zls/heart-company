package skillcatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadApprovedSkillSourcesIncludesKnowledgeAndExcludesRuntimeMetadata(t *testing.T) {
	root := t.TempDir()
	writeSkillTestFile(t, root, "SKILL.md", "# Skill\n行为说明")
	writeSkillTestFile(t, root, "chapters/ch01.md", "# 第一章\n章节里的唯一知识")
	writeSkillTestFile(t, root, "glossary.md", "# 术语\n软区域")
	writeSkillTestFile(t, root, "patterns.md", "# 模式\n划小圈")
	writeSkillTestFile(t, root, "cheatsheet.md", "# 速查\n练习清单")
	writeSkillTestFile(t, root, "agents/openai.yaml", "不应导入")
	writeSkillTestFile(t, root, "validation-report.md", "不应导入")

	sources, err := loadApprovedSkillSources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 5 {
		t.Fatalf("sources=%+v", sources)
	}
	var combined strings.Builder
	for _, source := range sources {
		combined.WriteString(source.RelativePath)
		combined.WriteString(source.Content)
	}
	text := combined.String()
	for _, expected := range []string{"SKILL.md", "chapters/ch01.md", "章节里的唯一知识", "glossary.md", "patterns.md", "cheatsheet.md"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("approved sources missing %q: %s", expected, text)
		}
	}
	for _, forbidden := range []string{"agents/openai.yaml", "validation-report.md", "不应导入"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("runtime metadata leaked into sources: %q", forbidden)
		}
	}
}

func TestSkillSourcesHashIncludesRelativePathAndChapterContent(t *testing.T) {
	base := []approvedSkillSource{{RelativePath: "SKILL.md", Content: "same"}, {RelativePath: "chapters/a.md", Content: "chapter A"}}
	changedContent := []approvedSkillSource{{RelativePath: "SKILL.md", Content: "same"}, {RelativePath: "chapters/a.md", Content: "chapter B"}}
	changedPath := []approvedSkillSource{{RelativePath: "SKILL.md", Content: "same"}, {RelativePath: "chapters/b.md", Content: "chapter A"}}
	if skillSourcesHash(base) == skillSourcesHash(changedContent) || skillSourcesHash(base) == skillSourcesHash(changedPath) {
		t.Fatal("source hash must cover file path and chapter content")
	}
}

func TestCompiledSkillInstructionsUseOnlyPublishedSkillRules(t *testing.T) {
	sources := []approvedSkillSource{
		{RelativePath: "SKILL.md", Content: "---\nname: art-of-learning\ndescription: test\n---\n# 学习之道\n介绍性知识不能提升。\n\n## 何时使用\n先确认用户目标。\n\n## 核心框架\n划小圈属于检索知识。\n\n## 默认工作流\n先定位，再行动。\n\n## 输出要求\n给出可观察的下一步。\n\n## 章节导航\n不进入系统规则。"},
		{RelativePath: "chapters/ch01.md", Content: "章节知识不应提升为行为规则"},
	}
	got := compiledSkillInstructions(
		BuiltinSkill{Key: "art-of-learning", Name: "学习之道"},
		BuiltinCategory{Name: "学习与成长"},
		sources,
	)
	for _, expected := range []string{"受控行为规则", "## 何时使用", "先确认用户目标", "## 默认工作流", "先定位，再行动", "给出可观察的下一步"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("instructions missing %q: %s", expected, got)
		}
	}
	for _, forbidden := range []string{"description: test", "介绍性知识不能提升", "核心框架", "划小圈属于检索知识", "章节导航", "章节知识不应提升为行为规则"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("instructions leaked %q: %s", forbidden, got)
		}
	}
}

func TestReviewManifestRequiresExplicitAuditableDecisions(t *testing.T) {
	catalog := LearningGrowthBuiltinCatalog()
	manifest := ReviewManifest{
		SchemaVersion:  1,
		CatalogKey:     catalog.Key,
		CatalogVersion: "1.0.0",
		ReviewPolicy:   "product-baseline-v1",
		DecisionRef:    "xinzhili-skill-library-baseline-2026-08-19",
		Skills:         map[string]SkillReviewDecision{},
	}
	for _, category := range catalog.Categories {
		for _, skill := range category.Skills {
			decision := SkillReviewDecision{Decision: "publish"}
			if skill.SourceNeeded {
				decision = SkillReviewDecision{Decision: "hide", SourceNeeded: true, RiskNotices: []string{"来源待补"}}
			} else if skill.ConditionalRelease {
				decision.RiskNotices = []string{"内容或来源存在已知限制，公开展示时必须提示"}
			}
			manifest.Skills[skill.Key] = decision
		}
	}
	if err := manifest.Validate(catalog, "1.0.0"); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	delete(manifest.Skills, "art-of-learning")
	if err := manifest.Validate(catalog, "1.0.0"); err == nil || !strings.Contains(err.Error(), "art-of-learning") {
		t.Fatalf("missing decision accepted: %v", err)
	}
}

func TestReviewManifestRejectsUnsafePublishDecisions(t *testing.T) {
	catalog := LearningGrowthBuiltinCatalog()
	manifest := validReviewManifestForTest(catalog)
	manifest.Skills["deliberate-practice"] = SkillReviewDecision{Decision: "publish", SourceNeeded: true}
	if err := manifest.Validate(catalog, "1.0.0"); err == nil || !strings.Contains(err.Error(), "sourceNeeded") {
		t.Fatalf("source-needed skill published: %v", err)
	}

	manifest = validReviewManifestForTest(catalog)
	manifest.Skills["american-higher-education-21c-partial"] = SkillReviewDecision{Decision: "publish"}
	if err := manifest.Validate(catalog, "1.0.0"); err == nil || !strings.Contains(err.Error(), "risk notice") {
		t.Fatalf("conditional skill without risk notice accepted: %v", err)
	}
}

func TestLoadReviewManifestPreservesMachineAuditablePolicy(t *testing.T) {
	manifest := validReviewManifestForTest(LearningGrowthBuiltinCatalog())
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, digest, err := LoadReviewManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ReviewPolicy != "product-baseline-v1" || loaded.DecisionRef == "" || len(digest) != 64 {
		t.Fatalf("loaded=%+v digest=%q", loaded, digest)
	}
}

func TestValidateCatalogCommandRequiresSemanticVersionAndExplicitAction(t *testing.T) {
	for _, command := range []CatalogCommand{
		{Action: "publish", Version: "v1", SourceDir: "/tmp/source", ManifestPath: "/tmp/review.json"},
		{Action: "publish", Version: "1.0.0", SourceDir: "/tmp/source"},
		{Action: "unknown", Version: "1.0.0"},
	} {
		if err := command.Validate(); err == nil {
			t.Fatalf("invalid command accepted: %+v", command)
		}
	}
	for _, action := range []string{"draft", "ready", "publish", "retire", "rollback"} {
		command := CatalogCommand{Action: action, Version: "1.2.3", SourceDir: "/tmp/source", ManifestPath: "/tmp/review.json"}
		if action == "retire" || action == "rollback" {
			command.SourceDir, command.ManifestPath = "", ""
		}
		if err := command.Validate(); err != nil {
			t.Fatalf("action %s rejected: %v", action, err)
		}
	}
}

func validReviewManifestForTest(catalog BuiltinCatalog) ReviewManifest {
	manifest := ReviewManifest{SchemaVersion: 1, CatalogKey: catalog.Key, CatalogVersion: "1.0.0", ReviewPolicy: "product-baseline-v1", DecisionRef: "baseline", Skills: map[string]SkillReviewDecision{}}
	for _, category := range catalog.Categories {
		for _, skill := range category.Skills {
			decision := SkillReviewDecision{Decision: "publish"}
			if skill.SourceNeeded {
				decision = SkillReviewDecision{Decision: "hide", SourceNeeded: true, RiskNotices: []string{"来源待补"}}
			} else if skill.ConditionalRelease {
				decision.RiskNotices = []string{"已知限制"}
			}
			manifest.Skills[skill.Key] = decision
		}
	}
	return manifest
}

func TestLoadApprovedSkillSourcesRejectsActiveContent(t *testing.T) {
	for _, content := range []string{
		"# Skill\n<script>alert('x')</script>",
		"# Skill\n[运行](javascript:alert(1))",
		"# Skill\n[本地文件](file:///etc/passwd)",
	} {
		root := t.TempDir()
		writeSkillTestFile(t, root, "SKILL.md", content)
		if _, err := loadApprovedSkillSources(root); err == nil {
			t.Fatalf("active content accepted: %s", content)
		}
	}
}

func TestSkillOpeningPromptsAreSkillSpecific(t *testing.T) {
	prompts := skillOpeningPrompts(
		BuiltinSkill{Key: "systems-thinking", Name: "系统思考"},
		BuiltinCategory{Name: "思考与决策"},
	)
	if len(prompts) != 3 {
		t.Fatalf("prompts=%+v", prompts)
	}
	for _, prompt := range prompts {
		if !strings.Contains(prompt, "系统思考") && !strings.Contains(prompt, "思考与决策") {
			t.Fatalf("generic opening prompt remained: %q", prompt)
		}
	}
}

func writeSkillTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
