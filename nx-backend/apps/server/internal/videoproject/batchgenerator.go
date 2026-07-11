package videoproject

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"nine-xing/nx-backend/apps/server/internal/video"
)

// BatchGenerator 批量生成器：按顺序生成项目的所有分镜
type BatchGenerator struct {
	generator shotGenerator
	store     *Store
}

type shotGenerator interface {
	GenerateShot(context.Context, string, ...string) (video.Generation, error)
}

func NewBatchGenerator(generator *Generator, store *Store) *BatchGenerator {
	return &BatchGenerator{
		generator: generator,
		store:     store,
	}
}

// BatchGenerateResult 批量生成结果
type BatchGenerateResult struct {
	ProjectID    string                 `json:"projectId"`
	TotalShots   int                    `json:"totalShots"`
	SuccessCount int                    `json:"successCount"`
	FailedCount  int                    `json:"failedCount"`
	ShotResults  []ShotGenerationResult `json:"shotResults"`
}

// ShotGenerationResult 单个分镜生成结果
type ShotGenerationResult struct {
	ShotID       string `json:"shotId"`
	ShotName     string `json:"shotName"`
	OrderNum     int    `json:"orderNum"`
	Status       string `json:"status"` // success, failed, skipped
	GenerationID string `json:"generationId"`
	ErrorMessage string `json:"errorMessage"`
}

type SafeBatchGenerateItem struct {
	RequestKey string `json:"requestKey"`
	ShotID     string `json:"shotId"`
}

func (bg *BatchGenerator) GenerateSafe(
	ctx context.Context,
	projectID string,
	items []SafeBatchGenerateItem,
	parallel bool,
) (BatchGenerateResult, error) {
	workflow, err := bg.store.GetWorkflowStatus(ctx, projectID)
	if err != nil {
		return BatchGenerateResult{}, err
	}
	shots := make(map[string]Shot, len(workflow.Shots))
	readiness := make(map[string]ShotReadiness, len(workflow.Shots))
	for _, item := range workflow.Shots {
		shots[item.Shot.ID] = item.Shot
		readiness[item.Shot.ID] = item.Readiness
	}
	return executeSafeBatch(ctx, bg.generator, projectID, shots, readiness, items, parallel), nil
}

func executeSafeBatch(
	ctx context.Context,
	generator shotGenerator,
	projectID string,
	shots map[string]Shot,
	readiness map[string]ShotReadiness,
	items []SafeBatchGenerateItem,
	parallel bool,
) BatchGenerateResult {
	requested := make([]string, 0, len(items))
	keys := make(map[string]string, len(items))
	for _, item := range items {
		shotID := strings.TrimSpace(item.ShotID)
		if _, exists := keys[shotID]; exists {
			continue
		}
		requested = append(requested, shotID)
		keys[shotID] = strings.TrimSpace(item.RequestKey)
	}
	eligibleIDs := FilterGeneratableShotIDs(readiness, requested)
	result := BatchGenerateResult{
		ProjectID:   projectID,
		TotalShots:  len(eligibleIDs),
		ShotResults: make([]ShotGenerationResult, len(eligibleIDs)),
	}
	generate := func(index int, shotID string) {
		shot := shots[shotID]
		item := ShotGenerationResult{ShotID: shot.ID, ShotName: shot.Name, OrderNum: shot.OrderNum}
		if keys[shotID] == "" {
			item.Status = "failed"
			item.ErrorMessage = "缺少生成请求键"
		} else {
			generation, err := generator.GenerateShot(ctx, shotID, keys[shotID])
			if err != nil {
				item.Status = "failed"
				item.ErrorMessage = err.Error()
			} else {
				item.Status = "success"
				item.GenerationID = generation.ID
			}
		}
		result.ShotResults[index] = item
	}
	if parallel {
		var wg sync.WaitGroup
		for index, shotID := range eligibleIDs {
			wg.Add(1)
			go func(index int, shotID string) {
				defer wg.Done()
				generate(index, shotID)
			}(index, shotID)
		}
		wg.Wait()
	} else {
		for index, shotID := range eligibleIDs {
			generate(index, shotID)
		}
	}
	for _, item := range result.ShotResults {
		if item.Status == "success" {
			result.SuccessCount++
		} else {
			result.FailedCount++
		}
	}
	return result
}

