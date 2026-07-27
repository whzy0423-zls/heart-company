package enterprisepromotion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound        = errors.New("enterprise promotion record not found")
	ErrVersionConflict = errors.New("enterprise promotion version conflict")
	ErrConflict        = errors.New("enterprise promotion record conflicts with existing data")
	ErrRestricted      = errors.New("enterprise promotion record is still referenced")
)

type CaseAggregate struct {
	Case        TrainingCase
	Media       []CaseMedia
	TopicIDs    []int64
	SolutionIDs []int64
}

type SolutionAggregate struct {
	Solution EnterpriseSolution
	CaseIDs  []int64
}

// PublicCase is deliberately separate from persistence records. It contains
// only fields reviewed for anonymous miniapp delivery.
type PublicCase struct {
	ID                 int64             `json:"id"`
	Slug               string            `json:"slug"`
	Title              string            `json:"title"`
	Summary            string            `json:"summary"`
	CoverAssetID       int64             `json:"coverAssetId,omitempty"`
	CompanyDisplayName string            `json:"companyDisplayName"`
	Industry           string            `json:"industry"`
	City               string            `json:"city"`
	ParticipantRange   string            `json:"participantRange"`
	DurationLabel      string            `json:"durationLabel"`
	BusinessChallenges []string          `json:"businessChallenges"`
	TrainingGoals      []string          `json:"trainingGoals"`
	TrainingModules    []string          `json:"trainingModules"`
	TrainingMethods    []string          `json:"trainingMethods"`
	TrainerID          int64             `json:"trainerId"`
	TrainerName        string            `json:"trainerName"`
	Featured           bool              `json:"featured"`
	Media              []PublicCaseMedia `json:"media"`
	Topics             []TrainingTopic   `json:"topics"`
}

type PublicCaseMedia struct {
	MediaAssetID int64     `json:"mediaAssetId"`
	Role         MediaRole `json:"role"`
	Position     int       `json:"position"`
	Caption      string    `json:"caption"`
}

type PublicCaseQuery struct {
	Topic  TopicKey
	Limit  int
	Offset int
}

// Store is the narrow contract needed by public content and aggregate editors.
type Store interface {
	ListPublicCases(context.Context, PublicCaseQuery) ([]PublicCase, error)
	GetPublicCaseBySlug(context.Context, string) (PublicCase, error)
	UpdateCase(context.Context, CaseAggregate, int64) (CaseAggregate, error)
	ReplaceCaseMedia(context.Context, int64, int64, []CaseMedia) (CaseAggregate, error)
}

type SQLStore struct{ db *sql.DB }

func NewStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

func FixedTopics() []TrainingTopic {
	keys := []TopicKey{TopicTeamCommunication, TopicLeadership, TopicCohesion, TopicCulture, TopicEmployeeGrowth}
	titles := []string{"团队沟通", "领导力", "团队凝聚", "企业文化", "员工成长"}
	out := make([]TrainingTopic, len(keys))
	for i := range keys {
		out[i] = TrainingTopic{Key: keys[i], Title: titles[i], SortOrder: i, Enabled: true}
	}
	return out
}

func jsonValue(v []string) ([]byte, error) { return json.Marshal(v) }
func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23503":
			return fmt.Errorf("%w: %s", ErrRestricted, pg.ConstraintName)
		case "23505":
			return fmt.Errorf("%w: %s", ErrConflict, pg.ConstraintName)
		}
	}
	return err
}

func (s *SQLStore) CreateTrainer(ctx context.Context, in EnterpriseTrainer) (EnterpriseTrainer, error) {
	sp, _ := jsonValue(in.Specialties)
	cr, _ := jsonValue(in.Credentials)
	si, _ := jsonValue(in.ServiceIndustries)
	err := s.db.QueryRowContext(ctx, `INSERT INTO enterprise_trainers
		(key,name,title,avatar_asset_id,short_bio,full_bio,specialties,credentials,service_industries,experience_summary,status,sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id,version,created_at,updated_at`, in.Key, in.Name, in.Title, nullableID(in.AvatarAssetID), in.ShortBio, in.FullBio, sp, cr, si, in.ExperienceSummary, in.Status, in.SortOrder).
		Scan(&in.ID, &in.Version, &in.CreatedAt, &in.UpdatedAt)
	return in, mapDBError(err)
}

