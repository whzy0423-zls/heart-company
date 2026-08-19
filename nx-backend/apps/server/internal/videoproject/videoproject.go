// Package videoproject 视频项目工作台：项目制管理剧本、资产、分镜、
// Seedance 2.0 提示词、视频版本与成片合成。
package videoproject

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

// ---------- 数据模型 ----------

type Project struct {
	ComposeStatus       string `json:"composeStatus"`
	CreateTime          string `json:"createTime"`
	Description         string `json:"description"`
	FinalVideoAssetID   string `json:"finalVideoAssetId"`
	FinalVideoInputHash string `json:"finalVideoInputHash"`
	FinalVideoURL       string `json:"finalVideoUrl"`
	ID                  string `json:"id"`
	Name                string `json:"name"`
	ScriptContent       string `json:"scriptContent"`
	ScriptRevision      int    `json:"scriptRevision"`
	Status              string `json:"status"`
	StyleGuide          string `json:"styleGuide"`
	Theme               string `json:"theme"`
	UpdateTime          string `json:"updateTime"`
	// 统计字段（列表页展示）
	CharacterCount int64 `json:"characterCount"`
	CompletedShots int64 `json:"completedShots"`
	SceneCount     int64 `json:"sceneCount"`
	TotalShots     int64 `json:"totalShots"`
}

type Character struct {
	AssetID           string `json:"assetId"`
	CreateTime        string `json:"createTime"`
	Description       string `json:"description"`
	ID                string `json:"id"`
	IsMain            bool   `json:"isMain"`
	Name              string `json:"name"`
	ProjectID         string `json:"projectId"`
	ReferenceImageURL string `json:"referenceImageUrl"`
}

type Scene struct {
	AssetID           string `json:"assetId"`
	CreateTime        string `json:"createTime"`
	Description       string `json:"description"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	ProjectID         string `json:"projectId"`
	ReferenceImageURL string `json:"referenceImageUrl"`
	ReferenceVideoURL string `json:"referenceVideoUrl"`
}

type Shot struct {
	ActionDescription          string      `json:"actionDescription"`
	AspectRatio                string      `json:"aspectRatio"`
	CameraMovement             string      `json:"cameraMovement"`
	CharacterIDs               []string    `json:"characterIds"`
	CreateTime                 string      `json:"createTime"`
	Duration                   int         `json:"duration"`
	DynamicDescription         string      `json:"dynamicDescription"`
	EndFrameURL                string      `json:"endFrameUrl"`
	ErrorMessage               string      `json:"errorMessage"`
	GeneratedPrompt            string      `json:"generatedPrompt"`
	GenerationID               string      `json:"generationId"`
	GenerationRevision         int         `json:"generationRevision"`
	GridStoryboardPrompt       string      `json:"gridStoryboardPrompt"`
	ID                         string      `json:"id"`
	ImageReferenceModes        []string    `json:"imageReferenceModes"`
	Name                       string      `json:"name"`
	OrderNum                   int         `json:"orderNum"`
	ProjectID                  string      `json:"projectId"`
	SceneID                    string      `json:"sceneId"`
	SelectedGenerationID       string      `json:"selectedGenerationId"`
	SelectedGenerationRevision int         `json:"selectedGenerationRevision"`
	SelectedGenerationStatus   string      `json:"selectedGenerationStatus"`
	SourceKey                  string      `json:"sourceKey"`
	SourceScriptRevision       int         `json:"sourceScriptRevision"`
	ScriptOriginalContent      string      `json:"scriptOriginalContent"`
	ShotAssets                 []ShotAsset `json:"shotAssets"`
	SoundAndPictureTogether    string      `json:"soundAndPictureTogether"`
	Status                     string      `json:"status"`
	StoryboardURL              string      `json:"storyboardUrl"`
	UpdateTime                 string      `json:"updateTime"`
	UsedAudios                 []string    `json:"usedAudios"`
	UsedImages                 []string    `json:"usedImages"`
	UsedVideos                 []string    `json:"usedVideos"`
	VideoModel                 string      `json:"videoModel"`
	VideoResolution            string      `json:"videoResolution"`
	VideoReferenceMode         string      `json:"videoReferenceMode"`
	// 联查生成记录的视频地址（前端预览）
	VideoURL string `json:"videoUrl"`
}

type ShotAsset struct {
	AssetType     string `json:"assetType"`
	CreateTime    string `json:"createTime"`
	ID            string `json:"id"`
	MimeType      string `json:"mimeType"`
	Name          string `json:"name"`
	ObjectURL     string `json:"objectUrl"`
	ReferenceRole string `json:"referenceRole"`
	ShotID        string `json:"shotId"`
	SizeBytes     int64  `json:"sizeBytes"`
	SortOrder     int    `json:"sortOrder"`
	SourceID      string `json:"sourceId"`
	SourceType    string `json:"sourceType"`
	UpdateTime    string `json:"updateTime"`
	UsageNote     string `json:"usageNote"`
}

type ShotAssetInput struct {
	AssetType     string `json:"assetType"`
	MimeType      string `json:"mimeType"`
	Name          string `json:"name"`
	ObjectURL     string `json:"objectUrl"`
	ReferenceRole string `json:"referenceRole"`
	SizeBytes     int64  `json:"sizeBytes"`
	SortOrder     int    `json:"sortOrder"`
	SourceID      string `json:"sourceId"`
	SourceType    string `json:"sourceType"`
	UsageNote     string `json:"usageNote"`
}

type ShotVideoVersion struct {
	AspectRatio        string `json:"aspectRatio"`
	BackupFlag         bool   `json:"backupFlag"`
	CreateTime         string `json:"createTime"`
	ErrorMessage       string `json:"errorMessage"`
	ID                 string `json:"id"`
	IsCurrent          bool   `json:"isCurrent"`
	Model              string `json:"model"`
	Prompt             string `json:"prompt"`
	Seconds            int    `json:"seconds"`
	ShotID             string `json:"shotId"`
	Status             string `json:"status"`
	SubtitleRemove     string `json:"subtitleRemove"`
	UpdateTime         string `json:"updateTime"`
	UpscaledFlag       bool   `json:"upscaledFlag"`
	UpscaledResolution string `json:"upscaledResolution"`
	VideoAssetID       string `json:"videoAssetId"`
	VideoURL           string `json:"videoUrl"`
	ViewedFlag         bool   `json:"viewedFlag"`
}

type ShotVideoVersionDetailReference struct {
	Label string `json:"label"`
	Type  string `json:"type"`
	URL   string `json:"url"`
}

type ShotVideoVersionDetail struct {
	References []ShotVideoVersionDetailReference `json:"references"`
	Shot       Shot                              `json:"shot"`
	Version    ShotVideoVersion                  `json:"version"`
}

type PageResult[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效的 ID")
	}
	return id, nil
}

func toJSONArray(items []string) string {
	if items == nil {
		items = []string{}
	}
	data, _ := json.Marshal(items)
	return string(data)
}

func normalizeRevisionText(raw string) string {
	raw = strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func normalizedStringSet(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func canSelectGeneration(shotID, ownerShotID, status, videoURL string) bool {
	if strings.TrimSpace(shotID) == "" || strings.TrimSpace(shotID) != strings.TrimSpace(ownerShotID) || strings.TrimSpace(videoURL) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "succeeded", "success":
		return true
	default:
		return false
	}
}

func scriptContentChanged(before, after string) bool {
	return normalizeRevisionText(before) != normalizeRevisionText(after)
}

func shotGenerationInputChanged(before, after ShotInput) bool {
	canonical := func(input ShotInput) ShotInput {
		input.ActionDescription = strings.TrimSpace(input.ActionDescription)
		input.DynamicDescription = strings.TrimSpace(input.DynamicDescription)
		input.GridStoryboardPrompt = strings.TrimSpace(input.GridStoryboardPrompt)
		input.ScriptOriginalContent = normalizeRevisionText(input.ScriptOriginalContent)
		input.SoundAndPictureTogether = strings.TrimSpace(input.SoundAndPictureTogether)
		input.StoryboardURL = strings.TrimSpace(input.StoryboardURL)
		input.VideoModel = strings.TrimSpace(input.VideoModel)
		input.VideoResolution = strings.TrimSpace(input.VideoResolution)
		input.VideoReferenceMode = strings.TrimSpace(input.VideoReferenceMode)
		input.CameraMovement = strings.TrimSpace(input.CameraMovement)
		input.SceneID = strings.TrimSpace(input.SceneID)
		input.CharacterIDs = normalizedStringSet(input.CharacterIDs)
		input.ImageReferenceModes = normalizedStringSet(input.ImageReferenceModes)
		input.Name = ""
		return input
	}
	b, _ := json.Marshal(canonical(before))
	a, _ := json.Marshal(canonical(after))
	return string(b) != string(a)
}

func fromJSONArray(raw []byte) []string {
	items := []string{}
	_ = json.Unmarshal(raw, &items)
	return items
}

func jsonbArrayLiteral(raw []byte) string {
	if strings.TrimSpace(string(raw)) == "" {
		return "[]"
	}
	return string(raw)
}

// ---------- 项目 CRUD ----------

type ProjectInput struct {
	Description string `json:"description"`
	Name        string `json:"name"`
	StyleGuide  string `json:"styleGuide"`
	Theme       string `json:"theme"`
}

func (s *Store) CreateProject(ctx context.Context, input ProjectInput) (Project, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Project{}, fmt.Errorf("请填写项目名称")
	}
	var id string
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO video_projects (name, description, theme, style_guide)
		 VALUES ($1,$2,$3,$4) RETURNING id::text`,
		name, strings.TrimSpace(input.Description), strings.TrimSpace(input.Theme), strings.TrimSpace(input.StyleGuide),
	).Scan(&id); err != nil {
		return Project{}, err
	}
	return s.GetProject(ctx, id)
}

