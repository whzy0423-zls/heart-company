package appuser

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidAccount         = errors.New("invalid account")
	ErrInvalidPassword        = errors.New("invalid password")
	ErrInvalidNickname        = errors.New("invalid nickname")
	ErrInvalidSMSCode         = errors.New("invalid sms code")
	ErrAccountTaken           = errors.New("account already exists")
	ErrPhoneAlreadyRegistered = errors.New("phone already registered")
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrUserDisabled           = errors.New("user disabled")
)

var accountPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,31}$`)

var phoneIdentifierPattern = regexp.MustCompile(`^1[3-9][0-9]{9}$`)

const passwordAuthenticationDummyHash = "$2b$10$sQ/LjuVpMbqmFYb/Ukb0.ebUav7SmTzniaVsy0sNW/ZmwD5HU.hPq"

type RegisterWithPasswordInput struct {
	Nickname    string
	Account     string
	Password    string
	Phone       string
	SMSCodeHash string
}

func NormalizeAccount(raw string) string {
	normalized := []byte(strings.TrimSpace(raw))
	for i, char := range normalized {
		if char >= 'A' && char <= 'Z' {
			normalized[i] = char + ('a' - 'A')
		}
	}
	return string(normalized)
}

func ValidateAccount(raw string) error {
	if !accountPattern.MatchString(strings.TrimSpace(raw)) {
		return ErrInvalidAccount
	}
	return nil
}

func ValidatePassword(raw string) error {
	if len(raw) < 6 || len(raw) > 72 {
		return ErrInvalidPassword
	}
	return nil
}

func NormalizeNickname(raw string) string {
	return strings.TrimSpace(raw)
}

func ValidateNickname(raw string) error {
	if !utf8.ValidString(raw) {
		return ErrInvalidNickname
	}
	length := utf8.RuneCountInString(NormalizeNickname(raw))
	if length < 1 || length > 32 {
		return ErrInvalidNickname
	}
	return nil
}

func isPhoneIdentifier(identifier string) bool {
	return phoneIdentifierPattern.MatchString(identifier)
}

func passwordHashForAuthentication(stored sql.NullString) (string, bool) {
	if !stored.Valid || stored.String == "" {
		return passwordAuthenticationDummyHash, false
	}
	cost, err := bcrypt.Cost([]byte(stored.String))
	if err != nil || cost != bcrypt.DefaultCost {
		return passwordAuthenticationDummyHash, false
	}
	return stored.String, true
}

func (s *Store) AuthenticateWithPassword(ctx context.Context, identifier, password string) (User, error) {
	identifier = strings.TrimSpace(identifier)

	var (
		userID       int64
		passwordHash sql.NullString
	)
	var err error
	if isPhoneIdentifier(identifier) {
		err = s.db.QueryRowContext(ctx, `
			SELECT id, password_hash
			FROM app_users
			WHERE phone=$1
		`, identifier).Scan(&userID, &passwordHash)
	} else {
		err = s.db.QueryRowContext(ctx, `
			SELECT id, password_hash
			FROM app_users
			WHERE account IS NOT NULL
			  AND btrim(account) <> ''
			  AND lower(account)=lower($1)
		`, identifier).Scan(&userID, &passwordHash)
	}

	found := true
	if errors.Is(err, sql.ErrNoRows) {
		found = false
	} else if err != nil {
		return User{}, fmt.Errorf("find appuser credentials: %w", err)
	}

	hash, hasPassword := passwordHashForAuthentication(passwordHash)
	passwordMatches := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	if !found || !hasPassword || !passwordMatches {
		return User{}, ErrInvalidCredentials
	}

	var user User
	var lastLogin, memberStartedAt, memberExpiresAt sql.NullTime
	var createTime, updateTime time.Time
	err = s.db.QueryRowContext(ctx, `
		UPDATE app_users
		SET last_login_at=now(), update_time=now()
		WHERE id=$1 AND status='active'
		RETURNING id, phone, COALESCE(account, ''), nickname, avatar, status, member_level,
		          member_started_at, member_expires_at, register_source, last_login_at,
		          create_time, update_time
	`, userID).Scan(
		&user.ID,
		&user.Phone,
		&user.Account,
		&user.Nickname,
		&user.Avatar,
		&user.Status,
		&user.MemberLevel,
		&memberStartedAt,
		&memberExpiresAt,
		&user.RegisterSource,
		&lastLogin,
		&createTime,
		&updateTime,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserDisabled
	}
	if err != nil {
		return User{}, fmt.Errorf("update appuser password login: %w", err)
	}

	if lastLogin.Valid {
		user.LastLoginAt = formatTime(lastLogin.Time)
	}
	user.CreateTime = formatTime(createTime)
	user.UpdateTime = formatTime(updateTime)
	applyMembershipTimes(&user, memberStartedAt, memberExpiresAt)
	return user, nil
}

func (s *Store) RegisterWithPassword(ctx context.Context, in RegisterWithPasswordInput) (User, error) {
	if err := ValidateAccount(in.Account); err != nil {
		return User{}, err
	}
	if err := ValidatePassword(in.Password); err != nil {
		return User{}, err
	}
	if err := ValidateNickname(in.Nickname); err != nil {
		return User{}, err
	}

	account := NormalizeAccount(in.Account)
	nickname := NormalizeNickname(in.Nickname)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash appuser password: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin appuser registration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var smsCodeID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM app_sms_codes
		WHERE phone=$1
		  AND code_hash=$2
		  AND used=false
		  AND expires_at > now()
		ORDER BY create_time DESC, id DESC
		LIMIT 1
		FOR UPDATE
	`, in.Phone, in.SMSCodeHash).Scan(&smsCodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidSMSCode
	}
	if err != nil {
		return User{}, fmt.Errorf("lock appuser registration SMS code: %w", err)
	}

	var (
		existingUserID       int64
		existingAccount      sql.NullString
		existingPasswordHash sql.NullString
		existingStatus       string
	)
	existingUser := true
	err = tx.QueryRowContext(ctx, `
		SELECT id, account, password_hash, status
		FROM app_users
		WHERE phone=$1
		FOR UPDATE
	`, in.Phone).Scan(&existingUserID, &existingAccount, &existingPasswordHash, &existingStatus)
	if errors.Is(err, sql.ErrNoRows) {
		existingUser = false
	} else if err != nil {
		return User{}, fmt.Errorf("lock appuser phone: %w", err)
	}
	if existingUser && existingStatus != "active" {
		return User{}, ErrUserDisabled
	}
	if existingUser && (strings.TrimSpace(existingAccount.String) != "" || strings.TrimSpace(existingPasswordHash.String) != "") {
		return User{}, ErrPhoneAlreadyRegistered
	}

	var accountOwnerID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM app_users
		WHERE account IS NOT NULL
		  AND btrim(account) <> ''
		  AND lower(account)=lower($1)
		ORDER BY id
		LIMIT 1
	`, account).Scan(&accountOwnerID)
	if err == nil && (!existingUser || accountOwnerID != existingUserID) {
		return User{}, ErrAccountTaken
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("check appuser account ownership: %w", err)
	}

	var userID int64
	if existingUser {
		err = tx.QueryRowContext(ctx, `
			UPDATE app_users
			SET account=$1,
			    password_hash=$2,
			    nickname=$3,
			    update_time=now()
			WHERE id=$4
			RETURNING id
		`, account, string(passwordHash), nickname, existingUserID).Scan(&userID)
	} else {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO app_users (phone, account, password_hash, nickname, register_source)
			VALUES ($1, $2, $3, $4, 'account_sms')
			RETURNING id
		`, in.Phone, account, string(passwordHash), nickname).Scan(&userID)
	}
	if err != nil {
		return User{}, fmt.Errorf("write appuser registration: %w", mapRegisterWithPasswordError(err))
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE app_sms_codes
		SET used=true
		WHERE id=$1 AND used=false
	`, smsCodeID)
	if err != nil {
		return User{}, fmt.Errorf("use appuser registration SMS code: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return User{}, fmt.Errorf("read appuser registration SMS update result: %w", err)
	}
	if rowsAffected != 1 {
		return User{}, ErrInvalidSMSCode
	}

	user, err := readRegisterWithPasswordUser(ctx, tx, userID)
	if err != nil {
		return User{}, fmt.Errorf("read registered appuser: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("commit appuser registration: %w", err)
	}
	return user, nil
}

func readRegisterWithPasswordUser(ctx context.Context, tx *sql.Tx, userID int64) (User, error) {
	var user User
	var lastLogin, memberStartedAt, memberExpiresAt sql.NullTime
	var createTime, updateTime time.Time
	err := tx.QueryRowContext(ctx, `
		SELECT id, phone, COALESCE(account, ''), nickname, avatar, status, member_level,
		       member_started_at, member_expires_at, register_source, last_login_at,
		       create_time, update_time
		FROM app_users
		WHERE id=$1
	`, userID).Scan(
		&user.ID,
		&user.Phone,
		&user.Account,
		&user.Nickname,
		&user.Avatar,
		&user.Status,
		&user.MemberLevel,
		&memberStartedAt,
		&memberExpiresAt,
		&user.RegisterSource,
		&lastLogin,
		&createTime,
		&updateTime,
	)
	if err != nil {
		return User{}, err
	}
	if lastLogin.Valid {
		user.LastLoginAt = formatTime(lastLogin.Time)
	}
	user.CreateTime = formatTime(createTime)
	user.UpdateTime = formatTime(updateTime)
	applyMembershipTimes(&user, memberStartedAt, memberExpiresAt)
	return user, nil
}

func mapRegisterWithPasswordError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch pgErr.ConstraintName {
	case "idx_app_users_account_unique":
		return ErrAccountTaken
	case "app_users_phone_key":
		return ErrPhoneAlreadyRegistered
	default:
		return err
	}
}
