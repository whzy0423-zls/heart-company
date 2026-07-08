package videoproject

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
	"nine-xing/nx-backend/apps/server/internal/video"
)

// Generator 分镜视频生成器：调用 PromptBuilder 组装参数，复用现有 video.Generate，
// 生成完成后自动提取尾帧供下一镜头继承。
type Generator struct {
	promptBuilder  *PromptBuilder
	store          *Store
	uploader       storage.ObjectUploader
	uploads        *uploadasset.Store
	videoStore     *video.Store
	frameExtractor *FrameExtractor
}

func NewGenerator(store *Store, videoStore *video.Store, uploads *uploadasset.Store, uploader storage.ObjectUploader) *Generator {
	return &Generator{
		promptBuilder:  NewPromptBuilder(store),
		store:          store,
		uploader:       uploader,
		uploads:        uploads,
		videoStore:     videoStore,
		frameExtractor: NewFrameExtractor("/tmp"),
	}
}

// GenerateShot 提交分镜视频生成任务，立即返回 video.Generation（异步生成）。
// 后台监控完成后自动提取尾帧。
func (g *Generator) GenerateShot(ctx context.Context, shotID string) (video.Generation, error) {
	// 1. 构建预览（提示词+参考素材）
	preview, err := g.promptBuilder.BuildPreview(ctx, shotID)
	if err != nil {
		return video.Generation{}, err
	}

	// 2. 验证提示词
	if !preview.Validation.IsValid {
		return video.Generation{}, fmt.Errorf("提示词验证失败：%s", strings.Join(preview.Validation.Errors, "; "))
	}

	// 3. 获取 Shot 信息
	shot, err := g.store.GetShot(ctx, shotID)
	if err != nil {
		return video.Generation{}, err
	}

	// 4. 调用现有的视频生成 API（复用现有逻辑，不改动）
	input := video.GenerateInput{
		AspectRatio: shot.AspectRatio,
		Images:      preview.Images,
		Model:       "video-ds-2.0-fast",
		Prompt:      preview.Prompt,
		Seconds:     shot.Duration,
		Videos:      preview.Videos,
	}

	generation, err := g.videoStore.Generate(ctx, input)
	if err != nil {
		return video.Generation{}, err
	}

	// 5. 记录到 Shot
	if err := g.store.MarkShotGenerating(ctx, shotID, generation.ID, preview.Prompt, preview.Images, preview.Videos); err != nil {
		return video.Generation{}, err
	}

	// 6. 异步监控生成状态
	go g.monitorGeneration(shotID, generation.ID)

	return generation, nil
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
