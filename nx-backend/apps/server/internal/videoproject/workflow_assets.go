package videoproject

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	imagegen "nine-xing/nx-backend/apps/server/internal/image"
	"nine-xing/nx-backend/apps/server/internal/netguard"
)

type AssetCandidateWrite struct {
	ProjectID           string
	TargetKind          string
	TargetID            string
	Prompt              string
	ImageAssetID        string
	ImageURL            string
	Source              string
	GenerationRequestID string
	Status              string
	ErrorMessage        string
}

type AssetCandidateUpdate struct {
	Status       string
	ImageAssetID string
	ImageURL     string
	ErrorMessage string
}

type GenerateAssetCandidateInput struct {
	ProjectID           string `json:"projectId"`
	TargetKind          string `json:"targetKind"`
	TargetID            string `json:"targetId"`
	Prompt              string `json:"prompt"`
	Model               string `json:"model"`
	Size                string `json:"size"`
	GenerationRequestID string `json:"generationRequestId"`
}

type ExternalAssetCandidateInput struct {
	ProjectID    string `json:"projectId"`
	TargetKind   string `json:"targetKind"`
	TargetID     string `json:"targetId"`
	Prompt       string `json:"prompt"`
	ImageAssetID string `json:"imageAssetId"`
	ImageURL     string `json:"imageUrl"`
	Source       string `json:"source"`
}

type ProjectImageGenerator interface {
	Generate(ctx context.Context, input imagegen.GenerateInput) (imagegen.Result, error)
}

type assetCandidateRepository interface {
	CreateAssetCandidate(ctx context.Context, write AssetCandidateWrite) (AssetCandidate, error)
	UpdateAssetCandidate(ctx context.Context, candidateID string, update AssetCandidateUpdate) (AssetCandidate, error)
	ListAssetCandidates(ctx context.Context, projectID, targetKind, targetID string) ([]AssetCandidate, error)
	SelectAssetCandidate(ctx context.Context, projectID, candidateID string) (AssetCandidate, error)
	RecoverStaleAssetCandidates(ctx context.Context, now time.Time, timeout time.Duration) (int, error)
}

type AssetWorkflowService struct {
	repository assetCandidateRepository
	images     ProjectImageGenerator
}

func NewAssetWorkflowService(repository assetCandidateRepository, images ProjectImageGenerator) *AssetWorkflowService {
	return &AssetWorkflowService{repository: repository, images: images}
}

func (service *AssetWorkflowService) GenerateCandidate(ctx context.Context, input GenerateAssetCandidateInput) (AssetCandidate, error) {
	if service == nil || service.repository == nil {
		return AssetCandidate{}, fmt.Errorf("资产候选服务尚未配置")
	}
	if service.images == nil {
		return AssetCandidate{}, fmt.Errorf("文生图服务尚未配置")
	}
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.TargetKind = strings.ToLower(strings.TrimSpace(input.TargetKind))
	input.TargetID = strings.TrimSpace(input.TargetID)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if err := validateAssetCandidateTarget(input.ProjectID, input.TargetKind, input.TargetID); err != nil {
		return AssetCandidate{}, err
	}
	if input.Prompt == "" {
		return AssetCandidate{}, fmt.Errorf("请先填写资产参考图提示词")
	}
	candidate, err := service.repository.CreateAssetCandidate(ctx, AssetCandidateWrite{
		ProjectID: input.ProjectID, TargetKind: input.TargetKind, TargetID: input.TargetID,
		Prompt: input.Prompt, Source: "generated", GenerationRequestID: strings.TrimSpace(input.GenerationRequestID), Status: "generating",
	})
	if err != nil {
		return AssetCandidate{}, err
	}
	generated, generateErr := service.images.Generate(ctx, imagegen.GenerateInput{
		Model: strings.TrimSpace(input.Model), Prompt: input.Prompt, Size: strings.TrimSpace(input.Size),
	})
	if generateErr != nil {
		failed, updateErr := service.repository.UpdateAssetCandidate(ctx, candidate.ID, AssetCandidateUpdate{
			Status: "failed", ErrorMessage: generateErr.Error(),
		})
		if updateErr != nil {
			return candidate, fmt.Errorf("%v；保存失败状态时出错：%w", generateErr, updateErr)
		}
		return failed, generateErr
	}
	imageURL := firstPublicAssetURL(generated.ObjectURL, generated.URL)
	if imageURL == "" {
		message := "图片已生成，但没有文件桶公网地址，请检查 OSS_PUBLIC_URL 配置"
		failed, updateErr := service.repository.UpdateAssetCandidate(ctx, candidate.ID, AssetCandidateUpdate{Status: "failed", ErrorMessage: message})
		if updateErr != nil {
			return candidate, fmt.Errorf("%s；保存失败状态时出错：%w", message, updateErr)
		}
		return failed, fmt.Errorf(message)
	}
	ready, err := service.repository.UpdateAssetCandidate(ctx, candidate.ID, AssetCandidateUpdate{
		Status: "ready", ImageAssetID: fmt.Sprint(generated.AssetID), ImageURL: imageURL,
	})
	if err != nil {
		return candidate, err
	}
	return ready, nil
}

