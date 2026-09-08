package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const managedSkillLibraryKey = "learning-growth-books"

type skillLibraryAdminCatalog struct {
	Libraries  []skillLibraryAdminLibrary  `json:"libraries"`
	Categories []skillLibraryAdminCategory `json:"categories"`
	Skills     []skillLibraryAdminSkill    `json:"skills"`
}

type skillLibraryAdminLibrary struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconKey     string `json:"iconKey"`
	Status      string `json:"status"`
	SortOrder   int    `json:"sortOrder"`
}

type skillLibraryAdminCategory struct {
	ID         int64  `json:"id"`
	LibraryID  int64  `json:"libraryId"`
	Key        string `json:"key"`
	Name       string `json:"name"`
	IconKey    string `json:"iconKey"`
	ColorToken string `json:"colorToken"`
	Status     string `json:"status"`
	SortOrder  int    `json:"sortOrder"`
	SkillCount int    `json:"skillCount"`
}

type skillLibraryAdminSkill struct {
	ID                  int64      `json:"id"`
	CategoryID          int64      `json:"categoryId"`
	CategoryKey         string     `json:"categoryKey"`
	CategoryName        string     `json:"categoryName"`
	Key                 string     `json:"key"`
	Name                string     `json:"name"`
	Summary             string     `json:"summary"`
	Description         string     `json:"description"`
	IconKey             string     `json:"iconKey"`
	ColorToken          string     `json:"colorToken"`
	Status              string     `json:"status"`
	SortOrder           int        `json:"sortOrder"`
	HasPublishedVersion bool       `json:"hasPublishedVersion"`
	PublishedVersion    string     `json:"publishedVersion,omitempty"`
	UpdatedAt           *time.Time `json:"updatedAt,omitempty"`
}

type skillLibraryAdminUpdate struct {
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	CategoryID  int64  `json:"categoryId"`
	IconKey     string `json:"iconKey"`
	ColorToken  string `json:"colorToken"`
	SortOrder   int    `json:"sortOrder"`
	Status      string `json:"status"`
}

type skillLibraryMetadataUpdate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IconKey     string `json:"iconKey"`
	ColorToken  string `json:"colorToken"`
	SortOrder   int    `json:"sortOrder"`
	Status      string `json:"status"`
}

func registerSkillLibraryAdminRoutes(mux *http.ServeMux, requirePermission func(string, http.HandlerFunc) http.HandlerFunc, s *Server) {
	mux.HandleFunc("/api/skill-library-management", requirePermission("App:SkillLibrary:View", s.skillLibraryAdminRouter))
	mux.HandleFunc("/api/skill-library-management/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if _, _, ok := parseSkillLibraryAdminPath(r.URL.Path); !ok {
			httpx.Fail(w, http.StatusNotFound, "管理对象不存在")
			return
		}
		requirePermission("App:SkillLibrary:Edit", s.skillLibraryAdminRouter)(w, r)
	})
}

