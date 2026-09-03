package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const storySkillLibraryKey = "story-skills"

var errPublishedStorySkillReadOnly = errors.New("已发布故事技能不支持修改")

var storySkillCategories = []struct{ key, name, icon, color string }{
	{"myth", "神话", "sparkles", "purple"},
	{"folk", "民间", "users", "green"},
	{"fairy_tale", "童话", "wand-sparkles", "pink"},
	{"novel", "小说", "book-open", "blue"},
	{"realistic", "现实", "file-text", "sand"},
}

type storySkillAdminItem struct {
	ID               int64      `json:"id"`
	CategoryID       int64      `json:"categoryId"`
	Category         string     `json:"category"`
	CategoryName     string     `json:"categoryName"`
	Key              string     `json:"key"`
	Name             string     `json:"name"`
	Summary          string     `json:"summary"`
	Version          string     `json:"version"`
	Status           string     `json:"status"`
	Instructions     string     `json:"instructions,omitempty"`
	HasDraft         bool       `json:"hasDraft"`
	PublishedVersion string     `json:"publishedVersion,omitempty"`
	UpdatedAt        *time.Time `json:"updatedAt"`
}

type storySkillDraftInput struct {
	Category, Key, Name, Summary, Version, Instructions string
}

func registerStorySkillAdminRoutes(mux *http.ServeMux, requirePermission func(string, http.HandlerFunc) http.HandlerFunc, s *Server) {
	mux.HandleFunc("/api/story-skills", requirePermission("App:StoryManagement:View", s.storySkillAdminRouter))
	mux.HandleFunc("/api/story-skills/upload", requirePermission("App:StoryManagement:Edit", s.storySkillAdminRouter))
	mux.HandleFunc("/api/story-skills/", func(w http.ResponseWriter, r *http.Request) {
		_, action, ok := parseStorySkillAdminPath(r.URL.Path)
		if !ok {
			httpx.Fail(w, http.StatusNotFound, "故事技能不存在")
			return
		}
		permission := ""
		switch {
		case r.Method == http.MethodGet && action == "":
			permission = "App:StoryManagement:View"
		case r.Method == http.MethodPatch && action == "":
			permission = "App:StoryManagement:Edit"
		case r.Method == http.MethodDelete && action == "":
			permission = "App:StoryManagement:Delete"
		case r.Method == http.MethodPost && action == "publish":
			permission = "App:StoryManagement:Publish"
		default:
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		requirePermission(permission, s.storySkillAdminRouter)(w, r)
	})
}

func parseStorySkillAdminPath(path string) (int64, string, bool) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/story-skills/"), "/"), "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		return 0, "", false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
		if action != "publish" {
			return 0, "", false
		}
	}
	return id, action, true
}

