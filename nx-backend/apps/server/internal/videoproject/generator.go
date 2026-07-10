package videoproject

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
	"nine-xing/nx-backend/apps/server/internal/video"
)

// Generator 分镜视频生成器：调用 PromptBuilder 组装参数，复用现有 video.Generate，
// 生成完成后自动提取尾帧供下一镜头继承。
type Generator struct {
	promptBuilder      *PromptBuilder
	store              *Store
	uploader           storage.ObjectUploader
	uploads            *uploadasset.Store
	videoStore         normalizedVideoStore
	frameExtractor     *FrameExtractor
	buildPreview       func(context.Context, string) (ShotPreview, error)
	loadShot           func(context.Context, string) (Shot, error)
	markShotGenerating func(context.Context, string, string, string, []string, []string, []string) error
	startMonitor       func(string, string)
}

type normalizedVideoStore interface {
	Capabilities(model string) video.Capabilities
	GenerateNormalized(ctx context.Context, request video.GenerateRequest, scope video.GenerationContext) (video.Generation, error)
	Refresh(ctx context.Context, id string) (video.Generation, error)
}

type GenerateShotInput struct {
	RequestKey        string `json:"requestKey"`
	CapabilityVersion string `json:"capabilityVersion"`
}

func NewGenerator(store *Store, videoStore normalizedVideoStore, uploads *uploadasset.Store, uploader storage.ObjectUploader) *Generator {
	generator := &Generator{
		promptBuilder:  NewPromptBuilder(store, videoStore.Capabilities),
		store:          store,
		uploader:       uploader,
		uploads:        uploads,
		videoStore:     videoStore,
		frameExtractor: NewFrameExtractor("/tmp"),
	}
	generator.buildPreview = generator.promptBuilder.BuildPreview
	generator.loadShot = store.GetShot
	generator.markShotGenerating = store.MarkShotGenerating
	generator.startMonitor = func(shotID, generationID string) {
		go generator.monitorGeneration(shotID, generationID)
	}
	return generator
}

// GenerateShot 提交分镜视频生成任务，立即返回 video.Generation（异步生成）。
// 后台监控完成后自动提取尾帧。
func (g *Generator) GenerateShot(ctx context.Context, shotID string) (video.Generation, error) {
	return g.GenerateShotWithInput(ctx, shotID, GenerateShotInput{})
}

func (g *Generator) GenerateShotWithInput(ctx context.Context, shotID string, input GenerateShotInput) (video.Generation, error) {
	// 1. 构建预览（提示词+参考素材）
	preview, err := g.buildPreview(ctx, shotID)
	if err != nil {
		return video.Generation{}, err
	}

	// 2. 验证提示词
	if !preview.Validation.IsValid {
		return video.Generation{}, fmt.Errorf("提示词验证失败：%s", strings.Join(preview.Validation.Errors, "; "))
	}
	// 3. 获取 Shot 信息
	shot, err := g.loadShot(ctx, shotID)
	if err != nil {
		return video.Generation{}, err
	}

	capabilities := g.videoStore.Capabilities(shot.VideoModel)
	if strings.TrimSpace(input.RequestKey) == "" {
		input.RequestKey, err = video.NewRequestKey()
		if err != nil {
			return video.Generation{}, err
		}
	}
	if strings.TrimSpace(input.CapabilityVersion) == "" {
		input.CapabilityVersion = capabilities.CapabilityVersion
	}
	request, images, videos, audios, err := buildShotGenerateRequest(shot, preview, capabilities, input)
	if err != nil {
		return video.Generation{}, err
	}
	if err := validateGatewayReferenceURLs(images, videos, audios); err != nil {
		return video.Generation{}, err
	}

	// 4. 所有项目生成统一走 video.Store 的规范化校验与提交状态机。
	generation, err := g.videoStore.GenerateNormalized(ctx, request, video.GenerationContext{
		ProjectID: shot.ProjectID,
		ShotID:    shot.ID,
	})
	if err != nil {
		return video.Generation{}, err
	}

	// 5. 记录到 Shot
	if err := g.markShotGenerating(ctx, shotID, generation.ID, preview.Prompt, images, videos, audios); err != nil {
		return video.Generation{}, err
	}

	// 6. 异步监控生成状态
	if g.startMonitor != nil {
		g.startMonitor(shotID, generation.ID)
	}

	return generation, nil
}

