package videoproject

import (
	"context"
	"fmt"
	"strings"
)

// PromptBuilder 智能提示词引擎：按即梦最佳实践把角色/场景/动作/风格组装为
// 结构化提示词，并按优先级挑选参考图片与参考视频，是降低抽卡率的核心。
//
// 提示词结构公式：主体 + 动作 + 环境 + 风格 + 光照 + 镜头 + 质量词
// 参考素材优先级：上一镜头尾帧 > 角色标准照 > 场景参考图（图片最多 4 张，视频最多 2 个）
type PromptBuilder struct {
	store *Store
}

func NewPromptBuilder(store *Store) *PromptBuilder {
	return &PromptBuilder{store: store}
}

// ShotPreview 是生成前的完整预览：提示词、参考素材、校验结果与预估成功率。
type ShotPreview struct {
	EstimatedSuccessRate int      `json:"estimatedSuccessRate"`
	Images               []string `json:"images"`
	Prompt               string   `json:"prompt"`
	Validation           struct {
		Errors   []string `json:"errors"`
		IsValid  bool     `json:"isValid"`
		Warnings []string `json:"warnings"`
	} `json:"validation"`
	Videos []string `json:"videos"`
}

// 即梦容易失败或与动画风格冲突的词，组装后统一剔除。
var promptBlacklist = []string{"nsfw", "gore", "violence", "photorealistic"}

// 中文动作 → 英文动作短语的基础词典。匹配不到时原样保留（即梦支持中文提示词，
// 但英文动作短语在动画风格下表现更稳定）。
var actionDictionary = map[string]string{
	"走进":   "walking into",
	"走":    "walking slowly and gracefully",
	"跑":    "running swiftly",
	"奔跑":   "running swiftly with determination",
	"看":    "looking",
	"四处张望": "looking around curiously",
	"东张西望": "looking around curiously",
	"笑":    "smiling warmly",
	"微笑":   "smiling gently",
	"转身":   "turning around smoothly",
	"挥手":   "waving hand gently",
	"坐":    "sitting",
	"站":    "standing",
	"加快脚步": "walking faster with excitement",
	"停下":   "stopping and pausing",
	"抬头":   "looking up",
	"低头":   "looking down",
	"推开":   "pushing open",
	"拿起":   "picking up",
	"蹲下":   "crouching down",
}

// BuildPreview 组装分镜的完整生成参数（不实际提交生成）。
func (b *PromptBuilder) BuildPreview(ctx context.Context, shotID string) (ShotPreview, error) {
	shot, err := b.store.GetShot(ctx, shotID)
	if err != nil {
		return ShotPreview{}, err
	}

	project, err := b.store.GetProject(ctx, shot.ProjectID)
	if err != nil {
		return ShotPreview{}, err
	}

	characters, err := b.resolveCharacters(ctx, shot)
	if err != nil {
		return ShotPreview{}, err
	}

	var scene *Scene
	if shot.SceneID != "" {
		sc, err := b.store.getScene(ctx, shot.SceneID)
		if err == nil {
			scene = &sc
		}
	}

	var prevShot *Shot
	if prev, ok, err := b.store.PreviousShot(ctx, shot.ProjectID, shot.OrderNum); err == nil && ok {
		prevShot = &prev
	}

	preview := ShotPreview{}
	preview.Prompt = b.buildPrompt(shot, characters, scene, project.StyleGuide)
	preview.Images = b.buildReferenceImages(shot, characters, scene, prevShot)
	preview.Videos = b.buildReferenceVideos(shot, scene, prevShot)

	errors, warnings := b.validatePrompt(preview.Prompt, characters)
	preview.Validation.Errors = errors
	preview.Validation.Warnings = warnings
	preview.Validation.IsValid = len(errors) == 0

	preview.EstimatedSuccessRate = b.estimateSuccessRate(shot, characters, preview)
	return preview, nil
}

