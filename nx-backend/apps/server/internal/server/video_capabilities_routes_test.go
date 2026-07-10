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
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/video"
)

const videoCapabilityModelConfigDriverName = "video_capability_model_config_test"

var registerVideoCapabilityModelConfigDriverOnce sync.Once

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
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	want := `s.mux.HandleFunc("/api/video/capabilities", s.method(http.MethodGet, s.requirePermission("Video:Generate:Manage", s.videoCapabilities)))`
	if !strings.Contains(string(source), want) {
		t.Fatalf("capability route must require Video:Generate:Manage with GET-only registration")
	}

	handler := New(config.Env{
		JWTSecret: "test-secret",
		Video: config.VideoConfig{
			Model:           "video-ds-2.0",
			GatewayContract: config.LegacyVideoGatewayContract(),
		},
	}, nil)
	response := performRawUnit(handler, http.MethodGet, "/api/video/capabilities?model=video-ds-2.0", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("protected capability route without credentials = %d body=%s", response.Code, response.Body.String())
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
