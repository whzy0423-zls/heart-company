# Seedance 2.0 Novice Video Workflow Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a beginner-first seven-step video production workflow that keeps the existing intermediary API, accurately adapts to Seedance 2.0 capabilities, guides prompts, generates/selects shot versions, and composes a final video.

**Architecture:** Add a versioned model-capability and gateway-contract layer in `internal/video`, then build a versioned project workflow domain in `internal/videoproject`. Keep the existing advanced workbench as a power-user route while making a new modular Vue wizard the default project workbench. All quota-consuming video-generation submissions are persisted before POST and use an explicit request-key state machine.

**Tech Stack:** Go 1.22, PostgreSQL schema migrations, `net/http`, existing MiniMax/OpenAI-compatible intermediary clients, Vue 3, TypeScript, Ant Design Vue/Vben, Vitest, FFmpeg, pnpm workspace.

**Design sources:**

- Spec: `docs/superpowers/specs/2026-07-10-seedance2-novice-video-workflow-design.md`
- Seedance API: <https://www.volcengine.com/docs/82379/1520757>
- Seedance prompt guide: <https://www.volcengine.com/docs/82379/2222480>
- UI rules from `ui-ux-pro-max`: one primary action per step, 44px controls, visible labels, inline recovery, responsive 375/768/1024/1440 layouts, existing semantic theme tokens, Lucide icons only.

---

## File Structure

### Backend foundation