func (service *AssetWorkflowService) CreateExternalCandidate(ctx context.Context, input ExternalAssetCandidateInput) (AssetCandidate, error) {
	if service == nil || service.repository == nil {
		return AssetCandidate{}, fmt.Errorf("资产候选服务尚未配置")
	}
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.TargetKind = strings.ToLower(strings.TrimSpace(input.TargetKind))
	input.TargetID = strings.TrimSpace(input.TargetID)
	input.Source = strings.ToLower(strings.TrimSpace(input.Source))
	input.ImageURL = strings.TrimSpace(input.ImageURL)
	if err := validateAssetCandidateTarget(input.ProjectID, input.TargetKind, input.TargetID); err != nil {
		return AssetCandidate{}, err
	}
	if input.Source != "upload" && input.Source != "library" {
		return AssetCandidate{}, fmt.Errorf("外部候选图来源只能是 upload 或 library")
	}
	if !netguard.IsPublicHTTPURL(input.ImageURL) {
		return AssetCandidate{}, fmt.Errorf("候选图需要文件桶公网 http(s) 地址")
	}
	return service.repository.CreateAssetCandidate(ctx, AssetCandidateWrite{
		ProjectID: input.ProjectID, TargetKind: input.TargetKind, TargetID: input.TargetID,
		Prompt: strings.TrimSpace(input.Prompt), ImageAssetID: strings.TrimSpace(input.ImageAssetID),
		ImageURL: input.ImageURL, Source: input.Source, Status: "ready",
	})
}

func (service *AssetWorkflowService) ListCandidates(ctx context.Context, projectID, targetKind, targetID string) ([]AssetCandidate, error) {
	if service == nil || service.repository == nil {
		return nil, fmt.Errorf("资产候选服务尚未配置")
	}
	if err := validateAssetCandidateTarget(projectID, targetKind, targetID); err != nil {
		return nil, err
	}
	return service.repository.ListAssetCandidates(ctx, strings.TrimSpace(projectID), strings.ToLower(strings.TrimSpace(targetKind)), strings.TrimSpace(targetID))
}

func (service *AssetWorkflowService) SelectCandidate(ctx context.Context, projectID, candidateID string) (AssetCandidate, error) {
	if service == nil || service.repository == nil {
		return AssetCandidate{}, fmt.Errorf("资产候选服务尚未配置")
	}
	if _, err := parseID(projectID); err != nil {
		return AssetCandidate{}, err
	}
	if _, err := parseID(candidateID); err != nil {
		return AssetCandidate{}, fmt.Errorf("无效的候选图 ID")
	}
	return service.repository.SelectAssetCandidate(ctx, strings.TrimSpace(projectID), strings.TrimSpace(candidateID))
}

func (service *AssetWorkflowService) RecoverStaleCandidates(ctx context.Context, now time.Time, timeout time.Duration) (int, error) {
	if service == nil || service.repository == nil {
		return 0, fmt.Errorf("资产候选服务尚未配置")
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("恢复超时时间必须大于 0")
	}
	return service.repository.RecoverStaleAssetCandidates(ctx, now, timeout)
}

func validateAssetCandidateTarget(projectID, targetKind, targetID string) error {
	if _, err := parseID(projectID); err != nil {
		return err
	}
	if _, err := parseID(targetID); err != nil {
		return fmt.Errorf("无效的资产 ID")
	}
	switch strings.ToLower(strings.TrimSpace(targetKind)) {
	case "character", "scene", "prop", "outfit", "style":
		return nil
	default:
		return fmt.Errorf("不支持的资产类型 %q", targetKind)
	}
}

func firstPublicAssetURL(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if netguard.IsPublicHTTPURL(value) {
			return value
		}
	}
	return ""
}

