package appuser

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestRegisterWithPasswordValidatesBeforeDatabaseAccess(t *testing.T) {
	store := NewStore(nil)
	tests := []struct {
		name string
		in   RegisterWithPasswordInput
		want error
	}{
		{
			name: "account",
			in: RegisterWithPasswordInput{
				Account:  "bad",
				Password: "secret123",
				Nickname: "Alice",
			},
			want: ErrInvalidAccount,
		},
		{
			name: "password",
			in: RegisterWithPasswordInput{
				Account:  "alice_01",
				Password: "short",
				Nickname: "Alice",
			},
			want: ErrInvalidPassword,
		},
		{
			name: "nickname",
			in: RegisterWithPasswordInput{
				Account:  "alice_01",
				Password: "secret123",
				Nickname: "   ",
			},
			want: ErrInvalidNickname,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.RegisterWithPassword(context.Background(), tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("RegisterWithPassword() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRegisterWithPasswordCreatesAccountUser(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	const (
		phone    = "13800000101"
		codeHash = "new-user-code-hash"
		password = "secret123"
	)
	stamp := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	olderCodeID := insertRegisterSMSCode(t, database, phone, codeHash, time.Now().Add(time.Hour), false, stamp)
	newerCodeID := insertRegisterSMSCode(t, database, phone, codeHash, time.Now().Add(time.Hour), false, stamp)

	user, err := NewStore(database).RegisterWithPassword(ctx, RegisterWithPasswordInput{
		Account:     "  Alice_01  ",
		Password:    password,
		Nickname:    "  Alice  ",
		Phone:       phone,
		SMSCodeHash: codeHash,
	})
	if err != nil {
		t.Fatalf("RegisterWithPassword() error = %v", err)
	}
	if user.ID == 0 || user.Phone != phone || user.Account != "alice_01" || user.Nickname != "Alice" {
		t.Fatalf("RegisterWithPassword() user = %+v", user)
	}
	if user.Avatar != "" || user.Status != "active" || user.MemberLevel != "free" || user.RegisterSource != "account_sms" {
		t.Fatalf("RegisterWithPassword() public defaults = %+v", user)
	}
	if user.CreateTime == "" || user.UpdateTime == "" {
		t.Fatalf("RegisterWithPassword() missing public timestamps: %+v", user)
	}

	var account, passwordHash, nickname, registerSource string
	if err := database.QueryRowContext(ctx, `
		SELECT account, password_hash, nickname, register_source
		FROM app_users
		WHERE id=$1
	`, user.ID).Scan(&account, &passwordHash, &nickname, &registerSource); err != nil {
		t.Fatalf("read registered user: %v", err)
	}
	if account != "alice_01" || nickname != "Alice" || registerSource != "account_sms" {
		t.Fatalf("stored registration fields = account %q nickname %q source %q", account, nickname, registerSource)
	}
	if passwordHash == password {
		t.Fatal("stored password was not hashed")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		t.Fatalf("stored password hash does not match password: %v", err)
	}
	if cost, err := bcrypt.Cost([]byte(passwordHash)); err != nil || cost != bcrypt.DefaultCost {
		t.Fatalf("bcrypt cost = %d, %v; want %d", cost, err, bcrypt.DefaultCost)
	}

	if used := registerSMSCodeUsed(t, database, olderCodeID); used {
		t.Fatal("older matching SMS code was used instead of the newest row")
	}
	if used := registerSMSCodeUsed(t, database, newerCodeID); !used {
		t.Fatal("newest matching SMS code was not marked used")
	}

	found, err := NewStore(database).FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("FindByID() after registration: %v", err)
	}
	if found.ID != user.ID || found.Account != user.Account || found.Phone != user.Phone || found.RegisterSource != user.RegisterSource {
		t.Fatalf("public user cannot be read back: registered=%+v found=%+v", user, found)
	}
	payload, err := json.Marshal(user)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "password") || strings.Contains(string(payload), passwordHash) {
		t.Fatalf("public user exposed password data: %s", payload)
	}
}

func TestRegisterWithPasswordBindsLegacySMSUserInPlace(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store := NewStore(database)

	const (
		phone    = "13800000102"
		codeHash = "legacy-code-hash"
		password = "legacy-secret"
	)
	legacy, err := store.FindOrCreateByPhone(ctx, phone)
	if err != nil {
		t.Fatalf("FindOrCreateByPhone() error = %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE app_users
		SET avatar='legacy-avatar',
		    member_level='vip',
		    member_started_at=now() - interval '2 days',
		    member_expires_at=now() + interval '30 days',
		    register_source='legacy_sms'
		WHERE id=$1
	`, legacy.ID); err != nil {
		t.Fatalf("seed legacy business data: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO app_user_markers (app_user_id, marker) VALUES ($1, $2)`, legacy.ID, "preserve-me"); err != nil {
		t.Fatalf("seed related marker: %v", err)
	}
	codeID := insertRegisterSMSCode(t, database, phone, codeHash, time.Now().Add(time.Hour), false, time.Now())

	user, err := store.RegisterWithPassword(ctx, RegisterWithPasswordInput{
		Account:     "Legacy_01",
		Password:    password,
		Nickname:    "  Bound User  ",
		Phone:       phone,
		SMSCodeHash: codeHash,
	})
	if err != nil {
		t.Fatalf("RegisterWithPassword() error = %v", err)
	}
	if user.ID != legacy.ID {
		t.Fatalf("legacy user ID changed from %d to %d", legacy.ID, user.ID)
	}
	if user.Account != "legacy_01" || user.Nickname != "Bound User" {
		t.Fatalf("legacy credentials were not bound: %+v", user)
	}
	if user.Avatar != "legacy-avatar" || user.MemberLevel != "vip" || user.RegisterSource != "legacy_sms" {
		t.Fatalf("legacy business fields changed: %+v", user)
	}
	if user.MemberStartedAt == "" || user.MemberExpiresAt == "" || user.RemainingDays <= 0 {
		t.Fatalf("legacy membership fields were not preserved: %+v", user)
	}

	var marker, passwordHash string
	if err := database.QueryRowContext(ctx, `
		SELECT m.marker, u.password_hash
		FROM app_user_markers m
		JOIN app_users u ON u.id=m.app_user_id
		WHERE m.app_user_id=$1
	`, legacy.ID).Scan(&marker, &passwordHash); err != nil {
		t.Fatalf("read preserved legacy data: %v", err)
	}
	if marker != "preserve-me" {
		t.Fatalf("related marker = %q, want preserve-me", marker)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		t.Fatalf("legacy password hash does not match: %v", err)
	}
	var phoneRows int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_users WHERE phone=$1`, phone).Scan(&phoneRows); err != nil {
		t.Fatal(err)
	}
	if phoneRows != 1 {
		t.Fatalf("users with legacy phone = %d, want 1", phoneRows)
	}
	if !registerSMSCodeUsed(t, database, codeID) {
		t.Fatal("legacy registration SMS code was not marked used")
	}
}

func TestRegisterWithPasswordRejectsCredentialedPhone(t *testing.T) {
	tests := []struct {
		name         string
		account      any
		passwordHash any
	}{
		{name: "existing account", account: "existing_01", passwordHash: nil},
		{name: "existing password hash", account: nil, passwordHash: "existing-hash"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database := openRegisterWithPasswordFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			phone := fmt.Sprintf("1380000011%d", i)
			if _, err := database.ExecContext(ctx, `
				INSERT INTO app_users (phone, account, password_hash, nickname)
				VALUES ($1, $2, $3, 'Existing User')
			`, phone, tt.account, tt.passwordHash); err != nil {
				t.Fatalf("seed credentialed user: %v", err)
			}
			codeID := insertRegisterSMSCode(t, database, phone, "credentialed-code", time.Now().Add(time.Hour), false, time.Now())

			_, err := NewStore(database).RegisterWithPassword(ctx, RegisterWithPasswordInput{
				Account:     fmt.Sprintf("new_user_%d", i),
				Password:    "secret123",
				Nickname:    "New User",
				Phone:       phone,
				SMSCodeHash: "credentialed-code",
			})
			if !errors.Is(err, ErrPhoneAlreadyRegistered) {
				t.Fatalf("RegisterWithPassword() error = %v, want ErrPhoneAlreadyRegistered", err)
			}
			if registerSMSCodeUsed(t, database, codeID) {
				t.Fatal("phone conflict consumed the SMS code")
			}
		})
	}
}

func TestRegisterWithPasswordDuplicateAccountRollsBackSMSCode(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if _, err := database.ExecContext(ctx, `
		INSERT INTO app_users (phone, account, password_hash, nickname, register_source)
		VALUES ('13800000120', 'Taken_User', 'existing-hash', 'Owner', 'account_sms')
	`); err != nil {
		t.Fatalf("seed account owner: %v", err)
	}
	const (
		phone    = "13800000121"
		codeHash = "duplicate-code-hash"
	)
	codeID := insertRegisterSMSCode(t, database, phone, codeHash, time.Now().Add(time.Hour), false, time.Now())
	store := NewStore(database)

	_, err := store.RegisterWithPassword(ctx, RegisterWithPasswordInput{
		Account:     "taken_user",
		Password:    "secret123",
		Nickname:    "First Attempt",
		Phone:       phone,
		SMSCodeHash: codeHash,
	})
	if !errors.Is(err, ErrAccountTaken) {
		t.Fatalf("case-insensitive duplicate error = %v, want ErrAccountTaken", err)
	}
	if registerSMSCodeUsed(t, database, codeID) {
		t.Fatal("duplicate account conflict consumed the SMS code")
	}

	user, err := store.RegisterWithPassword(ctx, RegisterWithPasswordInput{
		Account:     "available_01",
		Password:    "secret123",
		Nickname:    "Second Attempt",
		Phone:       phone,
		SMSCodeHash: codeHash,
	})
	if err != nil {
		t.Fatalf("retry with available account: %v", err)
	}
	if user.Account != "available_01" || user.Phone != phone {
		t.Fatalf("retry user = %+v", user)
	}
	if !registerSMSCodeUsed(t, database, codeID) {
		t.Fatal("successful retry did not consume the SMS code")
	}
}

func TestRegisterWithPasswordUniqueAccountFailureRollsBackSMSCode(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	const (
		phone      = "13800000150"
		ownerPhone = "13800000151"
		codeHash   = "forced-unique-code-hash"
	)
	if _, err := database.ExecContext(ctx, `
		CREATE FUNCTION force_registration_account_conflict() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.phone = '13800000150' AND NEW.account = 'forced_conflict' THEN
		    INSERT INTO app_users (phone, account, password_hash, nickname, register_source)
		    VALUES ('13800000151', NEW.account, 'trigger-hash', 'Trigger Owner', 'account_sms');
		  END IF;
		  RETURN NEW;
		END;
		$$;

		CREATE TRIGGER force_registration_account_conflict
		BEFORE INSERT ON app_users
		FOR EACH ROW EXECUTE FUNCTION force_registration_account_conflict();
	`); err != nil {
		t.Fatalf("create forced unique-account conflict: %v", err)
	}
	codeID := insertRegisterSMSCode(t, database, phone, codeHash, time.Now().Add(time.Hour), false, time.Now())
	store := NewStore(database)

	_, err := store.RegisterWithPassword(ctx, RegisterWithPasswordInput{
		Account:     "forced_conflict",
		Password:    "secret123",
		Nickname:    "First Attempt",
		Phone:       phone,
		SMSCodeHash: codeHash,
	})
	if !errors.Is(err, ErrAccountTaken) {
		t.Fatalf("forced unique-account error = %v, want ErrAccountTaken", err)
	}
	if registerSMSCodeUsed(t, database, codeID) {
		t.Fatal("unique-account write failure consumed the SMS code")
	}
	var rolledBackOwnerRows int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_users WHERE phone=$1`, ownerPhone).Scan(&rolledBackOwnerRows); err != nil {
		t.Fatal(err)
	}
	if rolledBackOwnerRows != 0 {
		t.Fatalf("forced conflict transaction left %d trigger-created users", rolledBackOwnerRows)
	}

	user, err := store.RegisterWithPassword(ctx, RegisterWithPasswordInput{
		Account:     "available_02",
		Password:    "secret123",
		Nickname:    "Second Attempt",
		Phone:       phone,
		SMSCodeHash: codeHash,
	})
	if err != nil {
		t.Fatalf("retry after unique-account rollback: %v", err)
	}
	if user.Account != "available_02" || user.Phone != phone {
		t.Fatalf("retry user = %+v", user)
	}
	if !registerSMSCodeUsed(t, database, codeID) {
		t.Fatal("successful retry did not consume the SMS code")
	}
}

func TestRegisterWithPasswordRejectsInvalidSMSCode(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, database *sql.DB, phone, codeHash string)
	}{
		{name: "missing"},
		{
			name: "expired",
			prepare: func(t *testing.T, database *sql.DB, phone, codeHash string) {
				insertRegisterSMSCode(t, database, phone, codeHash, time.Now().Add(-time.Minute), false, time.Now())
			},
		},
		{
			name: "used",
			prepare: func(t *testing.T, database *sql.DB, phone, codeHash string) {
				insertRegisterSMSCode(t, database, phone, codeHash, time.Now().Add(time.Hour), true, time.Now())
			},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database := openRegisterWithPasswordFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			phone := fmt.Sprintf("1380000013%d", i)
			const codeHash = "invalid-code-hash"
			if tt.prepare != nil {
				tt.prepare(t, database, phone, codeHash)
			}

			_, err := NewStore(database).RegisterWithPassword(ctx, RegisterWithPasswordInput{
				Account:     fmt.Sprintf("invalid_user_%d", i),
				Password:    "secret123",
				Nickname:    "Invalid Code",
				Phone:       phone,
				SMSCodeHash: codeHash,
			})
			if !errors.Is(err, ErrInvalidSMSCode) {
				t.Fatalf("RegisterWithPassword() error = %v, want ErrInvalidSMSCode", err)
			}
			var users int
			if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_users WHERE phone=$1`, phone).Scan(&users); err != nil {
				t.Fatal(err)
			}
			if users != 0 {
				t.Fatalf("invalid SMS code created %d users", users)
			}
		})
	}
}

func TestRegisterWithPasswordConcurrentSMSCodeUseHasOneWinner(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const (
		phone    = "13800000140"
		codeHash = "concurrent-code-hash"
	)
	codeID := insertRegisterSMSCode(t, database, phone, codeHash, time.Now().Add(time.Hour), false, time.Now())

	type result struct {
		user User
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var group sync.WaitGroup
	for _, account := range []string{"race_user_1", "race_user_2"} {
		group.Add(1)
		go func(account string) {
			defer group.Done()
			<-start
			user, err := NewStore(database).RegisterWithPassword(ctx, RegisterWithPasswordInput{
				Account:     account,
				Password:    "secret123",
				Nickname:    account,
				Phone:       phone,
				SMSCodeHash: codeHash,
			})
			results <- result{user: user, err: err}
		}(account)
	}
	close(start)
	group.Wait()
	close(results)

	successes := 0
	invalidCodes := 0
	for got := range results {
		switch {
		case got.err == nil:
			successes++
			if got.user.ID == 0 || got.user.Phone != phone {
				t.Fatalf("successful concurrent result = %+v", got.user)
			}
		case errors.Is(got.err, ErrInvalidSMSCode):
			invalidCodes++
		default:
			t.Fatalf("unexpected concurrent registration error: %v", got.err)
		}
	}
	if successes != 1 || invalidCodes != 1 {
		t.Fatalf("concurrent results: successes=%d invalid_codes=%d, want 1 each", successes, invalidCodes)
	}
	var users int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_users WHERE phone=$1`, phone).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("concurrent registrations created %d users, want 1", users)
	}
	if !registerSMSCodeUsed(t, database, codeID) {
		t.Fatal("winning concurrent registration did not consume SMS code")
	}
}