func buildShotGenerateRequest(shot Shot, preview ShotPreview, capabilities video.Capabilities, input GenerateShotInput) (video.GenerateRequest, []string, []string, []string, error) {
	references := cloneVideoReferences(preview.References)
	if len(references) == 0 {
		references = previewReferences(shot.ID, preview.Images, preview.Videos, preview.Audios)
	}
	canonical, err := video.CanonicalizeReferences(references)
	if err != nil {
		return video.GenerateRequest{}, nil, nil, nil, err
	}
	images, videos, audios := canonicalPreviewURLs(canonical)
	generateAudio, err := parseGenerateAudioMode(shot.SoundAndPictureTogether)
	if err != nil {
		return video.GenerateRequest{}, nil, nil, nil, err
	}
	resolution := normalizeVideoResolution(shot.VideoResolution)
	if !capabilities.SupportsResolution {
		resolution = ""
	}
	if !capabilities.SupportsGenerateAudio {
		generateAudio = nil
	}
	request := video.GenerateRequest{
		Model:             strings.TrimSpace(capabilities.Model),
		Prompt:            strings.TrimSpace(preview.Prompt),
		Duration:          shot.Duration,
		AspectRatio:       strings.TrimSpace(shot.AspectRatio),
		Resolution:        resolution,
		GenerateAudio:     generateAudio,
		TaskMode:          promptModeFromReferences(canonical),
		References:        make([]video.Reference, 0, len(canonical.References)),
		RequestKey:        strings.TrimSpace(input.RequestKey),
		CapabilityVersion: strings.TrimSpace(input.CapabilityVersion),
	}
	for _, reference := range canonical.References {
		request.References = append(request.References, reference.Reference)
	}
	return request, images, videos, audios, nil
}

func previewReferences(shotID string, images, videos, audios []string) []video.Reference {
	references := make([]video.Reference, 0, len(images)+len(videos)+len(audios))
	add := func(kind, role string, urls []string) {
		for index, rawURL := range urls {
			references = append(references, video.Reference{
				ID:         fmt.Sprintf("%s-%d", kind, index+1),
				Kind:       kind,
				Role:       role,
				URL:        strings.TrimSpace(rawURL),
				SortOrder:  len(references),
				SourceType: "project_preview",
				SourceID:   shotID,
			})
		}
	}
	add("image", "reference_image", images)
	add("video", "reference_video", videos)
	add("audio", "reference_audio", audios)
	return references
}

func cloneVideoReferences(references []video.Reference) []video.Reference {
	cloned := make([]video.Reference, 0, len(references))
	for _, reference := range references {
		copyReference := reference
		if reference.DurationSeconds != nil {
			duration := *reference.DurationSeconds
			copyReference.DurationSeconds = &duration
		}
		cloned = append(cloned, copyReference)
	}
	return cloned
}

func canonicalPreviewURLs(references video.CanonicalReferences) (images, videos, audios []string) {
	for _, reference := range references.References {
		switch reference.Kind {
		case "image":
			images = append(images, reference.URL)
		case "video":
			videos = append(videos, reference.URL)
		case "audio":
			audios = append(audios, reference.URL)
		}
	}
	return images, videos, audios
}

func parseGenerateAudioMode(raw string) (*bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return nil, nil
	case "enabled":
		value := true
		return &value, nil
	case "disabled":
		value := false
		return &value, nil
	default:
		return nil, fmt.Errorf("无效的音画同出模式")
	}
}

func normalizeVideoResolution(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

func validateGatewayReferenceURLs(images, videos, audios []string) error {
	for _, item := range []struct {
		label string
		urls  []string
	}{
		{label: "参考图片", urls: images},
		{label: "参考视频", urls: videos},
		{label: "参考音频", urls: audios},
	} {
		for _, raw := range item.urls {
			url := strings.TrimSpace(raw)
			if url == "" {
				continue
			}
			if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
				continue
			}
			return fmt.Errorf("%s需要阿里云 OSS 文件桶公网 http(s) 地址，请重新上传到文件桶后再生成: %s", item.label, url)
		}
	}
	return nil
}

func requirePublicAssetObjectURL(label, raw string) (string, error) {
	url := strings.TrimSpace(raw)
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url, nil
	}
	return "", fmt.Errorf("%s需要阿里云 OSS 文件桶公网 objectUrl，请配置 OSS_PUBLIC_URL/文件桶公网访问后重新上传", label)
}