func (s *Server) storySkillAdminRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/story-skills")
	switch {
	case path == "" && r.Method == http.MethodGet:
		s.listStorySkills(w, r)
	case path == "/upload" && r.Method == http.MethodPost:
		s.uploadStorySkill(w, r)
	case strings.HasPrefix(path, "/"):
		id, action, ok := parseStorySkillAdminPath(r.URL.Path)
		if !ok {
			httpx.Fail(w, http.StatusBadRequest, "技能编号无效")
			return
		}
		switch {
		case r.Method == http.MethodGet && action == "":
			s.getStorySkill(w, r, id)
		case r.Method == http.MethodPatch && action == "":
			s.updateStorySkill(w, r, id)
		case r.Method == http.MethodDelete && action == "":
			s.deleteStorySkill(w, r, id)
		case r.Method == http.MethodPost && action == "publish":
			s.publishStorySkill(w, r, id)
		default:
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listStorySkills(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT skill.id,COALESCE(draft_category.id,category.id),
		       COALESCE(NULLIF(version.source_metadata->>'storyStyle',''),category.key),COALESCE(draft_category.name,category.name),
		       COALESCE(NULLIF(version.source_metadata->>'draftKey',''),skill.key),
		       COALESCE(NULLIF(version.source_metadata->>'draftName',''),skill.name),
		       COALESCE(NULLIF(version.source_metadata->>'draftSummary',''),skill.summary),
		       COALESCE(version.version,''),CASE WHEN version.status='draft' THEN 'draft' ELSE skill.status END,
		       version.status='draft',COALESCE(published.version,''),skill.update_time
		FROM app_skills skill
		JOIN app_skill_categories category ON category.id=skill.category_id
		JOIN app_skill_libraries library ON library.id=category.library_id AND library.key=$1
		LEFT JOIN app_skill_versions version ON version.id=(SELECT id FROM app_skill_versions WHERE skill_id=skill.id AND status IN ('draft','published') ORDER BY CASE WHEN status='draft' THEN 0 ELSE 1 END,id DESC LIMIT 1)
		LEFT JOIN app_skill_versions published ON published.id=skill.latest_published_version_id
		LEFT JOIN app_skill_categories draft_category ON draft_category.library_id=library.id AND draft_category.key=version.source_metadata->>'storyStyle'
		WHERE skill.status<>'archived'
		ORDER BY category.sort_order,skill.sort_order,skill.id`, storySkillLibraryKey)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "故事技能读取失败")
		return
	}
	defer rows.Close()
	items := make([]storySkillAdminItem, 0)
	for rows.Next() {
		var item storySkillAdminItem
		if err := rows.Scan(&item.ID, &item.CategoryID, &item.Category, &item.CategoryName, &item.Key, &item.Name, &item.Summary, &item.Version, &item.Status, &item.HasDraft, &item.PublishedVersion, &item.UpdatedAt); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "故事技能读取失败")
			return
		}
		items = append(items, item)
	}
	httpx.OK(w, items)
}

func (s *Server) uploadStorySkill(w http.ResponseWriter, r *http.Request) {
	input, err := parseStorySkillDraftInput(r)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.createStorySkillDraft(r.Context(), userFromRequest(r).ID, input.Category, input.Key, input.Name, input.Summary, input.Version, input.Instructions)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.OK(w, item)
}

func parseStorySkillDraftInput(r *http.Request) (storySkillDraftInput, error) {
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		return storySkillDraftInput{}, errors.New("上传内容无效")
	}
	input := storySkillDraftInput{
		Category: strings.TrimSpace(r.FormValue("category")), Key: strings.TrimSpace(r.FormValue("key")),
		Name: strings.TrimSpace(r.FormValue("name")), Summary: strings.TrimSpace(r.FormValue("summary")),
		Version: strings.TrimSpace(r.FormValue("version")), Instructions: strings.TrimSpace(r.FormValue("instructions")),
	}
	if input.Version == "" {
		input.Version = "1.0.0"
	}
	if !validStorySkillCategory(input.Category) || !validStorySkillKey(input.Key) || input.Name == "" || input.Summary == "" {
		return storySkillDraftInput{}, errors.New("类型、技能标识、名称和摘要均不能为空")
	}
	if file, header, err := r.FormFile("file"); err == nil {
		defer file.Close()
		extension := strings.ToLower(filepath.Ext(strings.TrimSpace(header.Filename)))
		if extension != ".md" && extension != ".txt" {
			return storySkillDraftInput{}, errors.New("仅支持上传 SKILL.md 或 TXT 文件")
		}
		body, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		if readErr != nil {
			return storySkillDraftInput{}, errors.New("技能文件读取失败")
		}
		if len(body) > 1<<20 {
			return storySkillDraftInput{}, errors.New("技能文件不能超过 1MB")
		}
		input.Instructions = strings.TrimSpace(string(body))
	}
	if input.Instructions == "" {
		return storySkillDraftInput{}, errors.New("请上传 SKILL.md 或填写技能规则")
	}
	return input, nil
}

func (s *Server) getStorySkill(w http.ResponseWriter, r *http.Request, skillID int64) {
	var item storySkillAdminItem
	err := s.db.QueryRowContext(r.Context(), `
		SELECT skill.id,COALESCE(draft_category.id,category.id),
		       COALESCE(NULLIF(version.source_metadata->>'storyStyle',''),category.key),COALESCE(draft_category.name,category.name),
		       COALESCE(NULLIF(version.source_metadata->>'draftKey',''),skill.key),
		       COALESCE(NULLIF(version.source_metadata->>'draftName',''),skill.name),
		       COALESCE(NULLIF(version.source_metadata->>'draftSummary',''),skill.summary),
		       version.version,CASE WHEN version.status='draft' THEN 'draft' ELSE skill.status END,
		       version.instructions,version.status='draft',COALESCE(published.version,''),skill.update_time
		FROM app_skills skill
		JOIN app_skill_categories category ON category.id=skill.category_id
		JOIN app_skill_libraries library ON library.id=category.library_id AND library.key=$1
		JOIN app_skill_versions version ON version.id=(SELECT id FROM app_skill_versions WHERE skill_id=skill.id AND status IN ('draft','published') ORDER BY CASE WHEN status='draft' THEN 0 ELSE 1 END,id DESC LIMIT 1)
		LEFT JOIN app_skill_versions published ON published.id=skill.latest_published_version_id
		LEFT JOIN app_skill_categories draft_category ON draft_category.library_id=library.id AND draft_category.key=version.source_metadata->>'storyStyle'
		WHERE skill.id=$2 AND skill.status<>'archived'`, storySkillLibraryKey, skillID).Scan(
		&item.ID, &item.CategoryID, &item.Category, &item.CategoryName, &item.Key, &item.Name, &item.Summary,
		&item.Version, &item.Status, &item.Instructions, &item.HasDraft, &item.PublishedVersion, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		httpx.Fail(w, http.StatusNotFound, "故事技能不存在")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "故事技能读取失败")
		return
	}
	httpx.OK(w, item)
}

func (s *Server) updateStorySkill(w http.ResponseWriter, r *http.Request, skillID int64) {
	input, err := parseStorySkillDraftInput(r)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.saveStorySkillDraft(r.Context(), userFromRequest(r).ID, skillID, input)
	if err == sql.ErrNoRows {
		httpx.Fail(w, http.StatusNotFound, "故事技能不存在")
		return
	}
	if errors.Is(err, errPublishedStorySkillReadOnly) {
		httpx.Fail(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "技能版本或标识已存在，请修改后重试")
		return
	}
	httpx.OK(w, item)
}

func (s *Server) deleteStorySkill(w http.ResponseWriter, r *http.Request, skillID int64) {
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "删除失败")
		return
	}
	defer tx.Rollback()
	var exists int64
	if err := tx.QueryRowContext(r.Context(), `SELECT skill.id FROM app_skills skill JOIN app_skill_categories category ON category.id=skill.category_id JOIN app_skill_libraries library ON library.id=category.library_id AND library.key=$1 WHERE skill.id=$2 AND skill.status<>'archived' FOR UPDATE`, storySkillLibraryKey, skillID).Scan(&exists); err == sql.ErrNoRows {
		httpx.Fail(w, http.StatusNotFound, "故事技能不存在")
		return
	} else if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "删除失败")
		return
	}
	queries := []string{
		`UPDATE theory_library_releases SET status='failed',update_time=now() WHERE status='ready' AND id IN (SELECT theory_release_id FROM app_skill_versions WHERE skill_id=$1 AND status='draft')`,
		`UPDATE app_skill_versions SET status='failed',update_time=now() WHERE skill_id=$1 AND status='draft'`,
		`UPDATE theory_libraries SET status='disabled',update_time=now() WHERE id IN (SELECT release.library_id FROM app_skill_versions version JOIN theory_library_releases release ON release.id=version.theory_release_id WHERE version.skill_id=$1)`,
		`UPDATE app_skills SET status='archived',update_time=now() WHERE id=$1`,
	}
	for _, query := range queries {
		if _, err := tx.ExecContext(r.Context(), query, skillID); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "删除失败")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "删除失败")
		return
	}
	httpx.OK(w, map[string]any{"deleted": true, "id": skillID})
}

func (s *Server) createStorySkillDraft(ctx context.Context, actorID int64, category, key, name, summary, version, instructions string) (storySkillAdminItem, error) {
	return s.saveStorySkillDraft(ctx, actorID, 0, storySkillDraftInput{Category: category, Key: key, Name: name, Summary: summary, Version: version, Instructions: instructions})
}

func (s *Server) saveStorySkillDraft(ctx context.Context, actorID, skillID int64, input storySkillDraftInput) (storySkillAdminItem, error) {
	if s == nil || s.db == nil {
		return storySkillAdminItem{}, errors.New("故事技能服务未配置")
	}
	hash := sha256.Sum256([]byte(input.Instructions))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storySkillAdminItem{}, err
	}
	defer tx.Rollback()
	var libraryID, categoryID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO app_skill_libraries(key,name,description,icon_key,status) VALUES($1,'故事技能库','按故事类型选择已发布的写作技能','book-open','enabled') ON CONFLICT(key) DO UPDATE SET status='enabled',update_time=now() RETURNING id`, storySkillLibraryKey).Scan(&libraryID); err != nil {
		return storySkillAdminItem{}, err
	}
	categoryName, icon, color := storySkillCategoryMeta(input.Category)
	if err := tx.QueryRowContext(ctx, `INSERT INTO app_skill_categories(library_id,key,name,icon_key,color_token,sort_order,status) VALUES($1,$2,$3,$4,$5,(SELECT count(*)*100 FROM app_skill_categories WHERE library_id=$1),'enabled') ON CONFLICT(library_id,key) DO UPDATE SET name=EXCLUDED.name,status='enabled',update_time=now() RETURNING id`, libraryID, input.Category, categoryName, icon, color).Scan(&categoryID); err != nil {
		return storySkillAdminItem{}, err
	}
	var theoryID, releaseID, cardID int64
	if skillID == 0 {
		if err := tx.QueryRowContext(ctx, `INSERT INTO app_skills(category_id,key,name,summary,description,status,sort_order) VALUES($1,$2,$3,$4,$4,'draft',(SELECT count(*) FROM app_skills WHERE category_id=$1)) RETURNING id`, categoryID, input.Key, input.Name, input.Summary).Scan(&skillID); err != nil {
			return storySkillAdminItem{}, fmt.Errorf("技能标识已存在或保存失败: %w", err)
		}
		theoryKey := "story-skill-" + input.Key
		if err := tx.QueryRowContext(ctx, `INSERT INTO theory_libraries(key,name,description,status) VALUES($1,$2,$3,'draft') RETURNING id`, theoryKey, input.Name, input.Summary).Scan(&theoryID); err != nil {
			return storySkillAdminItem{}, err
		}
	} else {
		var hasPublished, duplicateKey bool
		if err := tx.QueryRowContext(ctx, `SELECT release.library_id,skill.latest_published_version_id IS NOT NULL,EXISTS(SELECT 1 FROM app_skills other WHERE other.key=$3 AND other.id<>skill.id) FROM app_skills skill JOIN app_skill_categories category ON category.id=skill.category_id JOIN app_skill_libraries library ON library.id=category.library_id AND library.key=$1 JOIN app_skill_versions version ON version.skill_id=skill.id JOIN theory_library_releases release ON release.id=version.theory_release_id WHERE skill.id=$2 AND skill.status<>'archived' ORDER BY version.id DESC LIMIT 1 FOR UPDATE OF skill`, storySkillLibraryKey, skillID, input.Key).Scan(&theoryID, &hasPublished, &duplicateKey); err != nil {
			return storySkillAdminItem{}, err
		}
		if err := validateStorySkillEditState(hasPublished); err != nil {
			return storySkillAdminItem{}, err
		}
		if duplicateKey {
			return storySkillAdminItem{}, errors.New("技能标识已存在")
		}
		if !hasPublished {
			if _, err := tx.ExecContext(ctx, `UPDATE app_skills SET category_id=$2,key=$3,name=$4,summary=$5,description=$5,status='draft',update_time=now() WHERE id=$1`, skillID, categoryID, input.Key, input.Name, input.Summary); err != nil {
				return storySkillAdminItem{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE theory_libraries SET name=$2,description=$3,update_time=now() WHERE id=$1`, theoryID, input.Name, input.Summary); err != nil {
			return storySkillAdminItem{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='failed',update_time=now() WHERE status='ready' AND id IN (SELECT theory_release_id FROM app_skill_versions WHERE skill_id=$1 AND status='draft')`, skillID); err != nil {
			return storySkillAdminItem{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE app_skill_versions SET status='failed',update_time=now() WHERE skill_id=$1 AND status='draft'`, skillID); err != nil {
			return storySkillAdminItem{}, err
		}
	}
	var nextVersion int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(version),0)+1 FROM theory_library_releases WHERE library_id=$1`, theoryID).Scan(&nextVersion); err != nil {
		return storySkillAdminItem{}, err
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO theory_library_releases(library_id,version,status,retrieval_mode,index_version) VALUES($1,$2,'ready','lexical_only',$3) RETURNING id`, theoryID, nextVersion, hex.EncodeToString(hash[:])[:16]).Scan(&releaseID); err != nil {
		return storySkillAdminItem{}, err
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO theory_cards(library_id,canonical_key,canonical_name,card_kind,summary,definition,epistemic_status,evidence_level,clinical_safety,authority_level,status,version) VALUES($1,$2,$3,'concept',$4,$5,'author_interpretation','unknown','caution',2,'draft',$6) RETURNING id`, theoryID, input.Key, input.Name, input.Summary, input.Instructions, nextVersion).Scan(&cardID); err != nil {
		return storySkillAdminItem{}, err
	}
	var chunkID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO theory_chunks(library_id,card_id,chunk_key,chunk_kind,title,content,keywords,tags,authority_level,evidence_level,clinical_safety,token_count,content_hash,version,status) VALUES($1,$2,$3,'card',$4,$5,'[]','[]',2,'unknown','caution',$6,$7,$8,'enabled') RETURNING id`, theoryID, cardID, input.Key, input.Name, input.Instructions, len([]rune(input.Instructions)), hex.EncodeToString(hash[:]), nextVersion).Scan(&chunkID); err != nil {
		return storySkillAdminItem{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO theory_release_cards(release_id,card_id,chunk_id) VALUES($1,$2,$3)`, releaseID, cardID, chunkID); err != nil {
		return storySkillAdminItem{}, err
	}
	opening, _ := json.Marshal([]string{"请用这个故事技能帮我重写刚生成的故事。"})
	metadata, _ := json.Marshal(storySkillDraftMetadata(input, actorID))
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_skill_versions(skill_id,version,runtime_version,instructions,opening_prompts,theory_release_id,safety_profile,content_hash,min_app_version,source_metadata,status) VALUES($1,$2,1,$3,$4::jsonb,$5,'general-v1',$6,'1.0.0',$7::jsonb,'draft')`, skillID, input.Version, input.Instructions, opening, releaseID, hex.EncodeToString(hash[:]), metadata); err != nil {
		return storySkillAdminItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return storySkillAdminItem{}, err
	}
	now := time.Now()
	return storySkillAdminItem{ID: skillID, CategoryID: categoryID, Category: input.Category, CategoryName: categoryName, Key: input.Key, Name: input.Name, Summary: input.Summary, Version: input.Version, Status: "draft", Instructions: input.Instructions, HasDraft: true, UpdatedAt: &now}, nil
}

func validateStorySkillEditState(hasPublished bool) error {
	if hasPublished {
		return errPublishedStorySkillReadOnly
	}
	return nil
}

func (s *Server) publishStorySkill(w http.ResponseWriter, r *http.Request, skillID int64) {
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "发布失败")
		return
	}
	defer tx.Rollback()
	var versionID, releaseID, theoryID, categoryID int64
	var category, key, name, summary string
	if err := tx.QueryRowContext(r.Context(), `SELECT version.id,version.theory_release_id,release.library_id,COALESCE(NULLIF(version.source_metadata->>'storyStyle',''),category.key),COALESCE(NULLIF(version.source_metadata->>'draftKey',''),skill.key),COALESCE(NULLIF(version.source_metadata->>'draftName',''),skill.name),COALESCE(NULLIF(version.source_metadata->>'draftSummary',''),skill.summary) FROM app_skill_versions version JOIN app_skills skill ON skill.id=version.skill_id AND skill.status<>'archived' JOIN app_skill_categories category ON category.id=skill.category_id JOIN app_skill_libraries library ON library.id=category.library_id AND library.key=$2 JOIN theory_library_releases release ON release.id=version.theory_release_id WHERE version.skill_id=$1 AND version.status='draft' ORDER BY version.id DESC LIMIT 1`, skillID, storySkillLibraryKey).Scan(&versionID, &releaseID, &theoryID, &category, &key, &name, &summary); err != nil {
		httpx.Fail(w, http.StatusNotFound, "草稿技能不存在")
		return
	}
	if err := tx.QueryRowContext(r.Context(), `SELECT category.id FROM app_skill_categories category JOIN app_skill_libraries library ON library.id=category.library_id AND library.key=$1 WHERE category.key=$2 AND category.status='enabled'`, storySkillLibraryKey, category).Scan(&categoryID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "发布失败")
		return
	}
	actorID := userFromRequest(r).ID
	queries := []struct {
		q    string
		args []any
	}{
		{`UPDATE theory_library_releases SET status='retired',update_time=now() WHERE library_id=$1 AND status='active' AND id<>$2`, []any{theoryID, releaseID}},
		{`UPDATE app_skill_versions SET status='retired',update_time=now() WHERE skill_id=$1 AND status='published' AND id<>$2`, []any{skillID, versionID}},
		{`UPDATE theory_cards SET status='superseded',update_time=now() WHERE library_id=$1 AND status='published' AND id NOT IN (SELECT card_id FROM theory_release_cards WHERE release_id=$2)`, []any{theoryID, releaseID}},
		{`UPDATE theory_cards SET status='published',published_at=now(),reviewed_by=$2,reviewed_at=now() WHERE id IN (SELECT card_id FROM theory_release_cards WHERE release_id=$1)`, []any{releaseID, actorID}},
		{`UPDATE theory_library_releases SET status='active',activated_at=COALESCE(activated_at,now()) WHERE id=$1`, []any{releaseID}},
		{`UPDATE theory_libraries SET status='enabled',current_version=(SELECT version FROM theory_library_releases WHERE id=$2),updated_by=$1,update_time=now() WHERE id=$3`, []any{actorID, releaseID, theoryID}},
		{`UPDATE app_skill_versions SET status='published',published_at=now() WHERE id=$1`, []any{versionID}},
		{`UPDATE app_skills SET category_id=$3,key=$4,name=$5,summary=$6,description=$6,status='enabled',latest_published_version_id=$2,update_time=now() WHERE id=$1`, []any{skillID, versionID, categoryID, key, name, summary}},
	}
	for _, item := range queries {
		if _, err := tx.ExecContext(r.Context(), item.q, item.args...); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "发布失败")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "发布失败")
		return
	}
	httpx.OK(w, map[string]any{"id": skillID, "status": "published"})
}

func storySkillDraftMetadata(input storySkillDraftInput, actorID int64) map[string]any {
	return map[string]any{
		"storyStyle": input.Category, "draftKey": input.Key, "draftName": input.Name,
		"draftSummary": input.Summary, "uploadedBy": actorID, "source": "admin-upload",
	}
}

func storySkillCategoryMeta(key string) (string, string, string) {
	for _, item := range storySkillCategories {
		if item.key == key {
			return item.name, item.icon, item.color
		}
	}
	return key, "book-open", "blue"
}
func validStorySkillCategory(value string) bool {
	for _, item := range storySkillCategories {
		if item.key == value {
			return true
		}
	}
	return false
}
func validStorySkillKey(value string) bool {
	if len(value) < 2 || len(value) > 80 {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
