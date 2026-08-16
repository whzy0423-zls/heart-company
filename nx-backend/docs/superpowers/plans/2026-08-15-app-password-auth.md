# App Password Authentication Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add App registration with SMS verification and password credentials, plus username-or-phone password login, while preserving existing SMS users and token behavior.

**Architecture:** Extend `app_users` with nullable credentials for backward compatibility. Put credential validation and transactional persistence in a focused `appuser/credentials.go`, expose thin HTTP handlers in `server/app_password_auth.go`, and reuse one App session issuer across SMS login, registration, and password login.

**Tech Stack:** Go 1.22, PostgreSQL, `database/sql`, pgx, bcrypt, `net/http`, Go testing

---

## Chunk 1: Schema and App user model

### Task 1: Specify and add the credential schema migration

**Files:**
- Create: `apps/server/internal/db/schema_app_password_auth_test.go`
- Modify: `apps/server/internal/db/schema.sql`

- [ ] **Step 1: Write the failing schema contract test**

Add `TestSchemaDefinesAppPasswordCredentials` and require all of these fragments after the `app_users` table definition:

```go
required := []string{
	"account         TEXT",
	"password_hash   TEXT",
	"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS account TEXT",
	"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS password_hash TEXT",
	"CREATE UNIQUE INDEX IF NOT EXISTS idx_app_users_account_unique",
	"ON app_users (lower(account))",
	"WHERE account IS NOT NULL AND btrim(account) <> ''",
}
```

Also assert that both `ALTER TABLE` statements and the index occur after `CREATE TABLE IF NOT EXISTS app_users` so a fresh database can execute the complete schema.

- [ ] **Step 2: Run the focused test to verify RED**

Run from `apps/server`:

```bash
go test ./internal/db -run AppPassword -count=1
```

Expected: FAIL because the credential columns and unique index are absent.

- [ ] **Step 3: Add the idempotent migration**

Update the fresh-table definition:

```sql
CREATE TABLE IF NOT EXISTS app_users (
  id              BIGSERIAL PRIMARY KEY,
  phone           TEXT NOT NULL UNIQUE,
  account         TEXT,
  password_hash   TEXT,
  nickname        TEXT NOT NULL DEFAULT '',
  ...
);
```

Immediately after the table definition, add:

```sql
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS account TEXT;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS password_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_users_account_unique
  ON app_users (lower(account))
  WHERE account IS NOT NULL AND btrim(account) <> '';
```

- [ ] **Step 4: Run the focused test to verify GREEN**

Run:

```bash
go test ./internal/db -run AppPassword -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the schema change**

```bash
git add apps/server/internal/db/schema.sql apps/server/internal/db/schema_app_password_auth_test.go
git commit -m "feat(db): add app password credentials"
```

### Task 2: Expose account safely in App user reads and privacy flows

**Files:**
- Modify: `apps/server/internal/appuser/store.go`
- Modify: `apps/server/internal/appuser/store_update_test.go`
- Modify: `apps/server/internal/appuser/insights_test.go`
- Modify: `apps/server/internal/server/app_privacy.go`
- Modify: `apps/server/internal/server/app_privacy_test.go`

- [ ] **Step 1: Write failing App user model and privacy tests**

Add assertions that:

- `appuser.User` JSON contains `account` but never `password_hash`.
- App user list keyword search includes `account` alongside phone and nickname.
- Account deletion sets `account = NULL` and `password_hash = NULL`.
- Privacy export returns the account field without exposing a password hash.

Update SQL-driver column fixtures in `store_update_test.go` and `insights_test.go` to include `account` in the same position used by production queries.

- [ ] **Step 2: Run focused tests to verify RED**

Run:

```bash
go test ./internal/appuser ./internal/server -run 'AccountField|AccountSearch|PrivacyDeleteAccount|PrivacyExport' -count=1
```

Expected: FAIL because `User` and the privacy deletion query do not handle credentials.

- [ ] **Step 3: Add the account field and update user projections**

Extend `User`:

```go
type User struct {
	ID      int64  `json:"id"`
	Phone   string `json:"phone"`
	Account string `json:"account,omitempty"`
	// Existing public fields remain unchanged. PasswordHash is deliberately absent.
}
```

Add `COALESCE(account, '')` to every App user `SELECT`, `RETURNING`, list row, and insight row that populates `User` or `UserInsight`; old SMS-only rows are allowed to keep SQL `NULL`, while Go receives an empty string. Update each corresponding `Scan`. Add `Account` to `UserInsight` only if the admin response exposes the same account field. Extend keyword matching to:

```sql
lower(account) LIKE lower($N)
OR lower(phone) LIKE lower($N)
OR lower(nickname) LIKE lower($N)
```

- [ ] **Step 4: Clear credentials during account deletion**

Extend the existing `UPDATE app_users` in `appPrivacyDeleteAccount`:

```sql
SET phone = 'deleted-' || ...,
    account = NULL,
    password_hash = NULL,
    nickname = '',
    avatar = '',
    status = 'disabled',
    member_level = 'free',
    update_time = now()
