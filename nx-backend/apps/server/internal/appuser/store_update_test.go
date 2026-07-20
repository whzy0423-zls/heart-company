package appuser

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdateAdminFieldsUpdatesStatusAndMemberLevel(t *testing.T) {
	var seenQuery string
	var seenArgs []driver.NamedValue
	database := openAppUserUpdateTestDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		seenQuery = query
		seenArgs = args
		return &appUserUpdateRows{
			values: []driver.Value{
				int64(42),
				"13800000021",
				"测试客户",
				"",
				"disabled",
				"vip",
				nil,
				nil,
				"app_sms",
				nil,
				time.Unix(100, 0),
				time.Unix(200, 0),
			},
		}, nil
	})
	store := NewStore(database)

	updated, err := store.UpdateAdminFields(context.Background(), 42, UpdateAdminFieldsInput{
		MemberLevel: "vip",
		Status:      "disabled",
	})
	if err != nil {
		t.Fatalf("update admin fields: %v", err)
	}

	if !strings.Contains(seenQuery, "UPDATE app_users") {
		t.Fatalf("expected update query, got %q", seenQuery)
	}
	if len(seenArgs) != 3 ||
		seenArgs[0].Value != "disabled" ||
		seenArgs[1].Value != "vip" ||
		seenArgs[2].Value != int64(42) {
		t.Fatalf("unexpected update args: %+v", seenArgs)
	}
	if updated.ID != 42 || updated.Status != "disabled" || updated.MemberLevel != "vip" {
		t.Fatalf("unexpected updated user: %+v", updated)
	}
}

func TestUpdateAdminFieldsAllowsSingleFieldPatch(t *testing.T) {
	tests := []struct {
		name           string
		input          UpdateAdminFieldsInput
		wantStatusArg  any
		wantLevelArg   any
		returnedStatus string
		returnedLevel  string
		expectedStatus string
		expectedLevel  string
	}{
		{
			name:           "status only",
			input:          UpdateAdminFieldsInput{Status: "disabled"},
			wantStatusArg:  "disabled",
			wantLevelArg:   nil,
			returnedStatus: "disabled",
			returnedLevel:  "free",
			expectedStatus: "disabled",
			expectedLevel:  "free",
		},
		{
			name:           "member level only",
			input:          UpdateAdminFieldsInput{MemberLevel: "vip"},
			wantStatusArg:  nil,
			wantLevelArg:   "vip",
			returnedStatus: "active",
			returnedLevel:  "vip",
			expectedStatus: "active",
			expectedLevel:  "vip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seenArgs []driver.NamedValue
			database := openAppUserUpdateTestDB(t, func(_ context.Context, _ string, args []driver.NamedValue) (driver.Rows, error) {
				seenArgs = args
				return &appUserUpdateRows{
					values: []driver.Value{
						int64(42),
						"13800000021",
						"测试客户",
						"",
						tt.returnedStatus,
						tt.returnedLevel,
						nil,
						nil,
						"app_sms",
						nil,
						time.Unix(100, 0),
						time.Unix(200, 0),
					},
				}, nil
			})
			store := NewStore(database)

			updated, err := store.UpdateAdminFields(context.Background(), 42, tt.input)
			if err != nil {
				t.Fatalf("update admin fields: %v", err)
			}

			if len(seenArgs) != 3 ||
				seenArgs[0].Value != tt.wantStatusArg ||
				seenArgs[1].Value != tt.wantLevelArg ||
				seenArgs[2].Value != int64(42) {
				t.Fatalf("unexpected update args: %+v", seenArgs)
			}
			if updated.Status != tt.expectedStatus || updated.MemberLevel != tt.expectedLevel {
				t.Fatalf("unexpected updated user: %+v", updated)
			}
		})
	}
}

func TestUpdateAdminFieldsRejectsInvalidInput(t *testing.T) {
	store := NewStore(nil)

	tests := []struct {
		name  string
		id    int64
		input UpdateAdminFieldsInput
	}{
		{
			name:  "missing id",
			id:    0,
			input: UpdateAdminFieldsInput{MemberLevel: "vip", Status: "active"},
		},
		{
			name:  "unknown status",
			id:    42,
			input: UpdateAdminFieldsInput{MemberLevel: "vip", Status: "paused"},
		},
		{
			name:  "unknown member level",
			id:    42,
			input: UpdateAdminFieldsInput{MemberLevel: "gold", Status: "active"},
		},
		{
			name:  "empty update",
			id:    42,
			input: UpdateAdminFieldsInput{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.UpdateAdminFields(context.Background(), tt.id, tt.input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

var appUserUpdateDriverSeq atomic.Int64

func openAppUserUpdateTestDB(
	t *testing.T,
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error),
) *sql.DB {
	t.Helper()
	driverName := fmt.Sprintf("app_user_update_test_%d", appUserUpdateDriverSeq.Add(1))
	sql.Register(driverName, appUserUpdateDriver{query: query})
	database, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type appUserUpdateDriver struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (d appUserUpdateDriver) Open(string) (driver.Conn, error) {
	return appUserUpdateConn{query: d.query}, nil
}

type appUserUpdateConn struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (appUserUpdateConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (appUserUpdateConn) Close() error                        { return nil }
func (appUserUpdateConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (c appUserUpdateConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type appUserUpdateRows struct {
	done   bool
	values []driver.Value
}

func (appUserUpdateRows) Columns() []string {
	return []string{
		"id",
		"phone",
		"nickname",
		"avatar",
		"status",
		"member_level",
		"member_started_at",
		"member_expires_at",
		"register_source",
		"last_login_at",
		"create_time",
		"update_time",
	}
}

func (appUserUpdateRows) Close() error {
	return nil
}

func (r *appUserUpdateRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}
