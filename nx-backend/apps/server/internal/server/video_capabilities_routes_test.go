package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/system"
	"nine-xing/nx-backend/apps/server/internal/video"
)

const videoCapabilityModelConfigDriverName = "video_capability_model_config_test"
const videoCapabilityPermissionDriverName = "video_capability_permission_test"
const videoCapabilityConcurrentConfigDriverName = "video_capability_concurrent_config_test"

var registerVideoCapabilityModelConfigDriverOnce sync.Once
var registerVideoCapabilityPermissionDriverOnce sync.Once
var registerVideoCapabilityConcurrentConfigDriverOnce sync.Once
var videoCapabilityConcurrentConfigStates sync.Map

func TestVideoCapabilitiesRouteReturnsFastModelIntersectionWithoutSecrets(t *testing.T) {
	models := []string{"video-ds-2.0-fast", "as-sd2.0-fast"}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			s := &Server{env: config.Env{Video: config.VideoConfig{
				APIBase:         "https://gateway-user:gateway-password@video.example.com/v1",
				APIKey:          "video-api-key-must-not-leak",
				Model:           model,
				GatewayContract: config.LegacyVideoGatewayContract(),
			}}}
			request := httptest.NewRequest(http.MethodGet, "/api/video/capabilities?model="+model, nil)
			response := httptest.NewRecorder()

			s.videoCapabilities(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
			}
			var body struct {
				Code int                `json:"code"`
				Data video.Capabilities `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != 0 || body.Data.Model != model || body.Data.ModelProfile != "fast" {
				t.Fatalf("unexpected fast capability response: %+v", body)
			}
			if !reflect.DeepEqual(body.Data.SupportedDurations, []int{5, 10, 15}) {
				t.Fatalf("legacy durations = %#v", body.Data.SupportedDurations)
			}
			if !reflect.DeepEqual(body.Data.AspectRatios, []string{"16:9", "9:16", "1:1"}) {
				t.Fatalf("legacy aspect ratios = %#v", body.Data.AspectRatios)
			}
			if body.Data.CapabilityVersion == "" || body.Data.Source.OfficialProfile == "" || len(body.Data.Degradations) == 0 {
				t.Fatalf("response must include version, source and degradations: %+v", body.Data)
			}
			if body.Data.Source.GatewayContract != "legacy_flat_v1" || body.Data.Source.GatewayContractVersion != "1" {
				t.Fatalf("route did not use the effective gateway contract: %+v", body.Data.Source)
			}

			raw := response.Body.String()
			for _, secret := range []string{
				"video-api-key-must-not-leak",
				"gateway-password",
				`"apiKey"`,
				`"apiBase"`,
				`"authorization"`,
				`"gatewayAuth"`,
			} {
				if strings.Contains(raw, secret) {
					t.Fatalf("capability response leaked secret field/value %q: %s", secret, raw)
				}
			}
		})
	}
}

func TestVideoCapabilitiesRouteUsesConfiguredModelWhenQueryIsMissing(t *testing.T) {
	s := &Server{env: config.Env{Video: config.VideoConfig{
		Model:           "video-ds-2.0",
		ModelProfile:    "standard",
		GatewayContract: config.LegacyVideoGatewayContract(),
	}}}
	request := httptest.NewRequest(http.MethodGet, "/api/video/capabilities", nil)
	response := httptest.NewRecorder()

	s.videoCapabilities(response, request)

	got := decodeVideoCapabilitiesResponse(t, response)
	if got.Model != "video-ds-2.0" || got.ModelProfile != "standard" || got.Source.Selection != "exact_model" {
		t.Fatalf("missing query did not resolve current configured model safely: %+v", got)
	}
}

func TestVideoCapabilitiesRouteUnknownModelFailsClosedThroughRegistry(t *testing.T) {
	s := &Server{env: config.Env{Video: config.VideoConfig{
		Model:           "video-ds-2.0",
		ModelProfile:    "standard",
		GatewayContract: config.LegacyVideoGatewayContract(),
	}}}
	request := httptest.NewRequest(http.MethodGet, "/api/video/capabilities?model=future-secret-model", nil)
	response := httptest.NewRecorder()

	s.videoCapabilities(response, request)

	got := decodeVideoCapabilitiesResponse(t, response)
	if got.Model != "future-secret-model" || got.ModelProfile != "generic_unknown" || got.Source.Selection != "generic_fallback" {
		t.Fatalf("unknown model did not use the fail-closed registry profile: %+v", got)
	}
	if got.SupportsResolution || got.SupportsGenerateAudio || got.SupportsEdit || got.SupportsExtend || got.SupportsSeed || got.SupportsCameraFixed {
		t.Fatalf("unknown model exposed unproven capabilities: %+v", got)
	}
}

func TestVideoCapabilitiesRouteRequiresGeneratePermission(t *testing.T) {
	handler, forbiddenToken, allowedToken := newVideoCapabilityPermissionHandler(t)
	tests := []struct {
		name   string
		method string
		token  string
		status int
	}{
		{name: "unauthenticated", method: http.MethodGet, status: http.StatusUnauthorized},
		{name: "forbidden", method: http.MethodGet, token: forbiddenToken, status: http.StatusForbidden},
		{name: "allowed", method: http.MethodGet, token: allowedToken, status: http.StatusOK},
		{name: "method not allowed", method: http.MethodPost, token: allowedToken, status: http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performVideoCapabilityHTTP(handler, tt.method, "/api/video/capabilities?model=video-ds-2.0", tt.token)
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d body=%s", response.Code, tt.status, response.Body.String())
			}
		})
	}
}

func TestVideoCapabilitiesRouteRuntimeConfigIsDeepCopied(t *testing.T) {
	contract := config.LegacyVideoGatewayContract()
	contract.Duration.ValueMap = map[string]string{"smart": "-1"}
	contract.References.RoleFields = map[string]string{"reference_video": "reference"}
	contract.Reconciliation.TaskIDPaths = []string{"data.id"}
	original := config.VideoConfig{
		Model:           "video-ds-2.0",
		ModelProfile:    "standard",
		GatewayContract: contract,
	}
	s := &Server{}

	s.replaceVideoRuntime(original)

	original.GatewayContract.DeclaredModes[0] = "edit"
	original.GatewayContract.Duration.ValueMap["smart"] = "mutated"
	original.GatewayContract.References.RoleFields["reference_video"] = "mutated"
	original.GatewayContract.Reconciliation.TaskIDPaths[0] = "mutated"

	first := s.effectiveVideoConfig()
	if first.GatewayContract.DeclaredModes[0] != "reference" ||
		first.GatewayContract.Duration.ValueMap["smart"] != "-1" ||
		first.GatewayContract.References.RoleFields["reference_video"] != "reference" ||
		first.GatewayContract.Reconciliation.TaskIDPaths[0] != "data.id" {
		t.Fatalf("runtime config retained caller-owned maps or slices: %+v", first.GatewayContract)
	}
	if s.videoStore() == nil {
		t.Fatal("runtime replacement must update the video client together with its capability config")
	}

	first.GatewayContract.DeclaredModes[0] = "extend"
	first.GatewayContract.Duration.ValueMap["smart"] = "returned-copy-mutated"
	first.GatewayContract.References.RoleFields["reference_video"] = "returned-copy-mutated"
	first.GatewayContract.Reconciliation.TaskIDPaths[0] = "returned-copy-mutated"
	second := s.effectiveVideoConfig()
	if second.GatewayContract.DeclaredModes[0] != "reference" ||
		second.GatewayContract.Duration.ValueMap["smart"] != "-1" ||
		second.GatewayContract.References.RoleFields["reference_video"] != "reference" ||
		second.GatewayContract.Reconciliation.TaskIDPaths[0] != "data.id" {
		t.Fatalf("effectiveVideoConfig leaked runtime maps or slices: %+v", second.GatewayContract)
	}
}

func TestVideoCapabilitiesRouteSuccessfulModelConfigPUTReplacesRuntimePair(t *testing.T) {
	database := openVideoCapabilityModelConfigDB(t, "success")
	base := config.VideoConfig{Model: "video-ds-2.0", GatewayContract: config.LegacyVideoGatewayContract()}
	s := &Server{db: database, env: config.Env{Video: base}}
	s.replaceVideoRuntime(base)
	beforeStore := s.videoStore()

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", `{"video":{"model":"video-ds-2.0-fast"}}`)

	if response.Code != http.StatusOK {
		t.Fatalf("successful model config PUT = %d body=%s", response.Code, response.Body.String())
	}
	after := s.effectiveVideoConfig()
	if after.Model != "video-ds-2.0-fast" || s.videoStore() == beforeStore {
		t.Fatalf("successful PUT did not replace client/config pair: config=%+v sameStore=%v", after, s.videoStore() == beforeStore)
	}
}

func TestVideoCapabilitiesRouteFailedModelConfigPUTKeepsRuntimePair(t *testing.T) {
	database := openVideoCapabilityModelConfigDB(t, "failure")
	base := config.VideoConfig{Model: "video-ds-2.0", GatewayContract: config.LegacyVideoGatewayContract()}
	s := &Server{db: database, env: config.Env{Video: base}}
	s.replaceVideoRuntime(base)
	beforeStore := s.videoStore()

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", `{"video":{"model":"video-ds-2.0-fast"}}`)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("failed model config PUT = %d body=%s", response.Code, response.Body.String())
	}
	after := s.effectiveVideoConfig()
	if after.Model != base.Model || s.videoStore() != beforeStore {
		t.Fatalf("failed PUT replaced runtime state: config=%+v sameStore=%v", after, s.videoStore() == beforeStore)
	}
}

func TestVideoCapabilitiesRouteConcurrentModelConfigPUTKeepsDatabaseRuntimeAndStoreConsistent(t *testing.T) {
	state := &videoCapabilityConcurrentConfigState{
		firstWrite:   make(chan struct{}),
		releaseFirst: make(chan struct{}),
		secondRead:   make(chan struct{}),
	}
	database := openVideoCapabilityConcurrentConfigDB(t, state)
	base := config.VideoConfig{Model: "video-ds-2.0", GatewayContract: config.LegacyVideoGatewayContract()}
	s := &Server{db: database, env: config.Env{Video: base}}
	s.replaceVideoRuntime(base)
	initialStore := s.videoStore()

	firstResponse := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		request := httptest.NewRequest(http.MethodPut, "/api/model-config", strings.NewReader(`{"video":{"model":"video-ds-2.0-fast"}}`))
		s.modelConfig(firstResponse, request)
	}()
	waitVideoCapabilitySignal(t, state.firstWrite, "first config write")

	secondResponse := httptest.NewRecorder()
	secondStarted := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		close(secondStarted)
		request := httptest.NewRequest(http.MethodPut, "/api/model-config", strings.NewReader(`{"video":{"model":"as-sd2.0-fast"}}`))
		s.modelConfig(secondResponse, request)
	}()
	<-secondStarted

	secondReadBeforeRelease := false
	select {
	case <-state.secondRead:
		secondReadBeforeRelease = true
		waitVideoCapabilitySignal(t, secondDone, "second unlocked config update")
	case <-time.After(250 * time.Millisecond):
	}
	close(state.releaseFirst)
	waitVideoCapabilitySignal(t, firstDone, "first config update")
	waitVideoCapabilitySignal(t, secondDone, "second config update")

	if firstResponse.Code != http.StatusOK || secondResponse.Code != http.StatusOK {
		t.Fatalf("concurrent PUT statuses: first=%d body=%s second=%d body=%s", firstResponse.Code, firstResponse.Body.String(), secondResponse.Code, secondResponse.Body.String())
	}
	stored := state.storedConfig(t)
	runtimeConfig := s.effectiveVideoConfig()
	storeModel := reflectedVideoStoreDefaultModel(t, s.videoStore())
	if stored.Video.Model != "as-sd2.0-fast" || runtimeConfig.Model != stored.Video.Model || storeModel != stored.Video.Model {
		t.Fatalf("concurrent PUT diverged: secondReadBeforeRelease=%v db=%q runtime=%q store=%q", secondReadBeforeRelease, stored.Video.Model, runtimeConfig.Model, storeModel)
	}
	if s.videoStore() == initialStore {
		t.Fatal("successful concurrent updates did not replace the initial video store")
	}
}

func decodeVideoCapabilitiesResponse(t *testing.T, response *httptest.ResponseRecorder) video.Capabilities {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Code int                `json:"code"`
		Data video.Capabilities `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 {
		t.Fatalf("unexpected API code: %+v", body)
	}
	return body.Data
}

func openVideoCapabilityModelConfigDB(t *testing.T, mode string) *sql.DB {
	t.Helper()
	registerVideoCapabilityModelConfigDriverOnce.Do(func() {
		sql.Register(videoCapabilityModelConfigDriverName, videoCapabilityModelConfigDriver{})
	})
	database, err := sql.Open(videoCapabilityModelConfigDriverName, mode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func openVideoCapabilityConcurrentConfigDB(t *testing.T, state *videoCapabilityConcurrentConfigState) *sql.DB {
	t.Helper()
	registerVideoCapabilityConcurrentConfigDriverOnce.Do(func() {
		sql.Register(videoCapabilityConcurrentConfigDriverName, videoCapabilityConcurrentConfigDriver{})
	})
	key := t.Name()
	videoCapabilityConcurrentConfigStates.Store(key, state)
	database, err := sql.Open(videoCapabilityConcurrentConfigDriverName, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Close()
		videoCapabilityConcurrentConfigStates.Delete(key)
	})
	return database
}

func waitVideoCapabilitySignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func reflectedVideoStoreDefaultModel(t *testing.T, store *video.Store) string {
	t.Helper()
	if store == nil {
		t.Fatal("video store is nil")
	}
	value := reflect.ValueOf(store).Elem().FieldByName("defaultModel")
	if !value.IsValid() || value.Kind() != reflect.String {
		t.Fatalf("video store defaultModel field is unavailable")
	}
	return value.String()
}

func newVideoCapabilityPermissionHandler(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	registerVideoCapabilityPermissionDriverOnce.Do(func() {
		sql.Register(videoCapabilityPermissionDriverName, videoCapabilityPermissionDriver{})
	})
	database, err := sql.Open(videoCapabilityPermissionDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	env := config.Env{
		JWTSecret: "video-capability-permission-secret",
		Video: config.VideoConfig{
			Model:           "video-ds-2.0",
			GatewayContract: config.LegacyVideoGatewayContract(),
		},
	}
	s := &Server{
		db:     database,
		env:    env,
		mux:    http.NewServeMux(),
		system: system.NewStore(database),
	}
	s.replaceVideoRuntime(env.Video)
	s.routes()

	forbiddenToken, err := auth.Sign(auth.UserInfo{ID: 2, TokenVersion: 1, Username: "viewer"}, env.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	allowedToken, err := auth.Sign(auth.UserInfo{ID: 3, TokenVersion: 1, Username: "video-user"}, env.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	return s.withCORS(s.mux), forbiddenToken, allowedToken
}

func performVideoCapabilityHTTP(handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type videoCapabilityModelConfigDriver struct{}

func (videoCapabilityModelConfigDriver) Open(name string) (driver.Conn, error) {
	return videoCapabilityModelConfigConn{mode: name}, nil
}

type videoCapabilityModelConfigConn struct {
	mode string
}

func (videoCapabilityModelConfigConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not supported")
}

func (videoCapabilityModelConfigConn) Close() error { return nil }

func (videoCapabilityModelConfigConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not supported")
}

func (videoCapabilityModelConfigConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return videoCapabilityEmptyRows{}, nil
}

func (c videoCapabilityModelConfigConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	if c.mode == "failure" {
		return nil, errors.New("forced model config write failure")
	}
	return driver.RowsAffected(1), nil
}

type videoCapabilityEmptyRows struct{}

func (videoCapabilityEmptyRows) Columns() []string { return []string{"config"} }
func (videoCapabilityEmptyRows) Close() error      { return nil }
func (videoCapabilityEmptyRows) Next([]driver.Value) error {
	return io.EOF
}

type videoCapabilityPermissionDriver struct{}

func (videoCapabilityPermissionDriver) Open(string) (driver.Conn, error) {
	return videoCapabilityPermissionConn{}, nil
}

type videoCapabilityPermissionConn struct{}

func (videoCapabilityPermissionConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not supported")
}

func (videoCapabilityPermissionConn) Close() error { return nil }

func (videoCapabilityPermissionConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not supported")
}

func (videoCapabilityPermissionConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	userID := int64(0)
	if len(args) > 0 {
		userID, _ = args[0].Value.(int64)
	}
	switch {
	case strings.Contains(query, "SELECT token_version FROM users"):
		return &videoCapabilityRows{columns: []string{"token_version"}, values: [][]driver.Value{{int64(1)}}}, nil
	case strings.Contains(query, "SELECT id, username, avatar, nickname, email, phone, remark FROM users"):
		return &videoCapabilityRows{
			columns: []string{"id", "username", "avatar", "nickname", "email", "phone", "remark"},
			values:  [][]driver.Value{{userID, videoCapabilityUsername(userID), "", "Video User", "", "", ""}},
		}, nil
	case strings.Contains(query, "SELECT r.code FROM roles"):
		return &videoCapabilityRows{columns: []string{"code"}, values: [][]driver.Value{{videoCapabilityRole(userID)}}}, nil
	case strings.Contains(query, "SELECT DISTINCT m.auth_code FROM menus"):
		if userID == 3 {
			return &videoCapabilityRows{columns: []string{"auth_code"}, values: [][]driver.Value{{"Video:Generate:Manage"}}}, nil
		}
		return &videoCapabilityRows{columns: []string{"auth_code"}}, nil
	default:
		return nil, errors.New("unexpected permission query: " + query)
	}
}

func videoCapabilityUsername(userID int64) string {
	if userID == 3 {
		return "video-user"
	}
	return "viewer"
}

func videoCapabilityRole(userID int64) string {
	if userID == 3 {
		return "video_user"
	}
	return "viewer"
}

type videoCapabilityRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *videoCapabilityRows) Columns() []string { return r.columns }
func (r *videoCapabilityRows) Close() error      { return nil }
func (r *videoCapabilityRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

type videoCapabilityConcurrentConfigDriver struct{}

func (videoCapabilityConcurrentConfigDriver) Open(name string) (driver.Conn, error) {
	value, ok := videoCapabilityConcurrentConfigStates.Load(name)
	if !ok {
		return nil, errors.New("missing concurrent config state")
	}
	return &videoCapabilityConcurrentConfigConn{state: value.(*videoCapabilityConcurrentConfigState)}, nil
}

type videoCapabilityConcurrentConfigConn struct {
	state *videoCapabilityConcurrentConfigState
}

func (*videoCapabilityConcurrentConfigConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not supported")
}

func (*videoCapabilityConcurrentConfigConn) Close() error { return nil }

func (*videoCapabilityConcurrentConfigConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not supported")
}

func (c *videoCapabilityConcurrentConfigConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	c.state.readCount++
	readCount := c.state.readCount
	raw := append([]byte(nil), c.state.raw...)
	c.state.mu.Unlock()
	if readCount == 2 {
		close(c.state.secondRead)
	}
	if len(raw) == 0 {
		return videoCapabilityEmptyRows{}, nil
	}
	return &videoCapabilityRows{columns: []string{"config"}, values: [][]driver.Value{{raw}}}, nil
}

func (c *videoCapabilityConcurrentConfigConn) ExecContext(_ context.Context, _ string, args []driver.NamedValue) (driver.Result, error) {
	if len(args) < 2 {
		return nil, errors.New("missing model config JSON")
	}
	raw, ok := args[1].Value.(string)
	if !ok {
		return nil, errors.New("model config JSON is not a string")
	}
	c.state.mu.Lock()
	c.state.writeCount++
	writeCount := c.state.writeCount
	c.state.raw = []byte(raw)
	c.state.mu.Unlock()
	if writeCount == 1 {
		close(c.state.firstWrite)
		<-c.state.releaseFirst
	}
	return driver.RowsAffected(1), nil
}

type videoCapabilityConcurrentConfigState struct {
	mu           sync.Mutex
	raw          []byte
	readCount    int
	writeCount   int
	firstWrite   chan struct{}
	releaseFirst chan struct{}
	secondRead   chan struct{}
}

func (s *videoCapabilityConcurrentConfigState) storedConfig(t *testing.T) modelconfig.Config {
	t.Helper()
	s.mu.Lock()
	raw := append([]byte(nil), s.raw...)
	s.mu.Unlock()
	var stored modelconfig.Config
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("decode stored model config: %v raw=%s", err, raw)
	}
	return stored
}