func (s *Store) UpdateProject(ctx context.Context, id string, input ProjectInput) (Project, error) {
	pid, err := parseID(id)
	if err != nil {
		return Project{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Project{}, fmt.Errorf("请填写项目名称")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE video_projects
		    SET name=$1, description=$2, theme=$3, style_guide=$4, update_time=now()
		  WHERE id=$5`,
		name, strings.TrimSpace(input.Description), strings.TrimSpace(input.Theme), strings.TrimSpace(input.StyleGuide), pid,
	); err != nil {
		return Project{}, err
	}
	return s.GetProject(ctx, id)
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	pid, err := parseID(id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM video_projects WHERE id=$1`, pid)
	return err
}

func (s *Store) GetProject(ctx context.Context, id string) (Project, error) {
	pid, err := parseID(id)
	if err != nil {
		return Project{}, err
	}
	var p Project
	var createTime, updateTime time.Time
	if err := s.db.QueryRowContext(ctx,
		`SELECT p.id::text, p.name, p.description, p.theme, p.style_guide,
		        p.script_content, p.script_revision, p.final_video_input_hash, p.status,
		        p.compose_status, COALESCE(p.final_video_asset_id::text,''), p.final_video_url,
		        p.create_time, p.update_time,
		        (SELECT count(*) FROM video_project_characters c WHERE c.project_id=p.id),
		        (SELECT count(*) FROM video_project_scenes sc WHERE sc.project_id=p.id),
		        (SELECT count(*) FROM video_shots sh WHERE sh.project_id=p.id),
		        (SELECT count(*) FROM video_shots sh WHERE sh.project_id=p.id AND sh.status='completed')
		   FROM video_projects p WHERE p.id=$1`, pid,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Theme, &p.StyleGuide,
		&p.ScriptContent, &p.ScriptRevision, &p.FinalVideoInputHash, &p.Status,
		&p.ComposeStatus, &p.FinalVideoAssetID, &p.FinalVideoURL,
		&createTime, &updateTime,
		&p.CharacterCount, &p.SceneCount, &p.TotalShots, &p.CompletedShots,
	); err != nil {
		if err == sql.ErrNoRows {
			return Project{}, fmt.Errorf("项目不存在")
		}
		return Project{}, err
	}
	p.CreateTime = formatTime(createTime)
	p.UpdateTime = formatTime(updateTime)
	return p, nil
}

func (s *Store) ListProjects(ctx context.Context, query url.Values) (PageResult[Project], error) {
	page, pageSize := pagination(query)
	where := []string{"1=1"}
	args := []any{}
	if keyword := strings.TrimSpace(query.Get("keyword")); keyword != "" {
		args = append(args, "%"+keyword+"%")
		where = append(where, fmt.Sprintf("(p.name ILIKE $%d OR p.theme ILIKE $%d)", len(args), len(args)))
	}
	condition := strings.Join(where, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx,
		"SELECT count(*) FROM video_projects p WHERE "+condition, args...).Scan(&total); err != nil {
		return PageResult[Project]{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.id::text, p.name, p.description, p.theme, p.style_guide,
		        p.script_content, p.script_revision, p.final_video_input_hash, p.status,
		        p.compose_status, COALESCE(p.final_video_asset_id::text,''), p.final_video_url,
		        p.create_time, p.update_time,
		        (SELECT count(*) FROM video_project_characters c WHERE c.project_id=p.id),
		        (SELECT count(*) FROM video_project_scenes sc WHERE sc.project_id=p.id),
		        (SELECT count(*) FROM video_shots sh WHERE sh.project_id=p.id),
		        (SELECT count(*) FROM video_shots sh WHERE sh.project_id=p.id AND sh.status='completed')
		   FROM video_projects p
		  WHERE `+condition+`
		  ORDER BY p.create_time DESC
		  LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[Project]{}, err
	}
	defer rows.Close()
	items := []Project{}
	for rows.Next() {
		var p Project
		var createTime, updateTime time.Time
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Theme, &p.StyleGuide,
			&p.ScriptContent, &p.ScriptRevision, &p.FinalVideoInputHash, &p.Status,
			&p.ComposeStatus, &p.FinalVideoAssetID, &p.FinalVideoURL,
			&createTime, &updateTime,
			&p.CharacterCount, &p.SceneCount, &p.TotalShots, &p.CompletedShots,
		); err != nil {
			return PageResult[Project]{}, err
		}
		p.CreateTime = formatTime(createTime)
		p.UpdateTime = formatTime(updateTime)
		items = append(items, p)
	}
	return PageResult[Project]{Items: items, Total: total}, rows.Err()
}

// ---------- 角色 CRUD ----------

type CharacterInput struct {
	AssetID           string `json:"assetId"`
	Description       string `json:"description"`
	IsMain            bool   `json:"isMain"`
	Name              string `json:"name"`
	ReferenceImageURL string `json:"referenceImageUrl"`
}

func (s *Store) CreateCharacter(ctx context.Context, projectID string, input CharacterInput) (Character, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return Character{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Character{}, fmt.Errorf("请填写角色名称")
	}
	var assetID any
	if raw := strings.TrimSpace(input.AssetID); raw != "" {
		aid, err := parseID(raw)
		if err != nil {
			return Character{}, fmt.Errorf("无效的资产 ID")
		}
		assetID = aid
	}
	var id string
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO video_project_characters (project_id, asset_id, name, description, reference_image_url, is_main)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		pid, assetID, name, strings.TrimSpace(input.Description), strings.TrimSpace(input.ReferenceImageURL), input.IsMain,
	).Scan(&id); err != nil {
		return Character{}, err
	}
	return s.getCharacter(ctx, id)
}