- Create: `nx-backend/apps/server/internal/video/capabilities.go`
- Create: `nx-backend/apps/server/internal/video/capabilities_test.go`
- Create: `nx-backend/apps/server/internal/video/gateway_contract.go`
- Create: `nx-backend/apps/server/internal/video/gateway_contract_test.go`
- Create: `nx-backend/apps/server/internal/video/submission.go`
- Create: `nx-backend/apps/server/internal/video/submission_test.go`
- Modify: `nx-backend/apps/server/internal/video/video.go`
- Modify: `nx-backend/apps/server/internal/video/video_test.go`
- Modify: `nx-backend/apps/server/internal/config/env.go`
- Modify: `nx-backend/apps/server/internal/modelconfig/model_config.go`
- Modify: `nx-backend/apps/server/internal/modelconfig/model_config_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

### Prompt and project workflow domain

- Create: `nx-backend/apps/server/internal/videoproject/seedance_prompt.go`
- Create: `nx-backend/apps/server/internal/videoproject/seedance_prompt_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/promptbuilder.go`
- Modify: `nx-backend/apps/server/internal/videoproject/promptbuilder_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_models.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_schema_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_migration.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_migration_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_script.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_script_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_breakdown.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_breakdown_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_assets.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_assets_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_storyboard.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_storyboard_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_hashes.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_hashes_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_status.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_status_test.go`
- Create: `nx-backend/apps/server/internal/llm/video_project_workflow.go`
- Create: `nx-backend/apps/server/internal/llm/video_project_workflow_test.go`
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_routes.go`
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_content_routes.go`
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_content_routes_test.go`
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_generation_routes.go`
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_generation_routes_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/generator.go`
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject.go`
- Modify: `nx-backend/apps/server/internal/videoproject/projectcomposer.go`
- Modify: `nx-backend/apps/server/internal/server/videoproject_routes.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`

### Frontend wizard

- Modify: `nx-backend/pnpm-lock.yaml`
- Modify: `nx-backend/apps/web-antd/package.json`
- Create: `nx-backend/apps/web-antd/playwright.config.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/video-workflow.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/video-workflow-types.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/index.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/model-config.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/workflow-state.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-loader.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-navigation.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-autopilot.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-generation-request-key.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-polling.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHeader.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowStepper.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AutopilotProgress.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHelpDrawer.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ScriptStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/BreakdownStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/BreakdownItemCard.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AssetsStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AssetCandidateCard.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/StoryboardStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/StoryboardShotEditor.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ReferenceRoleEditor.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/PromptStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/GenerateStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ShotVersionCard.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/SubmissionRecovery.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ComposeStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AdvancedSettingsDrawer.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/*.test.ts`
- Create: `nx-backend/apps/web-antd/e2e/video-workflow.spec.ts`
- Modify: `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/generate.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/generate.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/settings/model.vue`

---

## Preflight Baseline

Before Task 1, record the exact starting commit and baseline outputs without changing files:

```bash
set -o pipefail
cd /Users/wohenzaiyi/Desktop/nine-xing
git rev-parse HEAD > /tmp/nine-xing-seedance-baseline-commit
cd nx-backend/apps/server
go test ./internal/video ./internal/videoproject ./internal/server -count=1 | tee /tmp/nine-xing-seedance-baseline-go.txt
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video | tee /tmp/nine-xing-seedance-baseline-vitest.txt
```

If a later full-suite failure looks unrelated, reproduce the same command at the saved baseline commit in a temporary detached worktree. Classify it as pre-existing only when the same failure appears there. Never weaken or rewrite an assertion merely to make the new branch green.

## Chunk 1: Seedance Capability, Gateway and Prompt Foundation

### Task 1: Persist a typed, versioned intermediary gateway contract

**Files:**
- Modify: `nx-backend/apps/server/internal/config/env.go`
- Modify: `nx-backend/apps/server/internal/config/env_test.go`
- Modify: `nx-backend/apps/server/internal/modelconfig/model_config.go`
- Modify: `nx-backend/apps/server/internal/modelconfig/model_config_test.go`

- [ ] **Step 1: Write a failing environment-default test**

Add this focused assertion to `env_test.go`:

```go
func TestLoadDefaultsVideoGatewayContract(t *testing.T) {
    t.Setenv("VIDEO_GATEWAY_CONTRACT", "")
    env := Load()
    if env.Video.GatewayContract.Name != "legacy_flat_v1" {
        t.Fatalf("got %#v", env.Video.GatewayContract)
    }
    if env.Video.GatewayContract.Version != "1" {
        t.Fatalf("expected contract version 1")
    }
}
```

- [ ] **Step 2: Run only the new environment test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/config -run TestLoadDefaultsVideoGatewayContract -count=1
```

Expected: FAIL because `GatewayContract` does not exist.

- [ ] **Step 3: Add typed contract structs and environment parsing**

Use a full typed contract, not string-only mappings:

```go
type FieldEncoding struct {
    Name       string            `json:"name"`
    ValueType  string            `json:"valueType"` // string/int/bool
    ValueMap   map[string]string `json:"valueMap,omitempty"`
}

type ReferenceEncoding struct {
    Mode                string            `json:"mode"` // flat_arrays/content_items
    ImageField          string            `json:"imageField"`
    VideoField          string            `json:"videoField"`
    AudioField          string            `json:"audioField"`
    RoleFields          map[string]string `json:"roleFields,omitempty"`
    SupportsRoles       []string          `json:"supportsRoles"`
    RequiresTargetFirst bool              `json:"requiresTargetFirst"`
}

type MediaLimits struct {
    MaxImages            int     `json:"maxImages"`
    MaxVideos            int     `json:"maxVideos"`
    MaxAudios            int     `json:"maxAudios"`
    MaxVideoSecondsTotal float64 `json:"maxVideoSecondsTotal"`
    MaxAudioSecondsTotal float64 `json:"maxAudioSecondsTotal"`
}

type IdempotencyContract struct {
    Header string `json:"header"`
}

type ReconciliationContract struct {
    LookupByRequestKey bool     `json:"lookupByRequestKey"`
    Method             string   `json:"method"` // GET only in first release
    PathTemplate       string   `json:"pathTemplate"` // e.g. /v1/videos/by-request/{requestKey}
    TaskIDPaths        []string `json:"taskIdPaths"`
    StatusPaths        []string `json:"statusPaths"`
}

type GatewayContractConfig struct {
    Name           string               `json:"name"`
    Version        string               `json:"version"`
    DeclaredModes  []string             `json:"declaredModes"`
    Duration       FieldEncoding        `json:"duration"`
    AspectRatio    FieldEncoding        `json:"aspectRatio"`
    Resolution     FieldEncoding        `json:"resolution"`
    GenerateAudio  FieldEncoding        `json:"generateAudio"`
    TaskMode       FieldEncoding        `json:"taskMode"`
    References     ReferenceEncoding    `json:"references"`
    Limits         MediaLimits          `json:"limits"`
    Idempotency    IdempotencyContract  `json:"idempotency"`
    Reconciliation ReconciliationContract `json:"reconciliation"`
}
```

Add `ModelProfile` and `GatewayContract` to `config.VideoConfig`. Parse `VIDEO_MODEL_PROFILE`, `VIDEO_GATEWAY_CONTRACT`, `VIDEO_GATEWAY_CONTRACT_VERSION` and `VIDEO_GATEWAY_CONTRACT_JSON`. The built-in legacy contract declares only current flat-array fields and current conservative limits.

- [ ] **Step 4: Run the environment test and verify GREEN**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/config -run TestLoadDefaultsVideoGatewayContract -count=1
```

Expected: PASS.

- [ ] **Step 5: Write a failing model-config round-trip and merge test**

Add an exact JSON round-trip test with a `content_items` contract containing all role fields, media limits and idempotency lookup. Assert empty incoming API key preserves the stored key and incoming contract replaces the prior non-secret contract.

- [ ] **Step 6: Run the model-config test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/modelconfig -run 'TestVideoGatewayContractRoundTrip|TestMergeIncomingVideoContract' -count=1
```

Expected: FAIL until persistence fields are added.

- [ ] **Step 7: Persist and trim the typed contract**

Extend `modelconfig.VideoConfig`, `ApplyVideo`, `MergeIncoming` and `trimmed` until the round-trip/merge tests pass.

- [ ] **Step 8: Run the round-trip tests and verify GREEN**

Run the Step 6 command. Expected: PASS.

- [ ] **Step 9: Write failing typed-contract rejection tests**

Use one table test with exact invalid inputs and codes:

```go
cases := []struct{ name string; mutate func(*GatewayContractConfig); code string }{
    {"unsafe field", func(c *GatewayContractConfig){ c.Duration.Name = "content[0]" }, "invalid_field_name"},
    {"bad type", func(c *GatewayContractConfig){ c.Duration.ValueType = "object" }, "invalid_value_type"},
    {"bad role", func(c *GatewayContractConfig){ c.References.SupportsRoles = []string{"magic_role"} }, "invalid_reference_role"},
    {"authorization header", func(c *GatewayContractConfig){ c.Idempotency.Header = "Authorization" }, "reserved_header"},
    {"newline header", func(c *GatewayContractConfig){ c.Idempotency.Header = "X-Key\nInjected" }, "invalid_header_name"},
    {"generation reconcile method", func(c *GatewayContractConfig){ c.Reconciliation.Method = "POST" }, "invalid_reconciliation_method"},
    {"unsafe reconcile path", func(c *GatewayContractConfig){ c.Reconciliation.PathTemplate = "https://evil.example/{requestKey}" }, "invalid_reconciliation_path"},
}
```

- [ ] **Step 10: Run rejection tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/modelconfig -run TestValidateVideoGatewayContractRejectsUnsafeConfig -count=1
```

Expected: FAIL.

- [ ] **Step 11: Implement typed contract validation**

Validate field names against `^[A-Za-z0-9_.-]+$`, value types against `string/int/bool`, roles against the internal role allow-list, and idempotency headers against RFC token characters plus a reserved-header deny-list.

- [ ] **Step 12: Run all focused config tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/config ./internal/modelconfig -count=1
```

Expected: PASS.

- [ ] **Step 13: Commit**

```bash
git add nx-backend/apps/server/internal/config nx-backend/apps/server/internal/modelconfig
git commit -m "feat(video): configure Seedance gateway contracts"
```

### Task 2: Build the pure versioned effective capability registry

**Files:**
- Create: `nx-backend/apps/server/internal/video/capabilities.go`
- Create: `nx-backend/apps/server/internal/video/capabilities_test.go`

- [ ] **Step 1: Write failing table-driven capability tests**

Cover exact model IDs, explicit custom profile, unknown fallback and contract intersection:

```go
func TestResolveCapabilitiesUsesOfficialProfileIntersection(t *testing.T) {
    got := ResolveCapabilities(CapabilityConfig{
        Model: "video-ds-2.0",
        GatewayContract: LegacyFlatContract(),
    })
    if got.ModelProfile != "standard" { t.Fatal(got.ModelProfile) }
    if got.SupportsResolution { t.Fatal("legacy contract must hide resolution") }
    if got.SupportsEdit || got.SupportsExtend { t.Fatal("flat arrays cannot encode targets") }
    if got.CapabilityVersion == "" { t.Fatal("missing version") }
    if got.Limits.MaxImages != 4 { t.Fatalf("legacy contract should keep conservative proven limit, got %+v", got.Limits) }
}

func TestResolveCapabilitiesUnknownModelUsesGenericProfile(t *testing.T) {
    got := ResolveCapabilities(CapabilityConfig{Model: "custom-video"})
    if got.Source.OfficialProfile != "generic_unknown" { t.Fatal(got.Source) }
    if got.SupportsSmartDuration { t.Fatal("unknown capability must fail closed") }
}
```

Add exact cases for `standard`, `fast`, `mini`, explicit custom profile and `generic_unknown`. Assert supported values, per-media limits, total-duration limits, roles and a degradation reason for every hidden official feature. Resolve the same input twice and assert identical `capabilityVersion`; change one contract field and assert a new version.

- [ ] **Step 2: Run test to verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run 'TestResolveCapabilities' -count=1
```

Expected: FAIL with undefined capability types/functions.

- [ ] **Step 3: Implement only the pure registry and deterministic version hash**

Implement immutable official profiles and contract intersection. Compute `capabilityVersion` as SHA-256 of canonical JSON containing official profile version, gateway contract name/version/body, model and explicit profile. Do not add HTTP routing in this task.

- [ ] **Step 4: Run registry tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run 'TestResolveCapabilities' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/video/capabilities.go nx-backend/apps/server/internal/video/capabilities_test.go
git commit -m "feat(video): resolve effective Seedance capabilities"
```

### Task 2B: Expose capabilities and enforce a shared request validator

**Files:**
- Create: `nx-backend/apps/server/internal/video/request.go`
- Create: `nx-backend/apps/server/internal/video/validator.go`
- Create: `nx-backend/apps/server/internal/video/validator_test.go`
- Create: `nx-backend/apps/server/internal/server/video_capabilities_routes.go`
- Create: `nx-backend/apps/server/internal/server/video_capabilities_routes_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write a failing validator table test**

```go
func TestValidateGenerateRequestRejectsStaleCapabilityVersion(t *testing.T) {
    caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
    req := GenerateRequest{Model: caps.Model, CapabilityVersion: "old-version", Duration: 15, AspectRatio: "16:9"}
    err := ValidateGenerateRequest(req, caps)
    assertValidationCode(t, err, "capability_version_stale")
}
```

Add exact cases for unsupported `seed`, `camera_fixed`, resolution/audio, duration, aspect, role, per-type counts, total video seconds and total audio seconds. A missing media duration yields a warning object, not an invented value.

Add direct-request target predicates independent of prompt compilation:

```go
// reference mode: no edit/extend targets
// edit mode: exactly one edit_target, zero extend_target
// extend mode: exactly one extend_target, zero edit_target
// any mixed target roles: mixed_target_roles
// taskMode=edit with only extend_target: edit_target_required
```

Assert these return typed validation codes before gateway mapping.

- [ ] **Step 2: Run the validator test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run TestValidateGenerateRequest -count=1
```

Expected: FAIL because the validator is undefined.

- [ ] **Step 3: Add normalized request/reference types and the pure shared validator**

```go
type Reference struct {
    ID              string   `json:"id"`
    Kind            string   `json:"kind"`
    Role            string   `json:"role"`
    URL             string   `json:"url"`
    SortOrder       int      `json:"sortOrder"`
    SourceType      string   `json:"sourceType"`
    SourceID        string   `json:"sourceId"`
    DurationSeconds *float64 `json:"durationSeconds,omitempty"`
}

type GenerateRequest struct {
    Model             string      `json:"model"`
    Prompt            string      `json:"prompt"`
    Duration          int         `json:"duration"`
    AspectRatio       string      `json:"aspectRatio"`
    Resolution        string      `json:"resolution"`
    GenerateAudio     *bool       `json:"generateAudio"`
    TaskMode          string      `json:"taskMode"`
    References        []Reference `json:"references"`
    RequestKey        string      `json:"requestKey"`
    CapabilityVersion string      `json:"capabilityVersion"`
    Seed              *int        `json:"seed,omitempty"`
    CameraFixed       *bool       `json:"cameraFixed,omitempty"`
}
```

Return typed errors with `Code`, `Field`, `Message`, `Fix` and optional latest capabilities. Make the function the only parameter validator used by single generation, project generation and batch generation adapters.

- [ ] **Step 4: Run validator tests and verify GREEN**

Run the same command. Expected: PASS.

- [ ] **Step 5: Write a failing capability route test**

Test `GET /api/video/capabilities?model=video-ds-2.0-fast`, permission access, no secret fields, and `capabilityVersion/source/degradations` in JSON.

- [ ] **Step 6: Run the route test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server -run TestVideoCapabilitiesRoute -count=1
```

Expected: FAIL because the route is absent.

- [ ] **Step 7: Implement the focused route file and register it**

Do not add handler logic to the already large `server.go`; only register the focused handler there.

- [ ] **Step 8: Run route and validator tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video ./internal/server -run 'TestValidateGenerateRequest|TestVideoCapabilitiesRoute' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add nx-backend/apps/server/internal/video/request.go nx-backend/apps/server/internal/video/validator* nx-backend/apps/server/internal/server/video_capabilities_routes* nx-backend/apps/server/internal/server/server.go
git commit -m "feat(video): validate and expose effective capabilities"
```

### Task 3A: Define canonical references and deterministic numbering

**Files:**
- Create: `nx-backend/apps/server/internal/video/references.go`
- Create: `nx-backend/apps/server/internal/video/references_test.go`

- [ ] **Step 1: Write a failing canonical-order test**

Use the normalized `Reference` type from Task 2B, which includes a stable ID/tie-breaker and optional known media duration.

Exact test input must interleave kinds and equal sort orders:

```go
refs := []Reference{
    {ID:"30", Kind:"video", Role:"reference_video", URL:"v2", SortOrder:2},
    {ID:"20", Kind:"image", Role:"reference_image", URL:"i2", SortOrder:1},
    {ID:"10", Kind:"image", Role:"first_frame", URL:"i1", SortOrder:1},
    {ID:"40", Kind:"audio", Role:"reference_audio", URL:"a1", SortOrder:0},
    {ID:"50", Kind:"video", Role:"edit_target", URL:"v1", SortOrder:1},
}
```

Assert canonical order is `a1,i1,i2,v1,v2`, while per-kind ordinals are `音频1, 图片1, 图片2, 视频1, 视频2`. Add a second test proving the same URL with different roles remains distinct, and an exact duplicate `(kind,role,sourceType,sourceID,url)` returns `duplicate_reference`.

- [ ] **Step 2: Run the reference test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run 'TestCanonicalizeReferences|TestDuplicateReference' -count=1
```

Expected: FAIL because canonicalization is undefined.

- [ ] **Step 3: Implement one shared canonicalization function**

Return both globally ordered references and per-kind ordinal metadata. Sort by `sortOrder`, then stable numeric ID when possible, then lexical ID. Never URL-deduplicate.

- [ ] **Step 4: Run reference tests and verify GREEN**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run 'TestCanonicalizeReferences|TestDuplicateReference' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/video/references.go nx-backend/apps/server/internal/video/references_test.go
git commit -m "feat(video): canonicalize Seedance references"
```

### Task 3B: Map normalized requests and prevent create POST retries

**Files:**
- Create: `nx-backend/apps/server/internal/video/gateway_contract.go`
- Create: `nx-backend/apps/server/internal/video/gateway_contract_test.go`
- Modify: `nx-backend/apps/server/internal/video/video.go`
- Modify: `nx-backend/apps/server/internal/video/video_test.go`

- [ ] **Step 1: Write a failing legacy payload test with exact JSON**

```go
want := `{"aspect_ratio":"9:16","audios":["a1"],"images":["i1","i2"],"model":"video-ds-2.0-fast","prompt":"雨夜车站","seconds":"15","videos":["v1","v2"]}`
```

Canonicalize map keys before comparing. Assert no resolution, audio toggle, task mode or role fields exist under `legacy_flat_v1`.

- [ ] **Step 2: Run the legacy mapper test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run TestMapLegacyGatewayPayload -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement legacy mapping only and verify GREEN**

Keep old `Images/Videos/Audios` as an adapter into generic references. The mapper consumes only canonical references.

- [ ] **Step 4: Write failing configured-contract role tests**

Add exact cases for declared resolution/audio value encodings, content-item role encoding, `edit_target` at `视频2`, target-first rejection, and a role absent from the contract. Assert prompt ordinals and payload arrays/content items use the same canonical result object.

- [ ] **Step 5: Run configured mapper tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run 'TestMapConfiguredGateway|TestTargetFirstContract' -count=1
```

Expected: FAIL.

- [ ] **Step 6: Implement configured mapping and verify GREEN**

Use only declared field names/types/value maps. Do not silently coerce unsupported roles.

- [ ] **Step 7: Write failing idempotency-header tests**

Use two subtests around `CreateTask`:

```go
// contract.Idempotency.Header = "Idempotency-Key"
// requestKey = "req-123"
// expect request header exactly "req-123"

// contract.Idempotency.Header = ""
// expect no Idempotency-Key or X-Request-Key header
```

Both subtests use one-shot POST behavior and assert no other configured field becomes a header.

- [ ] **Step 8: Run idempotency-header tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run TestCreateTaskUsesDeclaredIdempotencyHeader -count=1
```

Expected: FAIL.

- [ ] **Step 9: Wire the declared header to the same `requestKey`**

Do not synthesize a different key and do not send a header when the contract leaves it blank.

- [ ] **Step 10: Write a failing one-shot POST test**

Use an `httptest.Server` that increments a counter, hijacks/closes the first connection after reading the body, and assert `CreateTask` returns an ambiguous error with `counter == 1`.

- [ ] **Step 11: Run the POST test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run TestCreateTaskDoesNotRetryPost -count=1
```

Expected: FAIL because current `doJSON` retries.

- [ ] **Step 12: Switch only create POST to one-shot JSON**

Keep safe GET polling retry behavior unchanged.

- [ ] **Step 13: Run all mapper/video tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -count=1
```

Expected: PASS.

- [ ] **Step 14: Commit**

```bash
git add nx-backend/apps/server/internal/video/gateway_contract* nx-backend/apps/server/internal/video/video.go nx-backend/apps/server/internal/video/video_test.go
git commit -m "feat(video): map Seedance requests without POST retries"
```

### Task 3C: Route every generation entry through the shared validator

**Files:**
- Modify: `nx-backend/apps/server/internal/video/video.go`
- Modify: `nx-backend/apps/server/internal/video/video_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/generator.go`
- Modify: `nx-backend/apps/server/internal/videoproject/generator_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/batchgenerator.go`
- Modify: `nx-backend/apps/server/internal/videoproject/generator_test.go`
- Modify: `nx-backend/apps/server/internal/server/server_unit_test.go`

- [ ] **Step 1: Write a failing direct-store stale-version test**

Call `Store.Generate` with an old capability version and assert `capability_version_stale` before the fake upstream is called.

- [ ] **Step 2: Run it and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run TestGenerateRejectsStaleCapabilityVersion -count=1
```

- [ ] **Step 3: Call the shared validator from `Store.Generate` and verify GREEN**

The store resolves current capabilities from its immutable runtime config. Normalized endpoints require the client version; legacy wrappers may explicitly inject the current version server-side and are labeled compatibility paths.

- [ ] **Step 4: Write failing project single/batch adapter tests**

Assert both adapters pass model, duration, aspect ratio, resolution, audio mode, task mode, ordered references and capability version into the same `GenerateRequest`. An unsupported role must fail before either adapter calls upstream.

- [ ] **Step 5: Run adapter tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestGenerateShotUsesNormalizedRequest|TestBatchGenerateUsesNormalizedRequest' -count=1
```

- [ ] **Step 6: Implement thin adapters and verify GREEN**

Remove duplicate duration/aspect validation from project paths. They may add project context but cannot change capability rules.

- [ ] **Step 7: Run all affected tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video ./internal/videoproject ./internal/server -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/video/video.go nx-backend/apps/server/internal/video/video_test.go nx-backend/apps/server/internal/videoproject/generator.go nx-backend/apps/server/internal/videoproject/generator_test.go nx-backend/apps/server/internal/videoproject/batchgenerator.go nx-backend/apps/server/internal/server/server_unit_test.go
git commit -m "refactor(video): share Seedance request validation"
```

### Task 4A: Persist the generation submission state machine and database locks

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/video/submission.go`
- Create: `nx-backend/apps/server/internal/video/submission_test.go`

- [ ] **Step 1: Write failing submission state tests**

Use a table test for these allowed transitions and separate rejected-transition cases:

```go
cases := []struct{ from, to string }{
    {"prepared", "submitting"},
    {"submitting", "accepted"},
    {"accepted", "completed"},
    {"accepted", "failed"},
    {"submitting", "unknown_outcome"},
    {"unknown_outcome", "reconciled"},
    {"prepared", "cancelled"},
}
```

Assert one active submission per shot, one row per `request_key`, same key returns the existing row, terminal state releases the active lock, and a deliberate “再生成一个版本” with a new key succeeds only after terminal state.

Add a schema contract test for a unique request key and a partial unique index on active `shot_id` states.

- [ ] **Step 2: Run test to verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run 'TestSubmission' -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement schema constraints and the submission store only**

Implement `Prepare`, `Transition`, `GetByRequestKey`, `FindActiveByShot` and `Reconcile`. Use compare-and-swap status predicates. Do not call the gateway in this task.

Never log API keys or signed URLs. Request snapshots use the same access control as video generations.

- [ ] **Step 4: Run tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video ./internal/db -run 'TestSubmission|TestVideoGenerationSubmissionSchema' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/video/submission*
git commit -m "feat(video): 持久化视频生成提交状态"
```

### Task 4B: Integrate generation submission, ambiguous outcomes and reconciliation

**Files:**
- Modify: `nx-backend/apps/server/internal/video/video.go`
- Modify: `nx-backend/apps/server/internal/video/video_test.go`
- Modify: `nx-backend/apps/server/internal/video/submission.go`
- Modify: `nx-backend/apps/server/internal/video/submission_test.go`

- [ ] **Step 1: Write a failing duplicate-request integration test**

Call `Store.Generate` twice with the same `requestKey`; the fake upstream must receive one POST and both calls return the same submission/generation identity.

- [ ] **Step 2: Run it and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run TestGenerateReusesRequestKey -count=1
```

Expected: FAIL.

- [ ] **Step 3: Wire prepare/submitting/accepted around the one-shot POST**

Return the existing row for duplicate keys. Reject a new key while the shot has an active submission. Mirror refresh success/failure into submission terminal states.

- [ ] **Step 4: Write a failing ambiguous transport test**

Assert one upstream POST, `unknown_outcome` state, a structured `UnknownOutcomeError{RequestKey: ...}`, and no normal retry path.

- [ ] **Step 5: Run it and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run TestGenerateStoresUnknownOutcome -count=1
```

Expected: FAIL.

- [ ] **Step 6: Implement unknown-outcome persistence and verify GREEN**

Run the same command. Expected: PASS with exactly one upstream POST.

- [ ] **Step 7: Write a failing local-linkage failure test**

Configure the fake database so the upstream returns `task-42` and the first local linkage update fails. Assert the returned typed error contains both `requestKey` and `task-42`, the prepared row remains reconcilable, and no second POST occurs.

- [ ] **Step 8: Run it and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run TestGenerateReturnsTaskIDWhenLocalLinkFails -count=1
```

Expected: FAIL.

- [ ] **Step 9: Write failing reconciliation semantics tests**

Add four exact cases:

```text
same requestKey + same taskID twice → one generation, success both times
same requestKey + different taskID → reconciliation_task_conflict
unknown/submitting row + taskID → reconciled and pollable
reconcile after a generation already exists → reuses that generation ID
```

- [ ] **Step 10: Run reconciliation tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -run TestReconcileSubmission -count=1
```

Expected: FAIL.

- [ ] **Step 11: Implement an idempotent reconciliation operation**

`Reconcile(ctx, requestKey, taskID)` attaches the same task ID repeatedly without duplicate generations, rejects a different task ID, and moves the row to `reconciled` before normal polling resumes. If the contract declares lookup by request key, a background/manual route may call that lookup; otherwise only an operator-supplied task ID can reconcile.

- [ ] **Step 12: Run all submission/video tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/video -count=1
```

Expected: PASS.

- [ ] **Step 13: Commit**

```bash
git add nx-backend/apps/server/internal/video/video.go nx-backend/apps/server/internal/video/video_test.go nx-backend/apps/server/internal/video/submission*
git commit -m "feat(video): 对账视频生成结果"
```

### Task 5A: Implement the pure Seedance prompt compiler and diagnostics

**Files:**
- Create: `nx-backend/apps/server/internal/videoproject/seedance_prompt.go`
- Create: `nx-backend/apps/server/internal/videoproject/seedance_prompt_test.go`

- [ ] **Step 1: Write exact failing golden tests for the three task modes**

Use this result contract:

```go
type PromptDiagnostic struct {
    Level   string `json:"level"`
    Code    string `json:"code"`
    Message string `json:"message"`
    Fix     string `json:"fix"`
}

type CompiledPrompt struct {
    Prompt            string             `json:"prompt"`
    PromptVersion     string             `json:"promptVersion"`
    RequestHash       string             `json:"requestHash"`
    DiagnosticsHash   string             `json:"diagnosticsHash"`
    Diagnostics       []PromptDiagnostic `json:"diagnostics"`
    OrderedReferences []video.Reference  `json:"orderedReferences"`
}
```

Use explicit inputs/outputs:

```go
func TestSeedancePromptReferenceGolden(t *testing.T) {
    in := PromptInput{
        Mode: "reference", Subject: "小夏", Action: "在雨夜车站快步回头",
        Scene: "蓝色霓虹灯下的站台", Camera: "缓慢跟拍",
        Dialogue: "别走", SoundEffect: "远处列车鸣笛",
        References: canonicalFixture(),
    }
    got := CompileSeedancePrompt(in, fullCaps())
    want := "参考图片1中的角色“小夏”外观，参考图片2中的雨夜车站，参考视频2的缓慢跟拍运镜，参考音频1的音色。小夏在蓝色霓虹灯下的站台快步回头，镜头缓慢跟拍，{别走}，<远处列车鸣笛>。保持无字幕、不要生成 Logo、不要生成水印。"
    if got.Prompt != want { t.Fatalf("got %q", got.Prompt) }
}
```

Add exact edit output `严格编辑视频2，将天空修改为黄昏，其余内容保持不变。...` and exact extend output `向后延长视频2，生成角色继续走入站台深处。...`. Require exactly one corresponding target, reject both target roles together, and never write “参考视频 N” for edit/extend.

- [ ] **Step 2: Run test to verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestSeedancePrompt' -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement only the pure compiler**

The compiler accepts structured shot data, the canonical reference result and effective capabilities. It never queries the database.

- [ ] **Step 4: Write failing diagnostic-boundary tests**

Assert these exact codes:

```text
missing_edit_target
multiple_edit_targets
mixed_target_roles
reference_number_missing
multiple_camera_movements
exact_time_segments_unstable
prompt_over_500_chinese_chars
unsupported_reference_role
```

Exactly 500 Chinese characters has no length warning; 501 has the warning. If input requests any `【字幕】`, omit `保持无字幕` but retain Logo/watermark constraints. Without subtitle request, include all three default constraints. Assert no forced English action dictionary, medium shot, static camera or animation style.

- [ ] **Step 5: Run diagnostic tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestSeedancePromptDiagnostics|TestSeedancePromptLengthBoundary' -count=1
```

- [ ] **Step 6: Implement diagnostics and verify GREEN**

Use rune counts for the 500-character guidance threshold. Diagnostics must include plain-language `Message` and `Fix`.

- [ ] **Step 7: Run pure compiler tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestSeedancePrompt' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/seedance_prompt.go nx-backend/apps/server/internal/videoproject/seedance_prompt_test.go
git commit -m "feat(video): compile Seedance prompt guidance"
```

### Task 5B: Adapt persisted shots and prove prompt/payload numbering is identical

**Files:**
- Modify: `nx-backend/apps/server/internal/videoproject/promptbuilder.go`
- Modify: `nx-backend/apps/server/internal/videoproject/promptbuilder_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/generator_test.go`

- [ ] **Step 1: Write a failing persisted-reference ordering test**

Create shot assets with equal `sort_order` and IDs `20,10`, interleaved image/video/audio kinds, and an edit target at video ordinal 2. Assert `BuildPreview` returns the same canonical ordered references and ordinals as `video.CanonicalizeReferences`.

- [ ] **Step 2: Run it and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestBuildPreviewUsesCanonicalReferenceOrder -count=1
```

- [ ] **Step 3: Replace new prompt construction with the pure compiler**

Remove blacklist, English action dictionary and forced animation defaults from `seedance2_v2`. Keep `legacy_v1` only when displaying a historical stored prompt. Return structured diagnostics plus legacy error/warning arrays during migration.

- [ ] **Step 4: Write a failing cross-layer golden test**

Compile a prompt, map the same canonical result through the gateway contract, and assert every `图片N/视频N/音频N` URL equals the corresponding final upstream array/content item. This test is the invariant preventing prompt/payload drift.

- [ ] **Step 5: Run it and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestBuildPreviewUsesCanonicalReferenceOrder|TestPromptIndicesMatchGatewayPayload' -count=1
```

Expected: FAIL.

- [ ] **Step 6: Implement the shared canonical result path**

Both prompt compilation and gateway mapping must receive the same immutable canonical-reference value; neither may sort independently.

- [ ] **Step 7: Run the focused tests and verify GREEN**

Run the Step 5 command again. Expected: PASS.

- [ ] **Step 8: Run all project tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/promptbuilder.go nx-backend/apps/server/internal/videoproject/promptbuilder_test.go nx-backend/apps/server/internal/videoproject/generator_test.go
git commit -m "feat(video): align prompt and gateway reference order"
```

## Chunk 2: Versioned Script, Assets and Storyboard Workflow Backend

### Task 6A: Add exact workflow tables, columns and database constraints

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_models.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_schema_test.go`

- [ ] **Step 1: Write a failing schema contract test with exact DDL fragments**

Require these additive project columns and defaults:

```sql
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_content TEXT NOT NULL DEFAULT '';
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS confirmed_script_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_step TEXT NOT NULL DEFAULT 'script';
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_mode TEXT NOT NULL DEFAULT 'guided' CHECK (workflow_mode IN ('guided','autopilot'));
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_settings JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_settings_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS asset_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_confirmed_at TIMESTAMPTZ;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS breakdown_confirmed_at TIMESTAMPTZ;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS storyboard_confirmed_at TIMESTAMPTZ;
```

Require character/scene columns `visual_prompt`, `source`, `status`, `required`, `breakdown_item_key`, `source_breakdown_id`; their status check permits `draft/confirmed/generating/ready/failed/detached`. Add unique indexes on non-empty `(project_id, breakdown_item_key)`.

Require new tables with explicit columns/checks/FKs:

```text
video_project_breakdowns:
  id, project_id FK CASCADE, version > 0, revision > 0,
  status CHECK draft/confirmed/superseded/failed,
  source_script_revision, script_snapshot,
  characters/scenes/props/outfits/styles/story_beats JSONB default [],
  raw_result/error_message/create_time/update_time

video_project_assets:
  id, project_id FK CASCADE, type CHECK prop/outfit/style,
  breakdown_item_key, source_breakdown_id FK SET NULL,
  name/description/visual_prompt/usage_note,
  required, global_asset_id FK SET NULL, reference_image_url,
  source CHECK ai/manual/library/legacy,
  status CHECK draft/confirmed/generating/ready/failed/detached,
  metadata JSONB, timestamps

video_project_asset_candidates:
  id, project_id FK CASCADE, target_type CHECK character/scene/prop/outfit/style,
  target_id, prompt, image_asset_id FK SET NULL, image_url,
  source CHECK generated/upload/library/legacy,
  generation_request_id, status CHECK queued/generating/ready/failed,
  error_message, selected, timestamps

video_project_storyboard_versions:
  id, project_id FK CASCADE, version > 0, revision > 0,
  status CHECK draft/confirmed/superseded/failed,
  source_script_revision, source_breakdown_id FK SET NULL,
  source_asset_revision, source_capability_version,
  baseline_storyboard_id FK SET NULL, shots JSONB default [],
  raw_result/error_message/create_time/update_time
```

Require exact indexes:

```sql
CREATE UNIQUE INDEX ... ON video_project_breakdowns(project_id, version);
CREATE UNIQUE INDEX ... ON video_project_breakdowns(project_id) WHERE status='confirmed';
CREATE UNIQUE INDEX ... ON video_project_storyboard_versions(project_id, version);
CREATE UNIQUE INDEX ... ON video_project_storyboard_versions(project_id) WHERE status='confirmed';
CREATE UNIQUE INDEX ... ON video_project_assets(project_id, breakdown_item_key) WHERE breakdown_item_key<>'';
CREATE UNIQUE INDEX ... ON video_project_asset_candidates(target_type, target_id) WHERE selected=true;
CREATE UNIQUE INDEX ... ON video_shots(project_id, source_key) WHERE source_key<>'' AND archived_at IS NULL;
CREATE INDEX ... ON video_shot_assets(shot_id, sort_order, id);
```

Require shot fields `generation_mode`, `prompt_override`, `prompt_version`, `audio_mode`, `prompt_diagnostics JSONB`, `source_key`, `archived_at`, `selected_generation_id FK video_generations SET NULL`, `selected_generation_ack_hash`; reference fields `reference_role`, `sort_order`, `source_type`, `source_id`, `usage_note`; and compose job `compose_input_hash`.

- [ ] **Step 2: Run the schema test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestWorkflowSchemaContract -count=1
```

Expected: FAIL on the first missing fragment.

- [ ] **Step 3: Implement the additive DDL only**

Use `ADD COLUMN IF NOT EXISTS`, `DO $$` guards for retrofitted checks where required, and deterministic constraint/index names. Equal sort orders are valid and `id` is the canonical tie-breaker. Do not add the exact duplicate-reference unique index until Task 6B has assigned stable legacy source IDs and handled old duplicates.

- [ ] **Step 4: Run the schema test and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Add focused persisted model types**

```go
type WorkflowStepStatus struct {
    Key        string            `json:"key"`
    Status     string            `json:"status"`
    Progress   int               `json:"progress"`
    Blockers   []WorkflowMessage `json:"blockers"`
    Warnings   []WorkflowMessage `json:"warnings"`
    Evidence   map[string]any    `json:"evidence"`
    NextAction string            `json:"nextAction"`
}

type WorkflowOverview struct {
    Project      Project              `json:"project"`
    CurrentStep  string               `json:"currentStep"`
    Overall      int                  `json:"overall"`
    Steps        []WorkflowStepStatus `json:"steps"`
    Capabilities video.Capabilities   `json:"capabilities"`
}
```

Add separate types for `BreakdownVersion`, `ProjectAsset`, `AssetCandidate`, `StoryboardVersion`, `StoryboardDiff` and conflict errors. Do not place database methods in the model file.

- [ ] **Step 6: Run package compile tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestWorkflowSchemaContract -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/videoproject/workflow_models.go nx-backend/apps/server/internal/videoproject/workflow_schema_test.go
git commit -m "feat(video): add project workflow schema"
```

### Task 6B: Backfill old projects and implement dual-read compatibility

**Files:**
- Create: `nx-backend/apps/server/internal/videoproject/workflow_migration.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_migration_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`

- [ ] **Step 1: Write a failing idempotent migration test**

Seed an old project with character/scene reference images, shots, same-timestamp shot assets, legacy reference modes, and generations in `queued`, `failed`, `completed`, `succeeded`. Run migration twice and assert identical final rows.

Expected exact rules:

```text
character key = legacy:character:<id>
scene key = legacy:scene:<id>
required = referenced by an unarchived shot
non-empty character/scene reference URL → one legacy candidate, ready + selected
old image/video/audio shot asset → reference_image/reference_video/reference_audio
sort_order = row_number() over (partition by shot_id order by create_time,id)
source_type = legacy_shot_asset; source_id = original shot asset id
selected_generation_id backfills only completed/succeeded current generation
```

- [ ] **Step 2: Run migration test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestWorkflowMigrationIsIdempotent -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement data backfill and verify GREEN**

Use idempotent `INSERT ... ON CONFLICT DO NOTHING`/guarded updates. Never overwrite a user-selected candidate or selected generation. After every old reference has a stable source ID, add the exact-identity unique expression index on `(shot_id, asset_type, reference_role, COALESCE(source_type,''), COALESCE(source_id,''), object_url)`; the migration test must prove pre-existing same-URL rows remain distinguishable by source ID and a third run is still unchanged.

- [ ] **Step 4: Write failing dual-read expansion tests**

Assert new explicit references win. Only when none exist, expand:

```text
prev_frame → first_frame
character_ref/scene_ref → reference_image
prev_video/scene_demo → reference_video
```

Use stable source IDs and run through the same canonical ordering function. Assert legacy arrays remain written for old-page display during the transition.

- [ ] **Step 5: Run dual-read tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestLegacyReferenceDualRead -count=1
```

- [ ] **Step 6: Implement the compatibility reader and verify GREEN**

Do not delete old mode fields or old snapshot arrays.

- [ ] **Step 7: Run migration/compatibility tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject ./internal/db -run 'TestWorkflowMigration|TestLegacyReferenceDualRead' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/videoproject/workflow_migration* nx-backend/apps/server/internal/videoproject/videoproject.go
git commit -m "feat(video): migrate legacy project workflows"
```

### Task 7A: Add the AI script-polish call and parser

**Files:**
- Create: `nx-backend/apps/server/internal/llm/video_project_workflow.go`
- Create: `nx-backend/apps/server/internal/llm/video_project_workflow_test.go`

- [ ] **Step 1: Write a failing script-polish HTTP/parser test**

Assert the request uses the existing conversation intermediary and a dedicated system prompt, then parses:

```go
type ScriptResult struct {
    Content string `json:"content"`
    Summary string `json:"summary"`
    Style   string `json:"style"`
}
```

Cover fenced JSON, think blocks and plain-text fallback while preserving the raw model response.

- [ ] **Step 2: Run the script test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/llm -run TestPolishVideoProjectScript -count=1
```

- [ ] **Step 3: Implement `PolishVideoProjectScript` only**

Reuse the safe configured client/timeout. Do not add breakdown/storyboard behavior yet.

- [ ] **Step 4: Run the script test and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/llm/video_project_workflow.go nx-backend/apps/server/internal/llm/video_project_workflow_test.go
git commit -m "feat(video): polish project scripts with AI"
```

### Task 7B: Parse and persist versioned AI breakdown drafts

**Files:**
- Modify: `nx-backend/apps/server/internal/llm/video_project_workflow.go`
- Modify: `nx-backend/apps/server/internal/llm/video_project_workflow_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_breakdown.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_breakdown_test.go`

- [ ] **Step 1: Write failing breakdown normalization tests**

```go
type BreakdownItem struct {
    Key, Name, Description, VisualPrompt, UsageNote string
    Required bool
    Decision string // pending/confirmed/ignored
}
```

Cover fenced JSON, think blocks, arrays-as-strings, missing optional fields and duplicate names. Omitted/duplicate keys normalize deterministically from category + ordinal + content hash; repeated parsing of identical raw output returns identical distinct keys. New items default to `pending`.

- [ ] **Step 2: Run parser tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/llm -run TestProjectBreakdown -count=1
```

- [ ] **Step 3: Implement only `BreakdownVideoProjectScript` and verify GREEN**

Return `(ProjectBreakdownResult, raw string, error)` so callers can persist failures.

- [ ] **Step 4: Write failing draft-persistence tests**

Assert serialized next-version allocation, `source_script_revision`, script snapshot, `draft` cleaned JSON/raw result on success, and `failed + raw_result + error_message` on model/parse failure. A second request creates a new version; it never overwrites the first.

- [ ] **Step 5: Run persistence tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestCreateBreakdownDraft -count=1
```

- [ ] **Step 6: Implement the focused breakdown draft service**

Use project-scoped serialization for `MAX(version)+1`; new draft revision starts at 1.

- [ ] **Step 7: Run focused tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/llm ./internal/videoproject -run 'TestProjectBreakdown|TestCreateBreakdownDraft' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/llm/video_project_workflow* nx-backend/apps/server/internal/videoproject/workflow_breakdown*
git commit -m "feat(video): persist AI breakdown drafts"
```

### Task 7C: Parse and persist versioned AI storyboard drafts

**Files:**
- Modify: `nx-backend/apps/server/internal/llm/video_project_workflow.go`
- Modify: `nx-backend/apps/server/internal/llm/video_project_workflow_test.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_storyboard.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_storyboard_test.go`

- [ ] **Step 1: Write failing storyboard parser tests**

Require stable `sourceKey`, duration, scene/character/asset keys, action, camera, composition, lighting, audio, dialogue, task mode and reference intentions. Missing/duplicate keys normalize deterministically by ordinal + content hash. Preserve raw failure output.

- [ ] **Step 2: Run parser tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/llm -run TestProjectStoryboard -count=1
```

- [ ] **Step 3: Implement `DesignVideoProjectStoryboard` parser/call**

Input carries confirmed script revision, breakdown ID, asset revision, capability version and confirmed asset summaries.

- [ ] **Step 4: Write failing storyboard-draft persistence tests**

Assert serialized next-version allocation, dependency snapshots, confirmed-baseline ID, raw failure persistence and no overwrite of older drafts.

- [ ] **Step 5: Run persistence tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestCreateStoryboardDraft -count=1
```

- [ ] **Step 6: Implement focused storyboard draft persistence**

Use the same project-scoped version allocation strategy as breakdown drafts.

- [ ] **Step 7: Run tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/llm ./internal/videoproject -run 'TestProjectStoryboard|TestCreateStoryboardDraft' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/llm/video_project_workflow* nx-backend/apps/server/internal/videoproject/workflow_storyboard*
git commit -m "feat(video): persist AI storyboard drafts"
```

### Task 8A: Confirm script revisions and materialize breakdowns without name merging

**Files:**
- Create: `nx-backend/apps/server/internal/videoproject/workflow_script.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_script_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/workflow_breakdown.go`
- Modify: `nx-backend/apps/server/internal/videoproject/workflow_breakdown_test.go`

- [ ] **Step 1: Write failing script revision tests**

Assert save increments `script_revision` only on content change; no-op save is idempotent; confirm requires `expectedRevision == script_revision`; stale confirm returns `workflow_revision_conflict`; confirm sets revision/timestamp without deleting downstream rows.

- [ ] **Step 2: Run script tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestScriptRevision -count=1
```

- [ ] **Step 3: Implement focused script save/confirm methods and verify GREEN**

- [ ] **Step 4: Write failing breakdown diff/confirmation tests**

```go
type BreakdownMapping struct {
    ItemKey, Decision, ExistingKind, ExistingID string
}
type ConfirmBreakdownInput struct {
    ExpectedRevision int
    DiffToken string
    Mappings []BreakdownMapping
}
```

The diff token hashes breakdown ID/revision, current confirmed baseline, current script revision, item keys and explicit mappings. Require every item confirmed/ignored, same-project/same-kind existing IDs, stale revision/token conflicts, distinct duplicate names, and no unspecified name matching.

- [ ] **Step 5: Run confirmation tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestPreviewBreakdownDiff|TestConfirmBreakdown' -count=1
```

- [ ] **Step 6: Implement preview and transactional materialization**

Supersede the old confirmed version, create/update only through stable keys/mappings, mark unmapped old assets detached, increment `asset_revision` once, and use `WHERE revision=$expected`.

- [ ] **Step 7: Run focused tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestScriptRevision|TestPreviewBreakdownDiff|TestConfirmBreakdown' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/workflow_script* nx-backend/apps/server/internal/videoproject/workflow_breakdown*
git commit -m "feat(video): confirm scripts and breakdown mappings"
```

### Task 8B: Define exact workflow hashes from normalized requests

**Files:**
- Create: `nx-backend/apps/server/internal/videoproject/workflow_hashes.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_hashes_test.go`

- [ ] **Step 1: Write exact stability and mutation tests**

`ShotRequestHash` hashes the exact normalized `video.GenerateRequest` used by Chunk 1 after removing only `requestKey`. `DiagnosticsHash` hashes shot content + canonical references + capability version + compiler version. Table mutations prove every relevant field changes the hash while timestamps/map insertion order do not.

Also test `SelectionAckHash(currentRequestHash,generationID)` and `ComposeInputHash(orderedSelectedGenerationIDs,settings)` for stability and changes on order/selection/settings.

- [ ] **Step 2: Run hash tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestShotRequestHash|TestDiagnosticsHash|TestSelectionAckHash|TestComposeInputHash' -count=1
```

- [ ] **Step 3: Implement pure hashes using the shared request/canonical JSON**

Do not create a second request representation.

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/workflow_hashes*
git commit -m "feat(video): hash workflow dependencies deterministically"
```

### Task 8C: Compute seven authoritative workflow predicates and invalidation

**Files:**
- Create: `nx-backend/apps/server/internal/videoproject/workflow_status.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_status_test.go`

- [ ] **Step 1: Write a table test for all seven step predicates**

Cover complete, blocked, stale and `skipped_existing` states. Evidence includes the exact IDs/revisions/hashes/capability version used. Target-duration variance is warning-only.

- [ ] **Step 2: Run predicate tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestWorkflowStatus -count=1
```

- [ ] **Step 3: Implement computed predicates only**

Do not persist derived progress.

- [ ] **Step 4: Write failing invalidation tests**

```text
script edit → downstream dependency mismatch
new breakdown confirm → asset revision changes
candidate selection → storyboard/prompt stale
shot/model/reference order → diagnostics hash stale
selection/order/compose settings → compose hash stale
legacy shots without script → skipped_existing evidence
```

- [ ] **Step 5: Run invalidation tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestWorkflowInvalidation -count=1
```

- [ ] **Step 6: Implement invalidation by comparison, preserving old data**

- [ ] **Step 7: Run status tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestWorkflowStatus|TestWorkflowInvalidation' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/workflow_status*
git commit -m "feat(video): compute authoritative workflow progress"
```

### Task 9A: Implement asset candidate persistence, recovery and idempotent selection

**Files:**
- Create: `nx-backend/apps/server/internal/videoproject/workflow_assets.go`
- Create: `nx-backend/apps/server/internal/videoproject/workflow_assets_test.go`

- [ ] **Step 1: Write failing candidate store tests**

Cover character, scene, prop, outfit and style targets. Assert:

- list returns persisted generating/failed/ready history;
- selecting ready public URL atomically clears old selection, updates compatibility URL and increments `assetRevision` once;
- selecting the already-selected candidate is idempotent and does not increment revision;
- non-ready/private URL selection is rejected;
- two concurrent selections cannot leave two selected rows;
- creating a retry candidate preserves the failed row.

- [ ] **Step 2: Run candidate tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestAssetCandidateStore|TestSelectAssetCandidate' -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement store operations and verify GREEN**

Use a transaction and the database partial unique index for selection.

- [ ] **Step 4: Write a failing interrupted-generation recovery test**

Seed `generating` candidates older than a configured timeout. `RecoverStaleCandidates(now, timeout)` must mark them `failed` with an interruption message, keep their prompt/history, and be idempotent. Recent generating candidates remain unchanged.

- [ ] **Step 5: Run recovery test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestRecoverStaleAssetCandidates -count=1
```

- [ ] **Step 6: Implement explicit recovery and verify GREEN**

Never auto-create a replacement candidate or charge again.

- [ ] **Step 7: Run asset store tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestAssetCandidate|TestRecoverStaleAssetCandidates' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/workflow_assets*
git commit -m "feat(video): persist and recover asset candidates"
```

### Task 9B: Generate/upload project asset candidates through existing services

**Files:**
- Modify: `nx-backend/apps/server/internal/videoproject/workflow_assets.go`
- Modify: `nx-backend/apps/server/internal/videoproject/workflow_assets_test.go`

- [ ] **Step 1: Write a failing image-generation lifecycle test**

Assert a `generating` row exists before the fake image provider is called, success updates it to `ready` with the OSS-backed asset/URL, and provider failure updates the same row to `failed` with the error.

- [ ] **Step 2: Run lifecycle test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestGenerateAssetCandidateLifecycle -count=1
```

- [ ] **Step 3: Implement behind a small interface**

```go
type ProjectImageGenerator interface {
    Generate(ctx context.Context, input image.GenerateInput) (videoasset.Asset, error)
}
```

Wire the existing image store and OSS-backed video asset creation; do not build a second image gateway.

- [ ] **Step 4: Write a failing uploaded/library candidate test**

Assert public uploaded/library assets create `ready` candidates without calling image generation and retain their source type.

- [ ] **Step 5: Implement uploaded/library candidate creation**

- [ ] **Step 6: Run focused and provider tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject ./internal/image ./internal/videoasset -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/workflow_assets*
git commit -m "feat(video): generate project asset candidates"
```

### Task 10A: Compute storyboard diffs and dependency-bound confirmation tokens

**Files:**
- Modify: `nx-backend/apps/server/internal/videoproject/workflow_storyboard.go`
- Modify: `nx-backend/apps/server/internal/videoproject/workflow_storyboard_test.go`

- [ ] **Step 1: Write failing revision and diff tests**

Test:

- draft edit requires expected revision and increments it once;
- diff returns exact `create/update/archive/unchanged` entries keyed by stable `sourceKey`;
- duplicate/empty source keys are rejected;
- diff token changes if any operation changes.

Token payload must contain draft ID/revision, baseline confirmed storyboard ID, current confirmed storyboard ID, source script revision, source breakdown ID, source asset revision, source capability version and canonical diff operations.

- [ ] **Step 2: Run diff tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestUpdateStoryboardDraft|TestStoryboardDiffToken' -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement draft editing and pure diff/token generation**

Do not write live shots in this task.

- [ ] **Step 4: Run diff tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/workflow_storyboard*
git commit -m "feat(video): preview storyboard version diffs"
```

### Task 10B: Confirm storyboard versions with stale-dependency protection

**Files:**
- Modify: `nx-backend/apps/server/internal/videoproject/workflow_storyboard.go`
- Modify: `nx-backend/apps/server/internal/videoproject/workflow_storyboard_test.go`
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject.go`

- [ ] **Step 1: Write failing stale-dependency tests**

After diff creation, independently change each of: script revision, confirmed breakdown ID, asset revision, current confirmed baseline, capability version, draft revision. Confirmation must return a specific 409 conflict code and no live-shot writes.

- [ ] **Step 2: Run stale tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestConfirmStoryboardRejectsStaleDependencies -count=1
```

- [ ] **Step 3: Implement dependency/token revalidation**

Recompute current diff/token inside the confirmation transaction before writes.

- [ ] **Step 4: Write failing materialization tests**

Assert stable source keys update existing shots, new keys create, unmatched keys archive, and all generations/versions remain. For an updated shot, preserve its existing `selected_generation_id` but make its request hash mismatch so workflow status requires explicit stale-result acknowledgement; never clear or silently replace it. Unchanged user edits/selections remain current. Unresolved scene/character/asset keys return an actionable JSON path. Reference intentions create ordered roles including `edit_target/extend_target`. New shots receive current validated project defaults.

- [ ] **Step 5: Run materialization tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run TestConfirmStoryboardMaterializesDiff -count=1
```

- [ ] **Step 6: Implement one transactional materialization**

Supersede the old confirmed version only after all writes validate; never delete shot generations.

- [ ] **Step 7: Run storyboard tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -run 'TestConfirmStoryboard|TestStoryboardDiff' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/server/internal/videoproject/workflow_storyboard* nx-backend/apps/server/internal/videoproject/videoproject.go
git commit -m "feat(video): version and confirm project storyboards"
```

### Task 11A: Expose focused script, breakdown, asset and storyboard routes

**Files:**
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_routes.go`
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_content_routes.go`
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_content_routes_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`

- [ ] **Step 1: Write failing content-route contract tests**

Cover:

```text
GET  /api/video/project-workflows/{projectId}
PUT  /api/video/project-workflows/{projectId}/preferences
PUT  /api/video/project-workflows/{projectId}/script
POST /api/video/project-workflows/{projectId}/script/polish
POST /api/video/project-workflows/{projectId}/script/confirm
POST /api/video/project-workflows/{projectId}/breakdowns
PUT  /api/video/project-workflows/{projectId}/breakdowns/{id}
POST /api/video/project-workflows/{projectId}/breakdowns/{id}/diff
POST /api/video/project-workflows/{projectId}/breakdowns/{id}/confirm
GET  /api/video/project-workflows/{projectId}/assets
POST /api/video/project-workflows/{projectId}/assets/{kind}/{id}/candidates
POST /api/video/project-workflows/{projectId}/candidates/{id}/select
POST /api/video/project-workflows/{projectId}/storyboards
PUT  /api/video/project-workflows/{projectId}/storyboards/{id}
POST /api/video/project-workflows/{projectId}/storyboards/{id}/diff
POST /api/video/project-workflows/{projectId}/storyboards/{id}/confirm
```

Assert exact JSON request structs, 400 validation, 404 ownership/not-found, 409 revisions/tokens/dependencies, 422 workflow blockers and project permission. `preferences` carries `expectedSettingsRevision`, persists `workflowMode/currentStep/workflowSettings`, and increments once only when values change. The breakdown diff response exposes the mapping operations and `diffToken` consumed by confirm. Handlers only parse/call/format.

- [ ] **Step 2: Run content route tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server -run TestVideoProjectWorkflowContentRoutes -count=1
```

- [ ] **Step 3: Implement the focused router and content handlers**

Keep route dispatch small; do not put workflow business logic in `server.go`.

- [ ] **Step 4: Run route/domain tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server ./internal/videoproject -run 'TestVideoProjectWorkflowContentRoutes|TestScript|TestBreakdown|TestAsset|TestStoryboard' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/server/videoproject_workflow_routes.go nx-backend/apps/server/internal/server/videoproject_workflow_content_routes* nx-backend/apps/server/internal/server/server.go
git commit -m "feat(video): expose project workflow content routes"
```

### Task 11B: Expose generation, selection, batch, reconciliation and compose routes

**Files:**
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_generation_routes.go`
- Create: `nx-backend/apps/server/internal/server/videoproject_workflow_generation_routes_test.go`
- Modify: `nx-backend/apps/server/internal/server/videoproject_workflow_routes.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/videoproject_routes.go`
- Modify: `nx-backend/apps/server/internal/videoproject/generator.go`
- Modify: `nx-backend/apps/server/internal/videoproject/batchgenerator.go`
- Modify: `nx-backend/apps/server/internal/videoproject/projectcomposer.go`
- Modify: `nx-backend/apps/server/internal/videoproject/videoproject.go`

- [ ] **Step 1: Write failing generation route tests**

Cover:

```text
POST /api/video/project-workflows/{projectId}/shots/{id}/prompt/preview
 POST /api/video/project-workflows/{projectId}/shots/{id}/generate
POST /api/video/project-workflows/{projectId}/shots/batch-generate
GET  /api/video/project-workflows/{projectId}/shots/batch-status
POST /api/video/project-workflows/{projectId}/submissions/{requestKey}/reconcile
POST /api/video/submissions/{requestKey}/reconcile
POST /api/video/project-workflows/{projectId}/shots/{id}/select/{generationId}
POST /api/video/project-workflows/{projectId}/compose
GET  /api/video/project-workflows/{projectId}/compose-status
```

Generation/batch tests require capability version and one request key per shot, prove duplicate keys do not create extra tasks, and return 409/422 typed errors.

- [ ] **Step 2: Run generation route tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server -run TestVideoProjectWorkflowGenerationRoutes -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement prompt/single/batch handlers and status polling**

Handlers only parse, authorize, call the workflow service and format structured errors. Generation must pass `requestKey`, `capabilityVersion`, resolution/audio settings and ordered references into the unified video store. Selection writes `selected_generation_id`; composition reads selected IDs and persists `compose_input_hash`.

- [ ] **Step 4: Write failing selection ownership/staleness tests**

Require the generation to belong to the same project and shot and have terminal success (`completed/succeeded`). If generation request hash differs from the current shot hash, selection requires `acknowledgeStale=true` and persists the exact selection ack hash; changing shot/selection invalidates it.

- [ ] **Step 5: Run selection tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server -run TestSelectShotGenerationRoute -count=1
```

- [ ] **Step 6: Implement selection validation**

- [ ] **Step 7: Write failing reconciliation lookup and ownership tests**

Use an `httptest.Server` and a saved contract:

```text
method = GET
pathTemplate = /v1/videos/by-request/{requestKey}
taskIdPaths = [data.task_id, task_id]
statusPaths = [data.status, status]
```

Assert `Client.LookupTaskByRequestKey(ctx, contract, requestKey)` URL-escapes the saved key, authenticates, sends no operator task ID/body, maps nested response fields, and never calls a video create POST. The project route verifies submission project/shot ownership. The standalone `/api/video/submissions/{requestKey}/reconcile` route handles submissions with no project/shot, requires `Video:Generate:Manage`, and cannot access a project-owned submission. Manual `taskId` reconciliation on either route additionally requires `System:Model:Config`, rejects conflicting IDs and is idempotent. Disabled/malformed lookup contract returns an explicit unsupported/config error.

- [ ] **Step 8: Run reconciliation route tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server -run TestReconcileSubmissionRoute -count=1
```

- [ ] **Step 9: Implement the exact lookup client and both reconciliation paths**

Only allow configured GET lookups in the first release; validate the path template contains exactly one `{requestKey}` placeholder and remains under the configured API base.

- [ ] **Step 10: Write failing compose/status tests**

Require every unarchived shot selected successfully; persist/return compose input hash; GET status reports current vs stale result.

- [ ] **Step 11: Run compose route tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server -run TestWorkflowComposeRoutes -count=1
```

- [ ] **Step 12: Implement compose integration**

Keep legacy `/shots-generate` and `/projects-compose` routes as compatibility wrappers around the same services.

- [ ] **Step 13: Run backend integration tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server ./internal/videoproject ./internal/video -run 'TestVideoProjectWorkflowGenerationRoutes|TestSelectShotGeneration|TestReconcile|TestCompose' -count=1
```

Expected: PASS.

- [ ] **Step 14: Commit**

```bash
git add nx-backend/apps/server/internal/server/videoproject_workflow_generation_routes* nx-backend/apps/server/internal/server/videoproject_workflow_routes.go nx-backend/apps/server/internal/server/server.go nx-backend/apps/server/internal/server/videoproject_routes.go nx-backend/apps/server/internal/videoproject/generator.go nx-backend/apps/server/internal/videoproject/batchgenerator.go nx-backend/apps/server/internal/videoproject/projectcomposer.go nx-backend/apps/server/internal/videoproject/videoproject.go
git commit -m "feat(video): expose workflow generation and compose routes"
```

## Chunk 3: Beginner-First Seven-Step Vue Wizard

### Task 12A: Add complete typed workflow API contracts and compile-time tests

**Files:**
- Create: `nx-backend/apps/web-antd/src/api/core/video-workflow.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/video-workflow-types.ts`
- Create: `nx-backend/apps/web-antd/src/api/core/video-workflow-types.test.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/index.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/workflow-state.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/workflow-state.test.ts`

- [ ] **Step 1: Write failing compile-time API shape assertions**

Use `expectTypeOf` plus `satisfies` fixtures for every request/response. Cover preferences/settings revision, script revision, breakdown/storyboard revisions and diff tokens, explicit mappings, candidate source/status, per-shot batch request keys, prompt diagnostics/capability version, safe/manual reconciliation, stale-selection acknowledgement, submission outcomes, selected generation hashes and compose current/stale status.

List every client function and exact method/path in the test so a missing endpoint fails source/runtime tests.

- [ ] **Step 2: Run type test and typecheck to verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/api/core/video-workflow-types.test.ts
pnpm --filter @vben/web-antd run typecheck
```

Expected: FAIL because the module/types do not exist.

- [ ] **Step 3: Implement only API types and request functions**

Match backend camelCase JSON exactly. Do not place UI state in the API module.

- [ ] **Step 4: Run compile-time/API tests and verify GREEN**

Run the Step 2 commands. Expected: PASS for the new type test and no new workflow type errors.

- [ ] **Step 5: Write failing pure workflow-state tests**

Pure helpers must cover:

```ts
export const WORKFLOW_STEP_KEYS = [
  'script', 'breakdown', 'assets', 'storyboard', 'prompt', 'generate', 'compose',
] as const;

export function resolveInitialStep(overview: WorkflowOverview): WorkflowStepKey;
export function primaryActionFor(step: WorkflowStepStatus): WorkflowAction;
export function canOpenStep(step: WorkflowStepStatus): boolean;
export function explainBlocker(message: WorkflowMessage): string;
```

Test that `skipped_existing` is navigable, blockers are not inferred from local counts, and unknown states fail safely to the server-provided current step.

- [ ] **Step 6: Run state test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/workflow-state.test.ts
```

Expected: FAIL.

- [ ] **Step 7: Implement pure state helpers and verify GREEN**

Run the Step 6 command. Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/web-antd/src/api/core/video-workflow* nx-backend/apps/web-antd/src/api/core/index.ts nx-backend/apps/web-antd/src/views/video/projects/workflow/workflow-state*
git commit -m "feat(video): add typed project workflow client"
```

### Task 12B: Manage generation request keys independently of rendering

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-generation-request-key.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-generation-request-key.test.ts`

- [ ] **Step 1: Write failing request-key lifecycle tests**

With a mocked UUID factory, prove:

```text
two clicks while pending → one key/one API call permission
transport retry → same key
manual safe retry before terminal → same key
component rerender/reload from server submission → pending key preserved
batch of N shots → one stable distinct key per shot
unknown_outcome → key remains locked; no new key
terminal + explicit “new version” → one newly generated key
```

- [ ] **Step 2: Run tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/use-generation-request-key.test.ts
```

- [ ] **Step 3: Implement the key manager/composable**

Keep keys in a map keyed by project/shot/intent and hydrate from server submission rows. It must not call APIs itself.

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/use-generation-request-key*
git commit -m "feat(video): 保持视频生成请求键稳定"
```

### Task 13A: Mount the wizard shell and make it the default route

**Files:**
- Modify: `nx-backend/apps/web-antd/package.json`
- Modify: `nx-backend/pnpm-lock.yaml`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-loader.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHeader.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowStepper.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/workflow-shell.test.ts`
- Modify: `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/production.test.ts`

- [ ] **Step 1: Add `@vue/test-utils` and write failing mounted shell tests**

Use `// @vitest-environment happy-dom`, mount with stubbed Ant components/router and mock the overview API. Exercise loading → success and loading → error → retry.

Assert:

- `/video/projects/:id/workbench` loads `workflow.vue`;
- `/video/projects/:id/advanced-workbench` loads the existing `workbench.vue` and remains hidden in the menu;
- shell renders seven labeled states and server progress;
- one primary action is present in the sticky action bar;
- “专业工作台” is secondary and preserves project ID;
- async load uses a skeleton and errors show cause plus retry;
- step controls are semantic buttons with accessible labels/text status, not color-only;
- primary/secondary actions emit the expected events when clicked.

- [ ] **Step 2: Run test to verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/workflow-shell.test.ts apps/web-antd/src/views/video/production.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Implement only the route, loader, header, stepper and shell**

The loader owns only load/refresh/error state. Leave unsaved navigation and automation to separate composables.

Use existing Ant Design/Vben theme tokens rather than new hardcoded palette/font imports. Apply 4/8px spacing, readable max-width, single vertical page scroll and a sticky bottom bar with reserved padding.

- [ ] **Step 4: Run mounted tests and typecheck**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/workflow-shell.test.ts apps/web-antd/src/views/video/production.test.ts
pnpm --filter @vben/web-antd run typecheck
```

Expected: tests PASS; typecheck has no new errors in workflow files.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/package.json nx-backend/pnpm-lock.yaml nx-backend/apps/web-antd/src/router/routes/modules/video.ts nx-backend/apps/web-antd/src/views/video/projects/workflow.vue nx-backend/apps/web-antd/src/views/video/projects/workflow nx-backend/apps/web-antd/src/views/video/production.test.ts
git commit -m "feat(video): make seven-step wizard the default workbench"
```

### Task 13B: Implement tested unsaved-state navigation and focus management

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-navigation.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-navigation.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing navigation tests**

Mount a small harness with mocked router/preferences API and test:

```text
internal step change while dirty → Save / Discard / Cancel choices
Cancel → step and form state unchanged
Discard → dirty state cleared, requested step opens, preferences currentStep saved
Save success → draft save called, preferences saved, new heading receives focus
Save failure → navigation cancelled and error focused
route leave → same three choices
beforeunload while dirty → event.preventDefault/returnValue set
clean state → no prompt
```

- [ ] **Step 2: Run tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/use-workflow-navigation.test.ts
```

- [ ] **Step 3: Implement the navigation composable**

Use one confirmation service for internal/router/unload behavior. Persist `currentStep` using `expectedSettingsRevision` and refresh on 409.

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-navigation* nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): protect unsaved workflow edits"
```

### Task 13C: Add optional automatic draft orchestration with a generation confirmation gate

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-autopilot.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-autopilot.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AutopilotProgress.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AutopilotProgress.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHeader.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing autopilot orchestration tests**

After confirmed script/style, assert the sequence creates breakdown draft, waits for/requests confirmation as configured, prepares asset drafts and creates storyboard draft. Persist `workflowMode='autopilot'` with settings revision. On any step failure, show the failed step, preserve completed outputs and offer resume/retry from that step.

Critically, assert no video generate/batch endpoint is called. The final autopilot state shows shot count/estimated quota consumption and requires a separate user confirmation.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/use-workflow-autopilot.test.ts apps/web-antd/src/views/video/projects/workflow/AutopilotProgress.test.ts
```

- [ ] **Step 3: Implement orchestration and visible mode control**

The header mode control is labeled, keyboard operable and explains the quota-consumption confirmation gate.

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-autopilot* nx-backend/apps/web-antd/src/views/video/projects/workflow/AutopilotProgress* nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHeader.vue nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): add safe automatic draft mode"
```

### Task 13D: Add a plain-language current-step help drawer

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHelpDrawer.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHelpDrawer.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHeader.vue`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing mounted help-drawer tests**

For every step key, assert a plain-language purpose, “你现在要做什么”, completion condition and next action. Feed server blockers/warnings and verify cause + fix appear. Prompt step includes reference/edit/extend examples and symbol guidance. Capability degradations show feature + intermediary reason without exposing raw secrets.

Test keyboard open/close, accessible drawer title, Escape close and focus returning to the help trigger.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/WorkflowHelpDrawer.test.ts
```

- [ ] **Step 3: Implement the focused help component**

Use text/Lucide icons and existing semantic tokens; no emoji or hidden hover-only help.

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHelpDrawer* nx-backend/apps/web-antd/src/views/video/projects/workflow/WorkflowHeader.vue nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): explain each novice workflow step"
```

### Task 14A: Implement and mount-test the script step

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ScriptStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ScriptStep.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing mounted interaction tests**

Mount with mocked APIs and exercise:

- visible textarea label and persistent helper text;
- save draft, AI polish and confirm are distinct actions, with only confirm/next styled primary;
- revision mismatch displays “内容已在别处修改” with reload option;
- long AI call keeps draft visible and shows progress;
- blur/debounced save sends current revision but never auto-confirms;
- failed save leaves text intact and focuses inline recovery.

- [ ] **Step 2: Run test to verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/ScriptStep.test.ts
```

- [ ] **Step 3: Implement the script component**

Put advanced style/target duration in a collapsed section; inline validate after blur. Use `aria-live` for async results.

- [ ] **Step 4: Run test and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/ScriptStep* nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): guide script confirmation"
```

### Task 14B: Implement editable breakdown mappings and diff confirmation

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/BreakdownStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/BreakdownItemCard.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/BreakdownStep.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing mounted breakdown tests**

Mount with two same-name/different-key items and exercise:

- tabs for 人物/场景/物品/服饰/风格;
- edit name, description, visual prompt, required and confirmed/ignored decision;
- recover an ignored item to pending;
- choose “关联已有资产” with exact kind/ID or “创建新资产” per item;
- unresolved/pending blocker renders beside the item;
- re-breakdown keeps current assets and opens a mapping diff;
- confirm sends `expectedRevision`, exact mappings and the server `diffToken` unchanged;
- a 409 preserves edits and offers reload/diff refresh;
- destructive reset requires confirmation.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/BreakdownStep.test.ts
```

- [ ] **Step 3: Implement card and step components**

Use stable item keys as Vue keys and payload identities; never infer mapping from display name.

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/BreakdownStep* nx-backend/apps/web-antd/src/views/video/projects/workflow/BreakdownItemCard.vue nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): confirm explicit breakdown mappings"
```

### Task 15A: Implement and mount-test asset candidate cards

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AssetsStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AssetCandidateCard.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AssetsStep.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing mounted asset tests**

Exercise:

- five asset categories share a consistent card layout;
- required/ready status includes text and icon;
- each asset shows image prompt, selected primary image and all candidates;
- generating/failed candidates survive refresh and failed cards have a clear retry action;
- upload and library selection are secondary to “AI 生成候选图”;
- selecting a candidate updates only that asset and exposes downstream stale warning;
- selecting the already-selected candidate does not call the API again;
- fixed image aspect ratios/lazy attributes are rendered;
- failed/generating history remains visible after API refresh.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/AssetsStep.test.ts
```

- [ ] **Step 3: Implement asset step/card**

Use one-column mobile and responsive card grid desktop, without nested horizontal scroll.

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/AssetsStep* nx-backend/apps/web-antd/src/views/video/projects/workflow/AssetCandidateCard.vue nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): manage project asset candidates"
```

### Task 15B: Implement storyboard editing, reference roles and safe confirmation

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/StoryboardStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/StoryboardShotEditor.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ReferenceRoleEditor.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/StoryboardStep.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing mounted storyboard tests**

Exercise:

- AI design creates a draft, not live shots;
- add/delete/edit shot order, duration, scene, characters, action, camera, audio/dialogue and task mode;
- reorder by visible Up/Down controls and keyboard shortcuts, with focus retained;
- add/remove references from project assets/uploads and change roles among capability-supported values;
- first/last frame and edit/extend target rules show inline errors before save;
- unresolved mappings point to the exact field;
- optimistic 409 preserves local draft and offers reload/compare;
- diff renders create/update/archive/unchanged and submits the exact token;
- archiving a shot with generations displays that history remains;
- confirm retains generated/selected stale history returned by the API.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/StoryboardStep.test.ts
```

- [ ] **Step 3: Implement focused editor/reference components**

On narrow screens open the editor in a drawer; desktop uses list/detail. Keep AI redesign and confirm separate.

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/StoryboardStep* nx-backend/apps/web-antd/src/views/video/projects/workflow/StoryboardShotEditor.vue nx-backend/apps/web-antd/src/views/video/projects/workflow/ReferenceRoleEditor.vue nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): edit and confirm Seedance storyboards"
```

### Task 16A: Render exact prompt/reference guidance from the API

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/PromptStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/PromptStep.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing mounted prompt tests**

Mock a preview whose canonical references contain explicit URL + per-kind ordinal. Assert rendered thumbnail/player URLs and labels exactly equal the response and that the copied prompt is the response prompt, not locally rebuilt. Exercise:

- ordered material thumbnails visibly match `图片1/视频1/音频1`;
- final copyable prompt and “为什么这样写” guidance are separate;
- errors, warnings and guides include recovery actions;
- unsupported role/parameter cannot be submitted;
- user override saves, marks diagnostics stale, and can reset to AI version;
- target at `视频2` displays `视频2` beside the same URL.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/PromptStep.test.ts
```

- [ ] **Step 3: Implement the prompt component without local numbering logic**

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/PromptStep* nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): explain exact Seedance prompts"
```

### Task 16B: Implement video generation, polling, recovery and explicit selection

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/GenerateStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/GenerateStep.test.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ShotVersionCard.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/SubmissionRecovery.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-polling.ts`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-polling.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing polling cleanup tests with fake timers**

Poll only non-terminal submissions, stop after terminal, stop on unmount, avoid overlapping requests, and resume from hydrated server state.

- [ ] **Step 2: Run polling tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/use-workflow-polling.test.ts
```

- [ ] **Step 3: Implement polling composable and verify GREEN**

- [ ] **Step 4: Write failing mounted generation/recovery tests**

Exercise:

- batch generation confirmation shows enabled shot count, estimated quota consumption and one stable request key per shot;
- rapid double click produces one API call and disabled/loading state;
- transport retry reuses the same key;
- `unknown_outcome` keeps that key, shows no normal retry and offers “安全查询中转站任务” only when `canLookupByRequestKey`;
- safe lookup calls reconciliation without task ID;
- manual task-ID field appears only with `canManualReconcile` and sends the same request key;
- versions show terminal/stale hash state and explicit selection;
- new-version button appears only after terminal and requests a new key;
- stale selection opens acknowledgement and sends `acknowledgeStale=true`;
- async changes announce through `aria-live`.

- [ ] **Step 5: Run mounted generation tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/GenerateStep.test.ts
```

- [ ] **Step 6: Implement generation/version/recovery components**

Reserve media dimensions, keep errors next to shots and never autoplay.

- [ ] **Step 7: Run generation and polling tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/GenerateStep.test.ts apps/web-antd/src/views/video/projects/workflow/use-workflow-polling.test.ts apps/web-antd/src/views/video/generation-error.test.ts
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/GenerateStep* nx-backend/apps/web-antd/src/views/video/projects/workflow/ShotVersionCard.vue nx-backend/apps/web-antd/src/views/video/projects/workflow/SubmissionRecovery.vue nx-backend/apps/web-antd/src/views/video/projects/workflow/use-workflow-polling* nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): generate reconcile and select shot versions"
```

### Task 16C: Implement current/stale final composition

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ComposeStep.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/ComposeStep.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`

- [ ] **Step 1: Write failing mounted compose tests**

Exercise exact missing-shot blockers, stable selected order/version IDs, explicit compose action, polling current/stale hash, stale successful video remaining downloadable, recombine action, secondary transition/music/subtitle controls, accessible non-autoplay video controls and download link.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/ComposeStep.test.ts
```

- [ ] **Step 3: Implement compose component**

- [ ] **Step 4: Run tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/ComposeStep* nx-backend/apps/web-antd/src/views/video/projects/workflow.vue
git commit -m "feat(video): compose current selected shot versions"
```

### Task 17A0: Expose the saved gateway contract in the model-config view

**Files:**
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/server_unit_test.go`

- [ ] **Step 1: Write a failing `buildModelConfigView` test**

Assert the video view returns `modelProfile`, full non-secret `gatewayContract` and contract version while continuing to expose only `apiKeySet`, never the key.

- [ ] **Step 2: Run test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server -run TestBuildModelConfigViewIncludesVideoContract -count=1
```

- [ ] **Step 3: Extend the view/audit snapshot and verify GREEN**

- [ ] **Step 4: Commit**

```bash
git add nx-backend/apps/server/internal/server/server.go nx-backend/apps/server/internal/server/server_unit_test.go
git commit -m "feat(video): expose gateway contract settings safely"
```

### Task 17A: Drive advanced/project settings from effective capabilities

**Files:**
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AdvancedSettingsDrawer.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/projects/workflow/AdvancedSettingsDrawer.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/projects/workflow.vue`
- Modify: `nx-backend/apps/web-antd/src/api/core/model-config.ts`
- Modify: `nx-backend/apps/web-antd/src/views/settings/model.vue`
- Create: `nx-backend/apps/web-antd/src/views/settings/model-video-contract.test.ts`

- [ ] **Step 1: Write failing mounted capability/settings tests**

Assert:

- beginner defaults come from capability API and project settings;
- duration includes 4–15/`-1` only when effective capabilities allow it;
- aspect ratio/resolution/audio controls use effective values;
- `seed` and `camera_fixed` never appear for Seedance 2.0;
- unsupported controls are hidden with a visible degradation explanation in help, not left inert;
- model change refreshes capability version and marks prompt/generation confirmation stale;
- preferences save carries `expectedSettingsRevision`;
- current legacy contract still shows 5/10/15 and the proven three aspect ratios;
- model settings page persists profile, contract, version and typed JSON, shows inline JSON/server validation, and keeps API key untouched.

- [ ] **Step 2: Run test to verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/AdvancedSettingsDrawer.test.ts apps/web-antd/src/views/settings/model-video-contract.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Implement the drawer and model-contract controls**

The drawer shows plain-language recommended settings first, then the technical upstream field mapping in a read-only details block. Changing model refreshes capabilities and requires re-confirmation before video generation.

- [ ] **Step 4: Run tests and verify GREEN**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/projects/workflow/AdvancedSettingsDrawer.test.ts apps/web-antd/src/views/settings/model-video-contract.test.ts
pnpm --filter @vben/web-antd run typecheck
```

Expected: tests PASS and no new type errors.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/projects/workflow/AdvancedSettingsDrawer* nx-backend/apps/web-antd/src/views/video/projects/workflow.vue nx-backend/apps/web-antd/src/api/core/model-config.ts nx-backend/apps/web-antd/src/views/settings/model.vue nx-backend/apps/web-antd/src/views/settings/model-video-contract.test.ts
git commit -m "feat(video): drive generation settings from capabilities"
```

### Task 17B: Mount-test and unify the existing single-generation page

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/video/generate.vue`
- Create: `nx-backend/apps/web-antd/src/views/video/generate.test.ts`
- Modify: `nx-backend/apps/web-antd/src/api/core/video.ts`

- [ ] **Step 1: Write failing mounted single-generation tests**

Mock capability and generate APIs. Assert:

- controls/options/limits come from capabilities; no hardcoded 14-image limit;
- legacy contract retains proven defaults;
- advanced role/parameter UI appears only when supported;
- submitted payload contains normalized fields, canonical ordered references, capability version and request key;
- rapid double click sends one call;
- transport retry reuses the same key;
- unknown outcome keeps the key and calls the standalone `/api/video/submissions/{requestKey}/reconcile` route for safe/manual reconciliation, never the project route or ordinary retry;
- successful terminal completion permits explicit new-version key;
- existing generation-history rendering remains.

- [ ] **Step 2: Run the exact test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/generate.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Refactor `generate.vue` to shared types/key manager**

Preserve the existing history table and error formatter.

- [ ] **Step 4: Run exact test and verify GREEN**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/src/views/video/generate.vue nx-backend/apps/web-antd/src/views/video/generate.test.ts nx-backend/apps/web-antd/src/api/core/video.ts
git commit -m "feat(video): unify single Seedance generation"
```

### Task 17C: Add browser-level responsive, keyboard and accessibility smoke coverage

**Files:**
- Modify: `nx-backend/apps/web-antd/package.json`
- Create: `nx-backend/apps/web-antd/playwright.config.ts`
- Create: `nx-backend/apps/web-antd/e2e/video-workflow.spec.ts`

- [ ] **Step 1: Write failing Playwright tests with mocked authenticated APIs**

Route/mock auth, overview and mutations. At widths 375, 768, 1024 and 1440 assert no horizontal overflow, step/action hit boxes are at least 44px, sticky action bar does not cover content, and long prompt/asset names wrap. Use keyboard only to change step, edit a field, cancel unsaved navigation, discard, and verify focus reaches the new step heading.

Run a reduced-motion context and assert workflow transitions do not use long animations. Assert status is not color-only by checking visible status text/accessible name.

- [ ] **Step 2: Run browser test and verify RED**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm --filter @vben/web-antd exec playwright test e2e/video-workflow.spec.ts
```

Expected: FAIL until config/fixtures/layout exist.

- [ ] **Step 3: Add the minimal Playwright config/script and fix actual browser issues**

Reuse the app dev server, do not add a production test route. Mock network responses and authenticated state only inside Playwright.

- [ ] **Step 4: Run browser test and verify GREEN**

Run the Step 2 command. Expected: PASS at all four viewports.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/web-antd/package.json nx-backend/apps/web-antd/playwright.config.ts nx-backend/apps/web-antd/e2e/video-workflow.spec.ts
git commit -m "test(video): cover workflow accessibility and breakpoints"
```

## Chunk 4: Compatibility, End-to-End Proof and Release Verification

### Task 18: Prove legacy project and route compatibility

**Files:**
- Modify: `nx-backend/apps/server/internal/server/video_project_versions_test.go`
- Modify: `nx-backend/apps/server/internal/db/menu_test.go`
- Modify: `nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts`
- Modify: `nx-backend/apps/web-antd/src/views/video/production.test.ts`
- Modify only if a compatibility assertion exposes a gap: `nx-backend/apps/server/internal/server/server.go`
- Modify only if a compatibility assertion exposes a gap: `nx-backend/apps/server/internal/server/videoproject_routes.go`
- Modify only if a compatibility assertion exposes a gap: `nx-backend/apps/server/internal/db/db.go`
- Modify only if a compatibility assertion exposes a gap: `nx-backend/apps/web-antd/src/router/routes/modules/video.ts`

- [ ] **Step 1: Extend the existing compatibility regression tests without duplicating Task 6B**

Keep `workflow_migration_test.go` owned by Task 6B and run it as authoritative migration evidence. Add focused assertions to the existing server/frontend compatibility tests for:

- all existing shot-version refresh/copy/regenerate routes remain registered;
- legacy `/shots-generate/`, `/projects-batch-generate/`, `/projects-batch-progress/`, `/projects-compose/` and `/projects-compose-status/` endpoints remain compatibility wrappers around the same generation/selection/compose services;
- legacy response snapshots still expose the old `used_images`, `used_videos` and `used_audios` fields while new role-aware references are present;
- historical stored prompts remain identified/rendered as `legacy_v1`, while every newly previewed prompt is `seedance2_v2`;
- `projects/:id` and `projects/:id/workbench` open the novice wizard;
- `projects/:id/advanced-workbench` opens the unchanged `workbench.vue`, stays hidden from the menu and preserves the project ID;
- the existing advanced-workbench source assertions still exercise generation, version refresh, copy, frame extraction and compose behavior;
- the project list and production entry still target the beginner project flow;
- the professional `/video/analysis` and `/video/storyboard` menu/component/API flows remain registered and accessible as separate expert tools.

Do not rewrite an existing assertion merely because the default route changed. Move route-specific expectations into `production.test.ts`; keep advanced-workbench behavior assertions pointed at the preserved `workbench.vue`.

- [ ] **Step 2: Run migration, server-route and frontend compatibility tests**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/videoproject -count=1
go test ./internal/server -count=1
go test ./internal/db -run 'TestDefaultMenusIncludeVideo(Analysis|Storyboard)$' -count=1
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/views/video/project-workbench-production-flow.test.ts apps/web-antd/src/views/video/production.test.ts
```

Expected: PASS if Tasks 6B, 11B and 13A preserved every contract. Any failure is a real compatibility gap; an old assertion's failure is not accepted as an expected migration outcome.

- [ ] **Step 3: Fix only compatibility gaps demonstrated by Step 2**

Keep old fields and routes as thin adapters over the new services. Do not fork a second generation implementation and do not alter Task 6B migration fixtures here. If Step 2 is already green, make no production-code change.

- [ ] **Step 4: Re-run the exact compatibility commands**

Run the same commands.

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing
git add -- nx-backend/apps/server/internal/server/video_project_versions_test.go nx-backend/apps/server/internal/server/server.go nx-backend/apps/server/internal/server/videoproject_routes.go nx-backend/apps/server/internal/db/menu_test.go nx-backend/apps/server/internal/db/db.go nx-backend/apps/web-antd/src/router/routes/modules/video.ts nx-backend/apps/web-antd/src/views/video/project-workbench-production-flow.test.ts nx-backend/apps/web-antd/src/views/video/production.test.ts
git commit -m "test(video): prove legacy workflow compatibility"
```

### Task 19: Add one full workflow integration scenario

**Files:**
- Create: `nx-backend/apps/server/internal/server/video_project_workflow_e2e_test.go`
- Modify only if the scenario exposes a wiring gap: `nx-backend/apps/server/internal/server/server.go`
- Modify only if the scenario exposes a wiring gap: `nx-backend/apps/server/internal/server/videoproject_workflow_routes.go`
- Modify only if the scenario exposes a wiring gap: `nx-backend/apps/server/internal/server/videoproject_workflow_content_routes.go`
- Modify only if the scenario exposes a wiring gap: `nx-backend/apps/server/internal/server/videoproject_workflow_generation_routes.go`
- Modify only if the scenario exposes a wiring gap: `nx-backend/apps/server/internal/videoproject/videoproject.go`

- [ ] **Step 1: Write the happy-path end-to-end test through public handlers**

Add `TestVideoProjectWorkflowEndToEndHappyPath`. The script includes two characters, two scenes, one prop and one outfit. Fake LLM, image, video and compose providers plus the test database must prove:

1. save and confirm script;
2. generate/edit/confirm five-category breakdown;
3. generate/select required asset candidates;
4. create/edit/diff/confirm at least three shots;
5. compile prompts whose numbering matches persisted references;
6. submit one quota-consuming video-generation request per explicit key through the intermediary contract;
7. produce two terminal versions for one shot and explicitly select one;
8. select every shot and compose;
9. return step 7 completed with the current ordered-selection compose hash;
10. reload the overview after every confirmation boundary and continue from persisted server state.

- [ ] **Step 2: Write the complete unknown-outcome recovery test**

Add `TestVideoProjectWorkflowEndToEndUnknownOutcomeRecovery` with a saved reconciliation contract and one project shot. Use a fake gateway that records method/path/request key and performs this exact sequence:

1. the only video create `POST /v1/videos` accepts the request and then returns an ambiguous transport result;
2. the submission is persisted as `unknown_outcome` with the original `requestKey` and no task ID;
3. repeating generation with the same key returns the existing submission, and a new key is rejected while it is active; the create POST count remains exactly one;
4. the authorized safe-reconcile route performs configured `GET /v1/videos/by-request/{requestKey}` with the same escaped key and no request body;
5. reconciliation attaches the discovered task ID idempotently, persists `reconciled`, and never calls the create POST;
6. normal polling resumes with safe GET requests; the submission stays `reconciled` as the audit outcome while its linked generation reaches terminal `completed`;
7. the recovered generation can be explicitly selected, composition succeeds, and the final overview reports a current compose hash;
8. fake-gateway totals prove one create POST across generate, duplicate click, reconciliation, polling, selection and compose.

Task 11B's route tests remain the authority for operator-supplied task-ID permissions: project ownership is required, standalone reconciliation requires `Video:Generate:Manage`, and manual task IDs additionally require `System:Model:Config`.

- [ ] **Step 3: Run both server scenarios**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server -run 'TestVideoProjectWorkflowEndToEnd(HappyPath|UnknownOutcomeRecovery)$' -count=1 -v
```

Expected: PASS when the complete public workflow is wired. Any failure identifies an integration gap; do not bypass handlers or weaken request-count assertions.

- [ ] **Step 4: Fix only integration gaps exposed by the scenarios**

Do not add new features. Resolve route wiring, transaction boundaries, response shapes and polling/selection linkage required by the approved spec.

- [ ] **Step 5: Run end-to-end plus the existing typed/generation/compose frontend contracts**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/server -run 'TestVideoProjectWorkflowEndToEnd(HappyPath|UnknownOutcomeRecovery)$' -count=1 -v
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/api/core/video-workflow-types.test.ts apps/web-antd/src/views/video/projects/workflow/GenerateStep.test.ts apps/web-antd/src/views/video/projects/workflow/ComposeStep.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing
git add -- nx-backend/apps/server/internal/server/video_project_workflow_e2e_test.go nx-backend/apps/server/internal/server/server.go nx-backend/apps/server/internal/server/videoproject_workflow_routes.go nx-backend/apps/server/internal/server/videoproject_workflow_content_routes.go nx-backend/apps/server/internal/server/videoproject_workflow_generation_routes.go nx-backend/apps/server/internal/videoproject/videoproject.go
git commit -m "test(video): prove the novice workflow end to end"
```

### Task 20: Run full verification and final UI quality audit

**Files:**
- Create: `docs/superpowers/evidence/2026-07-10-seedance2-novice-video-workflow.md`
- Update: `docs/superpowers/plans/2026-07-10-seedance2-novice-video-workflow.md` checkbox state as tasks complete.
- Modify only the owning feature files required to fix a verified feature regression; return to that task's exact test and commit before continuing.

- [ ] **Step 1: Invoke the required review and completion gates**

Use `@superpowers:requesting-code-review` for implementation review and resolve every blocking finding. Use `@superpowers:verification-before-completion` before any completion claim. Re-run the `ui-ux-pro-max` accessibility/interaction checklist against the rendered workflow and the Playwright evidence; findings must be fixed in their owning Task 13–17 files.

- [ ] **Step 2: Format only Go files changed by this feature**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing
BASE="$(cat /tmp/nine-xing-seedance-baseline-commit)"
git diff --name-only --diff-filter=ACMR "$BASE"..HEAD -- 'nx-backend/apps/server/**/*.go' > /tmp/nine-xing-seedance-feature-go-files
while IFS= read -r file; do
  if [ -n "$file" ]; then
    gofmt -w "$file"
  fi