const trainerColumns = `id,key,name,title,COALESCE(avatar_asset_id,0),short_bio,full_bio,specialties,credentials,service_industries,experience_summary,status,sort_order,version,created_at,updated_at`

func scanTrainer(row interface{ Scan(...any) error }) (EnterpriseTrainer, error) {
	var v EnterpriseTrainer
	var sp, cr, si []byte
	err := row.Scan(&v.ID, &v.Key, &v.Name, &v.Title, &v.AvatarAssetID, &v.ShortBio, &v.FullBio, &sp, &cr, &si, &v.ExperienceSummary, &v.Status, &v.SortOrder, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(sp, &v.Specialties)
		if err == nil {
			err = json.Unmarshal(cr, &v.Credentials)
		}
		if err == nil {
			err = json.Unmarshal(si, &v.ServiceIndustries)
		}
	}
	return v, mapDBError(err)
}
func (s *SQLStore) GetTrainer(ctx context.Context, id int64) (EnterpriseTrainer, error) {
	return scanTrainer(s.db.QueryRowContext(ctx, `SELECT `+trainerColumns+` FROM enterprise_trainers WHERE id=$1`, id))
}
func (s *SQLStore) ListTrainers(ctx context.Context) ([]EnterpriseTrainer, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+trainerColumns+` FROM enterprise_trainers ORDER BY sort_order,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EnterpriseTrainer
	for rows.Next() {
		v, e := scanTrainer(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *SQLStore) UpdateTrainer(ctx context.Context, in EnterpriseTrainer, expected int64) (EnterpriseTrainer, error) {
	sp, _ := jsonValue(in.Specialties)
	cr, _ := jsonValue(in.Credentials)
	si, _ := jsonValue(in.ServiceIndustries)
	err := s.db.QueryRowContext(ctx, `UPDATE enterprise_trainers SET key=$1,name=$2,title=$3,avatar_asset_id=$4,short_bio=$5,full_bio=$6,specialties=$7,credentials=$8,service_industries=$9,experience_summary=$10,status=$11,sort_order=$12,version=version+1,updated_at=now() WHERE id=$13 AND version=$14 RETURNING version,created_at,updated_at`, in.Key, in.Name, in.Title, nullableID(in.AvatarAssetID), in.ShortBio, in.FullBio, sp, cr, si, in.ExperienceSummary, in.Status, in.SortOrder, in.ID, expected).Scan(&in.Version, &in.CreatedAt, &in.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EnterpriseTrainer{}, ErrVersionConflict
	}
	return in, mapDBError(err)
}
func (s *SQLStore) DeleteTrainer(ctx context.Context, id int64) error {
	r, e := s.db.ExecContext(ctx, `DELETE FROM enterprise_trainers WHERE id=$1`, id)
	if e != nil {
		return mapDBError(e)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) UpsertFixedTopics(ctx context.Context) error {
	for _, v := range FixedTopics() {
		_, e := s.db.ExecContext(ctx, `INSERT INTO training_topics(key,title,sort_order,enabled) VALUES($1,$2,$3,$4) ON CONFLICT(key) DO UPDATE SET title=EXCLUDED.title,sort_order=EXCLUDED.sort_order,enabled=EXCLUDED.enabled,updated_at=now()`, v.Key, v.Title, v.SortOrder, v.Enabled)
		if e != nil {
			return mapDBError(e)
		}
	}
	return nil
}
func (s *SQLStore) ListTopics(ctx context.Context, enabledOnly bool) ([]TrainingTopic, error) {
	q := `SELECT id,key,title,sort_order,enabled FROM training_topics`
	if enabledOnly {
		q += ` WHERE enabled=true`
	}
	q += ` ORDER BY sort_order,id`
	rows, e := s.db.QueryContext(ctx, q)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []TrainingTopic
	for rows.Next() {
		var v TrainingTopic
		if e = rows.Scan(&v.ID, &v.Key, &v.Title, &v.SortOrder, &v.Enabled); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *SQLStore) DeleteTopic(ctx context.Context, id int64) error {
	r, err := s.db.ExecContext(ctx, `DELETE FROM training_topics WHERE id=$1`, id)
	if err != nil {
		return mapDBError(err)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func caseArgs(c TrainingCase) ([]any, error) {
	bc, e := jsonValue(c.BusinessChallenges)
	if e != nil {
		return nil, e
	}
	tg, _ := jsonValue(c.TrainingGoals)
	tm, _ := jsonValue(c.TrainingModules)
	meth, _ := jsonValue(c.TrainingMethods)
	return []any{c.Slug, c.Title, c.Summary, nullableID(c.CoverAssetID), c.CompanyDisplayName, c.CompanyInternalNameEncrypted, c.Industry, c.City, c.ParticipantRange, c.TrainingDate, c.DurationLabel, bc, tg, tm, meth, c.TrainerID, c.TrainerNameSnapshot, c.Status, c.AuthorizationStatus, c.Featured, c.SortOrder, c.PublishedAt}, nil
}

const caseColumns = `id,slug,title,summary,COALESCE(cover_asset_id,0),company_display_name,company_internal_name_encrypted,industry,city,participant_range,training_date,duration_label,business_challenges,training_goals,training_modules,training_methods,trainer_id,trainer_name_snapshot,status,authorization_status,featured,sort_order,version,published_at,created_at,updated_at`

func scanCase(row interface{ Scan(...any) error }) (TrainingCase, error) {
	var c TrainingCase
	var bc, tg, tm, meth []byte
	var td sql.NullTime
	var pa sql.NullTime
	err := row.Scan(&c.ID, &c.Slug, &c.Title, &c.Summary, &c.CoverAssetID, &c.CompanyDisplayName, &c.CompanyInternalNameEncrypted, &c.Industry, &c.City, &c.ParticipantRange, &td, &c.DurationLabel, &bc, &tg, &tm, &meth, &c.TrainerID, &c.TrainerNameSnapshot, &c.Status, &c.AuthorizationStatus, &c.Featured, &c.SortOrder, &c.Version, &pa, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, mapDBError(err)
	}
	if td.Valid {
		c.TrainingDate = &td.Time
	}
	if pa.Valid {
		c.PublishedAt = &pa.Time
	}
	for _, p := range []struct {
		b []byte
		v *[]string
	}{{bc, &c.BusinessChallenges}, {tg, &c.TrainingGoals}, {tm, &c.TrainingModules}, {meth, &c.TrainingMethods}} {
		if err = json.Unmarshal(p.b, p.v); err != nil {
			return c, err
		}
	}
	return c, nil
}

func (s *SQLStore) CreateCase(ctx context.Context, a CaseAggregate) (CaseAggregate, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return CaseAggregate{}, e
	}
	defer tx.Rollback()
	args, e := caseArgs(a.Case)
	if e != nil {
		return CaseAggregate{}, e
	}
	e = tx.QueryRowContext(ctx, `INSERT INTO training_cases(slug,title,summary,cover_asset_id,company_display_name,company_internal_name_encrypted,industry,city,participant_range,training_date,duration_label,business_challenges,training_goals,training_modules,training_methods,trainer_id,trainer_name_snapshot,status,authorization_status,featured,sort_order,published_at) VALUES (`+placeholders(22)+`) RETURNING id,version,created_at,updated_at`, args...).Scan(&a.Case.ID, &a.Case.Version, &a.Case.CreatedAt, &a.Case.UpdatedAt)
	if e != nil {
		return CaseAggregate{}, mapDBError(e)
	}
	if e = replaceCaseRelations(ctx, tx, a); e != nil {
		return CaseAggregate{}, mapDBError(e)
	}
	if e = tx.Commit(); e != nil {
		return CaseAggregate{}, e
	}
	normalizeCaseAggregate(&a)
	return a, nil
}

func normalizeCaseAggregate(a *CaseAggregate) {
	for i := range a.Media {
		a.Media[i].CaseID = a.Case.ID
		a.Media[i].Position = i
	}
}
func placeholders(n int) string {
	v := make([]string, n)
	for i := range v {
		v[i] = fmt.Sprintf("$%d", i+1)
	}
	return strings.Join(v, ",")
}

func replaceCaseRelations(ctx context.Context, tx *sql.Tx, a CaseAggregate) error {
	for _, q := range []string{`DELETE FROM training_case_media WHERE case_id=$1`, `DELETE FROM training_case_topics WHERE case_id=$1`, `DELETE FROM training_case_solutions WHERE case_id=$1`} {
		if _, e := tx.ExecContext(ctx, q, a.Case.ID); e != nil {
			return e
		}
	}
	for i, m := range a.Media {
		_, e := tx.ExecContext(ctx, `INSERT INTO training_case_media(case_id,media_asset_id,role,position,caption,status) VALUES($1,$2,$3,$4,$5,$6)`, a.Case.ID, m.MediaAssetID, m.Role, i, m.Caption, m.Status)
		if e != nil {
			return e
		}
	}
	for i, id := range a.TopicIDs {
		if _, e := tx.ExecContext(ctx, `INSERT INTO training_case_topics(case_id,topic_id,position) VALUES($1,$2,$3)`, a.Case.ID, id, i); e != nil {
			return e
		}
	}
	for i, id := range a.SolutionIDs {
		if _, e := tx.ExecContext(ctx, `INSERT INTO training_case_solutions(case_id,solution_id,position) VALUES($1,$2,$3)`, a.Case.ID, id, i); e != nil {
			return e
		}
	}
	return nil
}
func (s *SQLStore) GetCase(ctx context.Context, id int64) (CaseAggregate, error) {
	c, e := scanCase(s.db.QueryRowContext(ctx, `SELECT `+caseColumns+` FROM training_cases WHERE id=$1`, id))
	if e != nil {
		return CaseAggregate{}, e
	}
	a := CaseAggregate{Case: c}
	rows, e := s.db.QueryContext(ctx, `SELECT id,case_id,media_asset_id,role,position,caption,status FROM training_case_media WHERE case_id=$1 ORDER BY position,id`, id)
	if e != nil {
		return a, e
	}
	for rows.Next() {
		var m CaseMedia
		if e = rows.Scan(&m.ID, &m.CaseID, &m.MediaAssetID, &m.Role, &m.Position, &m.Caption, &m.Status); e != nil {
			rows.Close()
			return a, e
		}
		a.Media = append(a.Media, m)
	}
	rows.Close()
	for q, dst := range map[string]*[]int64{`SELECT topic_id FROM training_case_topics WHERE case_id=$1 ORDER BY position,topic_id`: &a.TopicIDs, `SELECT solution_id FROM training_case_solutions WHERE case_id=$1 ORDER BY position,solution_id`: &a.SolutionIDs} {
		r, e := s.db.QueryContext(ctx, q, id)
		if e != nil {
			return a, e
		}
		for r.Next() {
			var x int64
			if e = r.Scan(&x); e != nil {
				r.Close()
				return a, e
			}
			*dst = append(*dst, x)
		}
		r.Close()
	}
	return a, nil
}
func (s *SQLStore) UpdateCase(ctx context.Context, a CaseAggregate, expected int64) (CaseAggregate, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return CaseAggregate{}, e
	}
	defer tx.Rollback()
	args, _ := caseArgs(a.Case)
	args = append(args, a.Case.ID, expected)
	e = tx.QueryRowContext(ctx, `UPDATE training_cases SET slug=$1,title=$2,summary=$3,cover_asset_id=$4,company_display_name=$5,company_internal_name_encrypted=$6,industry=$7,city=$8,participant_range=$9,training_date=$10,duration_label=$11,business_challenges=$12,training_goals=$13,training_modules=$14,training_methods=$15,trainer_id=$16,trainer_name_snapshot=$17,status=$18,authorization_status=$19,featured=$20,sort_order=$21,published_at=$22,version=version+1,updated_at=now() WHERE id=$23 AND version=$24 RETURNING version,created_at,updated_at`, args...).Scan(&a.Case.Version, &a.Case.CreatedAt, &a.Case.UpdatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return CaseAggregate{}, ErrVersionConflict
	}
	if e != nil {
		return CaseAggregate{}, mapDBError(e)
	}
	if e = replaceCaseRelations(ctx, tx, a); e != nil {
		return CaseAggregate{}, mapDBError(e)
	}
	if e = tx.Commit(); e != nil {
		return CaseAggregate{}, e
	}
	normalizeCaseAggregate(&a)
	return a, nil
}
func (s *SQLStore) ReplaceCaseMedia(ctx context.Context, id, expected int64, media []CaseMedia) (CaseAggregate, error) {
	a, e := s.GetCase(ctx, id)
	if e != nil {
		return a, e
	}
	a.Media = media
	return s.UpdateCase(ctx, a, expected)
}
func (s *SQLStore) DeleteCase(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM training_case_media WHERE case_id=$1`, `DELETE FROM training_case_topics WHERE case_id=$1`, `DELETE FROM training_case_solutions WHERE case_id=$1`} {
		if _, err = tx.ExecContext(ctx, q, id); err != nil {
			return mapDBError(err)
		}
	}
	r, err := tx.ExecContext(ctx, `DELETE FROM training_cases WHERE id=$1`, id)
	if err != nil {
		return mapDBError(err)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func solutionArgs(v EnterpriseSolution) ([]any, error) {
	au, e := jsonValue(v.Audiences)
	if e != nil {
		return nil, e
	}
	pr, _ := jsonValue(v.Problems)
	goals, _ := jsonValue(v.Goals)
	mo, _ := jsonValue(v.Modules)
	dm, _ := jsonValue(v.DeliveryMethods)
	ci, _ := jsonValue(v.CustomizableItems)
	return []any{v.Slug, v.Title, v.Summary, nullableID(v.CoverAssetID), au, pr, goals, mo, dm, v.RecommendedParticipants, v.RecommendedDuration, ci, v.TrainerID, v.TrainerNameSnapshot, v.Status, v.Featured, v.SortOrder, v.PublishedAt}, nil
}

const solutionColumns = `id,slug,title,summary,COALESCE(cover_asset_id,0),audiences,problems,goals,modules,delivery_methods,recommended_participants,recommended_duration,customizable_items,trainer_id,trainer_name_snapshot,status,featured,sort_order,version,published_at,created_at,updated_at`

func scanSolution(row interface{ Scan(...any) error }) (EnterpriseSolution, error) {
	var v EnterpriseSolution
	var au, pr, goals, mo, dm, ci []byte
	var pa sql.NullTime
	e := row.Scan(&v.ID, &v.Slug, &v.Title, &v.Summary, &v.CoverAssetID, &au, &pr, &goals, &mo, &dm, &v.RecommendedParticipants, &v.RecommendedDuration, &ci, &v.TrainerID, &v.TrainerNameSnapshot, &v.Status, &v.Featured, &v.SortOrder, &v.Version, &pa, &v.CreatedAt, &v.UpdatedAt)
	if e != nil {
		return v, mapDBError(e)
	}
	if pa.Valid {
		v.PublishedAt = &pa.Time
	}
	for _, p := range []struct {
		b []byte
		v *[]string
	}{{au, &v.Audiences}, {pr, &v.Problems}, {goals, &v.Goals}, {mo, &v.Modules}, {dm, &v.DeliveryMethods}, {ci, &v.CustomizableItems}} {
		if e = json.Unmarshal(p.b, p.v); e != nil {
			return v, e
		}
	}
	return v, nil
}
func (s *SQLStore) CreateSolution(ctx context.Context, a SolutionAggregate) (SolutionAggregate, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return a, e
	}
	defer tx.Rollback()
	args, e := solutionArgs(a.Solution)
	if e != nil {
		return a, e
	}
	e = tx.QueryRowContext(ctx, `INSERT INTO enterprise_solutions(slug,title,summary,cover_asset_id,audiences,problems,goals,modules,delivery_methods,recommended_participants,recommended_duration,customizable_items,trainer_id,trainer_name_snapshot,status,featured,sort_order,published_at) VALUES (`+placeholders(18)+`) RETURNING id,version,created_at,updated_at`, args...).Scan(&a.Solution.ID, &a.Solution.Version, &a.Solution.CreatedAt, &a.Solution.UpdatedAt)
	if e != nil {
		return a, mapDBError(e)
	}
	for i, id := range a.CaseIDs {
		if _, e = tx.ExecContext(ctx, `INSERT INTO training_case_solutions(case_id,solution_id,position) VALUES($1,$2,$3)`, id, a.Solution.ID, i); e != nil {
			return a, mapDBError(e)
		}
	}
	if e = tx.Commit(); e != nil {
		return a, e
	}
	return a, nil
}
func (s *SQLStore) GetSolution(ctx context.Context, id int64) (SolutionAggregate, error) {
	v, e := scanSolution(s.db.QueryRowContext(ctx, `SELECT `+solutionColumns+` FROM enterprise_solutions WHERE id=$1`, id))
	if e != nil {
		return SolutionAggregate{}, e
	}
	a := SolutionAggregate{Solution: v}
	r, e := s.db.QueryContext(ctx, `SELECT case_id FROM training_case_solutions WHERE solution_id=$1 ORDER BY position,case_id`, id)
	if e != nil {
		return a, e
	}
	defer r.Close()
	for r.Next() {
		var x int64
		if e = r.Scan(&x); e != nil {
			return a, e
		}
		a.CaseIDs = append(a.CaseIDs, x)
	}
	return a, r.Err()
}
func (s *SQLStore) ListSolutions(ctx context.Context) ([]EnterpriseSolution, error) {
	r, e := s.db.QueryContext(ctx, `SELECT `+solutionColumns+` FROM enterprise_solutions ORDER BY sort_order,id`)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	var out []EnterpriseSolution
	for r.Next() {
		v, e := scanSolution(r)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, r.Err()
}
func (s *SQLStore) UpdateSolution(ctx context.Context, a SolutionAggregate, expected int64) (SolutionAggregate, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return a, e
	}
	defer tx.Rollback()
	args, _ := solutionArgs(a.Solution)
	args = append(args, a.Solution.ID, expected)
	e = tx.QueryRowContext(ctx, `UPDATE enterprise_solutions SET slug=$1,title=$2,summary=$3,cover_asset_id=$4,audiences=$5,problems=$6,goals=$7,modules=$8,delivery_methods=$9,recommended_participants=$10,recommended_duration=$11,customizable_items=$12,trainer_id=$13,trainer_name_snapshot=$14,status=$15,featured=$16,sort_order=$17,published_at=$18,version=version+1,updated_at=now() WHERE id=$19 AND version=$20 RETURNING version,created_at,updated_at`, args...).Scan(&a.Solution.Version, &a.Solution.CreatedAt, &a.Solution.UpdatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return a, ErrVersionConflict
	}
	if e != nil {
		return a, mapDBError(e)
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM training_case_solutions WHERE solution_id=$1`, a.Solution.ID); e != nil {
		return a, e
	}
	for i, id := range a.CaseIDs {
		if _, e = tx.ExecContext(ctx, `INSERT INTO training_case_solutions(case_id,solution_id,position) VALUES($1,$2,$3)`, id, a.Solution.ID, i); e != nil {
			return a, mapDBError(e)
		}
	}
	if e = tx.Commit(); e != nil {
		return a, e
	}
	return a, nil
}
func (s *SQLStore) DeleteSolution(ctx context.Context, id int64) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `DELETE FROM training_case_solutions WHERE solution_id=$1`, id); e != nil {
		return mapDBError(e)
	}
	r, e := tx.ExecContext(ctx, `DELETE FROM enterprise_solutions WHERE id=$1`, id)
	if e != nil {
		return mapDBError(e)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func projectPublicCase(c TrainingCase, media []CaseMedia, topics []TrainingTopic) PublicCase {
	publicMedia := make([]PublicCaseMedia, len(media))
	for i, m := range media {
		publicMedia[i] = PublicCaseMedia{MediaAssetID: m.MediaAssetID, Role: m.Role, Position: m.Position, Caption: m.Caption}
	}
	return PublicCase{ID: c.ID, Slug: c.Slug, Title: c.Title, Summary: c.Summary, CoverAssetID: c.CoverAssetID, CompanyDisplayName: c.CompanyDisplayName, Industry: c.Industry, City: c.City, ParticipantRange: c.ParticipantRange, DurationLabel: c.DurationLabel, BusinessChallenges: c.BusinessChallenges, TrainingGoals: c.TrainingGoals, TrainingModules: c.TrainingModules, TrainingMethods: c.TrainingMethods, TrainerID: c.TrainerID, TrainerName: c.TrainerNameSnapshot, Featured: c.Featured, Media: publicMedia, Topics: topics}
}
func (s *SQLStore) ListPublicCases(ctx context.Context, q PublicCaseQuery) ([]PublicCase, error) {
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	args := []any{limit, q.Offset}
	where := `c.status='published' AND c.authorization_status='approved'`
	if q.Topic.Valid() {
		args = append(args, q.Topic)
		where += ` AND EXISTS (SELECT 1 FROM training_case_topics ct JOIN training_topics t ON t.id=ct.topic_id WHERE ct.case_id=c.id AND t.key=$3 AND t.enabled=true)`
	}
	r, e := s.db.QueryContext(ctx, `SELECT `+caseColumns+` FROM training_cases c WHERE `+where+` ORDER BY c.featured DESC,c.sort_order,c.id DESC LIMIT $1 OFFSET $2`, args...)
	if e != nil {
		return nil, e
	}
	var records []TrainingCase
	for r.Next() {
		c, e := scanCase(r)
		if e != nil {
			r.Close()
			return nil, e
		}
		records = append(records, c)
	}
	if e = r.Err(); e != nil {
		r.Close()
		return nil, e
	}
	r.Close()
	var out []PublicCase
	for _, c := range records {
		a, e := s.GetCase(ctx, c.ID)
		if e != nil {
			return nil, e
		}
		topics, e := s.publicTopics(ctx, c.ID)
		if e != nil {
			return nil, e
		}
		var media []CaseMedia
		for _, m := range a.Media {
			if m.Status == CaseMediaPublished {
				media = append(media, m)
			}
		}
		out = append(out, projectPublicCase(c, media, topics))
	}
	return out, nil
}
func (s *SQLStore) publicTopics(ctx context.Context, id int64) ([]TrainingTopic, error) {
	r, e := s.db.QueryContext(ctx, `SELECT t.id,t.key,t.title,t.sort_order,t.enabled FROM training_case_topics ct JOIN training_topics t ON t.id=ct.topic_id WHERE ct.case_id=$1 AND t.enabled=true ORDER BY ct.position,t.id`, id)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	var out []TrainingTopic
	for r.Next() {
		var v TrainingTopic
		if e = r.Scan(&v.ID, &v.Key, &v.Title, &v.SortOrder, &v.Enabled); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, r.Err()
}
func (s *SQLStore) GetPublicCaseBySlug(ctx context.Context, slug string) (PublicCase, error) {
	c, e := scanCase(s.db.QueryRowContext(ctx, `SELECT `+caseColumns+` FROM training_cases WHERE slug=$1 AND status='published' AND authorization_status='approved'`, slug))
	if e != nil {
		return PublicCase{}, e
	}
	a, e := s.GetCase(ctx, c.ID)
	if e != nil {
		return PublicCase{}, e
	}
	topics, e := s.publicTopics(ctx, c.ID)
	if e != nil {
		return PublicCase{}, e
	}
	var media []CaseMedia
	for _, m := range a.Media {
		if m.Status == CaseMediaPublished {
			media = append(media, m)
		}
	}
	return projectPublicCase(c, media, topics), nil
}