```

- [ ] **Step 5: Run focused tests to verify GREEN**

Run:

```bash
go test ./internal/appuser ./internal/server -run 'AccountField|AccountSearch|PrivacyDeleteAccount|PrivacyExport' -count=1
```

Expected: PASS or PostgreSQL-backed cases SKIP only when `TEST_DATABASE_URL` is absent.

- [ ] **Step 6: Commit the public model and privacy change**

```bash
git add apps/server/internal/appuser/store.go apps/server/internal/appuser/store_update_test.go apps/server/internal/appuser/insights_test.go apps/server/internal/server/app_privacy.go apps/server/internal/server/app_privacy_test.go
git commit -m "feat(appuser): expose and anonymize login account"
```

---

## Chunk 2: Transactional credential store

### Task 3: Add credential validation and typed domain errors

**Files:**
- Create: `apps/server/internal/appuser/credentials.go`
- Create: `apps/server/internal/appuser/credentials_test.go`

- [ ] **Step 1: Write failing table-driven validation tests**

Cover:

- Valid accounts such as `xinzhili_01` and case normalization to `xinzhili_01`.
- Rejection of accounts shorter than 4 or longer than 32 characters.
- Rejection of an account that starts with a number, contains spaces, punctuation, or non-ASCII characters.
- Password length of 6 through 72 bytes; reject shorter and longer values.
- Nickname length of 1 through 32 Unicode characters.

Expected API:

```go
func NormalizeAccount(raw string) string
func ValidateAccount(raw string) error
func ValidatePassword(raw string) error
func ValidateNickname(raw string) error
```

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/appuser -run 'AccountValidation|PasswordValidation|NicknameValidation' -count=1
```

Expected: FAIL because the validation functions do not exist.

- [ ] **Step 3: Implement minimal validation and errors**

Define sentinel errors for handler mapping:

```go
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
```

Use a compiled expression equivalent to `^[A-Za-z][A-Za-z0-9_]{3,31}$`, lowercase normalized storage, byte length for passwords, and `utf8.RuneCountInString` for nicknames.

- [ ] **Step 4: Run tests to verify GREEN**

Run:

```bash
go test ./internal/appuser -run 'AccountValidation|PasswordValidation|NicknameValidation' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit validation**

```bash
git add apps/server/internal/appuser/credentials.go apps/server/internal/appuser/credentials_test.go
git commit -m "feat(appuser): validate password credentials"
```

### Task 4: Implement atomic registration and legacy-user binding

**Files:**
- Modify: `apps/server/internal/appuser/credentials.go`
- Modify: `apps/server/internal/appuser/credentials_test.go`

- [ ] **Step 1: Write failing PostgreSQL registration tests**

Use `testdb.OpenEnvIsolatedSchema(t, "app_password_register")` and create a focused test fixture containing `app_users`, `app_sms_codes`, and the case-insensitive account index. Keep the fixture columns aligned with the production projection, but do not depend on the unexported `db.schemaSQL`. The schema contract remains covered separately in Task 1. Cover:

- New phone creates one user with normalized account, bcrypt hash, nickname, and `register_source='account_sms'`.
- An SMS-only user created by `FindOrCreateByPhone` is bound in place; ID and existing related data remain unchanged.
- Existing credentialed phone returns `ErrPhoneAlreadyRegistered`.
- Case-insensitive duplicate account returns `ErrAccountTaken`.
- Invalid, expired, or already-used code returns `ErrInvalidSMSCode`.
- When account uniqueness fails, the SMS code remains unused and succeeds with a different account, proving transaction rollback.
- Two concurrent attempts with the same code yield exactly one success.

Define the input shape:

```go
type RegisterWithPasswordInput struct {
	Nickname     string
	Account      string
	Password     string
	Phone        string
	SMSCodeHash  string
}
```

- [ ] **Step 2: Run registration tests to verify RED**

Run:

```bash
go test ./internal/appuser -run 'RegisterWithPassword' -count=1
```

Expected: FAIL because registration persistence is absent, or SKIP only when the guarded test database is unavailable.

- [ ] **Step 3: Implement `RegisterWithPassword`**

Hash the password with `bcrypt.DefaultCost` before opening the transaction. Inside one transaction:

1. Select the newest matching unused, unexpired SMS code row `FOR UPDATE`.
2. Select the phone user `FOR UPDATE`.
3. Insert a new user or update an SMS-only user.
4. Convert PostgreSQL unique-violation `23505` for `idx_app_users_account_unique` into `ErrAccountTaken`.
5. Mark the selected SMS code used.
6. Read the public user row and commit.

Do not call the existing non-transactional `VerifyAndUseSMSCode`; keep it for SMS login.

- [ ] **Step 4: Run registration tests to verify GREEN**

Run:

```bash
go test ./internal/appuser -run 'RegisterWithPassword' -count=1
```

Expected: PASS with `TEST_DATABASE_URL`; otherwise guarded integration tests SKIP and validation unit tests remain PASS.

- [ ] **Step 5: Commit transactional registration**

```bash
git add apps/server/internal/appuser/credentials.go apps/server/internal/appuser/credentials_test.go
git commit -m "feat(appuser): register password users atomically"
```

### Task 5: Implement constant-work username-or-phone authentication

**Files:**
- Modify: `apps/server/internal/appuser/credentials.go`
- Modify: `apps/server/internal/appuser/credentials_test.go`

- [ ] **Step 1: Write failing authentication tests**

Cover successful login by lowercase username, differently-cased username, and exact phone. Cover a wrong password, unknown identifier, SMS-only user without a password, and a disabled user whose correct password returns `ErrUserDisabled`.

Assert successful authentication updates `last_login_at` and never returns a password hash.

- [ ] **Step 2: Run authentication tests to verify RED**

Run:

```bash
go test ./internal/appuser -run 'AuthenticateWithPassword' -count=1
```

Expected: FAIL because authentication persistence is absent.

- [ ] **Step 3: Implement `AuthenticateWithPassword`**

Use this public signature:

```go
func (s *Store) AuthenticateWithPassword(ctx context.Context, identifier, password string) (User, error)
```

Add a package-local `isPhoneIdentifier` helper using the same 11-digit mainland-number rules as the HTTP layer. If `identifier` matches it, query `phone = $1`; otherwise query `lower(account) = lower($1)`. Compare unknown users and users without a password against a fixed valid bcrypt dummy hash before returning `ErrInvalidCredentials`, reducing identifier timing differences. Only return `ErrUserDisabled` after the supplied password is correct. Update `last_login_at` after successful verification.

- [ ] **Step 4: Run authentication tests to verify GREEN**

Run:

```bash
go test ./internal/appuser -run 'AuthenticateWithPassword' -count=1
```

Expected: PASS with PostgreSQL integration available.

- [ ] **Step 5: Commit password authentication**

```bash
git add apps/server/internal/appuser/credentials.go apps/server/internal/appuser/credentials_test.go
git commit -m "feat(appuser): authenticate by account or phone"
```

---

## Chunk 3: HTTP routes, sessions, limits, and contract verification

### Task 6: Add registration and password-login HTTP handlers

**Files:**
- Create: `apps/server/internal/server/app_password_auth.go`
- Modify: `apps/server/internal/server/app_auth.go`
- Modify: `apps/server/internal/server/server.go`
- Modify: `apps/server/internal/server/app_auth_unit_test.go`

- [ ] **Step 1: Write failing route and validation tests**

Extend `TestAppAuthCompatibilityAliasRoutes` so these routes reach handlers rather than returning 404:

```text
POST /api/app/auth/register
POST /api/app/auth/login
```

Send malformed JSON and expect `400`. Add table-driven handler validation tests for invalid account, password, nickname, phone, and non-6-digit code without requiring a database.

- [ ] **Step 2: Run route tests to verify RED**

Run:

```bash
go test ./internal/server -run 'AppPassword|AppAuthCompatibilityAliasRoutes' -count=1
```

Expected: FAIL because the two routes and handlers are absent.

- [ ] **Step 3: Register the routes and add request handlers**

Register:

```go
s.mux.HandleFunc("/api/app/auth/register", s.method(http.MethodPost, s.appRegisterWithPassword))
s.mux.HandleFunc("/api/app/auth/login", s.method(http.MethodPost, s.appLoginWithPassword))
```

`appRegisterWithPassword` must:

- Limit request body size.
- Decode the current Flutter request fields.
- Validate all inputs before touching the store.
- Apply the existing phone/IP SMS verification limit.
- Hash the supplied code with `appuser.HashToken` and call `RegisterWithPassword`.
- Map typed domain errors to `400`, `401`, `403`, or `409` with Chinese user-facing messages.

`appLoginWithPassword` must validate non-empty identifier/password, apply password-login limits, call `AuthenticateWithPassword`, and use the same generic `401 账号或密码错误` for unknown account and wrong password.

- [ ] **Step 4: Extract one App session issuer**

Replace the duplicated access/refresh logic in `appVerifySMS` with:

```go
func (s *Server) writeAppSession(w http.ResponseWriter, r *http.Request, user appuser.User, deviceInfo string) {
	// Issue 15-minute access token.
	// Generate and persist 30-day refresh token.
	// Respond with accessToken, refreshToken, and user.
}
```

Call it from SMS login, registration, and password login. Preserve the existing response shape expected by Flutter `_completeLogin`.

- [ ] **Step 5: Run route tests to verify GREEN**

Run:

```bash
go test ./internal/server -run 'AppPassword|AppAuthCompatibilityAliasRoutes|AppAuthVerifySMS' -count=1
```

Expected: PASS, with PostgreSQL-backed external-package cases SKIP only when the test database is absent.

- [ ] **Step 6: Commit handlers and session reuse**

```bash
git add apps/server/internal/server/app_password_auth.go apps/server/internal/server/app_auth.go apps/server/internal/server/server.go apps/server/internal/server/app_auth_unit_test.go
git commit -m "feat(server): add app password auth routes"
```

### Task 7: Add independent App password-login rate limits

**Files:**
- Modify: `apps/server/internal/server/server.go`
- Modify: `apps/server/internal/server/app_password_auth.go`
- Modify: `apps/server/internal/server/app_auth_unit_test.go`
- Modify: `apps/server/internal/server/server_test.go`

- [ ] **Step 1: Write failing account and IP limit tests**

Add tests proving:

- Five attempts per normalized identifier are allowed in one minute and the sixth is denied.
- Thirty attempts per IP across changing identifiers are allowed and the next is denied.
- Nil limiters in route-only tests fail open, matching existing test conventions.
- App password scopes do not consume the `admin_login` limit.
- Database keys do not expose normalized accounts, phone numbers, or IP addresses.
- Recording a new attempt removes expired rows in the same scope.

- [ ] **Step 2: Run limit tests to verify RED**

Run:

```bash
go test ./internal/server -run 'AppPasswordLoginAttemptLimiter' -count=1
```

Expected: FAIL because App password limiters do not exist.

- [ ] **Step 3: Add dedicated in-memory and DB-backed limiters**

Add Server fields for account and IP in-memory limiters and separate DB limiters. Initialize them with distinct scopes such as `app_password_account` and `app_password_ip`. Keep normalized values only in memory; derive versioned HMAC-SHA256 database keys from `JWT_SECRET`, the limiter dimension, and the normalized value. Before each upsert, delete expired rows in that scope while preserving the current-key reset path.

Update `newTestServer` cleanup to delete only these test scopes in addition to `admin_login`.

Extend account deletion so its existing transaction removes both current digest keys and legacy plaintext account/phone limiter keys before anonymizing the user.

- [ ] **Step 4: Run limit tests to verify GREEN**

Run:

```bash
go test ./internal/server -run 'AppPasswordLoginAttemptLimiter' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit rate limiting**

