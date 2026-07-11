package videoproject

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

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
	Transition          string   `json:"transition"`      // 转场效果
	MusicURL            string   `json:"musicUrl"`        // 背景音乐
	EnableSubtitles     bool     `json:"enableSubtitles"` // 是否添加字幕
	ExcludedShotIDs     []string `json:"excludedShotIds"`
	PartialAcknowledged bool     `json:"partialAcknowledged"`
}

type ComposeShotFacts struct {
	GenerationRevision   int
	LatestGenerationID   string
	LegacyVideoURL       string
	OrderNum             int
	SelectedGenerationID string
	SelectedRevision     int
	SelectedStatus       string
	SelectedVideoURL     string
	ShotID               string
}

type ComposeParticipant struct {
	GenerationID string `json:"generationId"`
	OrderNum     int    `json:"orderNum"`
	ShotID       string `json:"shotId"`
	ShotRevision int    `json:"shotRevision"`
}

type ComposeInputSnapshot struct {
	EnableSubtitles     bool                 `json:"enableSubtitles"`
	ExcludedShotIDs     []string             `json:"excludedShotIds"`
	Included            []ComposeParticipant `json:"included"`
	InputHash           string               `json:"inputHash"`
	MusicURL            string               `json:"musicUrl"`
	PartialAcknowledged bool                 `json:"partialAcknowledged"`
	Transition          string               `json:"transition"`
}

func BuildComposeInputSnapshot(shots []ComposeShotFacts, input ComposeProjectInput) (ComposeInputSnapshot, error) {
	known := make(map[string]ComposeShotFacts, len(shots))
	for _, shot := range shots {
		known[strings.TrimSpace(shot.ShotID)] = shot
	}
	excluded := normalizedStringSet(input.ExcludedShotIDs)
	if len(excluded) > 0 && !input.PartialAcknowledged {
		return ComposeInputSnapshot{}, fmt.Errorf("partial compose requires per-request acknowledgement")
	}
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, shotID := range excluded {
		if _, exists := known[shotID]; !exists {
			return ComposeInputSnapshot{}, fmt.Errorf("unknown excluded shot %s", shotID)
		}
		excludedSet[shotID] = struct{}{}
	}
	sortedShots := append([]ComposeShotFacts(nil), shots...)
	sort.Slice(sortedShots, func(i, j int) bool {
		if sortedShots[i].OrderNum == sortedShots[j].OrderNum {
			return sortedShots[i].ShotID < sortedShots[j].ShotID
		}
		return sortedShots[i].OrderNum < sortedShots[j].OrderNum
	})
	included := make([]ComposeParticipant, 0, len(sortedShots))
	for _, shot := range sortedShots {
		if _, skip := excludedSet[shot.ShotID]; skip {
			continue
		}
		if strings.TrimSpace(shot.SelectedGenerationID) == "" {
			return ComposeInputSnapshot{}, fmt.Errorf("shot %s has no explicit selected generation", shot.ShotID)
		}
		if !canSelectGeneration(shot.ShotID, shot.ShotID, shot.SelectedStatus, shot.SelectedVideoURL) {
			return ComposeInputSnapshot{}, fmt.Errorf("shot %s selected generation is not successful", shot.ShotID)
		}
		if shot.SelectedRevision != shot.GenerationRevision {
			return ComposeInputSnapshot{}, fmt.Errorf("shot %s selected generation is stale", shot.ShotID)
		}
		included = append(included, ComposeParticipant{
			GenerationID: shot.SelectedGenerationID,
			OrderNum:     shot.OrderNum,
			ShotID:       shot.ShotID,
			ShotRevision: shot.SelectedRevision,
		})
	}
	if len(included) == 0 {
		return ComposeInputSnapshot{}, fmt.Errorf("compose requires at least one included shot")
	}
	snapshot := ComposeInputSnapshot{
		EnableSubtitles:     input.EnableSubtitles,
		ExcludedShotIDs:     excluded,
		Included:            included,
		MusicURL:            strings.TrimSpace(input.MusicURL),
		PartialAcknowledged: input.PartialAcknowledged,
		Transition:          strings.TrimSpace(input.Transition),
	}
	hashPayload := struct {
		EnableSubtitles bool                 `json:"enableSubtitles"`
		ExcludedShotIDs []string             `json:"excludedShotIds"`
		Included        []ComposeParticipant `json:"included"`
		MusicURL        string               `json:"musicUrl"`
		Transition      string               `json:"transition"`
	}{snapshot.EnableSubtitles, snapshot.ExcludedShotIDs, snapshot.Included, snapshot.MusicURL, snapshot.Transition}
	raw, _ := json.Marshal(hashPayload)
	snapshot.InputHash = fmt.Sprintf("%x", sha256.Sum256(raw))
	return snapshot, nil
}

