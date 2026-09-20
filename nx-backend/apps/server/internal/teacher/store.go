package teacher

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Get(ctx context.Context, key string) (Teacher, error) {
	item, err := scanTeacher(s.db.QueryRowContext(ctx, teacherSelect+" WHERE teacher_key=$1", strings.TrimSpace(key)))
	if errors.Is(err, sql.ErrNoRows) {
		return Teacher{}, ErrNotFound
	}
	return item, err
}

func (s *Store) List(ctx context.Context, includeDisabled bool) ([]Teacher, error) {
	where := ""
	if !includeDisabled {
		where = " WHERE enabled=true"
	}
	rows, err := s.db.QueryContext(ctx, teacherSelect+where+" ORDER BY sort_order,id")
	if err != nil {
		return nil, fmt.Errorf("list teachers: %w", err)
	}
	defer rows.Close()
	items := []Teacher{}
	for rows.Next() {
		item, err := scanTeacher(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Save(ctx context.Context, item Teacher) (Teacher, error) {
	if err := item.Validate(); err != nil {
		return Teacher{}, err
	}
	if item.Expertise == nil {
		item.Expertise = []string{}
	}
	expertise, err := json.Marshal(item.Expertise)
	if err != nil {
		return Teacher{}, err
	}
	service, err := json.Marshal(item.CustomerService)
	if err != nil {
		return Teacher{}, err
	}
	offline, err := json.Marshal(item.OfflineService)
	if err != nil {
		return Teacher{}, err
	}
	return scanTeacher(s.db.QueryRowContext(ctx, `
		INSERT INTO teacher_profiles (teacher_key,name,title,avatar,cover,short_intro,detail_intro,expertise,intro_video_url,customer_service,offline_service,show_on_home,show_in_drawer,sort_order,enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10::jsonb,$11::jsonb,$12,$13,$14,$15)
		ON CONFLICT (teacher_key) DO UPDATE SET name=EXCLUDED.name,title=EXCLUDED.title,avatar=EXCLUDED.avatar,cover=EXCLUDED.cover,short_intro=EXCLUDED.short_intro,detail_intro=EXCLUDED.detail_intro,expertise=EXCLUDED.expertise,intro_video_url=EXCLUDED.intro_video_url,customer_service=EXCLUDED.customer_service,offline_service=EXCLUDED.offline_service,show_on_home=EXCLUDED.show_on_home,show_in_drawer=EXCLUDED.show_in_drawer,sort_order=EXCLUDED.sort_order,enabled=EXCLUDED.enabled,updated_at=now()
		RETURNING `+teacherColumns, item.Key, item.Name, item.Title, item.Avatar, item.Cover, item.ShortIntro, item.DetailIntro, string(expertise), item.IntroVideoURL, string(service), string(offline), item.ShowOnHome, item.ShowInDrawer, item.SortOrder, item.Enabled))
}

func (s *Store) SetEnabled(ctx context.Context, key string, enabled bool) (Teacher, error) {
	return scanTeacher(s.db.QueryRowContext(ctx, "UPDATE teacher_profiles SET enabled=$1,updated_at=now() WHERE teacher_key=$2 RETURNING "+teacherColumns, enabled, strings.TrimSpace(key)))
}

func (s *Store) SaveProfileDraft(ctx context.Context, key string, appUserID int64, item Teacher) (int64, error) {
	if appUserID <= 0 || strings.TrimSpace(key) == "" {
		return 0, errors.New("teacher and app user are required")
	}
	if err := item.Validate(); err != nil {
		return 0, err
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `INSERT INTO teacher_profile_drafts(teacher_key,app_user_id,payload,review_status) VALUES($1,$2,$3::jsonb,'pending_review') RETURNING id`, strings.TrimSpace(key), appUserID, string(payload)).Scan(&id)
	return id, err
}

func (s *Store) ReviewProfileDraft(ctx context.Context, key string, state ReviewState, reason string, actorID int64) (Teacher, error) {
	if state != ReviewPublished && state != ReviewRejected && state != ReviewOffline {
		return Teacher{}, errors.New("invalid administrator review state")
	}
	if err := ValidateReviewReason(state, reason); err != nil {
		return Teacher{}, err
	}
	var payload []byte
	var draftID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id,payload FROM teacher_profile_drafts WHERE teacher_key=$1 AND review_status='pending_review' ORDER BY id DESC LIMIT 1`, strings.TrimSpace(key)).Scan(&draftID, &payload); err != nil {
		return Teacher{}, err
	}
	if state == ReviewPublished {
		var item Teacher
		if err := json.Unmarshal(payload, &item); err != nil {
			return Teacher{}, err
		}
		item.Key = key
		if _, err := s.Save(ctx, item); err != nil {
			return Teacher{}, err
		}
	}
	_, err := s.db.ExecContext(ctx, `UPDATE teacher_profile_drafts SET review_status=$1,review_reason=$2,reviewed_by=$3,reviewed_at=now(),updated_at=now() WHERE id=$4`, state, strings.TrimSpace(reason), actorID, draftID)
	if err != nil {
		return Teacher{}, err
	}
	return s.Get(ctx, key)
}

// Roles returns independent teacher and agent roles. Agent membership is read
// from distribution_agents; it is deliberately not copied into app_user_roles.
func (s *Store) Roles(ctx context.Context, appUserID int64) (RoleSet, error) {
	if s == nil || s.db == nil || appUserID <= 0 {
		return RoleSet{}, errors.New("app user id is required")
	}
	var roles RoleSet
	var role, key string
	// Filter the role row explicitly: a user can independently hold teacher and
	// agent identities, so selecting an arbitrary enabled role would make the
	// teacher workspace depend on database row order.
	if err := s.db.QueryRowContext(ctx, `SELECT role,COALESCE(teacher_key,'') FROM app_user_roles WHERE app_user_id=$1 AND role='teacher' AND enabled=true`, appUserID).Scan(&role, &key); err == nil {
		if role == "teacher" {
			roles.Teacher = true
			roles.TeacherKey = key
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return roles, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM distribution_agents WHERE app_user_id=$1 AND status='active'`, appUserID).Scan(&roles.AgentID); err == nil {
		roles.Agent = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return roles, err
	}
	return roles, nil
}

func (s *Store) BindTeacher(ctx context.Context, appUserID int64, key string, enabled bool, actorID *int64) (RoleSet, error) {
	key = strings.TrimSpace(key)
	if appUserID <= 0 || key == "" {
		return RoleSet{}, errors.New("app user and teacher key are required")
	}
	if _, err := s.Get(ctx, key); err != nil {
		return RoleSet{}, err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_user_roles(app_user_id,role,teacher_key,enabled,updated_by) VALUES($1,'teacher',$2,$3,$4) ON CONFLICT(app_user_id,role) DO UPDATE SET teacher_key=EXCLUDED.teacher_key,enabled=EXCLUDED.enabled,updated_by=EXCLUDED.updated_by,updated_at=now()`, appUserID, key, enabled, actorID)
	if err != nil {
		return RoleSet{}, fmt.Errorf("bind teacher: %w", err)
	}
	return s.Roles(ctx, appUserID)
}

func (s *Store) UnbindTeacher(ctx context.Context, appUserID int64, actorID *int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE app_user_roles SET enabled=false,updated_by=$2,updated_at=now() WHERE app_user_id=$1 AND role='teacher'`, appUserID, actorID)
	return err
}

func (s *Store) RecordReviewEvent(ctx context.Context, contentID int64, state ReviewState, reason string, actorID *int64) error {
	if contentID <= 0 {
		return errors.New("content id is required")
	}
	if err := ValidateReviewReason(state, reason); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO teacher_review_events(content_id,review_status,reason,actor_id) VALUES($1,$2,$3,$4)`, contentID, state, strings.TrimSpace(reason), actorID)
	return err
}

func (s *Store) CreateContent(ctx context.Context, item ContentDraft, creatorID int64) (ContentDraft, error) {
	if creatorID <= 0 || strings.TrimSpace(item.TeacherKey) == "" || strings.TrimSpace(item.Title) == "" {
		return ContentDraft{}, errors.New("teacher, creator and title are required")
	}
	if item.FeedType == "" {
		item.FeedType = "course"
	}
	if item.FeedType != "course" && item.FeedType != "daily" {
		return ContentDraft{}, errors.New("invalid feed type")
	}
	if item.FeedType == "daily" {
		item.ShowAsStandalone = true
	}
	row := s.db.QueryRowContext(ctx, `INSERT INTO classroom_contents(series_id,show_as_standalone,title,description,content_type,cover_url,teacher_key,feed_type,status,review_status,created_by,updated_by)
		VALUES($1,$2,$3,$4,'video',$5,$6,$7,'draft','draft',$8,$8)
		RETURNING id,series_id,show_as_standalone,title,description,teacher_key,feed_type,review_status,review_reason,replaces_content_id,published_at,created_at,updated_at`, item.SeriesID, item.ShowAsStandalone, strings.TrimSpace(item.Title), item.Description, item.CoverURL, item.TeacherKey, item.FeedType, creatorID)
	created, err := scanContentDraft(row)
	if err != nil {
		return ContentDraft{}, err
	}
	if err := s.setEngagementCounts(ctx, created.ID, item.LikeCount, item.FavoriteCount); err != nil {
		return ContentDraft{}, err
	}
	return s.GetContent(ctx, created.ID)
}

func (s *Store) GetContent(ctx context.Context, id int64) (ContentDraft, error) {
	item, err := scanContentDraft(s.db.QueryRowContext(ctx, `SELECT id,series_id,show_as_standalone,title,description,teacher_key,feed_type,review_status,review_reason,replaces_content_id,published_at,created_at,updated_at FROM classroom_contents WHERE id=$1`, id))
	if err == nil {
		_ = s.hydrateEngagement(ctx, &item)
	}
	return item, err
}

func (s *Store) UpdateContent(ctx context.Context, item ContentDraft, key string) (ContentDraft, error) {
	if item.ID <= 0 || strings.TrimSpace(key) == "" || strings.TrimSpace(item.Title) == "" {
		return ContentDraft{}, errors.New("content, teacher and title are required")
	}
	if item.FeedType == "" {
		item.FeedType = "course"
	}
	if item.FeedType != "course" && item.FeedType != "daily" {
		return ContentDraft{}, errors.New("invalid feed type")
	}
	_, err := scanContentDraft(s.db.QueryRowContext(ctx, `UPDATE classroom_contents SET title=$1,description=$2,cover_url=$3,feed_type=$4,show_as_standalone=$5,review_status='draft',review_reason='',updated_at=now() WHERE id=$6 AND teacher_key=$7 AND review_status IN ('draft','rejected') RETURNING id,series_id,show_as_standalone,title,description,teacher_key,feed_type,review_status,review_reason,replaces_content_id,published_at,created_at,updated_at`, strings.TrimSpace(item.Title), item.Description, item.CoverURL, item.FeedType, item.ShowAsStandalone, item.ID, strings.TrimSpace(key)))
	if err != nil {
		return ContentDraft{}, err
	}
	if err := s.setEngagementCounts(ctx, item.ID, item.LikeCount, item.FavoriteCount); err != nil {
		return ContentDraft{}, err
	}
	return s.GetContent(ctx, item.ID)
}

func (s *Store) setEngagementCounts(ctx context.Context, id int64, likes, favorites int) error {
	if id <= 0 || likes < 0 || favorites < 0 {
		return errors.New("engagement counts must be non-negative")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO classroom_content_engagement(content_id,like_count,favorite_count,updated_at) VALUES($1,$2,$3,now()) ON CONFLICT(content_id) DO UPDATE SET like_count=EXCLUDED.like_count,favorite_count=EXCLUDED.favorite_count,updated_at=now()`, id, likes, favorites)
	return err
}

func (s *Store) SetEngagementCounts(ctx context.Context, id int64, likes, favorites int) (ContentDraft, error) {
	if err := s.setEngagementCounts(ctx, id, likes, favorites); err != nil {
		return ContentDraft{}, err
	}
	return s.GetContent(ctx, id)
}

func (s *Store) hydrateEngagement(ctx context.Context, item *ContentDraft) error {
	return s.db.QueryRowContext(ctx, `SELECT like_count,favorite_count FROM classroom_content_engagement WHERE content_id=$1`, item.ID).Scan(&item.LikeCount, &item.FavoriteCount)
}

func (s *Store) ToggleEngagement(ctx context.Context, contentID, userID int64, kind string) (ContentDraft, bool, error) {
	if contentID <= 0 || userID <= 0 || (kind != "like" && kind != "favorite") {
		return ContentDraft{}, false, errors.New("invalid engagement")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ContentDraft{}, false, err
	}
	defer tx.Rollback()
	var active bool
	err = tx.QueryRowContext(ctx, `DELETE FROM classroom_content_engagement_actions WHERE content_id=$1 AND app_user_id=$2 AND kind=$3 RETURNING true`, contentID, userID, kind).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO classroom_content_engagement_actions(content_id,app_user_id,kind) VALUES($1,$2,$3)`, contentID, userID, kind)
		active = true
	}
	if err != nil {
		return ContentDraft{}, false, err
	}
	column := "like_count"
	if kind == "favorite" {
		column = "favorite_count"
	}
	delta := 1
	if !active {
		delta = -1
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO classroom_content_engagement(content_id,`+column+`) VALUES($1,$2) ON CONFLICT(content_id) DO UPDATE SET `+column+`=GREATEST(0,classroom_content_engagement.`+column+`+$2),updated_at=now()`, contentID, delta); err != nil {
		return ContentDraft{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return ContentDraft{}, false, err
	}
	item, err := s.GetContent(ctx, contentID)
	return item, active, err
}

func (s *Store) ListContent(ctx context.Context, key string, state ReviewState, feedType string, publishedOnly bool) ([]ContentDraft, error) {
	clauses, args := []string{"teacher_key=$1"}, []any{strings.TrimSpace(key)}
	if publishedOnly {
		clauses = append(clauses, "review_status='published'", "status='published'")
	}
	if state != "" {
		args = append(args, state)
		clauses = append(clauses, fmt.Sprintf("review_status=$%d", len(args)))
	}
	if feedType != "" {
		args = append(args, feedType)
		clauses = append(clauses, fmt.Sprintf("feed_type=$%d", len(args)))
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,series_id,show_as_standalone,title,description,teacher_key,feed_type,review_status,review_reason,replaces_content_id,published_at,created_at,updated_at FROM classroom_contents WHERE `+strings.Join(clauses, " AND ")+" ORDER BY created_at DESC,id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ContentDraft{}
	for rows.Next() {
		item, err := scanContentDraft(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		_ = s.hydrateEngagement(ctx, &items[len(items)-1])
	}
	return items, rows.Err()
}

// ListPublishedVideos returns the two teacher feed types as one app-facing
// stream. Visibility is intentionally stricter than the teacher workspace:
// both the classroom publication state and the teacher review state must be
// published.
func (s *Store) ListPublishedVideos(ctx context.Context, key string) ([]ContentDraft, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,series_id,show_as_standalone,title,description,teacher_key,feed_type,review_status,review_reason,replaces_content_id,published_at,created_at,updated_at FROM classroom_contents WHERE teacher_key=$1 AND feed_type IN ('course','daily') AND review_status='published' AND status='published' ORDER BY COALESCE(published_at,created_at) DESC,id DESC`, strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ContentDraft{}
	for rows.Next() {
		item, err := scanContentDraft(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		_ = s.hydrateEngagement(ctx, &items[len(items)-1])
	}
	return items, rows.Err()
}

func (s *Store) SubmitContent(ctx context.Context, id int64, key string) (ContentDraft, error) {
	row := s.db.QueryRowContext(ctx, `UPDATE classroom_contents SET review_status='pending_review',review_reason='',updated_at=now() WHERE id=$1 AND teacher_key=$2 AND review_status IN ('draft','rejected') RETURNING id,series_id,show_as_standalone,title,description,teacher_key,feed_type,review_status,review_reason,replaces_content_id,published_at,created_at,updated_at`, id, strings.TrimSpace(key))
	item, err := scanContentDraft(row)
	if err != nil {
		return ContentDraft{}, err
	}
	if err := s.RecordReviewEvent(ctx, id, ReviewPending, "", nil); err != nil {
		return ContentDraft{}, err
	}
	return item, nil
}

func (s *Store) ReviewContent(ctx context.Context, id int64, state ReviewState, reason string, actorID int64) (ContentDraft, error) {
	if state != ReviewPublished && state != ReviewRejected && state != ReviewOffline {
		return ContentDraft{}, errors.New("invalid administrator review state")
	}
	if err := ValidateReviewReason(state, reason); err != nil {
		return ContentDraft{}, err
	}
	status := "draft"
	if state == ReviewPublished {
		status = "published"
	}
	if state == ReviewOffline {
		status = "offline"
	}
	row := s.db.QueryRowContext(ctx, `UPDATE classroom_contents SET review_status=$1,review_reason=$2,status=$3,reviewed_by=$4,reviewed_at=now(),published_at=CASE WHEN $1='published' THEN COALESCE(published_at,now()) ELSE published_at END,updated_at=now() WHERE id=$5 RETURNING id,series_id,show_as_standalone,title,description,teacher_key,feed_type,review_status,review_reason,replaces_content_id,published_at,created_at,updated_at`, state, strings.TrimSpace(reason), status, actorID, id)
	item, err := scanContentDraft(row)
	if err != nil {
		return ContentDraft{}, err
	}
	if err = s.RecordReviewEvent(ctx, id, state, reason, &actorID); err != nil {
		return ContentDraft{}, err
	}
	_ = s.hydrateEngagement(ctx, &item)
	return item, nil
}

type scanner interface{ Scan(...any) error }

const teacherColumns = `id,teacher_key,name,title,avatar,cover,short_intro,detail_intro,expertise,intro_video_url,customer_service,offline_service,show_on_home,show_in_drawer,sort_order,enabled,created_at,updated_at`
const teacherSelect = `SELECT ` + teacherColumns + ` FROM teacher_profiles`

func scanTeacher(row scanner) (Teacher, error) {
	var t Teacher
	var expertise, service, offline []byte
	err := row.Scan(&t.ID, &t.Key, &t.Name, &t.Title, &t.Avatar, &t.Cover, &t.ShortIntro, &t.DetailIntro, &expertise, &t.IntroVideoURL, &service, &offline, &t.ShowOnHome, &t.ShowInDrawer, &t.SortOrder, &t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Teacher{}, sql.ErrNoRows
	}
	if err != nil {
		return Teacher{}, err
	}
	if len(expertise) > 0 {
		_ = json.Unmarshal(expertise, &t.Expertise)
	}
	if len(service) > 0 && string(service) != "null" {
		_ = json.Unmarshal(service, &t.CustomerService)
	}
	if len(offline) > 0 && string(offline) != "null" {
		_ = json.Unmarshal(offline, &t.OfflineService)
	}
	return t, nil
}

func scanContentDraft(row scanner) (ContentDraft, error) {
	var item ContentDraft
	err := row.Scan(&item.ID, &item.SeriesID, &item.ShowAsStandalone, &item.Title, &item.Description, &item.TeacherKey, &item.FeedType, &item.ReviewStatus, &item.ReviewReason, &item.ReplacesContentID, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ContentDraft{}, sql.ErrNoRows
	}
	return item, err
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