done < /tmp/nine-xing-seedance-feature-go-files
git diff --check
```

Expected: only feature-owned Go files are formatted; `git diff --check` exits 0. Never run `gofmt` on an entire package directory.

- [ ] **Step 3: Run explicit model-contract, backend feature and full backend verification**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./internal/config ./internal/modelconfig -count=1
go test ./internal/server -run 'TestBuildModelConfigViewIncludesVideoContract' -count=1
go test ./internal/video ./internal/videoproject ./internal/server -count=1
go test ./... -count=1
go vet ./...
```

Expected: model configuration and all packages PASS; `go vet` exits 0.

- [ ] **Step 4: Run explicit API/model UI tests, all video tests, typecheck, build and Playwright**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend
pnpm exec vitest run apps/web-antd/src/api/core/video-workflow-types.test.ts apps/web-antd/src/views/settings/model-video-contract.test.ts
pnpm exec vitest run apps/web-antd/src/views/video
pnpm --filter @vben/web-antd run typecheck
pnpm --filter @vben/web-antd run build
pnpm --filter @vben/web-antd exec playwright test e2e/video-workflow.spec.ts
```

Expected: API/model tests and the complete video suite PASS, typecheck exits 0, production build succeeds, and Playwright passes at 375/768/1024/1440 with reduced-motion coverage.

- [ ] **Step 5: Classify any apparently unrelated full-suite failure against the saved baseline**

Only use this step for a failing full-suite/typecheck/build command that is outside the focused feature tests. Re-run that exact command at the saved commit in a temporary detached worktree:

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing
BASE="$(cat /tmp/nine-xing-seedance-baseline-commit)"
BASELINE_TREE="/tmp/nine-xing-seedance-baseline-$$"
git worktree add --detach "$BASELINE_TREE" "$BASE"
cd "$BASELINE_TREE/nx-backend"
pnpm install --frozen-lockfile
cd "$BASELINE_TREE"
# Run only the matching baseline command below for the command that failed.
(cd nx-backend/apps/server && go test ./... -count=1)
(cd nx-backend/apps/server && go vet ./...)
(cd nx-backend && pnpm exec vitest run apps/web-antd/src/views/video)
(cd nx-backend && pnpm --filter @vben/web-antd run typecheck)
(cd nx-backend && pnpm --filter @vben/web-antd run build)
cd /Users/wohenzaiyi/Desktop/nine-xing
git worktree remove "$BASELINE_TREE"
```