```bash
git add apps/server/internal/server/server.go apps/server/internal/server/app_password_auth.go apps/server/internal/server/app_auth_unit_test.go apps/server/internal/server/server_test.go
git commit -m "feat(server): rate limit app password login"
```

### Task 8: Prove the complete App contract with PostgreSQL

**Files:**
- Modify: `apps/server/internal/server/app_auth_test.go`
- Modify: `apps/server/internal/server/app_privacy_test.go`

- [ ] **Step 1: Write end-to-end registration and login tests**

Using `newTestServer`, add `TestAppPasswordRegistrationAndLogin` covering:

1. Send SMS and read the development code.
2. Register with nickname/account/password/phone/code.
3. Assert access token, refresh token, user ID, phone, account, and nickname.
4. Login with differently-cased username.
5. Login with phone.
6. Reject a wrong password with the same generic response as an unknown identifier.
7. Refresh a password-login session successfully.

Add `TestAppPasswordRegistrationBindsLegacySMSUser` to prove an SMS-created user keeps the same ID after binding credentials. Add a duplicate-account rollback test that retries the same code with a free account and succeeds.

- [ ] **Step 2: Extend account-deletion integration coverage**

After deleting a credentialed account, query the test database and assert `account IS NULL`, `password_hash IS NULL`, status is disabled, phone is anonymized, and refresh tokens are revoked.