// ExtractShotVideoFrame 从指定视频版本抽取一帧，上传到文件桶后创建 image 类型分镜素材。
func (g *Generator) ExtractShotVideoFrame(ctx context.Context, shotID, generationID string) (ShotAsset, error) {
	version, err := g.store.GetShotVideoVersion(ctx, shotID, generationID)
	if err != nil {
		return ShotAsset{}, err
	}
	videoURL := strings.TrimSpace(version.VideoURL)
	if videoURL == "" && g.videoStore != nil {
		if generation, err := g.videoStore.Refresh(ctx, generationID); err == nil {
			videoURL = strings.TrimSpace(generation.VideoURL)
		}
	}
	if videoURL == "" {
		return ShotAsset{}, fmt.Errorf("视频版本还没有可抽帧的视频地址")
	}

	localVideo := fmt.Sprintf("/tmp/video_extract_%d.mp4", time.Now().UnixNano())
	if err := g.downloadFile(videoURL, localVideo); err != nil {
		return ShotAsset{}, fmt.Errorf("下载视频失败: %w", err)
	}
	defer os.Remove(localVideo)

	framePath, err := g.frameExtractor.ExtractFrameAtTime(ctx, localVideo, 0.1)
	if err != nil {
		return ShotAsset{}, err
	}
	defer os.Remove(framePath)

	data, err := os.ReadFile(framePath)
	if err != nil {
		return ShotAsset{}, err
	}

	filename := fmt.Sprintf("shot-%s-generation-%s-frame.jpg", shotID, generationID)
	objectURL, err := g.uploadFrame(ctx, data, filename)
	if err != nil {
		return ShotAsset{}, err
	}

	return g.store.CreateShotAsset(ctx, shotID, ShotAssetInput{
		AssetType: "image",
		MimeType:  "image/jpeg",
		Name:      "视频抽帧",
		ObjectURL: objectURL,
		SizeBytes: int64(len(data)),
	})
}