func (s *Store) UpdateCharacter(ctx context.Context, id string, input CharacterInput) (Character, error) {
	cid, err := parseID(id)
	if err != nil {
		return Character{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Character{}, fmt.Errorf("请填写角色名称")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE video_project_characters
		    SET name=$1, description=$2, reference_image_url=$3, is_main=$4, update_time=now()
		  WHERE id=$5`,
		name, strings.TrimSpace(input.Description), strings.TrimSpace(input.ReferenceImageURL), input.IsMain, cid,
	); err != nil {
		return Character{}, err
	}
	return s.getCharacter(ctx, id)
}

func (s *Store) DeleteCharacter(ctx context.Context, id string) error {
	cid, err := parseID(id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM video_project_characters WHERE id=$1`, cid)
	return err
}

func (s *Store) getCharacter(ctx context.Context, id string) (Character, error) {
	cid, err := parseID(id)
	if err != nil {
		return Character{}, err
	}
	var c Character
	var createTime time.Time
	if err := s.db.QueryRowContext(ctx,
		`SELECT id::text, project_id::text, COALESCE(asset_id::text,''), name, description,
		        reference_image_url, is_main, create_time
		   FROM video_project_characters WHERE id=$1`, cid,
	).Scan(&c.ID, &c.ProjectID, &c.AssetID, &c.Name, &c.Description,
		&c.ReferenceImageURL, &c.IsMain, &createTime,
	); err != nil {
		if err == sql.ErrNoRows {
			return Character{}, fmt.Errorf("角色不存在")
		}
		return Character{}, err
	}
	c.CreateTime = formatTime(createTime)
	return c, nil
}

func (s *Store) ListCharacters(ctx context.Context, projectID string) ([]Character, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text, project_id::text, COALESCE(asset_id::text,''), name, description,
		        reference_image_url, is_main, create_time
		   FROM video_project_characters
		  WHERE project_id=$1
		  ORDER BY is_main DESC, create_time ASC`, pid,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Character{}
	for rows.Next() {
		var c Character
		var createTime time.Time
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.AssetID, &c.Name, &c.Description,
			&c.ReferenceImageURL, &c.IsMain, &createTime,
		); err != nil {
			return nil, err
		}
		c.CreateTime = formatTime(createTime)
		items = append(items, c)
	}
	return items, rows.Err()
}

// ---------- 场景 CRUD ----------

type SceneInput struct {
	AssetID           string `json:"assetId"`
	Description       string `json:"description"`
	Name              string `json:"name"`
	ReferenceImageURL string `json:"referenceImageUrl"`
	ReferenceVideoURL string `json:"referenceVideoUrl"`
}

func (s *Store) CreateScene(ctx context.Context, projectID string, input SceneInput) (Scene, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return Scene{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Scene{}, fmt.Errorf("请填写场景名称")
	}
	var assetID any
	if raw := strings.TrimSpace(input.AssetID); raw != "" {
		aid, err := parseID(raw)
		if err != nil {
			return Scene{}, fmt.Errorf("无效的资产 ID")
		}
		assetID = aid
	}
	var id string
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO video_project_scenes (project_id, asset_id, name, description, reference_image_url, reference_video_url)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		pid, assetID, name, strings.TrimSpace(input.Description),
		strings.TrimSpace(input.ReferenceImageURL), strings.TrimSpace(input.ReferenceVideoURL),
	).Scan(&id); err != nil {
		return Scene{}, err
	}
	return s.getScene(ctx, id)
}

func (s *Store) UpdateScene(ctx context.Context, id string, input SceneInput) (Scene, error) {
	sid, err := parseID(id)
	if err != nil {
		return Scene{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Scene{}, fmt.Errorf("请填写场景名称")
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE video_project_scenes
		    SET name=$1, description=$2, reference_image_url=$3, reference_video_url=$4, update_time=now()
		  WHERE id=$5`,
		name, strings.TrimSpace(input.Description),
		strings.TrimSpace(input.ReferenceImageURL), strings.TrimSpace(input.ReferenceVideoURL), sid,
	); err != nil {
		return Scene{}, err
	}
	return s.getScene(ctx, id)
}

