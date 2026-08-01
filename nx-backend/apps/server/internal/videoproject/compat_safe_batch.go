package videoproject

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"nine-xing/nx-backend/apps/server/internal/video"
)

type SafeBatchGenerateItem struct {
	RequestKey string `json:"requestKey"`
	ShotID     string `json:"shotId"`
}

func (bg *BatchGenerator) GenerateSafe(ctx context.Context, projectID string, items []SafeBatchGenerateItem, parallel bool) (BatchGenerateResult, error) {
	workflowStore, ok := bg.store.(*Store)
	if !ok {
		return BatchGenerateResult{}, fmt.Errorf("workflow store unavailable")
	}
	workflow, err := workflowStore.GetWorkflowStatus(ctx, projectID)
	if err != nil {
		return BatchGenerateResult{}, err
	}
	shots := make(map[string]Shot, len(workflow.Shots))
	readiness := make(map[string]ShotReadiness, len(workflow.Shots))
	for _, item := range workflow.Shots {
		shots[item.Shot.ID] = item.Shot
		readiness[item.Shot.ID] = item.Readiness
	}
	requested := make([]string, 0, len(items))
	keys := make(map[string]string, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ShotID)
		if id == "" {
			continue
		}
		if _, ok := keys[id]; ok {
			continue
		}
		requested = append(requested, id)
		keys[id] = strings.TrimSpace(item.RequestKey)
	}
	eligible := FilterGeneratableShotIDs(readiness, requested)
	result := BatchGenerateResult{ProjectID: projectID, TotalShots: len(eligible), ShotResults: make([]ShotGenerationResult, len(eligible))}
	run := func(i int, id string) {
		shot := shots[id]
		out := ShotGenerationResult{ShotID: shot.ID, ShotName: shot.Name, OrderNum: shot.OrderNum}
		if keys[id] == "" {
			out.Status = "failed"
			out.ErrorMessage = "缺少生成请求键"
		} else if gen, e := bg.generator.GenerateShotWithInput(ctx, id, GenerateShotInput{RequestKey: keys[id]}); e != nil {
			out.Status = "failed"
			out.ErrorMessage = e.Error()
		} else {
			out.Status = "success"
			out.GenerationID = gen.ID
		}
		result.ShotResults[i] = out
	}
	if parallel {
		var wg sync.WaitGroup
		for i, id := range eligible {
			wg.Add(1)
			go func(i int, id string) { defer wg.Done(); run(i, id) }(i, id)
		}
		wg.Wait()
	} else {
		for i, id := range eligible {
			run(i, id)
		}
	}
	for _, item := range result.ShotResults {
		if item.Status == "success" {
			result.SuccessCount++
		} else {
			result.FailedCount++
		}
	}
	return result, nil
}

type legacySafeShotGenerator interface {
	GenerateShot(context.Context, string, ...string) (video.Generation, error)
}

func executeSafeBatch(ctx context.Context, generator legacySafeShotGenerator, projectID string, shots map[string]Shot, readiness map[string]ShotReadiness, items []SafeBatchGenerateItem, parallel bool) BatchGenerateResult {
	requested := make([]string, 0, len(items))
	keys := map[string]string{}
	for _, item := range items {
		id := strings.TrimSpace(item.ShotID)
		if id == "" {
			continue
		}
		if _, ok := keys[id]; ok {
			continue
		}
		keys[id] = strings.TrimSpace(item.RequestKey)
		requested = append(requested, id)
	}
	ids := FilterGeneratableShotIDs(readiness, requested)
	result := BatchGenerateResult{ProjectID: projectID, TotalShots: len(ids), ShotResults: make([]ShotGenerationResult, len(ids))}
	run := func(i int, id string) {
		shot := shots[id]
		out := ShotGenerationResult{ShotID: id, ShotName: shot.Name, OrderNum: shot.OrderNum}
		gen, err := generator.GenerateShot(ctx, id, keys[id])
		if err != nil {
			out.Status = "failed"
			out.ErrorMessage = err.Error()
		} else {
			out.Status = "success"
			out.GenerationID = gen.ID
		}
		result.ShotResults[i] = out
	}
	if parallel {
		var wg sync.WaitGroup
		for i, id := range ids {
			wg.Add(1)
			go func(i int, id string) { defer wg.Done(); run(i, id) }(i, id)
		}
		wg.Wait()
	} else {
		for i, id := range ids {
			run(i, id)
		}
	}
	for _, r := range result.ShotResults {
		if r.Status == "success" {
			result.SuccessCount++
		} else {
			result.FailedCount++
		}
	}
	return result
}
