package skillcatalog

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotFound         = errors.New("skill catalog: not found")
	ErrStoreUnavailable = errors.New("skill catalog: store unavailable")
	ErrInvalidCursor    = errors.New("skill catalog: invalid cursor")
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) available() error {
	if s == nil || s.db == nil {
		return ErrStoreUnavailable
	}
	return nil
}

func (s *Store) ListLibraries(ctx context.Context) ([]Library, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT library.id, library.key, library.name, library.description, library.icon_key, count(skill.id)
		FROM app_skill_libraries library
		JOIN app_skill_categories category ON category.library_id = library.id AND category.status = 'enabled'
		JOIN app_skills skill ON skill.category_id = category.id AND skill.status = 'enabled'
		JOIN app_skill_versions version ON skill.latest_published_version_id = version.id AND version.status = 'published'
		WHERE library.status = 'enabled'
		GROUP BY library.id
		ORDER BY library.sort_order, library.id`)
	if err != nil {
		return nil, fmt.Errorf("list skill libraries: %w", err)
	}
	defer rows.Close()
	out := make([]Library, 0)
	for rows.Next() {
		var item Library
		if err := rows.Scan(&item.ID, &item.Key, &item.Name, &item.Description, &item.IconKey, &item.SkillCount); err != nil {
			return nil, fmt.Errorf("list skill libraries: scan: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListCategories(ctx context.Context, libraryID int64) ([]Category, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if libraryID <= 0 {
		return nil, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT category.id, category.library_id, category.key, category.name,
		       category.icon_key, category.color_token, count(skill.id)
		FROM app_skill_libraries library
		JOIN app_skill_categories category ON category.library_id = library.id AND category.status = 'enabled'
		JOIN app_skills skill ON skill.category_id = category.id AND skill.status = 'enabled'
		JOIN app_skill_versions version ON skill.latest_published_version_id = version.id AND version.status = 'published'
		WHERE library.id = $1 AND library.status = 'enabled'
		GROUP BY category.id
		ORDER BY category.sort_order, category.id`, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list skill categories: %w", err)
	}
	defer rows.Close()
	out := make([]Category, 0)
	for rows.Next() {
		var item Category
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.Key, &item.Name, &item.IconKey, &item.ColorToken, &item.SkillCount); err != nil {
			return nil, fmt.Errorf("list skill categories: scan: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListSkills(ctx context.Context, filter SkillFilter) (SkillPage, error) {
	if err := s.available(); err != nil {
		return SkillPage{}, err
	}
	if filter.LibraryID <= 0 {
		return SkillPage{}, ErrNotFound
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	query := strings.TrimSpace(filter.Query)
	rows, err := s.db.QueryContext(ctx, `
		SELECT skill.id, category.id, category.key, category.name, skill.key, skill.name,
		       skill.summary, skill.icon_key, skill.color_token, version.id, version.version, skill.sort_order
		FROM app_skill_libraries library
		JOIN app_skill_categories category ON category.library_id = library.id AND category.status = 'enabled'
		JOIN app_skills skill ON skill.category_id = category.id AND skill.status = 'enabled'
		JOIN app_skill_versions version ON skill.latest_published_version_id = version.id AND version.status = 'published'
		WHERE library.id = $1 AND library.status = 'enabled'
		  AND ($2 = 0 OR category.id = $2)
		  AND ($3 = '' OR skill.name ILIKE '%' || $3 || '%' OR skill.summary ILIKE '%' || $3 || '%' OR skill.key ILIKE '%' || $3 || '%')
		  AND ($5 = 0 OR (skill.sort_order, skill.id) > ($4, $5))
		ORDER BY skill.sort_order, skill.id
		LIMIT $6`, filter.LibraryID, filter.CategoryID, query, filter.CursorSort, filter.CursorID, limit+1)
	if err != nil {
		return SkillPage{}, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	items := make([]SkillSummary, 0, limit+1)
	for rows.Next() {
		var item SkillSummary
		if err := rows.Scan(&item.ID, &item.CategoryID, &item.CategoryKey, &item.CategoryName,
			&item.Key, &item.Name, &item.Summary, &item.IconKey, &item.ColorToken,
			&item.VersionID, &item.Version, &item.SortOrder); err != nil {
			return SkillPage{}, fmt.Errorf("list skills: scan: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return SkillPage{}, fmt.Errorf("list skills: rows: %w", err)
	}
	page := SkillPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextSort, page.NextID = last.SortOrder, last.ID
		page.NextCursor = EncodeCursor(last.SortOrder, last.ID)
	}
	return page, nil
}

func (s *Store) GetSkill(ctx context.Context, skillID int64) (SkillDetail, error) {
	if err := s.available(); err != nil {
		return SkillDetail{}, err
	}
	var item SkillDetail
	var opening, source []byte
	var publishedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT skill.id, category.id, library.id, skill.key, skill.name, skill.summary,
		       skill.description, skill.icon_key, skill.color_token, category.name,
		       version.id, version.version, version.runtime_version, version.instructions,
		       version.opening_prompts, version.theory_release_id, version.safety_profile,
		       version.content_hash, version.min_app_version, version.source_metadata, version.published_at
		FROM app_skill_libraries library
		JOIN app_skill_categories category ON category.library_id = library.id AND category.status = 'enabled'
		JOIN app_skills skill ON skill.category_id = category.id AND skill.status = 'enabled'
		JOIN app_skill_versions version ON skill.latest_published_version_id = version.id AND version.status = 'published'
		WHERE skill.id = $1 AND library.status = 'enabled'`, skillID).Scan(
		&item.ID, &item.CategoryID, &item.LibraryID, &item.Key, &item.Name, &item.Summary,
		&item.Description, &item.IconKey, &item.ColorToken, &item.CategoryName,
		&item.Version.ID, &item.Version.Version, &item.Version.RuntimeVersion, &item.Version.Instructions,
		&opening, &item.Version.TheoryReleaseID, &item.Version.SafetyProfile, &item.Version.ContentHash,
		&item.Version.MinAppVersion, &source, &publishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SkillDetail{}, ErrNotFound
	}
	if err != nil {
		return SkillDetail{}, fmt.Errorf("get skill: %w", err)
	}
	if err := json.Unmarshal(opening, &item.Version.OpeningPrompts); err != nil {
		return SkillDetail{}, fmt.Errorf("get skill: opening prompts: %w", err)
	}
	item.Version.SourceMetadata = publicSourceMetadata(source)
	if publishedAt.Valid {
		item.Version.PublishedAt = publishedAt.Time.UTC().Format(time.RFC3339)
	}
	return item, nil
}

func publicSourceMetadata(raw []byte) json.RawMessage {
	var value struct {
		ReviewPolicy      string   `json:"reviewPolicy,omitempty"`
		ReviewDecisionRef string   `json:"reviewDecisionRef,omitempty"`
		ReviewDecision    string   `json:"reviewDecision,omitempty"`
		RiskNotices       []string `json:"riskNotices"`
		SourceNeeded      bool     `json:"sourceNeeded"`
		CompilerPolicy    string   `json:"compilerPolicy,omitempty"`
	}
	_ = json.Unmarshal(raw, &value)
	if value.RiskNotices == nil {
		value.RiskNotices = []string{}
	}
	out, _ := json.Marshal(value)
	return out
}

func EncodeCursor(sortOrder int, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(sortOrder) + ":" + strconv.FormatInt(id, 10)))
}

func DecodeCursor(value string) (int, int64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, 0, ErrInvalidCursor
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 {
		return 0, 0, ErrInvalidCursor
	}
	sortOrder, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, ErrInvalidCursor
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, 0, ErrInvalidCursor
	}
	return sortOrder, id, nil
}