func (store *Store) CreateAssetCandidate(ctx context.Context, write AssetCandidateWrite) (AssetCandidate, error) {
	projectID, err := parseID(write.ProjectID)
	if err != nil {
		return AssetCandidate{}, err
	}
	targetID, err := parseID(write.TargetID)
	if err != nil {
		return AssetCandidate{}, fmt.Errorf("无效的资产 ID")
	}
	targetKind := strings.ToLower(strings.TrimSpace(write.TargetKind))
	if err := validateAssetCandidateTarget(write.ProjectID, targetKind, write.TargetID); err != nil {
		return AssetCandidate{}, err
	}
	write.Source = strings.ToLower(strings.TrimSpace(write.Source))
	write.Status = strings.ToLower(strings.TrimSpace(write.Status))
	if !allowedAssetCandidateSource(write.Source) {
		return AssetCandidate{}, fmt.Errorf("无效的候选图来源")
	}
	if !allowedAssetCandidateStatus(write.Status) {
		return AssetCandidate{}, fmt.Errorf("无效的候选图状态")
	}
	if write.Status == "ready" && !netguard.IsPublicHTTPURL(strings.TrimSpace(write.ImageURL)) {
		return AssetCandidate{}, fmt.Errorf("候选图需要文件桶公网 http(s) 地址")
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return AssetCandidate{}, err
	}
	defer transaction.Rollback()
	if err := lockProjectAssetTarget(ctx, transaction, projectID, targetKind, targetID); err != nil {
		return AssetCandidate{}, err
	}
	var imageAssetID any
	if strings.TrimSpace(write.ImageAssetID) != "" {
		parsedImageAssetID, err := parseID(write.ImageAssetID)
		if err != nil {
			return AssetCandidate{}, fmt.Errorf("无效的图片资产 ID")
		}
		imageAssetID = parsedImageAssetID
	}
	var candidateID string
	if err := transaction.QueryRowContext(ctx,
		`INSERT INTO video_project_asset_candidates (
		   project_id, target_type, target_id, prompt, image_asset_id, image_url,
		   source, generation_request_id, status, error_message, selected
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,false)
		 RETURNING id::text`,
		projectID, targetKind, targetID, strings.TrimSpace(write.Prompt), imageAssetID,
		strings.TrimSpace(write.ImageURL), write.Source, strings.TrimSpace(write.GenerationRequestID),
		write.Status, strings.TrimSpace(write.ErrorMessage),
	).Scan(&candidateID); err != nil {
		return AssetCandidate{}, err
	}
	if err := transaction.Commit(); err != nil {
		return AssetCandidate{}, err
	}
	return store.getAssetCandidate(ctx, candidateID)
}

func (store *Store) UpdateAssetCandidate(ctx context.Context, candidateID string, update AssetCandidateUpdate) (AssetCandidate, error) {
	parsedCandidateID, err := parseID(candidateID)
	if err != nil {
		return AssetCandidate{}, fmt.Errorf("无效的候选图 ID")
	}
	update.Status = strings.ToLower(strings.TrimSpace(update.Status))
	if !allowedAssetCandidateStatus(update.Status) {
		return AssetCandidate{}, fmt.Errorf("无效的候选图状态")
	}
	update.ImageURL = strings.TrimSpace(update.ImageURL)
	if update.Status == "ready" && !netguard.IsPublicHTTPURL(update.ImageURL) {
		return AssetCandidate{}, fmt.Errorf("候选图需要文件桶公网 http(s) 地址")
	}
	var imageAssetID any
	if strings.TrimSpace(update.ImageAssetID) != "" {
		parsedImageAssetID, err := parseID(update.ImageAssetID)
		if err != nil {
			return AssetCandidate{}, fmt.Errorf("无效的图片资产 ID")
		}
		imageAssetID = parsedImageAssetID
	}
	result, err := store.db.ExecContext(ctx,
		`UPDATE video_project_asset_candidates
		    SET status=$1, image_asset_id=$2, image_url=$3, error_message=$4, update_time=now()
		  WHERE id=$5 AND selected=false`,
		update.Status, imageAssetID, update.ImageURL, strings.TrimSpace(update.ErrorMessage), parsedCandidateID,
	)
	if err != nil {
		return AssetCandidate{}, err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil {
		return AssetCandidate{}, rowsErr
	} else if affected != 1 {
		return AssetCandidate{}, fmt.Errorf("候选图不存在或已经被选中")
	}
	return store.getAssetCandidate(ctx, candidateID)
}

func (store *Store) ListAssetCandidates(ctx context.Context, projectID, targetKind, targetID string) ([]AssetCandidate, error) {
	parsedProjectID, err := parseID(projectID)
	if err != nil {
		return nil, err
	}
	parsedTargetID, err := parseID(targetID)
	if err != nil {
		return nil, fmt.Errorf("无效的资产 ID")
	}
	targetKind = strings.ToLower(strings.TrimSpace(targetKind))
	if err := validateAssetCandidateTarget(projectID, targetKind, targetID); err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx,
		`SELECT id::text, project_id::text, target_type, target_id::text, prompt,
		        COALESCE(image_asset_id::text,''), image_url, source, generation_request_id,
		        status, error_message, selected, create_time, update_time
		   FROM video_project_asset_candidates
		  WHERE project_id=$1 AND target_type=$2 AND target_id=$3
		  ORDER BY create_time ASC, id ASC`,
		parsedProjectID, targetKind, parsedTargetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AssetCandidate{}
	for rows.Next() {
		candidate, err := scanAssetCandidate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, candidate)
	}
	return items, rows.Err()
}