func filterShotsByIDs(shots []Shot, selectedShotIDs []string) []Shot {
	if len(selectedShotIDs) == 0 {
		return shots
	}
	wanted := make(map[string]bool, len(selectedShotIDs))
	for _, shotID := range selectedShotIDs {
		shotID = strings.TrimSpace(shotID)
		if shotID != "" {
			wanted[shotID] = true
		}
	}
	if len(wanted) == 0 {
		return shots
	}
	filtered := make([]Shot, 0, len(selectedShotIDs))
	for _, shot := range shots {
		if wanted[shot.ID] {
			filtered = append(filtered, shot)
		}
	}
	return filtered
}

// GenerateAllShots 批量生成项目的所有分镜（按顺序，等待上一个完成后再生成下一个）
func (bg *BatchGenerator) GenerateAllShots(ctx context.Context, projectID string) (BatchGenerateResult, error) {
	// 1. 获取项目的所有分镜
	shots, err := bg.store.ListShots(ctx, projectID)
	if err != nil {
		return BatchGenerateResult{}, fmt.Errorf("获取分镜列表失败: %v", err)
	}

	if len(shots) == 0 {
		return BatchGenerateResult{}, fmt.Errorf("项目没有分镜")
	}

	result := BatchGenerateResult{
		ProjectID:   projectID,
		TotalShots:  len(shots),
		ShotResults: make([]ShotGenerationResult, 0, len(shots)),
	}

	// 2. 按顺序生成每个分镜
	for _, shot := range shots {
		shotResult := ShotGenerationResult{
			ShotID:   shot.ID,
			ShotName: shot.Name,
			OrderNum: shot.OrderNum,
		}

		// 跳过已完成的分镜
		if shot.Status == "completed" {
			shotResult.Status = "skipped"
			shotResult.GenerationID = shot.GenerationID
			result.ShotResults = append(result.ShotResults, shotResult)
			continue
		}

		// 生成分镜
		generation, err := bg.generator.GenerateShot(ctx, shot.ID)
		if err != nil {
			shotResult.Status = "failed"
			shotResult.ErrorMessage = err.Error()
			result.FailedCount++
			log.Printf("生成分镜 %s 失败: %v", shot.ID, err)
		} else {
			shotResult.Status = "success"
			shotResult.GenerationID = generation.ID
			result.SuccessCount++
		}

		result.ShotResults = append(result.ShotResults, shotResult)

		// 等待生成完成后再继续下一个（确保首尾帧继承）
		if shotResult.Status == "success" {
			bg.waitForCompletion(ctx, shot.ID, generation.ID)
		}
	}

	return result, nil
}

func (bg *BatchGenerator) GenerateSelectedShots(ctx context.Context, projectID string, selectedShotIDs []string, parallel bool) (BatchGenerateResult, error) {
	shots, err := bg.store.ListShots(ctx, projectID)
	if err != nil {
		return BatchGenerateResult{}, fmt.Errorf("获取分镜列表失败: %v", err)
	}
	shots = filterShotsByIDs(shots, selectedShotIDs)
	if len(shots) == 0 {
		return BatchGenerateResult{}, fmt.Errorf("没有符合条件的分镜")
	}

	if parallel {
		result := BatchGenerateResult{
			ProjectID:   projectID,
			TotalShots:  len(shots),
			ShotResults: make([]ShotGenerationResult, len(shots)),
		}

		var wg sync.WaitGroup
		var mu sync.Mutex

		for i, shot := range shots {
			if shot.Status == "completed" {
				result.ShotResults[i] = ShotGenerationResult{
					ShotID:       shot.ID,
					ShotName:     shot.Name,
					OrderNum:     shot.OrderNum,
					Status:       "skipped",
					GenerationID: shot.GenerationID,
				}
				continue
			}

			wg.Add(1)
			go func(idx int, s Shot) {
				defer wg.Done()

				shotResult := ShotGenerationResult{
					ShotID:   s.ID,
					ShotName: s.Name,
					OrderNum: s.OrderNum,
				}

				generation, err := bg.generator.GenerateShot(ctx, s.ID)
				if err != nil {
					shotResult.Status = "failed"
					shotResult.ErrorMessage = err.Error()
					mu.Lock()
					result.FailedCount++
					mu.Unlock()
				} else {
					shotResult.Status = "success"
					shotResult.GenerationID = generation.ID
					mu.Lock()
					result.SuccessCount++
					mu.Unlock()
				}

				mu.Lock()
				result.ShotResults[idx] = shotResult
				mu.Unlock()
			}(i, shot)
		}

		wg.Wait()
		return result, nil
	}

	result := BatchGenerateResult{
		ProjectID:   projectID,
		TotalShots:  len(shots),
		ShotResults: make([]ShotGenerationResult, 0, len(shots)),
	}

	for _, shot := range shots {
		shotResult := ShotGenerationResult{
			ShotID:   shot.ID,
			ShotName: shot.Name,
			OrderNum: shot.OrderNum,
		}

		if shot.Status == "completed" {
			shotResult.Status = "skipped"
			shotResult.GenerationID = shot.GenerationID
			result.ShotResults = append(result.ShotResults, shotResult)
			continue
		}

		generation, err := bg.generator.GenerateShot(ctx, shot.ID)
		if err != nil {
			shotResult.Status = "failed"
			shotResult.ErrorMessage = err.Error()
			result.FailedCount++
			log.Printf("生成分镜 %s 失败: %v", shot.ID, err)
		} else {
			shotResult.Status = "success"
			shotResult.GenerationID = generation.ID
			result.SuccessCount++
		}

		result.ShotResults = append(result.ShotResults, shotResult)

		if shotResult.Status == "success" {
			bg.waitForCompletion(ctx, shot.ID, generation.ID)
		}
	}

	return result, nil
}

