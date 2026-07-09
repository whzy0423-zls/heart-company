package videoproject

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

// ProjectComposer 项目视频合成器：将项目的所有分镜合成为完整视频
type ProjectComposer struct {
	store    *Store
	composer *Composer
	uploader storage.ObjectUploader
	uploads  *uploadasset.Store
}

func NewProjectComposer(store *Store, uploader storage.ObjectUploader, uploads *uploadasset.Store) *ProjectComposer {
	return &ProjectComposer{
		store:    store,
		composer: NewComposer("/tmp"),
		uploader: uploader,
		uploads:  uploads,
	}
}

// ComposeProjectInput 合成输入参数
type ComposeProjectInput struct {
	Transition      string `json:"transition"`      // 转场效果
	MusicURL        string `json:"musicUrl"`        // 背景音乐
	EnableSubtitles bool   `json:"enableSubtitles"` // 是否添加字幕
}

// ComposeProjectResult 合成结果
type ComposeProjectResult struct {
	ProjectID    string  `json:"projectId"`
	VideoURL     string  `json:"videoUrl"`
	Duration     float64 `json:"duration"`
	FileSize     int64   `json:"fileSize"`
	ShotCount    int     `json:"shotCount"`
	Status       string  `json:"status"`
	ErrorMessage string  `json:"errorMessage"`
}

// ComposeProject 合成项目视频
func (pc *ProjectComposer) ComposeProject(ctx context.Context, projectID string, input ComposeProjectInput) (ComposeProjectResult, error) {
	// 1. 获取项目信息
	project, err := pc.store.GetProject(ctx, projectID)
	if err != nil {
		return ComposeProjectResult{}, fmt.Errorf("获取项目失败: %v", err)
	}

	// 2. 获取所有已完成的分镜（按顺序）
	shots, err := pc.store.ListShots(ctx, projectID)
	if err != nil {
		return ComposeProjectResult{}, fmt.Errorf("获取分镜列表失败: %v", err)
	}

	// 过滤出已完成的分镜
	completedShots := []Shot{}
	for _, shot := range shots {
		if shot.Status == "completed" && shot.VideoURL != "" {
			completedShots = append(completedShots, shot)
		}
	}

	if len(completedShots) == 0 {
		return ComposeProjectResult{}, fmt.Errorf("没有已完成的分镜视频")
	}

	// 3. 提取视频 URL 列表
	videoURLs := make([]string, len(completedShots))
	for i, shot := range completedShots {
		videoURLs[i] = shot.VideoURL
	}

	log.Printf("开始合成项目 %s 的 %d 个分镜视频", projectID, len(videoURLs))

	// 4. 调用 Composer 合成视频
	composeOpts := ComposeOptions{
		Transition:      input.Transition,
		MusicURL:        input.MusicURL,
		EnableSubtitles: input.EnableSubtitles,
		TransitionDur:   1.0,
	}

	composeResult, err := pc.composer.ComposeVideos(ctx, videoURLs, composeOpts)
	if err != nil {
		return ComposeProjectResult{}, fmt.Errorf("合成视频失败: %v", err)
	}
	defer os.Remove(composeResult.OutputPath)

	log.Printf("视频合成完成，本地路径: %s, 时长: %.2fs, 大小: %d bytes",
		composeResult.OutputPath, composeResult.Duration, composeResult.FileSize)

	// 5. 上传合成的视频
	videoURL, err := pc.uploadComposedVideo(ctx, composeResult.OutputPath, project.Name)
	if err != nil {
		return ComposeProjectResult{}, fmt.Errorf("上传视频失败: %v", err)
	}

	log.Printf("视频上传完成，URL: %s", videoURL)

	// 6. 更新项目记录
	if err := pc.updateProjectFinalVideo(ctx, projectID, videoURL); err != nil {
		log.Printf("更新项目记录失败: %v", err)
	}

	return ComposeProjectResult{
		ProjectID: projectID,
		VideoURL:  videoURL,
		Duration:  composeResult.Duration,
		FileSize:  composeResult.FileSize,
		ShotCount: len(completedShots),
		Status:    "completed",
	}, nil
}

// uploadComposedVideo 上传合成后的视频文件
func (pc *ProjectComposer) uploadComposedVideo(ctx context.Context, localPath, projectName string) (string, error) {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	// 生成文件名
	filename := fmt.Sprintf("%s_final_%d.mp4", sanitizeFilename(projectName),
		os.Getpid())

	// 优先使用 uploader（OSS）
	if pc.uploader != nil {
		result, err := pc.uploader.Upload(ctx, storage.UploadInput{
			ContentType: "video/mp4",
			Dir:         "video/composed",
			Filename:    filename,
			Reader:      bytes.NewReader(data),
			Size:        int64(len(data)),
		})
		if err != nil {
			return "", err
		}
		return result.URL, nil
	}

	// 否则使用 upload_assets
	asset, err := pc.uploads.Create(ctx, uploadasset.CreateInput{
		ContentType: "video/mp4",
		Data:        data,
		Dir:         "video/composed",
		Name:        filename,
		Size:        int64(len(data)),
	})
	if err != nil {
		return "", err
	}

	return asset.ObjectURL, nil
}

// updateProjectFinalVideo 更新项目的最终视频 URL
func (pc *ProjectComposer) updateProjectFinalVideo(ctx context.Context, projectID, videoURL string) error {
	pid, err := parseID(projectID)
	if err != nil {
		return err
	}

	_, err = pc.store.db.ExecContext(ctx,
		`UPDATE video_projects
		 SET final_video_url = $1, compose_status = 'completed', update_time = now()
		 WHERE id = $2`,
		videoURL, pid,
	)
	return err
}

// sanitizeFilename 清理文件名中的非法字符
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return '_'
		}
		return r
	}, name)
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// GetComposeStatus 获取合成状态
func (pc *ProjectComposer) GetComposeStatus(ctx context.Context, projectID string) (map[string]interface{}, error) {
	project, err := pc.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	shots, err := pc.store.ListShots(ctx, projectID)
	if err != nil {
		return nil, err
	}

	completedCount := 0
	for _, shot := range shots {
		if shot.Status == "completed" && shot.VideoURL != "" {
			completedCount++
		}
	}

	return map[string]interface{}{
		"projectId":      projectID,
		"composeStatus":  project.ComposeStatus,
		"finalVideoUrl":  project.FinalVideoURL,
		"totalShots":     len(shots),
		"completedShots": completedCount,
		"canCompose":     completedCount > 0,
	}, nil
}
