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

func TestRegisterWithPasswordRejectsDisabledLegacySMSUser(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store := NewStore(database)

	const (
		phone    = "13800000103"
		codeHash = "disabled-legacy-code-hash"
	)
	legacy, err := store.FindOrCreateByPhone(ctx, phone)
	if err != nil {
		t.Fatalf("FindOrCreateByPhone() error = %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_users SET status='disabled' WHERE id=$1`, legacy.ID); err != nil {
		t.Fatalf("disable legacy SMS user: %v", err)
	}
	codeID := insertRegisterSMSCode(t, database, phone, codeHash, time.Now().Add(time.Hour), false, time.Now())

	_, err = store.RegisterWithPassword(ctx, RegisterWithPasswordInput{
		Account:     "disabled_legacy",
		Password:    "secret123",
		Nickname:    "Disabled User",
		Phone:       phone,
		SMSCodeHash: codeHash,
	})
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("RegisterWithPassword() error = %v, want ErrUserDisabled", err)
	}

	var account, passwordHash sql.NullString
	if err := database.QueryRowContext(ctx, `
		SELECT account, password_hash
		FROM app_users
		WHERE id=$1
	`, legacy.ID).Scan(&account, &passwordHash); err != nil {
		t.Fatalf("read disabled legacy credentials: %v", err)
	}
	if account.Valid || passwordHash.Valid {
		t.Fatalf("disabled legacy credentials changed: account=%v password_hash=%v", account, passwordHash)
	}
	if registerSMSCodeUsed(t, database, codeID) {
		t.Fatal("disabled legacy user consumed the SMS code")
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
	blocker, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin SMS code blocker: %v", err)
	}
	defer func() { _ = blocker.Rollback() }()
	var lockedCodeID int64
	if err := blocker.QueryRowContext(ctx, `
		SELECT id
		FROM app_sms_codes
		WHERE id=$1
		FOR UPDATE
	`, codeID).Scan(&lockedCodeID); err != nil {
		t.Fatalf("lock SMS code blocker row: %v", err)
	}

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
	if err := waitForBlockedSMSCodeRegistrations(ctx, database, 2, 3*time.Second); err != nil {
		_ = blocker.Rollback()
		group.Wait()
		t.Fatal(err)
	}
	select {
	case got := <-results:
		_ = blocker.Rollback()
		group.Wait()
		t.Fatalf("registration returned while SMS code row was locked: user=%+v err=%v", got.user, got.err)
	default:
	}
	if err := blocker.Commit(); err != nil {
		_ = blocker.Rollback()
		group.Wait()
		t.Fatalf("release SMS code blocker: %v", err)
	}
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

func TestRegisterWithPasswordConcurrentPhoneUseMapsUniqueConstraint(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const phone = "13800000141"
	type attempt struct {
		account  string
		codeHash string
		codeID   int64
	}
	attempts := []attempt{
		{account: "phone_race_1", codeHash: "phone-race-code-1"},
		{account: "phone_race_2", codeHash: "phone-race-code-2"},
	}
	for i := range attempts {
		attempts[i].codeID = insertRegisterSMSCode(t, database, phone, attempts[i].codeHash, time.Now().Add(time.Hour), false, time.Now())
	}

	blocker, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin app_users blocker: %v", err)
	}
	defer func() { _ = blocker.Rollback() }()
	if _, err := blocker.ExecContext(ctx, `LOCK TABLE app_users IN SHARE MODE`); err != nil {
		t.Fatalf("lock app_users for concurrent insert barrier: %v", err)
	}

	type result struct {
		codeID int64
		user   User
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, len(attempts))
	var group sync.WaitGroup
	for _, candidate := range attempts {
		group.Add(1)
		go func(attempt attempt) {
			defer group.Done()
			<-start
			user, err := NewStore(database).RegisterWithPassword(ctx, RegisterWithPasswordInput{
				Account:     attempt.account,
				Password:    "secret123",
				Nickname:    attempt.account,
				Phone:       phone,
				SMSCodeHash: attempt.codeHash,
			})
			results <- result{codeID: attempt.codeID, user: user, err: err}
		}(candidate)
	}
	close(start)
	if err := waitForBlockedAppUserInserts(ctx, database, len(attempts), 3*time.Second); err != nil {
		_ = blocker.Rollback()
		group.Wait()
		t.Fatal(err)
	}
	select {
	case got := <-results:
		_ = blocker.Rollback()
		group.Wait()
		t.Fatalf("registration returned while app_users inserts were locked: user=%+v err=%v", got.user, got.err)
	default:
	}
	if err := blocker.Commit(); err != nil {
		_ = blocker.Rollback()
		group.Wait()
		t.Fatalf("release app_users blocker: %v", err)
	}
	group.Wait()
	close(results)

	successes := 0
	phoneConflicts := 0
	var successfulCodeID, failedCodeID int64
	for got := range results {
		switch {
		case got.err == nil:
			successes++
			successfulCodeID = got.codeID
			if got.user.ID == 0 || got.user.Phone != phone {
				t.Fatalf("successful same-phone result = %+v", got.user)
			}
		case errors.Is(got.err, ErrPhoneAlreadyRegistered):
			phoneConflicts++
			failedCodeID = got.codeID
		default:
			t.Fatalf("unexpected same-phone registration error: %v", got.err)
		}
	}
	if successes != 1 || phoneConflicts != 1 {
		t.Fatalf("same-phone results: successes=%d phone_conflicts=%d, want 1 each", successes, phoneConflicts)
	}
	if !registerSMSCodeUsed(t, database, successfulCodeID) {
		t.Fatal("successful same-phone registration did not consume its SMS code")
	}
	if registerSMSCodeUsed(t, database, failedCodeID) {
		t.Fatal("phone unique-constraint failure consumed its SMS code")
	}
	var users int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_users WHERE phone=$1`, phone).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("same-phone registrations created %d users, want 1", users)
	}
}

func TestAuthenticateWithPasswordIdentifierRules(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       bool
	}{
		{name: "lowest valid prefix", identifier: "13000000000", want: true},
		{name: "highest valid prefix", identifier: "19999999999", want: true},
		{name: "prefix too low", identifier: "12000000000"},
		{name: "too short", identifier: "1380000000"},
		{name: "too long", identifier: "138000000000"},
		{name: "contains non digit", identifier: "1380000000a"},
		{name: "outer whitespace is not part of phone syntax", identifier: " 13800000000 "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPhoneIdentifier(tt.identifier); got != tt.want {
				t.Fatalf("isPhoneIdentifier(%q) = %v, want %v", tt.identifier, got, tt.want)
			}
		})
	}
}

func TestAuthenticateWithPasswordDummyHashIsFixedDefaultCost(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(passwordAuthenticationDummyHash))
	if err != nil {
		t.Fatalf("passwordAuthenticationDummyHash is invalid: %v", err)
	}
	if cost != bcrypt.DefaultCost {
		t.Fatalf("passwordAuthenticationDummyHash cost = %d, want %d", cost, bcrypt.DefaultCost)
	}
}

func TestAuthenticateWithPasswordSelectsOnlyUsableStoredHash(t *testing.T) {
	defaultCostHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("generate DefaultCost hash: %v", err)
	}
	minCostHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate MinCost hash: %v", err)
	}
	highCostHash := string(defaultCostHash[:4]) + "31" + string(defaultCostHash[6:])
	if cost, err := bcrypt.Cost([]byte(highCostHash)); err != nil || cost != 31 {
		t.Fatalf("mechanical high-cost hash cost = %d, error = %v", cost, err)
	}

	tests := []struct {
		name       string
		stored     sql.NullString
		wantUsable bool
	}{
		{name: "SQL NULL"},
		{name: "empty", stored: sql.NullString{String: "", Valid: true}},
		{name: "damaged", stored: sql.NullString{String: "not-a-bcrypt-hash", Valid: true}},
		{name: "MinCost", stored: sql.NullString{String: string(minCostHash), Valid: true}},
		{name: "cost 31", stored: sql.NullString{String: highCostHash, Valid: true}},
		{name: "DefaultCost", stored: sql.NullString{String: string(defaultCostHash), Valid: true}, wantUsable: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHash, gotUsable := passwordHashForAuthentication(tt.stored)
			if gotUsable != tt.wantUsable {
				t.Fatalf("passwordHashForAuthentication() usable = %v, want %v", gotUsable, tt.wantUsable)
			}
			wantHash := passwordAuthenticationDummyHash
			if tt.wantUsable {
				wantHash = tt.stored.String
			}
			if gotHash != wantHash {
				t.Fatalf("passwordHashForAuthentication() hash = %q, want %q", gotHash, wantHash)
			}
		})
	}
}

func TestAuthenticateWithPasswordSucceedsByAccountOrPhone(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}

	tests := []struct {
		name       string
		phone      string
		account    string
		identifier string
	}{
		{
			name:       "lowercase account with outer whitespace",
			phone:      "13800000201",
			account:    "alice_01",
			identifier: "  alice_01  ",
		},
		{
			name:       "account with different casing",
			phone:      "13800000202",
			account:    "Case_User",
			identifier: "cAsE_uSeR",
		},
		{
			name:       "exact phone",
			phone:      "13800000203",
			account:    "phone_user",
			identifier: "13800000203",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var userID int64
			if err := database.QueryRowContext(ctx, `
				INSERT INTO app_users (phone, account, password_hash, nickname, register_source)
				VALUES ($1, $2, $3, $4, 'account_sms')
				RETURNING id
			`, tt.phone, tt.account, string(passwordHash), tt.name).Scan(&userID); err != nil {
				t.Fatalf("insert password user: %v", err)
			}

			user, err := NewStore(database).AuthenticateWithPassword(ctx, tt.identifier, "secret123")
			if err != nil {
				t.Fatalf("AuthenticateWithPassword() error = %v", err)
			}
			if user.ID != userID || user.Phone != tt.phone || user.Account != tt.account {
				t.Fatalf("AuthenticateWithPassword() user = %+v", user)
			}
			if user.LastLoginAt == "" {
				t.Fatalf("AuthenticateWithPassword() did not return last login time: %+v", user)
			}

			var lastLogin sql.NullTime
			if err := database.QueryRowContext(ctx, `SELECT last_login_at FROM app_users WHERE id=$1`, userID).Scan(&lastLogin); err != nil {
				t.Fatalf("read last login time: %v", err)
			}
			if !lastLogin.Valid {
				t.Fatal("AuthenticateWithPassword() did not persist last_login_at")
			}

			payload, err := json.Marshal(user)
			if err != nil {
				t.Fatalf("marshal authenticated user: %v", err)
			}
			if strings.Contains(strings.ToLower(string(payload)), "password") {
				t.Fatalf("authenticated user JSON leaked password material: %s", payload)
			}
		})
	}
}

func TestAuthenticateWithPasswordRejectsInvalidCredentialsBeforeStatus(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	minCostHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash MinCost fixture password: %v", err)
	}

	var activeUserID, disabledUserID, smsOnlyUserID, damagedHashUserID, minCostUserID int64
	if err := database.QueryRowContext(ctx, `
		INSERT INTO app_users (phone, account, password_hash, nickname)
		VALUES ('13800000211', 'active_user', $1, 'Active')
		RETURNING id
	`, string(passwordHash)).Scan(&activeUserID); err != nil {
		t.Fatalf("insert active password user: %v", err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO app_users (phone, account, password_hash, nickname, status)
		VALUES ('13800000212', 'disabled_user', $1, 'Disabled', 'disabled')
		RETURNING id
	`, string(passwordHash)).Scan(&disabledUserID); err != nil {
		t.Fatalf("insert disabled password user: %v", err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO app_users (phone, nickname, register_source)
		VALUES ('13800000213', 'SMS only', 'sms')
		RETURNING id
	`).Scan(&smsOnlyUserID); err != nil {
		t.Fatalf("insert SMS-only user: %v", err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO app_users (phone, account, password_hash, nickname)
		VALUES ('13800000214', 'damaged_hash_user', 'not-a-bcrypt-hash', 'Damaged hash')
		RETURNING id
	`).Scan(&damagedHashUserID); err != nil {
		t.Fatalf("insert damaged-hash user: %v", err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO app_users (phone, account, password_hash, nickname)
		VALUES ('13800000215', 'min_cost_user', $1, 'MinCost hash')
		RETURNING id
	`, string(minCostHash)).Scan(&minCostUserID); err != nil {
		t.Fatalf("insert MinCost-hash user: %v", err)
	}

	tests := []struct {
		name       string
		identifier string
		password   string
		want       error
		userID     int64
	}{
		{
			name:       "wrong password",
			identifier: "active_user",
			password:   "wrong-password",
			want:       ErrInvalidCredentials,
			userID:     activeUserID,
		},
		{
			name:       "password is never trimmed",
			identifier: "active_user",
			password:   " secret123 ",
			want:       ErrInvalidCredentials,
			userID:     activeUserID,
		},
		{
			name:       "unknown identifier",
			identifier: "missing_user",
			password:   "secret123",
			want:       ErrInvalidCredentials,
		},
		{
			name:       "SMS-only user has no password",
			identifier: "13800000213",
			password:   "secret123",
			want:       ErrInvalidCredentials,
			userID:     smsOnlyUserID,
		},
		{
			name:       "damaged stored hash",
			identifier: "damaged_hash_user",
			password:   "secret123",
			want:       ErrInvalidCredentials,
			userID:     damagedHashUserID,
		},
		{
			name:       "MinCost stored hash with correct password",
			identifier: "min_cost_user",
			password:   "secret123",
			want:       ErrInvalidCredentials,
			userID:     minCostUserID,
		},
		{
			name:       "disabled user with wrong password hides status",
			identifier: "disabled_user",
			password:   "wrong-password",
			want:       ErrInvalidCredentials,
			userID:     disabledUserID,
		},
		{
			name:       "disabled user with correct password returns disabled",
			identifier: "disabled_user",
			password:   "secret123",
			want:       ErrUserDisabled,
			userID:     disabledUserID,
		},
	}

	store := NewStore(database)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.AuthenticateWithPassword(ctx, tt.identifier, tt.password)
			if !errors.Is(err, tt.want) {
				t.Fatalf("AuthenticateWithPassword() error = %v, want %v", err, tt.want)
			}
			if tt.userID == 0 {
				return
			}
			var lastLogin sql.NullTime
			if err := database.QueryRowContext(ctx, `SELECT last_login_at FROM app_users WHERE id=$1`, tt.userID).Scan(&lastLogin); err != nil {
				t.Fatalf("read failed-login time: %v", err)
			}
			if lastLogin.Valid {
				t.Fatalf("failed authentication updated last_login_at to %v", lastLogin.Time)
			}
		})
	}
}

func TestAuthenticateWithPasswordRejectsHighCostHashWithoutComparingIt(t *testing.T) {
	database := openRegisterWithPasswordFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	defaultCostHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	highCostHash := string(defaultCostHash[:4]) + "31" + string(defaultCostHash[6:])
	if cost, err := bcrypt.Cost([]byte(highCostHash)); err != nil || cost != 31 {
		t.Fatalf("mechanical high-cost hash cost = %d, error = %v", cost, err)
	}

	var userID int64
	if err := database.QueryRowContext(ctx, `
		INSERT INTO app_users (phone, account, password_hash, nickname)
		VALUES ('13800000216', 'high_cost_user', $1, 'High cost hash')
		RETURNING id
	`, highCostHash).Scan(&userID); err != nil {
		t.Fatalf("insert high-cost-hash user: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := NewStore(database).AuthenticateWithPassword(ctx, "high_cost_user", "secret123")
		result <- err
	}()

	select {
	case err := <-result:
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("AuthenticateWithPassword() error = %v, want ErrInvalidCredentials", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AuthenticateWithPassword() appears to be comparing the cost-31 stored hash")
	}

	var lastLogin sql.NullTime
	if err := database.QueryRowContext(ctx, `SELECT last_login_at FROM app_users WHERE id=$1`, userID).Scan(&lastLogin); err != nil {
		t.Fatalf("read high-cost failed-login time: %v", err)
	}
	if lastLogin.Valid {
		t.Fatalf("high-cost hash authentication updated last_login_at to %v", lastLogin.Time)
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

func waitForBlockedSMSCodeRegistrations(ctx context.Context, database *sql.DB, want int, timeout time.Duration) error {
	return waitForPostgresLockCount(ctx, database, want, timeout, `
		SELECT count(DISTINCT activity.pid)
		FROM pg_stat_activity activity
		JOIN pg_locks relation_lock ON relation_lock.pid=activity.pid
		WHERE activity.wait_event_type='Lock'
		  AND relation_lock.locktype='relation'
		  AND relation_lock.relation=to_regclass('app_sms_codes')
		  AND relation_lock.mode='RowShareLock'
		  AND relation_lock.granted
	`)
}

func waitForBlockedAppUserInserts(ctx context.Context, database *sql.DB, want int, timeout time.Duration) error {
	return waitForPostgresLockCount(ctx, database, want, timeout, `
		SELECT count(*)
		FROM pg_locks
		WHERE locktype='relation'
		  AND relation=to_regclass('app_users')
		  AND mode='RowExclusiveLock'
		  AND granted=false
	`)
}

func waitForPostgresLockCount(ctx context.Context, database *sql.DB, want int, timeout time.Duration, query string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	lastCount := 0
	for {
		if err := database.QueryRowContext(ctx, query).Scan(&lastCount); err != nil {
			return fmt.Errorf("inspect PostgreSQL lock waiters: %w", err)
		}
		if lastCount >= want {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("PostgreSQL lock waiters=%d, want at least %d", lastCount, want)
		case <-ticker.C:
		}
	}
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