- [ ] **Step 3: Run integration tests to verify behavior**

Run with the guarded isolated PostgreSQL DSN:

```bash
TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/server -run 'AppPassword|AppPrivacyDeleteAccount' -count=1
```

Expected: PASS. If `TEST_DATABASE_URL` is not configured, record the tests as SKIPPED and perform the runtime probe in Task 9.

- [ ] **Step 4: Commit HTTP integration coverage**

```bash
git add apps/server/internal/server/app_auth_test.go apps/server/internal/server/app_privacy_test.go
git commit -m "test(server): cover app password auth contract"
```

### Task 9: Full verification and Flutter contract probe

**Files:**
- No additional production files expected

- [ ] **Step 1: Format and run focused suites**

Run:

```bash
gofmt -w internal/appuser/credentials.go internal/appuser/credentials_test.go internal/server/app_password_auth.go internal/server/app_auth.go internal/server/app_auth_unit_test.go internal/server/app_auth_test.go internal/server/app_privacy.go internal/server/app_privacy_test.go internal/server/server.go internal/server/server_test.go internal/db/schema_app_password_auth_test.go
go test ./internal/db ./internal/appuser ./internal/server -count=1
```

Expected: PASS, with explicitly guarded PostgreSQL tests skipped only when no test DSN is configured.

- [ ] **Step 2: Run the full server suite**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 3: Start the local backend and probe the real routes**

Start the server on an unused local port with the existing development database and `APP_ENV=dev`. Verify:

```text
POST /api/app/auth/send-sms -> 200 with devCode
POST /api/app/auth/register -> 200 with accessToken, refreshToken, user
POST /api/app/auth/login using username -> 200
POST /api/app/auth/login using phone -> 200
```

Do not print the real password, SMS code, refresh token, or full phone number in committed logs.

- [ ] **Step 4: Point the current Flutter branch at the local backend**

Use the existing `API_BASE=http://<development-host>:<port>/api/app` dart define, launch the current `feature/xinzhili-ui-theme-v2` App, and verify its unchanged registration and account-password login form completes successfully.

- [ ] **Step 5: Record final status**

Run:

```bash
git status --short --branch
git log --oneline --decorate -10
```

Expected: clean `codex/app-password-auth` branch with all planned commits and no unrelated changes.

### Task 10: Add phone password recovery

**Files:**
- Modify: `apps/server/internal/db/schema.sql`
- Modify: `apps/server/internal/db/schema_app_password_auth_test.go`
- Modify: `apps/server/internal/appuser/credentials.go`
- Create: `apps/server/internal/appuser/password_reset_store_test.go`
- Modify: `apps/server/internal/server/app_auth.go`
- Modify: `apps/server/internal/server/app_password_auth.go`
- Modify: `apps/server/internal/server/server.go`
- Modify: `apps/server/internal/server/app_auth_unit_test.go`
- Modify: `apps/server/internal/server/app_auth_test.go`

- [ ] **Step 1: Verify RED for schema and Store contracts**

Require a dedicated reset-code table and missing `StorePasswordResetCodeIfEligible` / `ResetPassword` methods.

- [ ] **Step 2: Implement transactional reset storage**

Store codes only for active password users. Consume the latest valid reset code, update the bcrypt hash, and revoke all refresh tokens in one transaction.

- [ ] **Step 3: Verify RED for HTTP route and validation**

Require `purpose=password_reset` handling and `POST /api/app/auth/reset-password`.

- [ ] **Step 4: Implement non-enumerating delivery and reset handlers**

Unknown or ineligible phones receive the same successful send response without a code. Reset failures use one generic authentication error.

- [ ] **Step 5: Run PostgreSQL recovery contract**

```bash
TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/appuser -run 'PasswordReset|ResetPassword' -count=1
TEST_DATABASE_URL="$TEST_DATABASE_URL" go test ./internal/server -run '^TestAppPasswordRecovery$' -count=1
```

Expected: both suites PASS; old password and refresh tokens stop working, new password works, and the reset code cannot be reused.