func ComposeResultIsCurrent(storedInputHash, currentInputHash string) bool {
	return strings.TrimSpace(storedInputHash) != "" && storedInputHash == currentInputHash
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

type ComposeJob struct {
	ErrorMessage  string               `json:"error"`
	ID            string               `json:"jobId"`
	InputHash     string               `json:"inputHash"`
	InputSnapshot ComposeInputSnapshot `json:"inputSnapshot"`
	IsCurrent     bool                 `json:"isCurrent"`
	Progress      int                  `json:"progress"`
	ProjectID     string               `json:"projectId"`
	Status        string               `json:"status"`
	VideoURL      string               `json:"videoUrl"`
}

type ComposeActiveJobError struct {
	ProjectID string
}

func (e *ComposeActiveJobError) Error() string {
	return fmt.Sprintf("project %s already has an active compose job", e.ProjectID)
}

func (pc *ProjectComposer) composeShotFacts(ctx context.Context, projectID string) ([]ComposeShotFacts, error) {
	shots, err := pc.store.ListShots(ctx, projectID)
	if err != nil {
		return nil, err
	}
	facts := make([]ComposeShotFacts, 0, len(shots))
	for _, shot := range shots {
		facts = append(facts, ComposeShotFacts{
			GenerationRevision:   shot.GenerationRevision,
			LatestGenerationID:   shot.GenerationID,
			LegacyVideoURL:       shot.VideoURL,
			OrderNum:             shot.OrderNum,
			SelectedGenerationID: shot.SelectedGenerationID,
			SelectedRevision:     shot.SelectedGenerationRevision,
			SelectedStatus:       shot.SelectedGenerationStatus,
			SelectedVideoURL:     shot.VideoURL,
			ShotID:               shot.ID,
		})
	}
	return facts, nil
}

func (pc *ProjectComposer) StartCompose(ctx context.Context, projectID string, input ComposeProjectInput) (ComposeJob, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return ComposeJob{}, err
	}
	shots, err := pc.composeShotFacts(ctx, projectID)
	if err != nil {
		return ComposeJob{}, err
	}
	snapshot, err := BuildComposeInputSnapshot(shots, input)
	if err != nil {
		return ComposeJob{}, err
	}
	rawSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		return ComposeJob{}, err
	}
	var jobID string
	err = pc.store.db.QueryRowContext(ctx, `
		INSERT INTO video_compose_jobs (
		  project_id, status, transition_type, music_url,
		  compose_input_hash, compose_input_snapshot, progress
		) VALUES ($1,'queued',$2,$3,$4,$5::jsonb,0)
		RETURNING id::text`,
		pid, snapshot.Transition, snapshot.MusicURL, snapshot.InputHash, rawSnapshot,
	).Scan(&jobID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "idx_video_compose_jobs_active_project" {
			return ComposeJob{}, &ComposeActiveJobError{ProjectID: projectID}
		}
		return ComposeJob{}, err
	}
	job := ComposeJob{ID: jobID, ProjectID: projectID, Status: "queued", Progress: 0, InputHash: snapshot.InputHash, InputSnapshot: snapshot}
	go pc.runComposeJob(projectID, jobID, snapshot)
	return job, nil
}