// RemoveShotVideoVersionSubtitle 处理指定视频版本，移除独立字幕轨并创建“无字幕”派生版本。
func (g *Generator) RemoveShotVideoVersionSubtitle(ctx context.Context, shotID, generationID string) (ShotVideoVersion, error) {
	version, err := g.store.GetShotVideoVersion(ctx, shotID, generationID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	videoURL := strings.TrimSpace(version.VideoURL)
	if videoURL == "" && g.videoStore != nil {
		if generation, err := g.videoStore.Refresh(ctx, generationID); err == nil {
			videoURL = strings.TrimSpace(generation.VideoURL)
		}
	}
	if videoURL == "" {
		return ShotVideoVersion{}, fmt.Errorf("视频版本还没有可擦字幕的视频地址")
	}
	if strings.EqualFold(strings.TrimSpace(version.SubtitleRemove), "REMOVED") {
		return ShotVideoVersion{}, fmt.Errorf("当前视频版本已经是无字幕版本")
	}

	localVideo := fmt.Sprintf("/tmp/video_subtitle_source_%d.mp4", time.Now().UnixNano())
	if err := g.downloadFile(videoURL, localVideo); err != nil {
		return ShotVideoVersion{}, fmt.Errorf("下载视频失败: %w", err)
	}
	defer os.Remove(localVideo)

	outputVideo := fmt.Sprintf("/tmp/video_subtitle_removed_%d.mp4", time.Now().UnixNano())
	if err := g.stripSubtitleTracks(ctx, localVideo, outputVideo); err != nil {
		return ShotVideoVersion{}, err
	}
	defer os.Remove(outputVideo)

	data, err := os.ReadFile(outputVideo)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	filename := fmt.Sprintf("shot-%s-generation-%s-no-subtitle.mp4", shotID, generationID)
	asset, publicURL, err := g.uploadProcessedVideo(ctx, data, filename, "video/subtitle-removed")
	if err != nil {
		return ShotVideoVersion{}, err
	}

	return g.store.CreateSubtitleRemovedShotVideoVersion(ctx, shotID, generationID, asset.ID, publicURL)
}

// UpscaleShotVideoVersion 将指定视频版本转码到目标分辨率，并创建“已超分”派生版本。
func (g *Generator) UpscaleShotVideoVersion(ctx context.Context, shotID, generationID, resolution string) (ShotVideoVersion, error) {
	version, err := g.store.GetShotVideoVersion(ctx, shotID, generationID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	label, width, height, err := parseUpscaleResolution(resolution, version.AspectRatio)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	if version.UpscaledFlag && strings.EqualFold(strings.TrimSpace(version.UpscaledResolution), label) {
		return ShotVideoVersion{}, fmt.Errorf("当前视频版本已经是 %s 超分版本", label)
	}

	videoURL := strings.TrimSpace(version.VideoURL)
	if videoURL == "" && g.videoStore != nil {
		if generation, err := g.videoStore.Refresh(ctx, generationID); err == nil {
			videoURL = strings.TrimSpace(generation.VideoURL)
		}
	}
	if videoURL == "" {
		return ShotVideoVersion{}, fmt.Errorf("视频版本还没有可超分的视频地址")
	}

	localVideo := fmt.Sprintf("/tmp/video_upscale_source_%d.mp4", time.Now().UnixNano())
	if err := g.downloadFile(videoURL, localVideo); err != nil {
		return ShotVideoVersion{}, fmt.Errorf("下载视频失败: %w", err)
	}
	defer os.Remove(localVideo)

	outputVideo := fmt.Sprintf("/tmp/video_upscaled_%d.mp4", time.Now().UnixNano())
	if err := g.upscaleVideoFile(ctx, localVideo, outputVideo, width, height); err != nil {
		return ShotVideoVersion{}, err
	}
	defer os.Remove(outputVideo)

	data, err := os.ReadFile(outputVideo)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	filename := fmt.Sprintf("shot-%s-generation-%s-upscaled-%s.mp4", shotID, generationID, strings.ToLower(label))
	asset, publicURL, err := g.uploadProcessedVideo(ctx, data, filename, "video/upscaled")
	if err != nil {
		return ShotVideoVersion{}, err
	}

	return g.store.CreateUpscaledShotVideoVersion(ctx, shotID, generationID, asset.ID, publicURL, label, width, height)
}

func parseUpscaleResolution(resolution string, aspectRatio string) (string, int, int, error) {
	normalized := strings.TrimSpace(strings.ToLower(resolution))
	if normalized == "" {
		normalized = "1080p"
	}
	target := 0
	switch normalized {
	case "2k":
		target = 1440
	case "4k":
		target = 2160
	default:
		normalized = strings.TrimSuffix(normalized, "p")
		value, err := strconv.Atoi(normalized)
		if err != nil || value <= 0 {
			return "", 0, 0, fmt.Errorf("不支持的超分辨率：%s", resolution)
		}
		target = value
	}
	switch target {
	case 720, 1080, 1440, 2160:
	default:
		return "", 0, 0, fmt.Errorf("超分辨率仅支持 720P、1080P、1440P 或 2160P")
	}

	label := fmt.Sprintf("%dP", target)
	aspect := strings.TrimSpace(aspectRatio)
	switch aspect {
	case "9:16":
		return label, even(target), even(target * 16 / 9), nil
	case "1:1":
		return label, even(target), even(target), nil
	default:
		return label, even(target * 16 / 9), even(target), nil
	}
}

func even(value int) int {
	if value%2 == 0 {
		return value
	}
	return value + 1
}

func (g *Generator) upscaleVideoFile(ctx context.Context, inputPath, outputPath string, width, height int) error {
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("视频文件不存在: %v", err)
	}
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", inputPath,
		"-map", "0:v:0",
		"-map", "0:a?",
		"-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", width, height),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "18",
		"-c:a", "aac",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("超分辨率处理失败: %v, 输出: %s", err, string(output))
	}
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("超分视频未生成: %v", err)
	}
	return nil
}

func (g *Generator) stripSubtitleTracks(ctx context.Context, inputPath, outputPath string) error {
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("视频文件不存在: %v", err)
	}
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", inputPath,
		"-map", "0",
		"-sn",
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("擦除字幕失败: %v, 输出: %s", err, string(output))
	}
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("无字幕视频未生成: %v", err)
	}
	return nil
}

func (g *Generator) uploadProcessedVideo(ctx context.Context, data []byte, filename string, dir string) (uploadasset.Asset, string, error) {
	const contentType = "video/mp4"
	var objectKey string
	var objectURL string
	if g.uploader != nil {
		result, err := g.uploader.Upload(ctx, storage.UploadInput{
			ContentType: contentType,
			Dir:         dir,
			Filename:    filename,
			Reader:      bytes.NewReader(data),
			Size:        int64(len(data)),
		})
		if err != nil {
			return uploadasset.Asset{}, "", err
		}
		objectKey = result.Key
		objectURL = result.URL
		if strings.TrimSpace(result.Name) != "" {
			filename = result.Name
		}
	}
	asset, err := g.uploads.Create(ctx, uploadasset.CreateInput{
		ContentType: contentType,
		Data:        data,
		Dir:         dir,
		Name:        filename,
		ObjectKey:   objectKey,
		ObjectURL:   objectURL,
		Size:        int64(len(data)),
	})
	if err != nil {
		return uploadasset.Asset{}, "", err
	}
	if strings.TrimSpace(asset.ObjectURL) != "" {
		publicURL, err := requirePublicAssetObjectURL("处理后视频", asset.ObjectURL)
		return asset, publicURL, err
	}
	publicURL, err := requirePublicAssetObjectURL("处理后视频", objectURL)
	return asset, publicURL, err
}