func (store *Store) SelectAssetCandidate(ctx context.Context, projectID, candidateID string) (AssetCandidate, error) {
	parsedProjectID, err := parseID(projectID)
	if err != nil {
		return AssetCandidate{}, err
	}
	parsedCandidateID, err := parseID(candidateID)
	if err != nil {
		return AssetCandidate{}, fmt.Errorf("无效的候选图 ID")
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return AssetCandidate{}, err
	}
	defer transaction.Rollback()

	var candidate AssetCandidate
	var createTime, updateTime time.Time
	if err := transaction.QueryRowContext(ctx,
		`SELECT id::text, project_id::text, target_type, target_id::text, prompt,
		        COALESCE(image_asset_id::text,''), image_url, source, generation_request_id,
		        status, error_message, selected, create_time, update_time
		   FROM video_project_asset_candidates
		  WHERE project_id=$1 AND id=$2
		  FOR UPDATE`,
		parsedProjectID, parsedCandidateID,
	).Scan(
		&candidate.ID, &candidate.ProjectID, &candidate.TargetType, &candidate.TargetID, &candidate.Prompt,
		&candidate.ImageAssetID, &candidate.ImageURL, &candidate.Source, &candidate.GenerationRequestID,
		&candidate.Status, &candidate.ErrorMessage, &candidate.Selected, &createTime, &updateTime,
	); err != nil {
		if err == sql.ErrNoRows {
			return AssetCandidate{}, fmt.Errorf("候选图不存在或不属于当前项目")
		}
		return AssetCandidate{}, err
	}
	if candidate.Status != "ready" {
		return AssetCandidate{}, fmt.Errorf("候选图尚未生成完成")
	}
	if !netguard.IsPublicHTTPURL(candidate.ImageURL) {
		return AssetCandidate{}, fmt.Errorf("候选图需要文件桶公网地址")
	}
	targetID, _ := parseID(candidate.TargetID)
	if err := lockProjectAssetTarget(ctx, transaction, parsedProjectID, candidate.TargetType, targetID); err != nil {
		return AssetCandidate{}, err
	}
	if candidate.Selected {
		if err := transaction.Commit(); err != nil {
			return AssetCandidate{}, err
		}
		candidate.CreateTime = formatTime(createTime)
		candidate.UpdateTime = formatTime(updateTime)
		return candidate, nil
	}
	if _, err := transaction.ExecContext(ctx,
		`UPDATE video_project_asset_candidates
		    SET selected=false, update_time=now()
		  WHERE target_type=$1 AND target_id=$2 AND selected=true`,
		candidate.TargetType, targetID,
	); err != nil {
		return AssetCandidate{}, err
	}
	if _, err := transaction.ExecContext(ctx,
		`UPDATE video_project_asset_candidates SET selected=true, update_time=now() WHERE id=$1`,
		parsedCandidateID,
	); err != nil {
		return AssetCandidate{}, err
	}
	if err := updateProjectAssetReferenceURL(ctx, transaction, parsedProjectID, candidate.TargetType, targetID, candidate.ImageURL); err != nil {
		return AssetCandidate{}, err
	}
	if _, err := transaction.ExecContext(ctx,
		`UPDATE video_projects SET asset_revision=asset_revision+1, update_time=now() WHERE id=$1`,
		parsedProjectID,
	); err != nil {
		return AssetCandidate{}, err
	}
	if err := transaction.Commit(); err != nil {
		return AssetCandidate{}, err
	}
	candidate.Selected = true
	candidate.CreateTime = formatTime(createTime)
	candidate.UpdateTime = formatTime(time.Now())
	return candidate, nil
}

