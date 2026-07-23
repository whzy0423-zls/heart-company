package businessmessage

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

func TestValidateRequiresStableIdentity(t *testing.T) {
	valid := Event{
		Type:         "signup",
		Title:        "新的官网报名",
		Content:      "张三提交了官网报名",
		Platform:     "website",
		EventKey:     "signup.created",
		BusinessID:   "1",
		BusinessType: "signup",
		TargetPath:   "/customer/signups?leadId=1&open=detail",
	}
	tests := []struct {
		name   string
		mutate func(*Event)
	}{
		{name: "empty event key", mutate: func(event *Event) { event.EventKey = " " }},
		{name: "empty business type", mutate: func(event *Event) { event.BusinessType = "" }},
		{name: "empty business id", mutate: func(event *Event) { event.BusinessID = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := valid
			tt.mutate(&event)
			if err := Validate(event); err == nil {
				t.Fatal("Validate() error = nil, want stable identity error")
			}
		})
	}
}

func TestValidateRejectsInvalidPlatformAndTargetPath(t *testing.T) {
	valid := Event{
		Type:         "miniapp",
		Title:        "新的小程序用户",
		Content:      "小明首次进入小程序",
		Platform:     "miniapp",
		EventKey:     "miniapp.user.created",
		BusinessID:   "2",
		BusinessType: "miniapp-user",
		TargetPath:   "/customer/miniapp-users?userId=2&open=detail",
	}
	tests := []struct {
		name   string
		mutate func(*Event)
	}{
		{name: "unknown platform", mutate: func(event *Event) { event.Platform = "desktop" }},
		{name: "relative path", mutate: func(event *Event) { event.TargetPath = "customer/users" }},
		{name: "empty title", mutate: func(event *Event) { event.Title = "" }},
		{name: "title too long", mutate: func(event *Event) { event.Title = strings.Repeat("题", 101) }},
		{name: "content too long", mutate: func(event *Event) { event.Content = strings.Repeat("文", 1001) }},
		{name: "path too long", mutate: func(event *Event) { event.TargetPath = "/" + strings.Repeat("a", 512) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := valid
			tt.mutate(&event)
			if err := Validate(event); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}

	for _, platform := range []string{"website", "miniapp", "system"} {
		event := valid
		event.Platform = platform
		if err := Validate(event); err != nil {
			t.Fatalf("Validate(platform=%q) error = %v", platform, err)
		}
	}
}

func TestEventConstructors(t *testing.T) {
	tests := []struct {
		name string
		got  Event
		want Event
	}{
		{
			name: "website signup",
			got:  WebsiteSignupCreated("42", "张三", "手机", "138****5678"),
			want: Event{Type: "signup", Title: "新的官网报名", Content: "张三提交了官网报名，手机：138****5678", Platform: "website", EventKey: "signup.created", BusinessID: "42", BusinessType: "signup", TargetPath: "/customer/signups?leadId=42&open=detail"},
		},
		{
			name: "miniapp user",
			got:  MiniappUserCreated("43", "小明"),
			want: Event{Type: "miniapp", Title: "新的小程序用户", Content: "小明首次进入小程序", Platform: "miniapp", EventKey: "miniapp.user.created", BusinessID: "43", BusinessType: "miniapp-user", TargetPath: "/customer/miniapp-users?userId=43&open=detail"},
		},
		{
			name: "miniapp quiz",
			got:  MiniappQuizSubmitted("44", "43", "小明", 9),
			want: Event{Type: "miniapp", Title: "新的小程序测评", Content: "小明提交了 9 型测评", Platform: "miniapp", EventKey: "miniapp.quiz.submitted", BusinessID: "44", BusinessType: "miniapp-test-record", TargetPath: "/customer/miniapp-users?userId=43&testRecordId=44&open=test"},
		},
		{
			name: "miniapp booking",
			got:  MiniappBookingCreated("45", "46", "小明", "138****5678"),
			want: Event{Type: "miniapp", Title: "新的小程序预约", Content: "小明提交了预约咨询，手机号：138****5678", Platform: "miniapp", EventKey: "miniapp.booking.created", BusinessID: "46", BusinessType: "signup", TargetPath: "/customer/signups?leadId=46&open=detail"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("constructor result = %#v, want %#v", tt.got, tt.want)
			}
			if strings.Contains(strings.ToLower(tt.got.Content), "openid") || strings.Contains(strings.ToLower(tt.got.Content), "unionid") {
				t.Fatalf("constructor leaked identity field in content: %q", tt.got.Content)
			}
		})
	}
}

func TestStoreCreateIsIdempotentAndMasksPhone(t *testing.T) {
	database, state := newMessageTestDB(t)
	store := Store{}
	event := WebsiteSignupCreated("42", "张三", "手机", "13812345678")

	created, err := store.Create(context.Background(), database, event)
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if !created {
		t.Fatal("first Create() created = false, want true")
	}
	created, err = store.Create(context.Background(), database, event)
	if err != nil {
		t.Fatalf("duplicate Create() error = %v", err)
	}
	if created {
		t.Fatal("duplicate Create() created = true, want false")
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.seen) != 1 {
		t.Fatalf("stored identities = %d, want 1", len(state.seen))
	}
	if state.lastContent != "张三提交了官网报名，手机：138****5678" {
		t.Fatalf("stored content = %q, want masked phone", state.lastContent)
	}
	if !strings.Contains(state.lastQuery, "ON CONFLICT (event_key,business_type,business_id) DO NOTHING") {
		t.Fatalf("query does not use stable identity conflict handling: %s", state.lastQuery)
	}
	if !strings.Contains(state.lastQuery, "RETURNING id") {
		t.Fatalf("query does not return inserted id: %s", state.lastQuery)
	}
}

func TestStoreCreateRejectsNilQueryTarget(t *testing.T) {
	_, err := (Store{}).Create(context.Background(), nil, WebsiteSignupCreated("1", "张三", "手机", "138****5678"))
	if err == nil {
		t.Fatal("Create() error = nil, want nil DBTX error")
	}
}

func TestStoreCreateValidatesBeforeCheckingQueryTarget(t *testing.T) {
	_, err := (Store{}).Create(context.Background(), nil, Event{})
	if err == nil {
		t.Fatal("Create() error = nil, want validation error")
	}
	if errors.Is(err, ErrNilDBTX) {
		t.Fatalf("Create() error = %v, want event validation before DBTX check", err)
	}
}

type messageDriverState struct {
	mu          sync.Mutex
	lastContent string
	lastQuery   string
	nextID      int64
	seen        map[string]struct{}
}

type messageConnector struct {
	state *messageDriverState
}

func (c messageConnector) Connect(context.Context) (driver.Conn, error) {
	return &messageConn{state: c.state}, nil
}

func (c messageConnector) Driver() driver.Driver { return messageDriver{state: c.state} }

type messageDriver struct {
	state *messageDriverState
}

func (d messageDriver) Open(string) (driver.Conn, error) { return &messageConn{state: d.state}, nil }

type messageConn struct {
	state *messageDriverState
}

func (*messageConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (*messageConn) Close() error              { return nil }
func (*messageConn) Begin() (driver.Tx, error) { return nil, errors.New("transaction not supported") }

func (c *messageConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) != 8 {
		return nil, errors.New("unexpected message argument count")
	}
	stringsByOrdinal := make(map[int]string, len(args))
	for _, arg := range args {
		value, ok := arg.Value.(string)
		if !ok {
			return nil, errors.New("message argument is not a string")
		}
		stringsByOrdinal[arg.Ordinal] = value
	}
	identity := stringsByOrdinal[5] + "\x00" + stringsByOrdinal[7] + "\x00" + stringsByOrdinal[6]

	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.lastQuery = strings.Join(strings.Fields(query), " ")
	c.state.lastContent = stringsByOrdinal[3]
	if _, exists := c.state.seen[identity]; exists {
		return &messageRows{columns: []string{"id"}}, nil
	}
	c.state.seen[identity] = struct{}{}
	c.state.nextID++
	return &messageRows{columns: []string{"id"}, values: [][]driver.Value{{c.state.nextID}}}, nil
}

type messageRows struct {
	columns []string
	index   int
	values  [][]driver.Value
}

func (r *messageRows) Columns() []string { return r.columns }
func (*messageRows) Close() error        { return nil }
func (r *messageRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func newMessageTestDB(t *testing.T) (*sql.DB, *messageDriverState) {
	t.Helper()
	state := &messageDriverState{seen: make(map[string]struct{})}
	database := sql.OpenDB(messageConnector{state: state})
	t.Cleanup(func() { _ = database.Close() })
	return database, state
}