func parseSkillLibraryAdminPath(path string) (string, int64, bool) {
	rest := strings.Trim(strings.TrimPrefix(path, "/api/skill-library-management/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || (parts[0] != "library" && parts[0] != "categories" && parts[0] != "skills") {
		return "", 0, false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return "", 0, false
	}
	return parts[0], id, true
}

func (s *Server) skillLibraryAdminRouter(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/skill-library-management" {
		if r.Method != http.MethodGet {
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.listSkillLibraryAdminCatalog(w, r)
		return
	}
	resource, id, ok := parseSkillLibraryAdminPath(r.URL.Path)
	if !ok || r.Method != http.MethodPatch {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	switch resource {
	case "library":
		s.updateSkillLibraryMetadata(w, r, id)
	case "categories":
		s.updateSkillCategory(w, r, id)
	case "skills":
		s.updateManagedSkill(w, r, id)
	}
}

func (s *Server) listSkillLibraryAdminCatalog(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "成长技能库服务未配置")
		return
	}
	catalog := skillLibraryAdminCatalog{
		Libraries:  make([]skillLibraryAdminLibrary, 0, 1),
		Categories: make([]skillLibraryAdminCategory, 0),
		Skills:     make([]skillLibraryAdminSkill, 0),
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id,key,name,description,icon_key,status,sort_order
		FROM app_skill_libraries WHERE key=$1 ORDER BY sort_order,id`, managedSkillLibraryKey)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "技能库读取失败")
		return
	}
	for rows.Next() {
		var item skillLibraryAdminLibrary
		if err := rows.Scan(&item.ID, &item.Key, &item.Name, &item.Description, &item.IconKey, &item.Status, &item.SortOrder); err != nil {
			rows.Close()
			httpx.Fail(w, http.StatusInternalServerError, "技能库读取失败")
			return
		}
		catalog.Libraries = append(catalog.Libraries, item)
	}
	if err := rows.Close(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "技能库读取失败")
		return
	}

	rows, err = s.db.QueryContext(r.Context(), `
		SELECT category.id,category.library_id,category.key,category.name,category.icon_key,
		       category.color_token,category.status,category.sort_order,count(skill.id)
		FROM app_skill_categories category
		JOIN app_skill_libraries library ON library.id=category.library_id AND library.key=$1
		LEFT JOIN app_skills skill ON skill.category_id=category.id AND skill.status<>'archived'
		GROUP BY category.id ORDER BY category.sort_order,category.id`, managedSkillLibraryKey)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "技能分类读取失败")
		return
	}
	for rows.Next() {
		var item skillLibraryAdminCategory
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.Key, &item.Name, &item.IconKey, &item.ColorToken, &item.Status, &item.SortOrder, &item.SkillCount); err != nil {
			rows.Close()
			httpx.Fail(w, http.StatusInternalServerError, "技能分类读取失败")
			return
		}
		catalog.Categories = append(catalog.Categories, item)
	}
	if err := rows.Close(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "技能分类读取失败")
		return
	}

	rows, err = s.db.QueryContext(r.Context(), `
		SELECT skill.id,category.id,category.key,category.name,skill.key,skill.name,skill.summary,skill.description,
		       skill.icon_key,skill.color_token,skill.status,skill.sort_order,
		       skill.latest_published_version_id IS NOT NULL,COALESCE(version.version,''),skill.update_time
		FROM app_skills skill
		JOIN app_skill_categories category ON category.id=skill.category_id
		JOIN app_skill_libraries library ON library.id=category.library_id AND library.key=$1
		LEFT JOIN app_skill_versions version ON version.id=skill.latest_published_version_id
		WHERE skill.status<>'archived'
		ORDER BY category.sort_order,skill.sort_order,skill.id`, managedSkillLibraryKey)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "技能数据读取失败")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item skillLibraryAdminSkill
		if err := rows.Scan(
			&item.ID, &item.CategoryID, &item.CategoryKey, &item.CategoryName,
			&item.Key, &item.Name, &item.Summary, &item.Description,
			&item.IconKey, &item.ColorToken, &item.Status, &item.SortOrder,
			&item.HasPublishedVersion, &item.PublishedVersion, &item.UpdatedAt,
		); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "技能数据读取失败")
			return
		}
		catalog.Skills = append(catalog.Skills, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "技能数据读取失败")
		return
	}
	httpx.OK(w, catalog)
}

func decodeSkillAdminUpdate(r *http.Request) (skillLibraryAdminUpdate, error) {
	var input skillLibraryAdminUpdate
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&input); err != nil {
		return input, errors.New("技能信息格式无效")
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Summary = strings.TrimSpace(input.Summary)
	input.Description = strings.TrimSpace(input.Description)
	input.IconKey = strings.TrimSpace(input.IconKey)
	input.ColorToken = strings.TrimSpace(input.ColorToken)
	input.Status = strings.TrimSpace(input.Status)
	if input.Name == "" || input.Summary == "" || input.CategoryID <= 0 {
		return input, errors.New("技能名称、简介和所属分类不能为空")
	}
	if input.Description == "" {
		input.Description = input.Summary
	}
	if input.Status != "enabled" && input.Status != "disabled" {
		return input, errors.New("启用状态无效")
	}
	return input, nil
}

func decodeSkillLibraryMetadataUpdate(r *http.Request) (skillLibraryMetadataUpdate, error) {
	var input skillLibraryMetadataUpdate
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&input); err != nil {
		return input, errors.New("管理信息格式无效")
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.IconKey = strings.TrimSpace(input.IconKey)
	input.ColorToken = strings.TrimSpace(input.ColorToken)
	input.Status = strings.TrimSpace(input.Status)
	if input.Name == "" {
		return input, errors.New("名称不能为空")
	}
	if input.Status != "enabled" && input.Status != "disabled" {
		return input, errors.New("启用状态无效")
	}
	return input, nil
}

func (s *Server) updateSkillLibraryMetadata(w http.ResponseWriter, r *http.Request, id int64) {
	input, err := decodeSkillLibraryMetadataUpdate(r)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	query := "UPDATE app_skill_libraries SET name=$2,description=$3,icon_key=$4,sort_order=$5,status=$6,update_time=now() WHERE id=$1 AND key=$7"
	result, err := s.db.ExecContext(r.Context(), query, id, input.Name, input.Description, input.IconKey, input.SortOrder, input.Status, managedSkillLibraryKey)
	respondSkillLibraryAdminUpdate(w, result, err, id, "技能库")
}

func (s *Server) updateSkillCategory(w http.ResponseWriter, r *http.Request, id int64) {
	input, err := decodeSkillLibraryMetadataUpdate(r)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	query := "UPDATE app_skill_categories SET name=$2,icon_key=$3,color_token=$4,sort_order=$5,status=$6,update_time=now() WHERE id=$1 AND library_id IN (SELECT id FROM app_skill_libraries WHERE key=$7)"
	result, err := s.db.ExecContext(r.Context(), query, id, input.Name, input.IconKey, input.ColorToken, input.SortOrder, input.Status, managedSkillLibraryKey)
	respondSkillLibraryAdminUpdate(w, result, err, id, "技能分类")
}

func (s *Server) updateManagedSkill(w http.ResponseWriter, r *http.Request, id int64) {
	input, err := decodeSkillAdminUpdate(r)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	query := "UPDATE app_skills SET category_id=$2,name=$3,summary=$4,description=$5,icon_key=$6,color_token=$7,sort_order=$8,status=$9,update_time=now() " +
		"WHERE id=$1 AND category_id IN (SELECT category.id FROM app_skill_categories category JOIN app_skill_libraries library ON library.id=category.library_id WHERE library.key=$10) " +
		"AND $2 IN (SELECT category.id FROM app_skill_categories category JOIN app_skill_libraries library ON library.id=category.library_id WHERE library.key=$10) " +
		"AND ($9='disabled' OR latest_published_version_id IS NOT NULL)"
	result, err := s.db.ExecContext(r.Context(), query, id, input.CategoryID, input.Name, input.Summary, input.Description, input.IconKey, input.ColorToken, input.SortOrder, input.Status, managedSkillLibraryKey)
	respondSkillLibraryAdminUpdate(w, result, err, id, "技能")
}

func respondSkillLibraryAdminUpdate(w http.ResponseWriter, result sql.Result, err error, id int64, label string) {
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, label+"保存失败")
		return
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil || rows == 0 {
		httpx.Fail(w, http.StatusNotFound, label+"不存在，或尚无可启用的发布版本")
		return
	}
	httpx.OK(w, map[string]any{"id": id, "updated": true})
}
