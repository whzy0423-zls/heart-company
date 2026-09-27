package quiz

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCreateCardIgnoresLegacyWingTypeAndWritesCompatibilityZero(t *testing.T) {
	state := &noWingStoreState{}
	db := sql.OpenDB(noWingStoreConnector{state: state})
	t.Cleanup(func() { _ = db.Close() })

	var input CardInput
	if err := json.Unmarshal([]byte(`{"name":"朋友","relation":"friend","mainType":4,"wingType":5}`), &input); err != nil {
		t.Fatalf("legacy request should remain decodable: %v", err)
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(encodedInput)), "wing") {
		t.Fatalf("normalized request still exposes wing data: %s", encodedInput)
	}

	card, err := NewStore(db).CreateCardWithLimit(context.Background(), 7, 1, input)
	if err != nil {
		t.Fatal(err)
	}
	if state.insertWing != "0" {
		t.Fatalf("compatibility wing column = %s, want 0", state.insertWing)
	}
	if strings.Contains(strings.ToLower(state.insertProfile), "wing") {
		t.Fatalf("stored profile still contains wing data: %s", state.insertProfile)
	}
	encodedCard, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(encodedCard)), "wing") {
		t.Fatalf("card response still exposes wing data: %s", encodedCard)
	}
}

type noWingStoreState struct {
	insertWing    string
	insertProfile string
}

type noWingStoreConnector struct {
	state *noWingStoreState
}

func (c noWingStoreConnector) Connect(context.Context) (driver.Conn, error) {
	return &noWingStoreConn{state: c.state}, nil
}

func (noWingStoreConnector) Driver() driver.Driver { return noWingStoreDriver{} }

type noWingStoreDriver struct{}

func (noWingStoreDriver) Open(string) (driver.Conn, error) {
	return nil, fmt.Errorf("use connector")
}

type noWingStoreConn struct {
	state *noWingStoreState
}

func (*noWingStoreConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*noWingStoreConn) Close() error                        { return nil }
func (*noWingStoreConn) Begin() (driver.Tx, error)           { return noWingStoreTx{}, nil }
func (*noWingStoreConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return noWingStoreTx{}, nil
}
func (*noWingStoreConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (c *noWingStoreConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	switch {
	case strings.Contains(query, "SELECT id FROM app_users"):
		return &noWingStoreRows{columns: []string{"id"}, values: [][]driver.Value{{int64(7)}}}, nil
	case strings.Contains(query, "SELECT count(*) FROM app_user_cards"):
		return &noWingStoreRows{columns: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
	case strings.Contains(query, "INSERT INTO app_user_cards"):
		c.state.insertWing = fmt.Sprint(args[4].Value)
		profile := []byte(fmt.Sprint(args[5].Value))
		if raw, ok := args[5].Value.([]byte); ok {
			profile = raw
		} else if raw, ok := args[5].Value.(json.RawMessage); ok {
			profile = []byte(raw)
		}
		c.state.insertProfile = string(profile)
		return &noWingStoreRows{
			columns: strings.Split(cardCols, ", "),
			values:  [][]driver.Value{{int64(11), int64(7), "secondary", "朋友", "friend", int64(4), int64(0), profile, "active", now, now}},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

type noWingStoreTx struct{}

func (noWingStoreTx) Commit() error   { return nil }
func (noWingStoreTx) Rollback() error { return nil }

type noWingStoreRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *noWingStoreRows) Columns() []string { return r.columns }
func (r *noWingStoreRows) Close() error      { return nil }
func (r *noWingStoreRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
