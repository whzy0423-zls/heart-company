// Package videoproject 视频项目工作台：项目制管理角色/场景/分镜，
// 通过结构化提示词组装与参考素材策略降低即梦视频生成的抽卡率。
package videoproject

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
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
	ComposeStatus     string `json:"composeStatus"`
	CreateTime        string `json:"createTime"`
	Description       string `json:"description"`
	FinalVideoAssetID string `json:"finalVideoAssetId"`
	FinalVideoURL     string `json:"finalVideoUrl"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	Status            string `json:"status"`
	StyleGuide        string `json:"styleGuide"`
	Theme             string `json:"theme"`
	UpdateTime        string `json:"updateTime"`
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
	ActionDescription   string   `json:"actionDescription"`
	AspectRatio         string   `json:"aspectRatio"`
	CameraMovement      string   `json:"cameraMovement"`
	CharacterIDs        []string `json:"characterIds"`
	CreateTime          string   `json:"createTime"`
	Duration            int      `json:"duration"`
	EndFrameURL         string   `json:"endFrameUrl"`
	ErrorMessage        string   `json:"errorMessage"`
	GeneratedPrompt     string   `json:"generatedPrompt"`
	GenerationID        string   `json:"generationId"`
	ID                  string   `json:"id"`
	ImageReferenceModes []string `json:"imageReferenceModes"`
	Name                string   `json:"name"`
	OrderNum            int      `json:"orderNum"`
	ProjectID           string   `json:"projectId"`
	SceneID             string   `json:"sceneId"`
	Status              string   `json:"status"`
	UpdateTime          string   `json:"updateTime"`
	UsedImages          []string `json:"usedImages"`
	UsedVideos          []string `json:"usedVideos"`
	VideoReferenceMode  string   `json:"videoReferenceMode"`
	// 联查生成记录的视频地址（前端预览）
	VideoURL string `json:"videoUrl"`
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

func fromJSONArray(raw []byte) []string {
	items := []string{}
	_ = json.Unmarshal(raw, &items)
	return items
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
		`SELECT p.id::text, p.name, p.description, p.theme, p.style_guide, p.status,
		        p.compose_status, COALESCE(p.final_video_asset_id::text,''), p.final_video_url,
		        p.create_time, p.update_time,
		        (SELECT count(*) FROM video_project_characters c WHERE c.project_id=p.id),
		        (SELECT count(*) FROM video_project_scenes sc WHERE sc.project_id=p.id),
		        (SELECT count(*) FROM video_shots sh WHERE sh.project_id=p.id),
		        (SELECT count(*) FROM video_shots sh WHERE sh.project_id=p.id AND sh.status='completed')
		   FROM video_projects p WHERE p.id=$1`, pid,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Theme, &p.StyleGuide, &p.Status,
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
		`SELECT p.id::text, p.name, p.description, p.theme, p.style_guide, p.status,
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
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Theme, &p.StyleGuide, &p.Status,
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
	ActionDescription   string   `json:"actionDescription"`
	AspectRatio         string   `json:"aspectRatio"`
	CameraMovement      string   `json:"cameraMovement"`
	CharacterIDs        []string `json:"characterIds"`
	Duration            int      `json:"duration"`
	ImageReferenceModes []string `json:"imageReferenceModes"`
	Name                string   `json:"name"`
	OrderNum            int      `json:"orderNum"`
	SceneID             string   `json:"sceneId"`
	VideoReferenceMode  string   `json:"videoReferenceMode"`
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
		`INSERT INTO video_shots (project_id, order_num, name, action_description, duration, aspect_ratio,
		                          character_ids, scene_id, image_reference_modes, video_reference_mode, camera_movement)
		 VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9::jsonb,$10,$11) RETURNING id::text`,
		pid, input.OrderNum, strings.TrimSpace(input.Name), input.ActionDescription,
		input.Duration, input.AspectRatio, toJSONArray(input.CharacterIDs), sceneID,
		toJSONArray(input.ImageReferenceModes), input.VideoReferenceMode, strings.TrimSpace(input.CameraMovement),
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
		    SET name=$1, action_description=$2, duration=$3, aspect_ratio=$4,
		        character_ids=$5::jsonb, scene_id=$6, image_reference_modes=$7::jsonb,
		        video_reference_mode=$8, camera_movement=$9,
		        order_num=CASE WHEN $10 > 0 THEN $10 ELSE order_num END,
		        update_time=now()
		  WHERE id=$11`,
		strings.TrimSpace(input.Name), input.ActionDescription, input.Duration, input.AspectRatio,
		toJSONArray(input.CharacterIDs), sceneID, toJSONArray(input.ImageReferenceModes),
		input.VideoReferenceMode, strings.TrimSpace(input.CameraMovement), input.OrderNum, shotID,
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

const shotSelectColumns = `s.id::text, s.project_id::text, s.order_num, s.name, s.action_description,
	        s.duration, s.aspect_ratio, s.character_ids, COALESCE(s.scene_id::text,''),
	        s.image_reference_modes, s.video_reference_mode, s.camera_movement,
	        COALESCE(s.generation_id::text,''), s.generated_prompt, s.used_images, s.used_videos,
	        s.end_frame_url, s.status, s.error_message, s.create_time, s.update_time,
	        COALESCE(g.video_url,'')`

func scanShot(scanner interface{ Scan(...any) error }) (Shot, error) {
	var sh Shot
	var characterIDs, imageModes, usedImages, usedVideos []byte
	var createTime, updateTime time.Time
	if err := scanner.Scan(&sh.ID, &sh.ProjectID, &sh.OrderNum, &sh.Name, &sh.ActionDescription,
		&sh.Duration, &sh.AspectRatio, &characterIDs, &sh.SceneID,
		&imageModes, &sh.VideoReferenceMode, &sh.CameraMovement,
		&sh.GenerationID, &sh.GeneratedPrompt, &usedImages, &usedVideos,
		&sh.EndFrameURL, &sh.Status, &sh.ErrorMessage, &createTime, &updateTime,
		&sh.VideoURL,
	); err != nil {
		return Shot{}, err
	}
	sh.CharacterIDs = fromJSONArray(characterIDs)
	sh.ImageReferenceModes = fromJSONArray(imageModes)
	sh.UsedImages = fromJSONArray(usedImages)
	sh.UsedVideos = fromJSONArray(usedVideos)
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
		   LEFT JOIN video_generations g ON s.generation_id = g.id
		  WHERE s.id=$1`, shotID,
	)
	sh, err := scanShot(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Shot{}, fmt.Errorf("分镜不存在")
		}
		return Shot{}, err
	}
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
		   LEFT JOIN video_generations g ON s.generation_id = g.id
		  WHERE s.project_id=$1
		  ORDER BY s.order_num ASC, s.create_time ASC`, pid,
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
		items = append(items, sh)
	}
	return items, rows.Err()
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
func (s *Store) MarkShotGenerating(ctx context.Context, shotID, generationID, prompt string, images, videos []string) error {
	sid, err := parseID(shotID)
	if err != nil {
		return err
	}
	gid, err := parseID(generationID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE video_shots
		    SET generation_id=$1, generated_prompt=$2, used_images=$3::jsonb, used_videos=$4::jsonb,
		        status='generating', error_message='', update_time=now()
		  WHERE id=$5`,
		gid, prompt, toJSONArray(images), toJSONArray(videos), sid,
	)
	return err
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