func (s *Store) DeleteScene(ctx context.Context, id string) error {
	sid, err := parseID(id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM video_project_scenes WHERE id=$1`, sid)
	return err
}

func (s *Store) getScene(ctx context.Context, id string) (Scene, error) {
	sid, err := parseID(id)
	if err != nil {
		return Scene{}, err
	}
	var sc Scene
	var createTime time.Time
	if err := s.db.QueryRowContext(ctx,
		`SELECT id::text, project_id::text, COALESCE(asset_id::text,''), name, description,
		        reference_image_url, reference_video_url, create_time
		   FROM video_project_scenes WHERE id=$1`, sid,
	).Scan(&sc.ID, &sc.ProjectID, &sc.AssetID, &sc.Name, &sc.Description,
		&sc.ReferenceImageURL, &sc.ReferenceVideoURL, &createTime,
	); err != nil {
		if err == sql.ErrNoRows {
			return Scene{}, fmt.Errorf("场景不存在")
		}
		return Scene{}, err
	}
	sc.CreateTime = formatTime(createTime)
	return sc, nil
}

func (s *Store) ListScenes(ctx context.Context, projectID string) ([]Scene, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text, project_id::text, COALESCE(asset_id::text,''), name, description,
		        reference_image_url, reference_video_url, create_time
		   FROM video_project_scenes
		  WHERE project_id=$1
		  ORDER BY create_time ASC`, pid,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Scene{}
	for rows.Next() {
		var sc Scene
		var createTime time.Time
		if err := rows.Scan(&sc.ID, &sc.ProjectID, &sc.AssetID, &sc.Name, &sc.Description,
			&sc.ReferenceImageURL, &sc.ReferenceVideoURL, &createTime,
		); err != nil {
			return nil, err
		}
		sc.CreateTime = formatTime(createTime)
		items = append(items, sc)
	}
	return items, rows.Err()
}

// ---------- 分镜 CRUD ----------

type ShotInput struct {
	ActionDescription       string   `json:"actionDescription"`
	AspectRatio             string   `json:"aspectRatio"`
	CameraMovement          string   `json:"cameraMovement"`
	CharacterIDs            []string `json:"characterIds"`
	Duration                int      `json:"duration"`
	DynamicDescription      string   `json:"dynamicDescription"`
	GridStoryboardPrompt    string   `json:"gridStoryboardPrompt"`
	ImageReferenceModes     []string `json:"imageReferenceModes"`
	Name                    string   `json:"name"`
	OrderNum                int      `json:"orderNum"`
	SceneID                 string   `json:"sceneId"`
	ScriptOriginalContent   string   `json:"scriptOriginalContent"`
	SoundAndPictureTogether string   `json:"soundAndPictureTogether"`
	SourceKey               string   `json:"sourceKey"`
	SourceScriptRevision    int      `json:"sourceScriptRevision"`
	StoryboardURL           string   `json:"storyboardUrl"`
	VideoModel              string   `json:"videoModel"`
	VideoResolution         string   `json:"videoResolution"`
	VideoReferenceMode      string   `json:"videoReferenceMode"`
}

var allowedImageRefModes = map[string]bool{"prev_frame": true, "character_ref": true, "scene_ref": true}
var allowedVideoRefModes = map[string]bool{"none": true, "prev_video": true, "scene_demo": true}
var allowedDurations = map[int]bool{5: true, 10: true, 15: true}
var allowedAspectRatios = map[string]bool{"16:9": true, "9:16": true, "1:1": true}

func normalizeShotInput(input *ShotInput) error {
	input.ActionDescription = strings.TrimSpace(input.ActionDescription)
	if input.ActionDescription == "" {
		return fmt.Errorf("请填写动作描述")
	}
	if input.Duration == 0 {
		input.Duration = 15
	}
	if !allowedDurations[input.Duration] {
		return fmt.Errorf("时长仅支持 5/10/15 秒")
	}
	if input.AspectRatio == "" {
		input.AspectRatio = "16:9"
	}
	if !allowedAspectRatios[input.AspectRatio] {
		return fmt.Errorf("画幅仅支持 16:9 / 9:16 / 1:1")
	}
	if input.VideoReferenceMode == "" {
		input.VideoReferenceMode = "none"
	}
	if !allowedVideoRefModes[input.VideoReferenceMode] {
		return fmt.Errorf("无效的视频参考模式")
	}
	// 图片参考模式：默认使用「上一帧 + 角色标准照」，这是降低抽卡率的推荐组合。
	if len(input.ImageReferenceModes) == 0 {
		input.ImageReferenceModes = []string{"prev_frame", "character_ref"}
	}
	cleaned := []string{}
	for _, mode := range input.ImageReferenceModes {
		mode = strings.TrimSpace(mode)
		if mode == "" {
			continue
		}
		if !allowedImageRefModes[mode] {
			return fmt.Errorf("无效的图片参考模式: %s", mode)
		}
		cleaned = append(cleaned, mode)
	}
	input.ImageReferenceModes = cleaned
	return nil
}

func (s *Store) CreateShot(ctx context.Context, projectID string, input ShotInput) (Shot, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return Shot{}, err
	}
	if err := normalizeShotInput(&input); err != nil {
		return Shot{}, err
	}
	// order_num 未指定时自动排到末尾。
	if input.OrderNum <= 0 {
		_ = s.db.QueryRowContext(ctx,
			`SELECT COALESCE(max(order_num),0)+1 FROM video_shots WHERE project_id=$1`, pid,
		).Scan(&input.OrderNum)
	}
	var sceneID any
	if raw := strings.TrimSpace(input.SceneID); raw != "" {
		sid, err := parseID(raw)
		if err != nil {
			return Shot{}, fmt.Errorf("无效的场景 ID")
		}
		sceneID = sid
	}
	var id string
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO video_shots (project_id, order_num, name, script_original_content,
		                          action_description, dynamic_description, grid_storyboard_prompt, storyboard_url,
		                          video_model, video_resolution, sound_and_picture_together,
		                          duration, aspect_ratio, character_ids, scene_id, image_reference_modes,
		                          video_reference_mode, camera_movement, source_key, source_script_revision)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15,$16::jsonb,$17,$18,$19,$20) RETURNING id::text`,
		pid, input.OrderNum, strings.TrimSpace(input.Name), strings.TrimSpace(input.ScriptOriginalContent),
		input.ActionDescription, strings.TrimSpace(input.DynamicDescription), strings.TrimSpace(input.GridStoryboardPrompt),
		strings.TrimSpace(input.StoryboardURL), strings.TrimSpace(input.VideoModel),
		strings.TrimSpace(input.VideoResolution), strings.TrimSpace(input.SoundAndPictureTogether),
		input.Duration, input.AspectRatio, toJSONArray(input.CharacterIDs), sceneID,
		toJSONArray(input.ImageReferenceModes), input.VideoReferenceMode, strings.TrimSpace(input.CameraMovement),
		strings.TrimSpace(input.SourceKey), input.SourceScriptRevision,
	).Scan(&id); err != nil {
		return Shot{}, err
	}
	return s.GetShot(ctx, id)
}

func (s *Store) UpdateShot(ctx context.Context, id string, input ShotInput) (Shot, error) {
	shotID, err := parseID(id)
	if err != nil {
		return Shot{}, err
	}
	if err := normalizeShotInput(&input); err != nil {
		return Shot{}, err
	}
	var sceneID any
	if raw := strings.TrimSpace(input.SceneID); raw != "" {
		sid, err := parseID(raw)
		if err != nil {
			return Shot{}, fmt.Errorf("无效的场景 ID")
		}
		sceneID = sid
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE video_shots
		    SET name=$1, script_original_content=$2, action_description=$3,
		        dynamic_description=$4, grid_storyboard_prompt=$5, storyboard_url=$6,
		        video_model=$7, video_resolution=$8, sound_and_picture_together=$9,
		        duration=$10, aspect_ratio=$11,
		        character_ids=$12::jsonb, scene_id=$13, image_reference_modes=$14::jsonb,
		        video_reference_mode=$15, camera_movement=$16,
		        order_num=CASE WHEN $17 > 0 THEN $17 ELSE order_num END,
		        source_key=CASE WHEN btrim($18)<>'' THEN btrim($18) ELSE source_key END,
		        source_script_revision=CASE WHEN $19>0 THEN $19 ELSE source_script_revision END,
		        generation_revision=generation_revision+CASE WHEN
		          btrim(action_description) IS DISTINCT FROM btrim($3)
		          OR btrim(dynamic_description) IS DISTINCT FROM btrim($4)
		          OR btrim(grid_storyboard_prompt) IS DISTINCT FROM btrim($5)
		          OR btrim(storyboard_url) IS DISTINCT FROM btrim($6)
		          OR btrim(video_model) IS DISTINCT FROM btrim($7)
		          OR btrim(video_resolution) IS DISTINCT FROM btrim($8)
		          OR btrim(sound_and_picture_together) IS DISTINCT FROM btrim($9)
		          OR duration IS DISTINCT FROM $10
		          OR aspect_ratio IS DISTINCT FROM $11
		          OR character_ids IS DISTINCT FROM $12::jsonb
		          OR scene_id IS DISTINCT FROM $13
		          OR image_reference_modes IS DISTINCT FROM $14::jsonb
		          OR video_reference_mode IS DISTINCT FROM $15
		          OR btrim(camera_movement) IS DISTINCT FROM btrim($16)
		          OR btrim(script_original_content) IS DISTINCT FROM btrim($2)
		        THEN 1 ELSE 0 END,
		        update_time=now()
		  WHERE id=$20`,
		strings.TrimSpace(input.Name), strings.TrimSpace(input.ScriptOriginalContent), input.ActionDescription,
		strings.TrimSpace(input.DynamicDescription), strings.TrimSpace(input.GridStoryboardPrompt), strings.TrimSpace(input.StoryboardURL),
		strings.TrimSpace(input.VideoModel), strings.TrimSpace(input.VideoResolution), strings.TrimSpace(input.SoundAndPictureTogether),
		input.Duration, input.AspectRatio,
		toJSONArray(input.CharacterIDs), sceneID, toJSONArray(input.ImageReferenceModes),
		input.VideoReferenceMode, strings.TrimSpace(input.CameraMovement), input.OrderNum,
		strings.TrimSpace(input.SourceKey), input.SourceScriptRevision, shotID,
	); err != nil {
		return Shot{}, err
	}
	return s.GetShot(ctx, id)
}