func TestRegisterWithPasswordMapsPostgresUniqueErrors(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		want       error
	}{
		{name: "account index", constraint: "idx_app_users_account_unique", want: ErrAccountTaken},
		{name: "phone constraint", constraint: "app_users_phone_key", want: ErrPhoneAlreadyRegistered},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			databaseErr := fmt.Errorf("write app user: %w", &pgconn.PgError{
				Code:           "23505",
				ConstraintName: tt.constraint,
			})
			if got := mapRegisterWithPasswordError(databaseErr); !errors.Is(got, tt.want) {
				t.Fatalf("mapRegisterWithPasswordError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func openRegisterWithPasswordFixture(t *testing.T) *sql.DB {
	t.Helper()
	database, _ := testdb.OpenEnvIsolatedSchema(t, "app_password_register")
	database.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := database.ExecContext(ctx, registerWithPasswordFixtureSchema); err != nil {
		t.Fatalf("create RegisterWithPassword fixture: %v", err)
	}
	return database
}

func insertRegisterSMSCode(t *testing.T, database *sql.DB, phone, codeHash string, expiresAt time.Time, used bool, createTime time.Time) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`
		INSERT INTO app_sms_codes (phone, code_hash, expires_at, used, create_time)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, phone, codeHash, expiresAt, used, createTime).Scan(&id); err != nil {
		t.Fatalf("insert SMS code: %v", err)
	}
	return id
}

func registerSMSCodeUsed(t *testing.T, database *sql.DB, id int64) bool {
	t.Helper()
	var used bool
	if err := database.QueryRow(`SELECT used FROM app_sms_codes WHERE id=$1`, id).Scan(&used); err != nil {
		t.Fatalf("read SMS code %d: %v", id, err)
	}
	return used
}

const registerWithPasswordFixtureSchema = `
CREATE TABLE app_users (
  id                BIGSERIAL PRIMARY KEY,
  phone             TEXT NOT NULL UNIQUE,
  account           TEXT,
  password_hash     TEXT,
  nickname          TEXT NOT NULL DEFAULT '',
  avatar            TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL DEFAULT 'active',
  member_level      TEXT NOT NULL DEFAULT 'free',
  member_started_at TIMESTAMPTZ,
  member_expires_at TIMESTAMPTZ,
  register_source   TEXT NOT NULL DEFAULT 'sms',
  last_login_at     TIMESTAMPTZ,
  create_time       TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_app_users_account_unique
  ON app_users (lower(account))
  WHERE account IS NOT NULL AND btrim(account) <> '';

CREATE TABLE app_sms_codes (
  id          BIGSERIAL PRIMARY KEY,
  phone       TEXT NOT NULL,
  code_hash   TEXT NOT NULL,
  expires_at  TIMESTAMPTZ NOT NULL,
  used        BOOLEAN NOT NULL DEFAULT false,
  send_ip     TEXT NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE app_user_markers (
  app_user_id BIGINT PRIMARY KEY REFERENCES app_users(id) ON DELETE CASCADE,
  marker      TEXT NOT NULL
);
`