func (b *PromptBuilder) resolveCharacters(ctx context.Context, shot Shot) ([]Character, error) {
	if len(shot.CharacterIDs) == 0 {
		return nil, nil
	}
	all, err := b.store.ListCharacters(ctx, shot.ProjectID)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]bool, len(shot.CharacterIDs))
	for _, id := range shot.CharacterIDs {
		wanted[id] = true
	}
	selected := []Character{}
	for _, c := range all {
		if wanted[c.ID] {
			selected = append(selected, c)
		}
	}
	return selected, nil
}

// buildPrompt 按「主体+动作+环境+风格+光照+镜头+质量词」结构组装提示词。
func (b *PromptBuilder) buildPrompt(shot Shot, characters []Character, scene *Scene, styleGuide string) string {
	parts := []string{}

	// 1. 主体：角色详细描述（人物一致性的文本保障）。
	for _, char := range characters {
		desc := strings.TrimSpace(char.Description)
		if desc == "" {
			desc = char.Name
		}
		parts = append(parts, desc)
	}

	// 2. 动作：中文描述翻译增强。
	if action := b.enhanceAction(shot.ActionDescription); action != "" {
		parts = append(parts, action)
	}

	// 3. 环境：场景详细描述。
	if scene != nil {
		desc := strings.TrimSpace(scene.Description)
		if desc == "" {
			desc = scene.Name
		}
		if desc != "" {
			parts = append(parts, desc)
		}
	}

	// 4. 风格：全局风格锁定（跨分镜一致性）。
	style := strings.TrimSpace(styleGuide)
	if style == "" {
		style = "high quality animation style"
	}
	parts = append(parts, style)

	// 5. 光照：按场景类型自动选择。
	parts = append(parts, b.selectLighting(scene))

	// 6. 镜头：中景最稳定；运镜默认静态，减少画面崩坏。
	parts = append(parts, "medium shot")
	movement := strings.TrimSpace(shot.CameraMovement)
	if movement == "" {
		movement = "static camera"
	}
	parts = append(parts, movement)

	// 7. 质量词。
	parts = append(parts, "high quality, detailed, smooth animation")

	prompt := strings.Join(parts, ", ")

	// 剔除禁用词，避免与动画风格冲突或触发内容审核。
	lower := strings.ToLower(prompt)
	for _, word := range promptBlacklist {
		if idx := strings.Index(lower, word); idx >= 0 {
			prompt = prompt[:idx] + prompt[idx+len(word):]
			lower = strings.ToLower(prompt)
		}
	}
	return strings.TrimSpace(prompt)
}

// enhanceAction 把中文动作描述转换为英文动作短语；未命中词典时保留原文。
func (b *PromptBuilder) enhanceAction(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return ""
	}
	// 长词优先匹配，避免「走进」被「走」提前替换。
	best := ""
	for cn := range actionDictionary {
		if strings.Contains(action, cn) && len(cn) > len(best) {
			best = cn
		}
	}
	if best != "" {
		return strings.ReplaceAll(action, best, actionDictionary[best])
	}
	return action
}

// selectLighting 按场景名称/描述关键词自动匹配光照方案。
func (b *PromptBuilder) selectLighting(scene *Scene) string {
	if scene == nil {
		return "soft natural lighting"
	}
	text := strings.ToLower(scene.Name + " " + scene.Description)
	switch {
	case strings.Contains(text, "night") || strings.Contains(text, "夜"):
		return "moonlight, atmospheric lighting"
	case strings.Contains(text, "forest") || strings.Contains(text, "森林") || strings.Contains(text, "树林"):
		return "dappled sunlight through trees, soft natural lighting"
	case strings.Contains(text, "indoor") || strings.Contains(text, "室内") || strings.Contains(text, "屋"):
		return "warm indoor lighting"
	case strings.Contains(text, "sunset") || strings.Contains(text, "日落") || strings.Contains(text, "黄昏"):
		return "golden hour lighting, warm tones"
	default:
		return "soft natural lighting"
	}
}