func (store *Store) RecoverStaleAssetCandidates(ctx context.Context, now time.Time, timeout time.Duration) (int, error) {
	if timeout <= 0 {
		return 0, fmt.Errorf("恢复超时时间必须大于 0")
	}
	result, err := store.db.ExecContext(ctx,
		`UPDATE video_project_asset_candidates
		    SET status='failed',
		        error_message='图片生成因服务重启或超时中断，请手动重试',
		        update_time=$1
		  WHERE status='generating' AND update_time < $2`,
		now, now.Add(-timeout),
	)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	return int(affected), err
}

func (store *Store) getAssetCandidate(ctx context.Context, candidateID string) (AssetCandidate, error) {
	parsedCandidateID, err := parseID(candidateID)
	if err != nil {
		return AssetCandidate{}, fmt.Errorf("无效的候选图 ID")
	}
	candidate, err := scanAssetCandidate(store.db.QueryRowContext(ctx,
		`SELECT id::text, project_id::text, target_type, target_id::text, prompt,
		        COALESCE(image_asset_id::text,''), image_url, source, generation_request_id,
		        status, error_message, selected, create_time, update_time
		   FROM video_project_asset_candidates WHERE id=$1`,
		parsedCandidateID,
	))
	if err == sql.ErrNoRows {
		return AssetCandidate{}, fmt.Errorf("候选图不存在")
	}
	return candidate, err
}

func scanAssetCandidate(scanner interface{ Scan(...any) error }) (AssetCandidate, error) {
	var candidate AssetCandidate
	var createTime, updateTime time.Time
	if err := scanner.Scan(
		&candidate.ID, &candidate.ProjectID, &candidate.TargetType, &candidate.TargetID, &candidate.Prompt,
		&candidate.ImageAssetID, &candidate.ImageURL, &candidate.Source, &candidate.GenerationRequestID,
		&candidate.Status, &candidate.ErrorMessage, &candidate.Selected, &createTime, &updateTime,
	); err != nil {
		return AssetCandidate{}, err
	}
	candidate.CreateTime = formatTime(createTime)
	candidate.UpdateTime = formatTime(updateTime)
	return candidate, nil
}

func lockProjectAssetTarget(ctx context.Context, transaction *sql.Tx, projectID int64, targetKind string, targetID int64) error {
	table, extraCondition, err := projectAssetTargetTable(targetKind)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`SELECT id FROM %s WHERE project_id=$1 AND id=$2%s FOR UPDATE`, table, extraCondition)
	var lockedID int64
	if err := transaction.QueryRowContext(ctx, query, projectID, targetID).Scan(&lockedID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("资产不存在、类型不匹配或不属于当前项目")
		}
		return err
	}
	return nil
}

func updateProjectAssetReferenceURL(ctx context.Context, transaction *sql.Tx, projectID int64, targetKind string, targetID int64, imageURL string) error {
	table, extraCondition, err := projectAssetTargetTable(targetKind)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`UPDATE %s SET reference_image_url=$1, status='ready', update_time=now() WHERE project_id=$2 AND id=$3%s`, table, extraCondition)
	result, err := transaction.ExecContext(ctx, query, imageURL, projectID, targetID)
	if err != nil {
		return err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil {
		return rowsErr
	} else if affected != 1 {
		return fmt.Errorf("资产不存在、类型不匹配或不属于当前项目")
	}
	return nil
}

func projectAssetTargetTable(targetKind string) (string, string, error) {
	switch targetKind {
	case "character":
		return "video_project_characters", "", nil
	case "scene":
		return "video_project_scenes", "", nil
	case "prop", "outfit", "style":
		return "video_project_assets", fmt.Sprintf(" AND type='%s'", targetKind), nil
	default:
		return "", "", fmt.Errorf("不支持的资产类型 %q", targetKind)
	}
}

func allowedAssetCandidateSource(source string) bool {
	switch source {
	case "generated", "upload", "library", "legacy":
		return true
	default:
		return false
	}
}

func allowedAssetCandidateStatus(status string) bool {
	switch status {
	case "queued", "generating", "ready", "failed":
		return true
	default:
		return false
	}
}
