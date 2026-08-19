package skillchat

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGetSessionEnforcesUserSceneAndVersionJoinsInSQL(t *testing.T) {
	registerSkillSessionBoundaryDriver.Do(func() { sql.Register("skill_session_boundary_test", skillSessionBoundaryDriver{}) })
	database, err := sql.Open("skill_session_boundary_test", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	session, err := NewStore(database).GetSession(context.Background(), 7, 41)
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != 41 || session.AppUserID != 7 || session.SkillVersionID != 91 || !session.Runnable() {
		t.Fatalf("session=%+v", session)
	}
	if session.MinAppVersion != "1.0.1" || session.SourceMetadata.ReviewPolicy != "product-baseline-v1" || len(session.SourceMetadata.RiskNotices) != 1 {
		t.Fatalf("fixed version metadata=%+v", session)
	}
	raw, _ := json.Marshal(session)
	for _, forbidden := range []string{"/private/source/SKILL.md", "manifest-secret-hash", "sourceContentHash"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("session DTO leaked internal metadata %q: %s", forbidden, raw)
		}
	}
}

var registerSkillSessionBoundaryDriver sync.Once

type skillSessionBoundaryDriver struct{}

func (skillSessionBoundaryDriver) Open(string) (driver.Conn, error) {
	return skillSessionBoundaryConn{}, nil
}

type skillSessionBoundaryConn struct{}

func (skillSessionBoundaryConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (skillSessionBoundaryConn) Close() error                        { return nil }
func (skillSessionBoundaryConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (skillSessionBoundaryConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	for _, fragment := range []string{
		"JOIN app_skill_versions version ON version.id = session.skill_version_id",
		"JOIN app_skills skill ON skill.id = version.skill_id",
		"JOIN app_skill_categories category ON category.id = skill.category_id",
		"JOIN app_skill_libraries library ON library.id = category.library_id",
		"session.id = $1",
		"session.app_user_id = $2",
		"session.scene = 'skill_chat'",
	} {
		if !strings.Contains(query, fragment) {
			return nil, errors.New("session query missing boundary: " + fragment)
		}
	}
	if len(args) != 2 || args[0].Value != int64(41) || args[1].Value != int64(7) {
		return nil, errors.New("session query arguments crossed ownership boundary")
	}
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	return &skillSessionRows{values: [][]driver.Value{{
		int64(41), int64(7), int64(9), int64(91), "art-of-learning", "学习之道", "school",
		"练习", "skill_chat", "1.1.0", "规则", int64(71), "general-learning-v1",
		"1.0.1", []byte(`{"reviewPolicy":"product-baseline-v1","reviewDecisionRef":"baseline","riskNotices":["缺页提示"],"source":"/private/source/SKILL.md","reviewManifestHash":"manifest-secret-hash","sourceContentHash":"secret"}`),
		"published", "enabled", "enabled", "enabled", int64(4), now, now,
	}}}, nil
}

type skillSessionRows struct {
	values [][]driver.Value
	index  int
}

func (r *skillSessionRows) Columns() []string {
	return []string{"id", "app_user_id", "skill_id", "skill_version_id", "skill_key", "skill_name", "skill_icon_key", "title", "scene", "version", "instructions", "theory_release_id", "safety_profile", "min_app_version", "source_metadata", "version_status", "library_status", "category_status", "skill_status", "generation_revision", "updated_at", "create_time"}
}
func (r *skillSessionRows) Close() error { return nil }
func (r *skillSessionRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var _ driver.QueryerContext = skillSessionBoundaryConn{}
