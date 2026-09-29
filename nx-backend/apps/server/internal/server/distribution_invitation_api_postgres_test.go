package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
)

const distributionInviteFixturePassword = "local-fixture-password"

type distributionInviteHTTPFixture struct {
	db  *sql.DB
	s   *Server
	web *httptest.Server
}

// Uses actual HTTP and production auth/permission middleware, with a focused
// route set so no SMS, payment, chat or background workers are started.
func newDistributionInviteHTTPFixture(t *testing.T) *distributionInviteHTTPFixture {
	t.Helper()
	database := openAppOrderLifecycleTestDatabase(t)
	_, err := database.Exec(`INSERT INTO app_users(id,phone,nickname) VALUES
 (100,'19900000100','fixture-root-a'),(200,'19900000200','fixture-root-b'),
 (300,'19900000300','fixture-paused'),(110,'19900000110','fixture-child-a'),
 (400,'19900000400','fixture-user-a'),(410,'19900000410','fixture-user-b'),
 (420,'19900000420','fixture-child-candidate'),(500,'19900000500','fixture-unbound');
 INSERT INTO distribution_agents(id,app_user_id,agent_code,level,parent_agent_id,root_agent_id,agent_path,status) VALUES
 (10,100,'A2B3C4',1,NULL,10,'/10/','active'),
 (20,200,'Z2X3C4',1,NULL,20,'/20/','active'),
 (30,300,'P2Q3R4',1,NULL,30,'/30/','paused'),
 (11,110,'D2E3F4',2,10,10,'/10/11/','active');
 INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) VALUES(110,10),(400,10),(410,20),(420,10);
 SELECT setval('app_users_id_seq',500);
 SELECT setval('distribution_agents_id_seq',30);`)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{db: database, appUsers: appuser.NewStore(database), env: config.Env{
		AppEnv: "test", JWTSecret: "distribution-invitation-isolated-http-fixture",
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/public/distribution/invite", s.method(http.MethodGet, s.publicDistributionInvite))
	mux.HandleFunc("/api/app/auth/register", s.method(http.MethodPost, s.appRegisterWithPassword))
	mux.HandleFunc("/api/app/auth/login", s.method(http.MethodPost, s.appLoginWithPassword))
	mux.HandleFunc("/api/app/auth/verify-sms", s.method(http.MethodPost, s.appVerifySMS))
	mux.HandleFunc("/api/app/distribution/bind", s.method(http.MethodPost, s.requireAppAuth(s.appDistributionBind)))
	mux.HandleFunc("/api/app/distribution/children", s.method(http.MethodPost, s.requireAppAuth(s.appDistributionCreateChild)))
	mux.HandleFunc("/api/app/distribution/users", s.method(http.MethodGet, s.requireAppAuth(s.appDistributionUsers)))
	mux.HandleFunc("/api/app/distribution/agents", s.method(http.MethodGet, s.requireAppAuth(s.appDistributionAgents)))
	mux.HandleFunc("/api/agent/distribution/agents", s.requireMethodPermission(map[string]string{
		http.MethodGet: "Agent:Distribution:View", http.MethodPost: "Agent:Distribution:Write",
	}, s.agentDistributionAgentsRouter))
	mux.HandleFunc("/api/admin/distribution/agents", s.requirePermission("Customer:App:List", s.adminDistributionAgents))
	web := httptest.NewServer(mux)
	t.Cleanup(web.Close)
	return &distributionInviteHTTPFixture{db: database, s: s, web: web}
}

func (f *distributionInviteHTTPFixture) request(t *testing.T, method, path string, body any, userID int64, backend bool) (int, []byte) {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequest(method, f.web.URL+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if userID != 0 {
		kind, roles := auth.TokenKindApp, []string{"app_user"}
		if backend {
			kind, roles = auth.TokenKindBackend, []string{"agent"}
		}
		token, err := auth.Sign(auth.UserInfo{ID: userID, TokenKind: kind, Roles: roles}, f.s.env.JWTSecret)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := f.web.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw
}

func (f *distributionInviteHTTPFixture) sms(t *testing.T, phone string) {
	t.Helper()
	if err := f.s.appUsers.StoreSMSCode(context.Background(), phone, appuser.HashToken("123456"), "127.0.0.1", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func (f *distributionInviteHTTPFixture) owner(t *testing.T, phone string, want int64) {
	t.Helper()
	var owner int64
	err := f.db.QueryRow(`SELECT COALESCE((SELECT direct_agent_id FROM distribution_user_relations r JOIN app_users u ON u.id=r.app_user_id WHERE u.phone=$1),0)`, phone).Scan(&owner)
	if err != nil || owner != want {
		t.Fatalf("attribution owner=%d want=%d err=%v", owner, want, err)
	}
}

func distributionInviteRegistration(phone, code string) map[string]string {
	return map[string]string{"phone": phone, "nickname": "邀请测试", "password": distributionInviteFixturePassword, "code": "123456", "agentCode": code}
}

func TestDistributionInvitationHTTPPublicCodes(t *testing.T) {
	f := newDistributionInviteHTTPFixture(t)
	for _, tc := range []struct {
		name, query string
		status      int
		valid       bool
	}{
		{"valid", "agent=A2B3C4", 200, true},
		{"case_and_whitespace", "agent=%20a2b3c4%20", 200, true},
		{"paused", "agent=P2Q3R4", 200, false},
		{"unknown", "agent=X2Y3Z4", 404, false},
		{"database_id_is_not_invite_code", "agent=10", 404, false},
		{"agent_number_is_not_query_field", "agentNumber=A2B3C4", 400, false},
		{"missing", "", 400, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := f.request(t, http.MethodGet, "/api/public/distribution/invite?"+tc.query, nil, 0, false)
			if status != tc.status {
				t.Fatalf("status=%d want=%d", status, tc.status)
			}
			if status == 200 {
				var result struct {
					Data struct {
						Valid bool `json:"valid"`
					} `json:"data"`
				}
				if err := json.Unmarshal(raw, &result); err != nil || result.Data.Valid != tc.valid {
					t.Fatalf("valid=%v want=%v err=%v", result.Data.Valid, tc.valid, err)
				}
			}
		})
	}
}

func TestDistributionInvitationHTTPRegistrationAndLoginAttribution(t *testing.T) {
	f := newDistributionInviteHTTPFixture(t)
	t.Run("new_registration_binds_case_insensitive_code_once", func(t *testing.T) {
		phone := "19900000601"
		f.sms(t, phone)
		status, _ := f.request(t, http.MethodPost, "/api/app/auth/register", distributionInviteRegistration(phone, " a2b3c4 "), 0, false)
		if status != 200 {
			t.Fatalf("registration status=%d", status)
		}
		f.owner(t, phone, 10)
		var count int
		if err := f.db.QueryRow(`SELECT count(*) FROM distribution_invite_events e JOIN app_users u ON e.app_user_id=u.id WHERE u.phone=$1 AND event_type='register' AND agent_id=10`, phone).Scan(&count); err != nil || count != 1 {
			t.Fatalf("registration events=%d err=%v", count, err)
		}
		f.sms(t, phone)
		status, _ = f.request(t, http.MethodPost, "/api/app/auth/register", distributionInviteRegistration(phone, "Z2X3C4"), 0, false)
		if status != 409 {
			t.Fatalf("repeat registration status=%d want=409", status)
		}
		f.owner(t, phone, 10)
		status, _ = f.request(t, http.MethodPost, "/api/app/auth/login", map[string]string{"phone": phone, "password": distributionInviteFixturePassword, "agentCode": "Z2X3C4"}, 0, false)
		if status != 200 {
			t.Fatalf("existing bound login status=%d", status)
		}
		f.owner(t, phone, 10)
	})
	for index, code := range []string{"X2Y3Z4", "P2Q3R4", "10"} {
		t.Run("reject_registration_"+code, func(t *testing.T) {
			phone := fmt.Sprintf("199000006%02d", index+10)
			f.sms(t, phone)
			status, _ := f.request(t, http.MethodPost, "/api/app/auth/register", distributionInviteRegistration(phone, code), 0, false)
			if status != 400 {
				t.Fatalf("invalid invite registration status=%d want=400", status)
			}
			var users, unused int
			if err := f.db.QueryRow(`SELECT count(*) FROM app_users WHERE phone=$1`, phone).Scan(&users); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRow(`SELECT count(*) FROM app_sms_codes WHERE phone=$1 AND NOT used`, phone).Scan(&unused); err != nil {
				t.Fatal(err)
			}
			if users != 0 || unused != 1 {
				t.Fatalf("invalid invite created users=%d unused codes=%d", users, unused)
			}
		})
	}
	t.Run("existing_sms_account_credential_completion_does_not_bind", func(t *testing.T) {
		phone := "19900000500"
		f.sms(t, phone)
		status, _ := f.request(t, http.MethodPost, "/api/app/auth/register", distributionInviteRegistration(phone, "A2B3C4"), 0, false)
		if status != 200 {
			t.Fatalf("credential completion status=%d", status)
		}
		f.owner(t, phone, 0)
	})
	t.Run("existing_unbound_password_login_does_not_bind", func(t *testing.T) {
		phone := "19900000602"
		f.sms(t, phone)
		status, _ := f.request(t, http.MethodPost, "/api/app/auth/register", distributionInviteRegistration(phone, ""), 0, false)
		if status != 200 {
			t.Fatalf("unbound registration status=%d", status)
		}
		for _, code := range []string{"A2B3C4", "Z2X3C4", "P2Q3R4", "X2Y3Z4"} {
			status, _ := f.request(t, http.MethodPost, "/api/app/auth/login", map[string]string{"phone": phone, "password": distributionInviteFixturePassword, "agentCode": code}, 0, false)
			if status != 200 {
				t.Fatalf("legacy login with invite status=%d", status)
			}
			f.owner(t, phone, 0)
		}
	})
}

func TestDistributionInvitationHTTPPermissionsAndChildCodes(t *testing.T) {
	f := newDistributionInviteHTTPFixture(t)
	for _, tc := range []struct {
		name   string
		user   int64
		code   string
		status int
	}{
		{"unauthenticated", 0, "A2B3C4", 401}, {"self", 100, "A2B3C4", 403},
		{"unbound", 500, "A2B3C4", 403}, {"repeat", 400, "A2B3C4", 403}, {"hijack", 400, "Z2X3C4", 403},
	} {
		t.Run("bind_"+tc.name, func(t *testing.T) {
			status, _ := f.request(t, http.MethodPost, "/api/app/distribution/bind", map[string]string{"agentCode": tc.code}, tc.user, false)
			if status != tc.status {
				t.Fatalf("bind status=%d want=%d", status, tc.status)
			}
		})
	}
	f.owner(t, "19900000400", 10)
	f.owner(t, "19900000500", 0)
	for _, tc := range []struct {
		name, path string
		backend    bool
	}{
		{"app_users", "/api/app/distribution/users", false},
		{"app_agents", "/api/app/distribution/agents", false},
		{"backend_agents", "/api/agent/distribution/agents", true},
	} {
		t.Run("scope_"+tc.name, func(t *testing.T) {
			status, raw := f.request(t, http.MethodGet, tc.path+"?agentId=20&appUserId=200&agentCode=Z2X3C4&path=/20/", nil, 100, tc.backend)
			if status != 200 {
				t.Fatalf("list status=%d", status)
			}
			if bytes.Contains(raw, []byte("fixture-user-b")) || bytes.Contains(raw, []byte("Z2X3C4")) {
				t.Fatal("cross-agent data disclosed")
			}
			if !bytes.Contains(raw, []byte("fixture-child-a")) {
				t.Fatal("own child missing from list")
			}
		})
	}
	for _, backend := range []bool{false, true} {
		path := "/api/app/distribution/children"
		if backend {
			path = "/api/agent/distribution/agents"
		}
		for _, tc := range []struct {
			name         string
			user, target int64
			status       int
		}{
			{"cross_tree", 100, 410, 403}, {"self", 100, 100, 403}, {"already_agent", 100, 110, 409},
			{"non_agent", 500, 400, 403}, {"paused", 300, 400, 403},
		} {
			t.Run(fmt.Sprintf("child_%v_%s", backend, tc.name), func(t *testing.T) {
				want := tc.status
				if backend && (tc.user == 300 || tc.user == 500) {
					want = 401
				}
				status, _ := f.request(t, http.MethodPost, path, map[string]int64{"appUserId": tc.target}, tc.user, backend)
				if status != want {
					t.Fatalf("child status=%d want=%d", status, want)
				}
			})
		}
	}
	t.Run("backend_child_uses_same_six_character_code_as_app", func(t *testing.T) {
		status, raw := f.request(t, http.MethodPost, "/api/agent/distribution/agents", map[string]int64{"appUserId": 420}, 100, true)
		if status != 200 {
			t.Fatalf("backend child status=%d", status)
		}
		var result struct {
			Data distributionAgentResponse `json:"data"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if !validateDistributionAgentCodeFormat(result.Data.AgentCode) {
			t.Fatalf("child invite code has unsupported format: %q", result.Data.AgentCode)
		}
		if result.Data.ParentAgentID != 10 || result.Data.RootAgentID != 10 || !strings.HasPrefix(result.Data.Path, "/10/") {
			t.Fatalf("unexpected child hierarchy: %+v", result.Data)
		}
		inviteStatus, _ := f.request(t, http.MethodGet, "/api/public/distribution/invite?agent="+result.Data.AgentCode, nil, 0, false)
		if inviteStatus != 200 {
			t.Fatalf("new child invite status=%d", inviteStatus)
		}
	})
	t.Run("agent_cannot_access_admin", func(t *testing.T) {
		status, _ := f.request(t, http.MethodGet, "/api/admin/distribution/agents", nil, 100, true)
		if status != 403 {
			t.Fatalf("admin status=%d want=403", status)
		}
	})
}