func (s *Store) DeleteShot(ctx context.Context, id string) error {
	shotID, err := parseID(id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM video_shots WHERE id=$1`, shotID)
	return err
}

const shotSelectColumns = `s.id::text, s.project_id::text, s.order_num, s.name,
	        s.script_original_content, s.action_description, s.dynamic_description,
	        s.grid_storyboard_prompt, s.storyboard_url,
	        s.video_model, s.video_resolution, s.sound_and_picture_together,
	        s.duration, s.aspect_ratio, s.character_ids, COALESCE(s.scene_id::text,''),
	        s.image_reference_modes, s.video_reference_mode, s.camera_movement,
	        COALESCE(s.generation_id::text,''), COALESCE(s.selected_generation_id::text,''),
	        s.generation_revision, s.source_key, s.source_script_revision,
	        s.generated_prompt, s.used_images, s.used_videos, s.used_audios,
	        s.end_frame_url, s.status, s.error_message, s.create_time, s.update_time,
	        COALESCE(g.video_url,''), COALESCE(g.status,''), COALESCE(g.shot_revision,0)`

func scanShot(scanner interface{ Scan(...any) error }) (Shot, error) {
	var sh Shot
	var characterIDs, imageModes, usedImages, usedVideos, usedAudios []byte
	var createTime, updateTime time.Time
	if err := scanner.Scan(&sh.ID, &sh.ProjectID, &sh.OrderNum, &sh.Name,
		&sh.ScriptOriginalContent, &sh.ActionDescription, &sh.DynamicDescription,
		&sh.GridStoryboardPrompt, &sh.StoryboardURL,
		&sh.VideoModel, &sh.VideoResolution, &sh.SoundAndPictureTogether,
		&sh.Duration, &sh.AspectRatio, &characterIDs, &sh.SceneID,
		&imageModes, &sh.VideoReferenceMode, &sh.CameraMovement,
		&sh.GenerationID, &sh.SelectedGenerationID, &sh.GenerationRevision, &sh.SourceKey, &sh.SourceScriptRevision,
		&sh.GeneratedPrompt, &usedImages, &usedVideos,
		&usedAudios, &sh.EndFrameURL, &sh.Status, &sh.ErrorMessage, &createTime, &updateTime,
		&sh.VideoURL, &sh.SelectedGenerationStatus, &sh.SelectedGenerationRevision,
	); err != nil {
		return Shot{}, err
	}
	sh.CharacterIDs = fromJSONArray(characterIDs)
	sh.ImageReferenceModes = fromJSONArray(imageModes)
	sh.UsedImages = fromJSONArray(usedImages)
	sh.UsedVideos = fromJSONArray(usedVideos)
	sh.UsedAudios = fromJSONArray(usedAudios)
	sh.CreateTime = formatTime(createTime)
	sh.UpdateTime = formatTime(updateTime)
	return sh, nil
}

func (s *Store) GetShot(ctx context.Context, id string) (Shot, error) {
	shotID, err := parseID(id)
	if err != nil {
		return Shot{}, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+shotSelectColumns+`
		   FROM video_shots s
		   LEFT JOIN video_generations g ON COALESCE(s.selected_generation_id, s.generation_id) = g.id
		  WHERE s.id=$1`, shotID,
	)
	sh, err := scanShot(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Shot{}, fmt.Errorf("分镜不存在")
		}
		return Shot{}, err
	}
	assets, err := s.ListShotAssets(ctx, id)
	if err != nil {
		return Shot{}, err
	}
	sh.ShotAssets = assets
	return sh, nil
}

func (s *Store) ListShots(ctx context.Context, projectID string) ([]Shot, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+shotSelectColumns+`
		   FROM video_shots s
		   LEFT JOIN video_generations g ON COALESCE(s.selected_generation_id, s.generation_id) = g.id
		  WHERE s.project_id=$1
		  ORDER BY s.order_num ASC, s.id ASC`, pid,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Shot{}
	for rows.Next() {
		sh, err := scanShot(rows)
		if err != nil {
			return nil, err
		}
		assets, err := s.ListShotAssets(ctx, sh.ID)
		if err != nil {
			return nil, err
		}
		sh.ShotAssets = assets
		items = append(items, sh)
	}
	return items, rows.Err()
}

var allowedShotAssetTypes = map[string]bool{"image": true, "video": true, "audio": true}
var allowedShotReferenceRoles = map[string]string{
	"reference_image": "image",
	"first_frame":     "image",
	"last_frame":      "image",
	"reference_video": "video",
	"reference_audio": "audio",
	"edit_target":     "video",
	"extend_target":   "video",
}

func normalizeShotAssetInput(input *ShotAssetInput) error {
	input.AssetType = strings.TrimSpace(input.AssetType)
	if input.AssetType == "" {
		input.AssetType = "image"
	}
	if !allowedShotAssetTypes[input.AssetType] {
		return fmt.Errorf("无效的分镜素材类型")
	}
	input.ObjectURL = strings.TrimSpace(input.ObjectURL)
	if input.ObjectURL == "" {
		return fmt.Errorf("请上传分镜素材")
	}
	if _, err := requirePublicAssetObjectURL("分镜参考素材", input.ObjectURL); err != nil {
		return err
	}
	input.Name = strings.TrimSpace(input.Name)
	input.MimeType = strings.TrimSpace(input.MimeType)
	input.ReferenceRole = strings.TrimSpace(input.ReferenceRole)
	if input.ReferenceRole == "" {
		input.ReferenceRole = defaultShotReferenceRole(input.AssetType)
	}
	if requiredType, exists := allowedShotReferenceRoles[input.ReferenceRole]; !exists || requiredType != input.AssetType {
		return fmt.Errorf("分镜素材类型与用途不匹配")
	}
	if input.SortOrder < 0 {
		return fmt.Errorf("分镜素材排序不能小于 0")
	}
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.UsageNote = strings.TrimSpace(input.UsageNote)
	if input.SizeBytes < 0 {
		input.SizeBytes = 0
	}
	return nil
}

func defaultShotReferenceRole(assetType string) string {
	switch assetType {
	case "video":
		return "reference_video"
	case "audio":
		return "reference_audio"
	default:
		return "reference_image"
	}
}

func scanShotAsset(scanner interface{ Scan(...any) error }) (ShotAsset, error) {
	var asset ShotAsset
	var createTime, updateTime time.Time
	if err := scanner.Scan(
		&asset.ID,
		&asset.ShotID,
		&asset.AssetType,
		&asset.ObjectURL,
		&asset.Name,
		&asset.MimeType,
		&asset.SizeBytes,
		&asset.ReferenceRole,
		&asset.SortOrder,
		&asset.SourceType,
		&asset.SourceID,
		&asset.UsageNote,
		&createTime,
		&updateTime,
	); err != nil {
		return ShotAsset{}, err
	}
	asset.CreateTime = formatTime(createTime)
	asset.UpdateTime = formatTime(updateTime)
	return asset, nil
}

func (s *Store) ListShotAssets(ctx context.Context, shotID string) ([]ShotAsset, error) {
	sid, err := parseID(shotID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id::text, shot_id::text, asset_type, object_url, name, mime_type, size_bytes,
		        reference_role, sort_order, source_type, source_id, usage_note, create_time, update_time
		   FROM video_shot_assets
		  WHERE shot_id=$1
		  ORDER BY sort_order ASC, id ASC`, sid,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ShotAsset{}
	for rows.Next() {
		asset, err := scanShotAsset(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, asset)
	}
	return items, rows.Err()
}

func (s *Store) CreateShotAsset(ctx context.Context, shotID string, input ShotAssetInput) (ShotAsset, error) {
	sid, err := parseID(shotID)
	if err != nil {
		return ShotAsset{}, err
	}
	if err := normalizeShotAssetInput(&input); err != nil {
		return ShotAsset{}, err
	}
	var id string
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO video_shot_assets (
			shot_id, asset_type, object_url, name, mime_type, size_bytes,
			reference_role, sort_order, source_type, source_id, usage_note
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 RETURNING id::text`,
		sid, input.AssetType, input.ObjectURL, input.Name, input.MimeType, input.SizeBytes,
		input.ReferenceRole, input.SortOrder, input.SourceType, input.SourceID, input.UsageNote,
	).Scan(&id); err != nil {
		return ShotAsset{}, err
	}
	return s.GetShotAsset(ctx, id)
}

func (s *Store) GetShotAsset(ctx context.Context, id string) (ShotAsset, error) {
	assetID, err := parseID(id)
	if err != nil {
		return ShotAsset{}, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id::text, shot_id::text, asset_type, object_url, name, mime_type, size_bytes,
		        reference_role, sort_order, source_type, source_id, usage_note, create_time, update_time
		   FROM video_shot_assets
		  WHERE id=$1`, assetID,
	)
	asset, err := scanShotAsset(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ShotAsset{}, fmt.Errorf("分镜素材不存在")
		}
		return ShotAsset{}, err
	}
	return asset, nil
}