func (g *Generator) uploadFrame(ctx context.Context, data []byte, filename string) (string, error) {
	if g.uploader != nil {
		result, err := g.uploader.Upload(ctx, storage.UploadInput{
			ContentType: "image/jpeg",
			Dir:         "video/frames",
			Filename:    filename,
			Reader:      bytes.NewReader(data),
			Size:        int64(len(data)),
		})
		if err != nil {
			return "", err
		}
		return requirePublicAssetObjectURL("视频抽帧", result.URL)
	}

	asset, err := g.uploads.Create(ctx, uploadasset.CreateInput{
		ContentType: "image/jpeg",
		Data:        data,
		Dir:         "video/frames",
		Name:        filename,
		Size:        int64(len(data)),
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(asset.ObjectURL) != "" {
		return requirePublicAssetObjectURL("视频抽帧", asset.ObjectURL)
	}
	return requirePublicAssetObjectURL("视频抽帧", "")
}

// monitorGeneration 后台轮询生成状态，完成后提取尾帧。
func (g *Generator) monitorGeneration(shotID, generationID string) {
	ctx := context.Background()
	for {
		time.Sleep(5 * time.Second)

		// 刷新状态
		gen, err := g.videoStore.Refresh(ctx, generationID)
		if err != nil {
			continue
		}

		switch gen.Status {
		case "completed", "succeeded":
			// 成功：提取尾帧
			endFrameURL, err := g.extractEndFrame(ctx, gen.VideoURL)
			if err != nil {
				_ = g.store.MarkShotCompleted(ctx, shotID, "")
			} else {
				_ = g.store.MarkShotCompleted(ctx, shotID, endFrameURL)
			}
			return

		case "failed":
			// 失败：记录错误
			_ = g.store.MarkShotFailed(ctx, shotID, gen.ErrorMessage)
			return
		}
	}
}

// extractEndFrame 使用 FrameExtractor 提取视频尾帧，上传到 OSS。
func (g *Generator) extractEndFrame(ctx context.Context, videoURL string) (string, error) {
	if videoURL == "" {
		return "", fmt.Errorf("视频 URL 为空")
	}

	// 1. 下载视频到临时文件
	localVideo := fmt.Sprintf("/tmp/video_%d.mp4", time.Now().UnixNano())
	if err := g.downloadFile(videoURL, localVideo); err != nil {
		return "", fmt.Errorf("下载视频失败: %w", err)
	}
	defer os.Remove(localVideo)

	// 2. 使用 FrameExtractor 提取尾帧
	endFramePath, err := g.frameExtractor.ExtractEndFrame(ctx, localVideo)
	if err != nil {
		return "", fmt.Errorf("提取尾帧失败: %w", err)
	}
	defer os.Remove(endFramePath)

	// 3. 上传到 OSS
	data, err := os.ReadFile(endFramePath)
	if err != nil {
		return "", err
	}

	if g.uploader != nil {
		result, err := g.uploader.Upload(ctx, storage.UploadInput{
			ContentType: "image/jpeg",
			Dir:         "video/frames",
			Filename:    filepath.Base(endFramePath),
			Reader:      bytes.NewReader(data),
			Size:        int64(len(data)),
		})
		if err != nil {
			return "", err
		}
		return result.URL, nil
	}

	// 无 uploader 时存储到 upload_assets
	asset, err := g.uploads.Create(ctx, uploadasset.CreateInput{
		ContentType: "image/jpeg",
		Data:        data,
		Dir:         "video/frames",
		Name:        filepath.Base(endFramePath),
		Size:        int64(len(data)),
	})
	if err != nil {
		return "", err
	}
	return asset.ObjectURL, nil
}

// downloadFile 下载 URL 到本地文件（简单实现，生产环境建议增加重试）
func (g *Generator) downloadFile(url, dest string) error {
	cmd := exec.Command("curl", "-L", "-o", dest, url)
	return cmd.Run()
}