func (pc *ProjectComposer) runComposeJob(projectID, jobID string, snapshot ComposeInputSnapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	fail := func(err error) {
		message := "合成任务失败"
		if err != nil {
			message = err.Error()
		}
		_, _ = pc.store.db.ExecContext(ctx, `
			UPDATE video_compose_jobs
			SET status='failed', error_message=$1, update_time=now()
			WHERE id=$2`, message, jobID)
		_, _ = pc.store.db.ExecContext(ctx, `
			UPDATE video_projects SET compose_status='failed', update_time=now() WHERE id=$1`, projectID)
	}
	if _, err := pc.store.db.ExecContext(ctx, `
		UPDATE video_compose_jobs SET status='processing', progress=10, update_time=now() WHERE id=$1`, jobID); err != nil {
		return
	}
	videoURLs := make([]string, 0, len(snapshot.Included))
	for _, participant := range snapshot.Included {
		var videoURL string
		if err := pc.store.db.QueryRowContext(ctx, `
			SELECT video_url FROM video_generations
			WHERE id=$1::bigint AND shot_id=$2::bigint
			  AND status IN ('completed','succeeded') AND video_url<>''`,
			participant.GenerationID, participant.ShotID,
		).Scan(&videoURL); err != nil {
			fail(fmt.Errorf("读取分镜 %s 的选中版本失败: %w", participant.ShotID, err))
			return
		}
		videoURLs = append(videoURLs, videoURL)
	}
	_, _ = pc.store.db.ExecContext(ctx, `UPDATE video_compose_jobs SET progress=30, update_time=now() WHERE id=$1`, jobID)
	result, err := pc.composer.ComposeVideos(ctx, videoURLs, ComposeOptions{
		Transition: snapshot.Transition, MusicURL: snapshot.MusicURL,
		EnableSubtitles: snapshot.EnableSubtitles, TransitionDur: 1,
	})
	if err != nil {
		fail(err)
		return
	}
	defer os.Remove(result.OutputPath)
	_, _ = pc.store.db.ExecContext(ctx, `UPDATE video_compose_jobs SET progress=70, update_time=now() WHERE id=$1`, jobID)
	project, err := pc.store.GetProject(ctx, projectID)
	if err != nil {
		fail(err)
		return
	}
	videoURL, err := pc.uploadComposedVideo(ctx, result.OutputPath, project.Name)
	if err != nil {
		fail(err)
		return
	}
	_, _ = pc.store.db.ExecContext(ctx, `UPDATE video_compose_jobs SET progress=90, update_time=now() WHERE id=$1`, jobID)
	tx, err := pc.store.db.BeginTx(ctx, nil)
	if err != nil {
		fail(err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE video_compose_jobs
		SET status='completed', progress=100, final_video_url=$1, error_message='', update_time=now()
		WHERE id=$2`, videoURL, jobID); err != nil {
		fail(err)
		return
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE video_projects
		SET final_video_url=$1, final_video_input_hash=$2, compose_status='completed', update_time=now()
		WHERE id=$3`, videoURL, snapshot.InputHash, projectID); err != nil {
		fail(err)
		return
	}
	if err := tx.Commit(); err != nil {
		fail(err)
	}
}

func (pc *ProjectComposer) GetComposeJob(ctx context.Context, projectID, jobID string) (ComposeJob, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return ComposeJob{}, err
	}
	jid, err := parseID(jobID)
	if err != nil {
		return ComposeJob{}, err
	}
	_, _ = pc.store.db.ExecContext(ctx, `
		UPDATE video_compose_jobs
		SET status='failed', error_message='服务重启导致合成中断，请重试', update_time=now()
		WHERE project_id=$1 AND status IN ('queued','processing')
		  AND update_time < now() - interval '15 minutes'`, pid)
	var job ComposeJob
	var rawSnapshot []byte
	err = pc.store.db.QueryRowContext(ctx, `
		SELECT id::text, project_id::text, status, progress, error_message,
		       compose_input_hash, compose_input_snapshot, final_video_url
		FROM video_compose_jobs WHERE id=$1 AND project_id=$2`, jid, pid,
	).Scan(&job.ID, &job.ProjectID, &job.Status, &job.Progress, &job.ErrorMessage,
		&job.InputHash, &rawSnapshot, &job.VideoURL)
	if err != nil {
		if err == sql.ErrNoRows {
			return ComposeJob{}, fmt.Errorf("合成任务不存在")
		}
		return ComposeJob{}, err
	}
	if err := json.Unmarshal(rawSnapshot, &job.InputSnapshot); err != nil {
		return ComposeJob{}, err
	}
	shots, err := pc.composeShotFacts(ctx, projectID)
	if err == nil {
		current, buildErr := BuildComposeInputSnapshot(shots, ComposeProjectInput{
			Transition: job.InputSnapshot.Transition, MusicURL: job.InputSnapshot.MusicURL,
			EnableSubtitles: job.InputSnapshot.EnableSubtitles,
			ExcludedShotIDs: job.InputSnapshot.ExcludedShotIDs, PartialAcknowledged: job.InputSnapshot.PartialAcknowledged,
		})
		job.IsCurrent = buildErr == nil && job.Status == "completed" && ComposeResultIsCurrent(job.InputHash, current.InputHash)
	}
	return job, nil
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