func (s *Store) DeleteShotAsset(ctx context.Context, id string) error {
	assetID, err := parseID(id)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM video_shot_assets WHERE id=$1`, assetID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return fmt.Errorf("分镜素材不存在")
	}
	return nil
}

func scanShotVideoVersion(scanner interface{ Scan(...any) error }) (ShotVideoVersion, error) {
	var version ShotVideoVersion
	var createTime, updateTime time.Time
	if err := scanner.Scan(
		&version.ID,
		&version.ShotID,
		&version.Model,
		&version.Prompt,
		&version.Status,
		&version.ErrorMessage,
		&version.VideoURL,
		&version.VideoAssetID,
		&version.Seconds,
		&version.AspectRatio,
		&version.IsCurrent,
		&version.ViewedFlag,
		&version.BackupFlag,
		&version.SubtitleRemove,
		&version.UpscaledFlag,
		&version.UpscaledResolution,
		&createTime,
		&updateTime,
	); err != nil {
		return ShotVideoVersion{}, err
	}
	version.CreateTime = formatTime(createTime)
	version.UpdateTime = formatTime(updateTime)
	return version, nil
}

func shotStatusFromGenerationStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "succeeded", "success":
		return "completed"
	case "failed", "error":
		return "failed"
	case "queued", "pending", "submitted", "in_progress", "processing", "running":
		return "generating"
	default:
		return "draft"
	}
}

func (s *Store) ListShotVideoVersions(ctx context.Context, shotID string) ([]ShotVideoVersion, error) {
	sid, err := parseID(shotID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT g.id::text,
		        s.id::text,
		        g.model,
		        g.prompt,
		        g.status,
		        g.error_message,
		        g.video_url,
		        COALESCE(g.video_asset_id::text,''),
		        g.seconds,
		        g.aspect_ratio,
		        COALESCE(s.generation_id = g.id, false) AS is_current,
		        COALESCE(g.viewed_flag, false),
		        COALESCE(g.backup_flag, false),
		        COALESCE(g.subtitle_remove, ''),
		        COALESCE(g.upscaled_flag, false),
		        COALESCE(g.upscaled_resolution, ''),
		        g.create_time,
		        g.update_time
		   FROM video_shots s
		   JOIN video_generations g ON g.shot_id = s.id OR g.id = s.generation_id
		  WHERE s.id=$1
		  ORDER BY COALESCE(s.generation_id = g.id, false) DESC, COALESCE(g.backup_flag, false) DESC, g.create_time DESC, g.id DESC`,
		sid,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ShotVideoVersion{}
	for rows.Next() {
		version, err := scanShotVideoVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, version)
	}
	return items, rows.Err()
}

func (s *Store) GetShotVideoVersion(ctx context.Context, shotID, generationID string) (ShotVideoVersion, error) {
	sid, err := parseID(shotID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	gid, err := parseID(generationID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT g.id::text,
		        s.id::text,
		        g.model,
		        g.prompt,
		        g.status,
		        g.error_message,
		        g.video_url,
		        COALESCE(g.video_asset_id::text,''),
		        g.seconds,
		        g.aspect_ratio,
		        COALESCE(s.generation_id = g.id, false) AS is_current,
		        COALESCE(g.viewed_flag, false),
		        COALESCE(g.backup_flag, false),
		        COALESCE(g.subtitle_remove, ''),
		        COALESCE(g.upscaled_flag, false),
		        COALESCE(g.upscaled_resolution, ''),
		        g.create_time,
		        g.update_time
		   FROM video_shots s
		   JOIN video_generations g ON g.id=$2
		  WHERE s.id=$1 AND (g.shot_id = s.id OR g.id = s.generation_id)`,
		sid, gid,
	)
	version, err := scanShotVideoVersion(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ShotVideoVersion{}, fmt.Errorf("视频版本不存在或不属于当前分镜")
		}
		return ShotVideoVersion{}, err
	}
	return version, nil
}

func (s *Store) GetShotVideoVersionDetail(ctx context.Context, shotID, generationID string) (ShotVideoVersionDetail, error) {
	shot, err := s.GetShot(ctx, shotID)
	if err != nil {
		return ShotVideoVersionDetail{}, err
	}
	version, err := s.GetShotVideoVersion(ctx, shotID, generationID)
	if err != nil {
		return ShotVideoVersionDetail{}, err
	}
	getShotVideoVersionUsedReferences := func() ([]string, []string, []string, error) {
		sid, err := parseID(shotID)
		if err != nil {
			return nil, nil, nil, err
		}
		gid, err := parseID(generationID)
		if err != nil {
			return nil, nil, nil, err
		}
		var generationUsedImages, generationUsedVideos, generationUsedAudios []byte
		err = s.db.QueryRowContext(ctx,
			`SELECT g.used_images, g.used_videos, g.used_audios
			   FROM video_shots s
			   JOIN video_generations g ON g.id=$2
			  WHERE s.id=$1 AND (g.shot_id = s.id OR g.id = s.generation_id)`,
			sid, gid,
		).Scan(&generationUsedImages, &generationUsedVideos, &generationUsedAudios)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, nil, nil, fmt.Errorf("视频版本不存在或不属于当前分镜")
			}
			return nil, nil, nil, err
		}
		return fromJSONArray(generationUsedImages), fromJSONArray(generationUsedVideos), fromJSONArray(generationUsedAudios), nil
	}
	generationUsedImages, generationUsedVideos, generationUsedAudios, err := getShotVideoVersionUsedReferences()
	if err != nil {
		return ShotVideoVersionDetail{}, err
	}

	references := []ShotVideoVersionDetailReference{}
	seen := map[string]bool{}
	pushReference := func(assetType, url, label string) {
		assetType = strings.TrimSpace(assetType)
		url = strings.TrimSpace(url)
		if assetType == "" || url == "" {
			return
		}
		key := assetType + ":" + url
		if seen[key] {
			return
		}
		seen[key] = true
		if strings.TrimSpace(label) == "" {
			switch assetType {
			case "image":
				label = "参考图片"
			case "video":
				label = "参考视频"
			case "audio":
				label = "参考音频"
			default:
				label = "参考素材"
			}
		}
		references = append(references, ShotVideoVersionDetailReference{
			Label: label,
			Type:  assetType,
			URL:   url,
		})
	}

	fallbackToCurrentShotUsedReferences := version.IsCurrent &&
		len(generationUsedImages) == 0 &&
		len(generationUsedVideos) == 0 &&
		len(generationUsedAudios) == 0
	if fallbackToCurrentShotUsedReferences {
		for _, asset := range shot.ShotAssets {
			if asset.AssetType == "image" || asset.AssetType == "video" || asset.AssetType == "audio" {
				pushReference(asset.AssetType, asset.ObjectURL, asset.Name)
			}
		}
		generationUsedImages = shot.UsedImages
		generationUsedVideos = shot.UsedVideos
		generationUsedAudios = shot.UsedAudios
	}
	for _, image := range generationUsedImages {
		pushReference("image", image, "生成使用图片")
	}
	for _, videoURL := range generationUsedVideos {
		pushReference("video", videoURL, "生成使用视频")
	}
	for _, audio := range generationUsedAudios {
		pushReference("audio", audio, "生成使用音频")
	}

	return ShotVideoVersionDetail{
		References: references,
		Shot:       shot,
		Version:    version,
	}, nil
}

func (s *Store) SetShotVideoVersionBackup(ctx context.Context, shotID, generationID string, backupFlag bool) (ShotVideoVersion, error) {
	sid, err := parseID(shotID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	gid, err := parseID(generationID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE video_generations g
		    SET backup_flag=$3, update_time=now()
		   FROM video_shots s
		  WHERE s.id=$1
		    AND g.id=$2
		    AND (g.shot_id = s.id OR g.id = s.generation_id)`,
		sid, gid, backupFlag,
	)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ShotVideoVersion{}, fmt.Errorf("视频版本不存在或不属于当前分镜")
	}
	return s.GetShotVideoVersion(ctx, shotID, generationID)
}

