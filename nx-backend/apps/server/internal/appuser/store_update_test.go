package appuser

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccountFieldJSONAndLegacyNullScan(t *testing.T) {
	t.Run("public JSON includes account without password hash", func(t *testing.T) {
		var user User
		if err := json.Unmarshal([]byte(`{
			"id": 42,
			"phone": "13800000021",
			"account": "legacy_user",
			"password_hash": "secret-hash",
			"passwordHash": "secret-hash"
		}`), &user); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(user)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(payload, &fields); err != nil {
			t.Fatal(err)
		}
		if fields["account"] != "legacy_user" {
			t.Fatalf("public user JSON account = %v, want legacy_user; payload=%s", fields["account"], payload)
		}
		for _, key := range []string{"password_hash", "passwordHash"} {
			if _, ok := fields[key]; ok {
				t.Fatalf("public user JSON exposed %q: %s", key, payload)
			}
		}
	})

	t.Run("legacy SQL NULL account scans as empty string", func(t *testing.T) {
		var seenQuery string
		database := openAppUserUpdateTestDB(t, func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
			seenQuery = query
			var account driver.Value
			if strings.Contains(query, "COALESCE(account, '')") {
				account = ""
			}
			return &appUserUpdateRows{
				values: []driver.Value{
					int64(42),
					"13800000021",
					account,
					"测试客户",
					"",
					"active",
					"free",
					nil,
					nil,
					"app_sms",
					nil,
					time.Unix(100, 0),
					time.Unix(200, 0),
				},
			}, nil
		})

		updated, err := NewStore(database).UpdateAdminFields(context.Background(), 42, UpdateAdminFieldsInput{Status: "active"})
		if err != nil {
			t.Fatalf("scan legacy account: %v", err)
		}
		if !strings.Contains(seenQuery, "COALESCE(account, '')") {
			t.Fatalf("account projection must coalesce legacy NULL, query=%s", seenQuery)
		}
		payload, err := json.Marshal(updated)
		if err != nil {
			t.Fatal(err)
		}
		if bytes := string(payload); strings.Contains(bytes, `"account"`) {
			t.Fatalf("empty legacy account should be omitted from JSON, got %s", bytes)
		}
	})
}

func TestPublicUserProjectionsCoalesceAccountInScanOrder(t *testing.T) {
	const account = "projection_user"
	assertProjection := func(t *testing.T, query string) {
		t.Helper()
		if !strings.Contains(query, "id, phone, COALESCE(account, ''), nickname") {
			t.Fatalf("public account projection must coalesce after phone, query=%s", query)
		}
	}
	assertUser := func(t *testing.T, user User, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if user.ID != 42 || user.Phone != "13800000021" || user.Account != account || user.Nickname != "测试客户" {
			t.Fatalf("public user scan order mismatch: %+v", user)
		}
	}

	t.Run("find or create by phone", func(t *testing.T) {
		var seenQuery string
		database := openAppUserUpdateTestDB(t, func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
			seenQuery = query
			return &appUserUpdateRows{values: appUserProjectionValues(account)}, nil
		})

		user, err := NewStore(database).FindOrCreateByPhone(context.Background(), "13800000021")
		assertUser(t, user, err)
		assertProjection(t, seenQuery)
	})

	t.Run("find by id", func(t *testing.T) {
		var seenQuery string
		database := openAppUserUpdateTestDB(t, func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
			seenQuery = query
			return &appUserUpdateRows{values: appUserProjectionValues(account)}, nil
		})

		user, err := NewStore(database).FindByID(context.Background(), 42)
		assertUser(t, user, err)
		assertProjection(t, seenQuery)
	})

	t.Run("list", func(t *testing.T) {
		var seenQuery string
		database := openAppUserUpdateTestDB(t, func(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
			if strings.Contains(query, "SELECT count(*)") {
				return &appUserInsightsRows{
					columns: []string{"count"},
					values:  [][]driver.Value{{int64(1)}},
				}, nil
			}
			seenQuery = query
			return &appUserUpdateRows{values: appUserProjectionValues(account)}, nil
		})

		result, err := NewStore(database).List(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if result.Total != 1 || len(result.Items) != 1 {
			t.Fatalf("unexpected public user page: %+v", result)
		}
		assertUser(t, result.Items[0], nil)
		assertProjection(t, seenQuery)
	})
}

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
				"admin_user",
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
						"admin_user",
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

func (appUserUpdateConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
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
		"account",
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

func appUserProjectionValues(account string) []driver.Value {
	return []driver.Value{
		int64(42),
		"13800000021",
		account,
		"测试客户",
		"",
		"active",
		"free",
		nil,
		nil,
		"app_sms",
		nil,
		time.Unix(100, 0),
		time.Unix(200, 0),
	}
}

var _ driver.ExecerContext = appUserUpdateConn{}
