package server

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
)

func TestVoiceChatRejectsReadOnlyCardBeforeMultipartProcessing(t *testing.T) {
	registerVoiceMembershipGateDriver()
	db, err := sql.Open(voiceMembershipGateDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	store.cardID = 2
	s := newVoiceChatTestServer(store, &voiceChatGenerator{answer: "must not run"})
	s.db = db
	s.appUsers = appuser.NewStore(db)

	// No multipart body is supplied deliberately. The membership gate must run
	// immediately after session ownership is resolved and return before parsing.
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", bytes.NewBufferString("not multipart"))
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatRouter(response, req)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s, want 403", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"requiredPlanLevel":"vip"`) {
		t.Fatalf("missing required plan metadata: %s", response.Body.String())
	}
}

func TestAppChatFeedbackRequiresOwnedMessageSession(t *testing.T) {
	store := &messageSessionGateChatStore{
		fakeVoiceChatStore: &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()},
		err:                chat.ErrNotFound,
	}
	s := &Server{appChat: store}
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/messages/99/feedback", bytes.NewBufferString(`{"feedback":"helpful"}`))
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatMessageRouter(response, req)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s, want 404", response.Code, response.Body.String())
	}
}

func TestAppChatFavoriteRequiresOwnedMessageSession(t *testing.T) {
	store := &messageSessionGateChatStore{
		fakeVoiceChatStore: &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()},
		err:                chat.ErrNotFound,
	}
	s := &Server{appChat: store}
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/messages/99/favorite", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatMessageRouter(response, req)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s, want 404", response.Code, response.Body.String())
	}
}

// messageSessionGateChatStore supplies the optional owner lookup used by
// mutation handlers while retaining the broad fake chat store fixture.
type messageSessionGateChatStore struct {
	*fakeVoiceChatStore
	session chat.Session
	err     error
}

func (s *messageSessionGateChatStore) GetMessageSession(context.Context, int64, int64) (chat.Session, error) {
	if s.err != nil {
		return chat.Session{}, s.err
	}
	return s.session, nil
}

const voiceMembershipGateDriverName = "app_chat_voice_membership_gate_test"

var registerVoiceMembershipGateDriverOnce sync.Once

func registerVoiceMembershipGateDriver() {
	registerVoiceMembershipGateDriverOnce.Do(func() {
		sql.Register(voiceMembershipGateDriverName, voiceMembershipGateDriver{})
	})
}

type voiceMembershipGateDriver struct{}

func (voiceMembershipGateDriver) Open(string) (driver.Conn, error) {
	return voiceMembershipGateConn{}, nil
}

type voiceMembershipGateConn struct{}

func (voiceMembershipGateConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (voiceMembershipGateConn) Close() error                        { return nil }
func (voiceMembershipGateConn) Begin() (driver.Tx, error)            { return voiceMembershipGateTx{}, nil }

func (voiceMembershipGateConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT card_type,status"):
		return &voiceMembershipGateRows{
			columns: []string{"card_type", "status"},
			values:  [][]driver.Value{{"secondary", "active"}},
		}, nil
	case strings.Contains(query, "FROM app_membership_resource_access"):
		return &voiceMembershipGateRows{
			columns: []string{"resource_id", "state", "required_plan_level", "priority_rank", "reason", "retention_until"},
			values:  [][]driver.Value{{int64(2), resourceAccessReadOnlyOverLimit, "vip", int64(2), "over limit", nil}},
		}, nil
	default:
		// Returning driver.ErrSkip for the membership-column lookup exercises
		// the legacy-schema fallback; the existing ledger row remains decisive.
		return nil, driver.ErrSkip
	}
}

type voiceMembershipGateTx struct{}

func (voiceMembershipGateTx) Commit() error   { return nil }
func (voiceMembershipGateTx) Rollback() error { return nil }

type voiceMembershipGateRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r voiceMembershipGateRows) Columns() []string { return r.columns }
func (r voiceMembershipGateRows) Close() error      { return nil }
func (r *voiceMembershipGateRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
