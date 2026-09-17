package skillchat

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestToggleFavoriteScopesMessageToAssistantOwnerAndSkillChat(t *testing.T) {
	registerSkillFavoriteDriver.Do(func() { sql.Register("skill_favorite_test", skillFavoriteDriver{}) })
	database, err := sql.Open("skill_favorite_test", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	favorite, err := NewStore(database).ToggleFavorite(context.Background(), 7, 31)
	if err != nil {
		t.Fatal(err)
	}
	if !favorite {
		t.Fatal("favorite=false, want true")
	}
}

var registerSkillFavoriteDriver sync.Once

type skillFavoriteDriver struct{}

func (skillFavoriteDriver) Open(string) (driver.Conn, error) { return skillFavoriteConn{}, nil }

type skillFavoriteConn struct{}

func (skillFavoriteConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (skillFavoriteConn) Close() error                        { return nil }
func (skillFavoriteConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (skillFavoriteConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	for _, fragment := range []string{
		"UPDATE app_chat_messages",
		"favorite = NOT message.favorite",
		"session.app_user_id=$2",
		"session.scene='skill_chat'",
		"message.role='assistant'",
		"RETURNING message.favorite",
	} {
		if !strings.Contains(query, fragment) {
			return nil, errors.New("favorite query missing boundary: " + fragment)
		}
	}
	if len(args) != 2 || args[0].Value != int64(31) || args[1].Value != int64(7) {
		return nil, errors.New("favorite query arguments crossed ownership boundary")
	}
	return &skillFavoriteRows{values: [][]driver.Value{{true}}}, nil
}

type skillFavoriteRows struct {
	values [][]driver.Value
	index  int
}

func (r *skillFavoriteRows) Columns() []string { return []string{"favorite"} }
func (r *skillFavoriteRows) Close() error      { return nil }
func (r *skillFavoriteRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var _ driver.QueryerContext = skillFavoriteConn{}