Classify a failure as pre-existing only when the same command fails with the same signature at the saved commit. A newly added focused test cannot be classified this way. Record the comparison in the evidence document; never change an assertion to hide the failure.

- [ ] **Step 6: Create the requirement-to-authoritative-evidence matrix**

Create `docs/superpowers/evidence/2026-07-10-seedance2-novice-video-workflow.md`. For every row below, record the exact test file/test name or browser assertion, the fresh command, exit code and observed count/result. Source inspection alone is not sufficient when an executable contract exists.

| ID | Requirement | Required authoritative evidence |
|---|---|---|
| R1 | Existing intermediary address/key and `/v1/videos` remain the generation channel | gateway exact-JSON/auth tests plus model-config non-secret view test |
| R2 | Standard/Fast/Mini/unknown capability intersection, stale capability-version rejection and visible degradation | capability registry/validator tests plus advanced-settings mounted test |
| R3 | Unsupported parameters are rejected and undeclared fields are not sent | request-validator and gateway-mapper tests |
| R4 | Image/video/audio numbering is stable by canonical order and same URL/different role is retained | canonical-reference and prompt/payload cross-layer golden tests |
| R5 | `reference`, `edit` and `extend` prompts follow Seedance guidance; `seed`/`camera_fixed` are absent | prompt compiler golden/diagnostic tests and PromptStep mounted test |
| R6 | Script confirmation and five-category breakdown are versioned, optimistic-locked, editable, not name-merged and transactionally materialized | script/breakdown domain and content-route tests |
| R7 | Character/scene/prop/outfit/style candidates recover and require explicit selection | asset store/provider tests and AssetsStep mounted test |
| R8 | Storyboards support draft edit, diff token, stale-dependency rejection and confirmation | storyboard domain/route tests and StoryboardStep mounted test |
| R9 | Seven completion states use server revisions/hashes and invalidate downstream state correctly | workflow hash/status tests and shell/state tests |
| R10 | Video generation uses one persisted request key, one-shot POST and no ordinary network retry | submission/video tests and generation-key/GenerateStep tests |
| R11 | `unknown_outcome` keeps the same key, reconciles by safe GET or permissioned manual task ID, resumes polling and never adds a create POST | reconciliation route tests plus `TestVideoProjectWorkflowEndToEndUnknownOutcomeRecovery` |
| R12 | Every shot requires an explicit successful selection, stale selections require acknowledgement, and composition uses the current ordered-selection hash | selection/compose route tests, GenerateStep/ComposeStep tests and happy-path E2E |
| R13 | Old projects migrate idempotently with stable asset order, legacy role expansion, successful-only selection backfill and `skipped_existing` | Task 6B migration/dual-read tests |
| R14 | Historical prompts stay `legacy_v1`; old shot/batch/progress/compose/status/version routes, single-generation page, advanced workbench and professional analysis/storyboard flow remain accessible | Task 18 server/menu/frontend compatibility tests and `generate.test.ts` |
| R15 | Novice UI has one primary CTA, plain-language help, quota-consumption confirmation gate, keyboard/focus/reduced-motion/44px/responsive behavior | workflow mounted tests and Playwright |
| R16 | Two-character/two-scene/prop/outfit script completes breakdown, assets, 3+ shots, generation, explicit selection and final composition | `TestVideoProjectWorkflowEndToEndHappyPath` |