// GenerateAllShotsParallel 并行批量生成（不保证顺序，速度更快）
func (bg *BatchGenerator) GenerateAllShotsParallel(ctx context.Context, projectID string) (BatchGenerateResult, error) {
	shots, err := bg.store.ListShots(ctx, projectID)
	if err != nil {
		return BatchGenerateResult{}, fmt.Errorf("获取分镜列表失败: %v", err)
	}

	if len(shots) == 0 {
		return BatchGenerateResult{}, fmt.Errorf("项目没有分镜")
	}

	result := BatchGenerateResult{
		ProjectID:   projectID,
		TotalShots:  len(shots),
		ShotResults: make([]ShotGenerationResult, len(shots)),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, shot := range shots {
		// 跳过已完成的分镜
		if shot.Status == "completed" {
			result.ShotResults[i] = ShotGenerationResult{
				ShotID:       shot.ID,
				ShotName:     shot.Name,
				OrderNum:     shot.OrderNum,
				Status:       "skipped",
				GenerationID: shot.GenerationID,
			}
			continue
		}

		wg.Add(1)
		go func(idx int, s Shot) {
			defer wg.Done()

			shotResult := ShotGenerationResult{
				ShotID:   s.ID,
				ShotName: s.Name,
				OrderNum: s.OrderNum,
			}

			generation, err := bg.generator.GenerateShot(ctx, s.ID)
			if err != nil {
				shotResult.Status = "failed"
				shotResult.ErrorMessage = err.Error()
				mu.Lock()
				result.FailedCount++
				mu.Unlock()
			} else {
				shotResult.Status = "success"
				shotResult.GenerationID = generation.ID
				mu.Lock()
				result.SuccessCount++
				mu.Unlock()
			}

			mu.Lock()
			result.ShotResults[idx] = shotResult
			mu.Unlock()
		}(i, shot)
	}

	wg.Wait()
	return result, nil
}

// waitForCompletion 等待单个分镜生成完成
func (bg *BatchGenerator) waitForCompletion(ctx context.Context, shotID, generationID string) {
	maxWait := 10 * time.Minute
	timeout := time.After(maxWait)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			log.Printf("等待分镜 %s 生成超时", shotID)
			return
		case <-ticker.C:
			shot, err := bg.store.GetShot(ctx, shotID)
			if err != nil {
				continue
			}
			if shot.Status == "completed" || shot.Status == "failed" {
				return
			}
		}
	}
}

// GetBatchProgress 获取批量生成进度
func (bg *BatchGenerator) GetBatchProgress(ctx context.Context, projectID string) (map[string]interface{}, error) {
	shots, err := bg.store.ListShots(ctx, projectID)
	if err != nil {
		return nil, err
	}

	total := len(shots)
	completed := 0
	generating := 0
	failed := 0
	pending := 0

	for _, shot := range shots {
		switch shot.Status {
		case "completed":
			completed++
		case "generating":
			generating++
		case "failed":
			failed++
		default:
			pending++
		}
	}

	progress := 0
	if total > 0 {
		progress = (completed * 100) / total
	}

	return map[string]interface{}{
		"projectId":  projectID,
		"total":      total,
		"completed":  completed,
		"generating": generating,
		"failed":     failed,
		"pending":    pending,
		"progress":   progress,
	}, nil
}