func (s *Store) CreateSubtitleRemovedShotVideoVersion(ctx context.Context, shotID, generationID string, assetID int64, videoURL string) (ShotVideoVersion, error) {
	sid, err := parseID(shotID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	gid, err := parseID(generationID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	videoURL = strings.TrimSpace(videoURL)
	if assetID <= 0 || videoURL == "" {
		return ShotVideoVersion{}, fmt.Errorf("无字幕视频资产无效")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	defer tx.Rollback()

	var projectID int64
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM video_shots WHERE id=$1`, sid).Scan(&projectID); err != nil {
		if err == sql.ErrNoRows {
			return ShotVideoVersion{}, fmt.Errorf("分镜不存在")
		}
		return ShotVideoVersion{}, err
	}

	var newID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO video_generations (
		     provider, model, prompt, image_url, task_id, seconds, aspect_ratio,
		     video_asset_id, video_url, duration, fps, width, height,
		     status, error_message, viewed_flag, backup_flag, subtitle_remove,
		     used_images, used_videos, used_audios, project_id, shot_id
		   )
		   SELECT 'local-subtitle-removal', g.model, g.prompt, g.image_url, '', g.seconds, g.aspect_ratio,
		          $3, $4, g.duration, g.fps, g.width, g.height,
		          'completed', '', false, false, 'REMOVED',
		          CASE WHEN jsonb_array_length(g.used_images) = 0 AND s.generation_id = g.id THEN s.used_images ELSE g.used_images END,
		          CASE WHEN jsonb_array_length(g.used_videos) = 0 AND s.generation_id = g.id THEN s.used_videos ELSE g.used_videos END,
		          CASE WHEN jsonb_array_length(g.used_audios) = 0 AND s.generation_id = g.id THEN s.used_audios ELSE g.used_audios END,
		          $5, $1
		     FROM video_generations g
		     JOIN video_shots s ON s.id=$1
		    WHERE g.id=$2 AND (g.shot_id = s.id OR g.id = s.generation_id)
		   RETURNING id`,
		sid, gid, assetID, videoURL, projectID,
	).Scan(&newID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ShotVideoVersion{}, fmt.Errorf("视频版本不存在或不属于当前分镜")
		}
		return ShotVideoVersion{}, err
	}

	if err := tx.Commit(); err != nil {
		return ShotVideoVersion{}, err
	}
	return s.GetShotVideoVersion(ctx, shotID, fmt.Sprint(newID))
}

func (s *Store) CreateUpscaledShotVideoVersion(ctx context.Context, shotID, generationID string, assetID int64, videoURL, resolution string, width, height int) (ShotVideoVersion, error) {
	sid, err := parseID(shotID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	gid, err := parseID(generationID)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	videoURL = strings.TrimSpace(videoURL)
	resolution = strings.TrimSpace(resolution)
	if assetID <= 0 || videoURL == "" || resolution == "" {
		return ShotVideoVersion{}, fmt.Errorf("超分视频资产无效")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ShotVideoVersion{}, err
	}
	defer tx.Rollback()

	var projectID int64
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM video_shots WHERE id=$1`, sid).Scan(&projectID); err != nil {
		if err == sql.ErrNoRows {
			return ShotVideoVersion{}, fmt.Errorf("分镜不存在")
		}
		return ShotVideoVersion{}, err
	}

	var newID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO video_generations (
		     provider, model, prompt, image_url, task_id, seconds, aspect_ratio,
		     video_asset_id, video_url, duration, fps, width, height,
		     status, error_message, viewed_flag, backup_flag, subtitle_remove,
		     upscaled_flag, upscaled_resolution, used_images, used_videos, used_audios, project_id, shot_id
		   )
		   SELECT 'local-upscale', g.model, g.prompt, g.image_url, '', g.seconds, g.aspect_ratio,
		          $3, $4, g.duration, g.fps, $6, $7,
		          'completed', '', false, false, g.subtitle_remove,
		          true, $5,
		          CASE WHEN jsonb_array_length(g.used_images) = 0 AND s.generation_id = g.id THEN s.used_images ELSE g.used_images END,
		          CASE WHEN jsonb_array_length(g.used_videos) = 0 AND s.generation_id = g.id THEN s.used_videos ELSE g.used_videos END,
		          CASE WHEN jsonb_array_length(g.used_audios) = 0 AND s.generation_id = g.id THEN s.used_audios ELSE g.used_audios END,
		          $8, $1
		     FROM video_generations g
		     JOIN video_shots s ON s.id=$1
		    WHERE g.id=$2 AND (g.shot_id = s.id OR g.id = s.generation_id)
		   RETURNING id`,
		sid, gid, assetID, videoURL, resolution, width, height, projectID,
	).Scan(&newID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ShotVideoVersion{}, fmt.Errorf("视频版本不存在或不属于当前分镜")
		}
		return ShotVideoVersion{}, err
	}

	if err := tx.Commit(); err != nil {
		return ShotVideoVersion{}, err
	}
	return s.GetShotVideoVersion(ctx, shotID, fmt.Sprint(newID))
}

func (s *Store) SetShotVideoVersion(ctx context.Context, shotID, generationID string) (Shot, error) {
	sid, err := parseID(shotID)
	if err != nil {
		return Shot{}, err
	}
	gid, err := parseID(generationID)
	if err != nil {
		return Shot{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Shot{}, err
	}
	defer tx.Rollback()

	var projectID int64
	var genStatus, genPrompt, genError string
	var genUsedImages, genUsedVideos, genUsedAudios []byte
	err = tx.QueryRowContext(ctx,
		`SELECT s.project_id, g.status, g.prompt, g.error_message,
		        CASE WHEN jsonb_array_length(g.used_images) = 0 AND s.generation_id = g.id THEN s.used_images ELSE g.used_images END,
		        CASE WHEN jsonb_array_length(g.used_videos) = 0 AND s.generation_id = g.id THEN s.used_videos ELSE g.used_videos END,
		        CASE WHEN jsonb_array_length(g.used_audios) = 0 AND s.generation_id = g.id THEN s.used_audios ELSE g.used_audios END
		   FROM video_shots s
		   JOIN video_generations g ON g.id=$2
		  WHERE s.id=$1 AND (g.shot_id = s.id OR g.id = s.generation_id)`,
		sid, gid,
	).Scan(&projectID, &genStatus, &genPrompt, &genError, &genUsedImages, &genUsedVideos, &genUsedAudios)
	if err != nil {
		if err == sql.ErrNoRows {
			return Shot{}, fmt.Errorf("视频版本不存在或不属于当前分镜")
		}
		return Shot{}, err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE video_generations
		    SET project_id=$1, shot_id=$2, update_time=now()
		  WHERE id=$3`,
		projectID, sid, gid,
	); err != nil {
		return Shot{}, err
	}

	shotStatus := shotStatusFromGenerationStatus(genStatus)
	if _, err := tx.ExecContext(ctx,
		`UPDATE video_shots
		    SET generation_id=$1,
		        generated_prompt=$2,
		        status=$3,
		        error_message=CASE WHEN $3='failed' THEN $4 ELSE '' END,
		        used_images=$5::jsonb,
		        used_videos=$6::jsonb,
		        used_audios=$7::jsonb,
		        update_time=now()
		  WHERE id=$8`,
		gid, genPrompt, shotStatus, genError,
		jsonbArrayLiteral(genUsedImages), jsonbArrayLiteral(genUsedVideos), jsonbArrayLiteral(genUsedAudios), sid,
	); err != nil {
		return Shot{}, err
	}

	if err := tx.Commit(); err != nil {
		return Shot{}, err
	}
	return s.GetShot(ctx, shotID)
}

func (s *Store) MarkShotVideoVersionViewed(ctx context.Context, generationID string) error {
	gid, err := parseID(generationID)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE video_generations
		    SET viewed_flag=true, update_time=now()
		  WHERE id=$1`,
		gid,
	)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return fmt.Errorf("视频版本不存在")
	}
	return nil
}