- [ ] **Step 7: Fix verified gaps in their owning task, then re-run the complete verification set**

For any failure or review finding caused by this feature, return to the owning task, add/adjust the smallest regression test, fix it, run that focused command, and commit with the owning task's explicit file list. Then repeat Steps 2–4 in full. If no feature fix is required, do not create a `fix(video)` commit.

- [ ] **Step 8: Safely commit verification evidence and push**

First confirm that no unrelated or generated content is staged:

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing
git status --short
git diff --check
git add -- docs/superpowers/plans/2026-07-10-seedance2-novice-video-workflow.md docs/superpowers/evidence/2026-07-10-seedance2-novice-video-workflow.md
git diff --cached --name-only
UNEXPECTED="$(git diff --cached --name-only | rg -v '^docs/superpowers/(plans/2026-07-10-seedance2-novice-video-workflow\.md|evidence/2026-07-10-seedance2-novice-video-workflow\.md)$' || true)"
test -z "$UNEXPECTED"
test "$(git diff --cached --name-only | wc -l | tr -d ' ')" = "2"
if git diff --cached --name-only | rg -q '^artifacts/'; then
  echo 'refusing to commit artifacts/' >&2
  exit 1
fi
```

Expected staged paths: only the implementation plan checkbox updates and the evidence document. `artifacts/` and every unrelated file remain unstaged.

```bash
git commit -m "docs(video): record Seedance workflow verification"
git push origin detail-tuning-video-management
git status --short --branch
LOCAL_HEAD="$(git rev-parse HEAD)"
REMOTE_HEAD="$(git ls-remote --heads origin detail-tuning-video-management | awk '{print $1}')"
test "$LOCAL_HEAD" = "$REMOTE_HEAD"
```

Expected: the evidence commit is non-empty, the push succeeds, the remote hash equals local `HEAD`, and `artifacts/` remains untracked.

---

## Execution Notes

- Preserve the user's unrelated work and the untracked `artifacts/` directory.
- Never print or commit real API keys, signed URLs or model-config secrets.
- Every quota-consuming create code path must be reviewed separately from safe GET polling retries.
- Keep commits small and use the exact task-level commits above where practical.
- If the intermediary contract cannot express a Seedance feature, complete the beginner workflow with that option visibly unavailable; do not invent an upstream field.