// buildReferenceImages 按优先级组装参考图片（最多 4 张）：
// 上一镜头尾帧（连贯性） > 角色标准照（人物一致性） > 场景参考图（环境约束）。
func (b *PromptBuilder) buildReferenceImages(shot Shot, characters []Character, scene *Scene, prevShot *Shot) []string {
	modes := make(map[string]bool, len(shot.ImageReferenceModes))
	for _, m := range shot.ImageReferenceModes {
		modes[m] = true
	}

	images := []string{}
	seen := map[string]bool{}
	add := func(url string) {
		url = strings.TrimSpace(url)
		if url == "" || seen[url] || len(images) >= 4 {
			return
		}
		seen[url] = true
		images = append(images, url)
	}

	// 优先级1：上一镜头尾帧。
	if modes["prev_frame"] && prevShot != nil {
		add(prevShot.EndFrameURL)
	}

	// 优先级2：角色标准照（主角优先）。
	if modes["character_ref"] {
		for _, char := range characters {
			if char.IsMain {
				add(char.ReferenceImageURL)
			}
		}
		for _, char := range characters {
			if !char.IsMain {
				add(char.ReferenceImageURL)
			}
		}
	}

	// 优先级3：场景参考图。
	if modes["scene_ref"] && scene != nil {
		add(scene.ReferenceImageURL)
	}

	return images
}

// buildReferenceVideos 组装参考视频（最多 2 个）。
func (b *PromptBuilder) buildReferenceVideos(shot Shot, scene *Scene, prevShot *Shot) []string {
	videos := []string{}
	switch shot.VideoReferenceMode {
	case "prev_video":
		if prevShot != nil && strings.TrimSpace(prevShot.VideoURL) != "" {
			videos = append(videos, prevShot.VideoURL)
		}
	case "scene_demo":
		if scene != nil && strings.TrimSpace(scene.ReferenceVideoURL) != "" {
			videos = append(videos, scene.ReferenceVideoURL)
		}
	}
	return videos
}

// validatePrompt 生成前校验：错误阻断提交，警告仅提示。
func (b *PromptBuilder) validatePrompt(prompt string, characters []Character) (errors, warnings []string) {
	errors = []string{}
	warnings = []string{}

	if len(prompt) > 800 {
		warnings = append(warnings, "提示词过长（超过 800 字符），建议精简角色或场景描述")
	}
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "realistic") && strings.Contains(lower, "animation") {
		errors = append(errors, "风格冲突：不能同时要求 realistic 和 animation")
	}
	for _, char := range characters {
		if strings.TrimSpace(char.Description) == "" {
			warnings = append(warnings, fmt.Sprintf("角色「%s」缺少详细描述，人物一致性会下降", char.Name))
		}
		if strings.TrimSpace(char.ReferenceImageURL) == "" {
			warnings = append(warnings, fmt.Sprintf("角色「%s」缺少标准照，建议上传参考图以保持人物一致", char.Name))
		}
	}
	return errors, warnings
}

// estimateSuccessRate 预估成功率：基础 50%，参考素材逐项加成，封顶 95%。
func (b *PromptBuilder) estimateSuccessRate(shot Shot, characters []Character, preview ShotPreview) int {
	rate := 50

	// 上一帧继承 +20（连贯性最强的约束）。
	for _, m := range shot.ImageReferenceModes {
		if m == "prev_frame" && shot.OrderNum > 1 && len(preview.Images) > 0 {
			rate += 20
			break
		}
	}

	// 角色标准照 +15。
	hasCharRef := false
	for _, char := range characters {
		if strings.TrimSpace(char.ReferenceImageURL) != "" {
			hasCharRef = true
			break
		}
	}
	if hasCharRef {
		rate += 15
	}

	// 视频参考 +10。
	if len(preview.Videos) > 0 {
		rate += 10
	}

	// 角色有详细描述 +5。
	for _, char := range characters {
		if strings.TrimSpace(char.Description) != "" {
			rate += 5
			break
		}
	}

	if rate > 95 {
		rate = 95
	}
	return rate
}