func (s *Store) CopyShotVideoVersion(ctx context.Context, sourceShotID, generationID, targetShotID string) (Shot, error) {
	sourceSID, err := parseID(sourceShotID)
	if err != nil {
		return Shot{}, err
	}
	gid, err := parseID(generationID)
	if err != nil {
		return Shot{}, err
	}
	targetSID, err := parseID(targetShotID)
	if err != nil {
		return Shot{}, err
	}
	if sourceSID == targetSID {
		return Shot{}, fmt.Errorf("不能复制到当前分镜")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Shot{}, err
	}
	defer tx.Rollback()

	var sourceProjectID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT project_id FROM video_shots WHERE id=$1`,
		sourceSID,
	).Scan(&sourceProjectID); err != nil {
		if err == sql.ErrNoRows {
			return Shot{}, fmt.Errorf("源分镜不存在")
		}
		return Shot{}, err
	}

	var targetExists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT true FROM video_shots WHERE project_id=$1 AND id=$2`,
		sourceProjectID, targetSID,
	).Scan(&targetExists); err != nil {
		if err == sql.ErrNoRows {
			return Shot{}, fmt.Errorf("目标分镜不存在或不属于当前项目")
		}
		return Shot{}, err
	}

	var copiedID int64
	var copiedStatus, copiedPrompt, copiedError string
	var copiedUsedImages, copiedUsedVideos, copiedUsedAudios []byte
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO video_generations (
		     provider, model, prompt, image_url, task_id, seconds, aspect_ratio,
		     video_asset_id, video_url, duration, fps, width, height,
		     status, error_message, subtitle_remove, upscaled_flag, upscaled_resolution,
		     used_images, used_videos, used_audios, project_id, shot_id
		   )
		   SELECT g.provider, g.model, g.prompt, g.image_url, g.task_id, g.seconds, g.aspect_ratio,
		          g.video_asset_id, g.video_url, g.duration, g.fps, g.width, g.height,
		          g.status, g.error_message, g.subtitle_remove, g.upscaled_flag, g.upscaled_resolution,
		          CASE WHEN jsonb_array_length(g.used_images) = 0 AND s.generation_id = g.id THEN s.used_images ELSE g.used_images END,
		          CASE WHEN jsonb_array_length(g.used_videos) = 0 AND s.generation_id = g.id THEN s.used_videos ELSE g.used_videos END,
		          CASE WHEN jsonb_array_length(g.used_audios) = 0 AND s.generation_id = g.id THEN s.used_audios ELSE g.used_audios END,
		          $3, $4
		     FROM video_generations g
		     JOIN video_shots s ON s.id=$1
		    WHERE g.id=$2 AND (g.shot_id = s.id OR g.id = s.generation_id)
		   RETURNING id, status, prompt, error_message, used_images, used_videos, used_audios`,
		sourceSID, gid, sourceProjectID, targetSID,
	).Scan(&copiedID, &copiedStatus, &copiedPrompt, &copiedError, &copiedUsedImages, &copiedUsedVideos, &copiedUsedAudios); err != nil {
		if err == sql.ErrNoRows {
			return Shot{}, fmt.Errorf("视频版本不存在或不属于源分镜")
		}
		return Shot{}, err
	}

	shotStatus := shotStatusFromGenerationStatus(copiedStatus)
	if _, err := tx.ExecContext(ctx,
		`UPDATE video_shots
		    SET generation_id=$1,
		        generated_prompt=$2,
		        status=$3,
		        error_message=CASE WHEN $3='failed' THEN $4 ELSE '' END,
		        used_images=$5::jsonb,
		        used_videos=$6::jsonb,
		        used_audios=$7::jsonb,
		        update_time=now()
		  WHERE id=$8`,
		copiedID, copiedPrompt, shotStatus, copiedError,
		jsonbArrayLiteral(copiedUsedImages), jsonbArrayLiteral(copiedUsedVideos), jsonbArrayLiteral(copiedUsedAudios), targetSID,
	); err != nil {
		return Shot{}, err
	}

	if err := tx.Commit(); err != nil {
		return Shot{}, err
	}
	return s.GetShot(ctx, targetShotID)
}

func (s *Store) DeleteShotVideoVersion(ctx context.Context, shotID, generationID string) error {
	sid, err := parseID(shotID)
	if err != nil {
		return err
	}
	gid, err := parseID(generationID)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var isCurrent bool
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(s.generation_id = g.id, false) AS is_current
		   FROM video_shots s
		   JOIN video_generations g ON g.id=$2
		  WHERE s.id=$1 AND (g.shot_id = s.id OR g.id = s.generation_id)`,
		sid, gid,
	).Scan(&isCurrent)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("视频版本不存在或不属于当前分镜")
		}
		return err
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM video_generations WHERE id=$1`, gid)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return fmt.Errorf("视频版本不存在")
	}

	if isCurrent {
		var nextID int64
		var nextStatus, nextPrompt, nextError string
		var nextUsedImages, nextUsedVideos, nextUsedAudios []byte
		err := tx.QueryRowContext(ctx,
			`SELECT id, status, prompt, error_message, used_images, used_videos, used_audios
			   FROM video_generations
			  WHERE shot_id=$1
			  ORDER BY create_time DESC, id DESC
			  LIMIT 1`,
			sid,
		).Scan(&nextID, &nextStatus, &nextPrompt, &nextError, &nextUsedImages, &nextUsedVideos, &nextUsedAudios)
		switch {
		case err == sql.ErrNoRows:
			if _, err := tx.ExecContext(ctx,
				`UPDATE video_shots
				    SET generation_id=NULL, generated_prompt='', used_images='[]'::jsonb, used_videos='[]'::jsonb, used_audios='[]'::jsonb,
				        end_frame_url='', status='draft', error_message='', update_time=now()
				  WHERE id=$1`,
				sid,
			); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			shotStatus := shotStatusFromGenerationStatus(nextStatus)
			if _, err := tx.ExecContext(ctx,
				`UPDATE video_shots
				    SET generation_id=$1,
				        generated_prompt=$2,
				        status=$3,
				        error_message=CASE WHEN $3='failed' THEN $4 ELSE '' END,
				        used_images=$5::jsonb,
				        used_videos=$6::jsonb,
				        used_audios=$7::jsonb,
				        update_time=now()
				  WHERE id=$8`,
				nextID, nextPrompt, shotStatus, nextError,
				jsonbArrayLiteral(nextUsedImages), jsonbArrayLiteral(nextUsedVideos), jsonbArrayLiteral(nextUsedAudios), sid,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// PreviousShot 返回同项目中 order_num 小于当前分镜的最近一个分镜（用于首帧继承）。
func (s *Store) PreviousShot(ctx context.Context, projectID string, orderNum int) (Shot, bool, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return Shot{}, false, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+shotSelectColumns+`
		   FROM video_shots s
		   LEFT JOIN video_generations g ON s.generation_id = g.id
		  WHERE s.project_id=$1 AND s.order_num < $2
		  ORDER BY s.order_num DESC
		  LIMIT 1`, pid, orderNum,
	)
	sh, err := scanShot(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Shot{}, false, nil
		}
		return Shot{}, false, err
	}
	return sh, true, nil
}

// MarkShotGenerating 记录生成任务提交后的分镜状态。
func (s *Store) MarkShotGenerating(ctx context.Context, shotID, generationID, prompt string, images, videos, audios []string) error {
	sid, err := parseID(shotID)
	if err != nil {
		return err
	}
	gid, err := parseID(generationID)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var projectID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT project_id FROM video_shots WHERE id=$1`,
		sid,
	).Scan(&projectID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE video_generations
		    SET project_id=$1,
		        shot_id=$2,
		        used_images=$3::jsonb,
		        used_videos=$4::jsonb,
		        used_audios=$5::jsonb,
		        update_time=now()
		  WHERE id=$6`,
		projectID, sid, toJSONArray(images), toJSONArray(videos), toJSONArray(audios), gid,
	); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE video_shots
		    SET generation_id=$1, generated_prompt=$2, used_images=$3::jsonb, used_videos=$4::jsonb, used_audios=$5::jsonb,
		        status='generating', error_message='', update_time=now()
		  WHERE id=$6`,
		gid, prompt, toJSONArray(images), toJSONArray(videos), toJSONArray(audios), sid,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// MarkShotCompleted 生成完成后回填尾帧。
func (s *Store) MarkShotCompleted(ctx context.Context, shotID, endFrameURL string) error {
	sid, err := parseID(shotID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE video_shots SET status='completed', end_frame_url=$1, error_message='', update_time=now() WHERE id=$2`,
		endFrameURL, sid,
	)
	return err
}

// MarkShotFailed 生成失败时记录错误信息。
func (s *Store) MarkShotFailed(ctx context.Context, shotID, message string) error {
	sid, err := parseID(shotID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(message) == "" {
		message = "视频生成失败"
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE video_shots SET status='failed', error_message=$1, update_time=now() WHERE id=$2`,
		message, sid,
	)
	return err
}

func pagination(query url.Values) (int, int) {
	page, _ := strconv.Atoi(query.Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(query.Get("pageSize"))
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	return page, pageSize
}
