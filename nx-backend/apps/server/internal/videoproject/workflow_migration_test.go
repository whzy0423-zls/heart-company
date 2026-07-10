package videoproject

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestWorkflowMigrationSQLContract(t *testing.T) {
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := normalizeSchemaSQL(string(raw))
	assertSchemaFragments(t, schema, []string{
		"UPDATE video_project_characters c SET breakdown_item_key = 'legacy:character:' || c.id::text",
		"UPDATE video_project_scenes sc SET breakdown_item_key = 'legacy:scene:' || sc.id::text",
		"s.archived_at IS NULL AND s.character_ids ? c.id::text",
		"s.archived_at IS NULL AND s.scene_id = sc.id",
		"'character', c.id, COALESCE(NULLIF(c.visual_prompt,''), c.description), va.asset_id, c.reference_image_url, 'legacy', 'ready', true",
		"'scene', sc.id, COALESCE(NULLIF(sc.visual_prompt,''), sc.description), va.asset_id, sc.reference_image_url, 'legacy', 'ready', true",
		"row_number() OVER (PARTITION BY shot_id ORDER BY create_time, id)",
		"source_type = CASE WHEN asset.source_type = '' THEN 'legacy_shot_asset' ELSE asset.source_type END",
		"source_id = CASE WHEN asset.source_id = '' THEN ranked.id::text ELSE asset.source_id END",
		"selected_generation_id = s.generation_id",
		"g.status IN ('completed','succeeded')",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_video_shot_assets_exact_reference",
		"shot_id, asset_type, reference_role, COALESCE(source_type,''), COALESCE(source_id,''), object_url",
	})
}

func TestWorkflowMigrationIsIdempotent(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NINE_XING_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set NINE_XING_TEST_POSTGRES_DSN to run PostgreSQL migration integration test")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rawSchema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, string(rawSchema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	for _, table := range []string{
		"video_project_asset_candidates",
		"video_generation_submissions",
		"video_shot_assets",
		"video_compose_jobs",
		"video_generations",
		"video_shots",
		"video_project_assets",
		"video_project_storyboard_versions",
		"video_project_characters",
		"video_project_scenes",
		"video_project_breakdowns",
		"video_projects",
	} {
		if _, err := database.ExecContext(ctx, "TRUNCATE "+table+" RESTART IDENTITY CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	if _, err := database.ExecContext(ctx, `DROP INDEX IF EXISTS uq_video_shot_assets_exact_reference`); err != nil {
		t.Fatal(err)
	}

	var projectID, characterID, sceneID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO video_projects(name) VALUES ('旧项目') RETURNING id`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_project_characters(project_id,name,description,reference_image_url,is_main)
		VALUES ($1,'小夏','红色风衣少女','https://cdn.example.com/character.png',true)
		RETURNING id`, projectID).Scan(&characterID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_project_scenes(project_id,name,description,reference_image_url)
		VALUES ($1,'雨夜车站','蓝色霓虹站台','https://cdn.example.com/scene.png')
		RETURNING id`, projectID).Scan(&sceneID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO video_project_asset_candidates(
		  project_id,target_type,target_id,prompt,image_url,source,status,selected
		) VALUES ($1,'scene',$2,'用户选择','https://cdn.example.com/user-scene.png','upload','ready',true)`, projectID, sceneID); err != nil {
		t.Fatal(err)
	}

	shotIDs := make([]int64, 4)
	for index := range shotIDs {
		characterIDs, _ := json.Marshal([]string{stringID(characterID)})
		if err := database.QueryRowContext(ctx, `
			INSERT INTO video_shots(project_id,order_num,name,action_description,character_ids,scene_id)
			VALUES ($1,$2,$3,'人物走入站台',$4,$5)
			RETURNING id`, projectID, index+1, "镜头", characterIDs, sceneID).Scan(&shotIDs[index]); err != nil {
			t.Fatal(err)
		}
	}

	statuses := []string{"queued", "failed", "completed", "succeeded"}
	generationIDs := make([]int64, len(statuses))
	for index, status := range statuses {
		if err := database.QueryRowContext(ctx, `
			INSERT INTO video_generations(model,prompt,task_id,status,project_id,shot_id)
			VALUES ('video-ds-2.0','测试',$1,$2,$3,$4)
			RETURNING id`, "task-"+status, status, projectID, shotIDs[index]).Scan(&generationIDs[index]); err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(ctx, `UPDATE video_shots SET generation_id=$1 WHERE id=$2`, generationIDs[index], shotIDs[index]); err != nil {
			t.Fatal(err)
		}
	}

	stamp := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		kind string
		url  string
	}{
		{kind: "image", url: "https://cdn.example.com/reference.png"},
		{kind: "video", url: "https://cdn.example.com/reference.mp4"},
		{kind: "audio", url: "https://cdn.example.com/reference.mp3"},
		{kind: "image", url: "https://cdn.example.com/reference.png"},
	} {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO video_shot_assets(shot_id,asset_type,object_url,name,create_time,update_time)
			VALUES ($1,$2,$3,$2,$4,$4)`, shotIDs[0], item.kind, item.url, stamp); err != nil {
			t.Fatal(err)
		}
	}

	if err := runLegacyWorkflowMigration(ctx, database); err != nil {
		t.Fatal(err)
	}
	first := captureLegacyWorkflowState(t, ctx, database, projectID)
	if err := runLegacyWorkflowMigration(ctx, database); err != nil {
		t.Fatal(err)
	}
	second := captureLegacyWorkflowState(t, ctx, database, projectID)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("migration changed on second run:\nfirst=%s\nsecond=%s", first, second)
	}

	if !strings.Contains(first, `legacy:character:`+stringID(characterID)) || !strings.Contains(first, `legacy:scene:`+stringID(sceneID)) {
		t.Fatalf("legacy keys missing: %s", first)
	}
	for _, fragment := range []string{
		`"reference_image","legacy_shot_asset","1",1`,
		`"reference_video","legacy_shot_asset","2",2`,
		`"reference_audio","legacy_shot_asset","3",3`,
		`"reference_image","legacy_shot_asset","4",4`,
		`"character",` + stringID(characterID) + `,"legacy","https://cdn.example.com/character.png","ready",true`,
		`"scene",` + stringID(sceneID) + `,"upload","https://cdn.example.com/user-scene.png","ready",true`,
		`"selected":[null,null,` + stringID(generationIDs[2]) + `,` + stringID(generationIDs[3]) + `]`,
	} {
		if !strings.Contains(first, fragment) {
			t.Fatalf("migration state missing %q: %s", fragment, first)
		}
	}
}

func TestLegacyReferenceDualRead(t *testing.T) {
	capabilities := fullPromptCapabilities()
	character := Character{ID: "7", Name: "小夏", ReferenceImageURL: "https://cdn.example.com/character.png"}
	scene := Scene{ID: "8", Name: "雨夜车站", ReferenceImageURL: "https://cdn.example.com/scene.png", ReferenceVideoURL: "https://cdn.example.com/scene.mp4"}
	previous := Shot{ID: "6", EndFrameURL: "https://cdn.example.com/previous.png", VideoURL: "https://cdn.example.com/previous.mp4"}

	t.Run("explicit references win", func(t *testing.T) {
		shot := Shot{
			ID: "9", ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"}, VideoReferenceMode: "scene_demo",
			ShotAssets: []ShotAsset{{ID: "20", AssetType: "image", ObjectURL: "https://cdn.example.com/explicit.png", ReferenceRole: "reference_image", SortOrder: 2, SourceType: "upload", SourceID: "20"}},
		}
		builder := &PromptBuilder{}
		got, err := builder.buildCanonicalReferences(shot, []Character{character}, &scene, &previous, capabilities)
		if err != nil {
			t.Fatal(err)
		}
		if labels := canonicalPromptLabels(got); !reflect.DeepEqual(labels, []string{"图片1"}) {
			t.Fatalf("labels = %#v", labels)
		}
		if got.References[0].URL != "https://cdn.example.com/explicit.png" {
			t.Fatalf("references = %+v", got.References)
		}
	})

	t.Run("legacy modes expand only without explicit references", func(t *testing.T) {
		shot := Shot{
			ID: "9", ImageReferenceModes: []string{"prev_frame", "character_ref", "scene_ref"}, VideoReferenceMode: "prev_video",
		}
		builder := &PromptBuilder{}
		got, err := builder.buildCanonicalReferences(shot, []Character{character}, &scene, &previous, capabilities)
		if err != nil {
			t.Fatal(err)
		}
		wantRoles := []string{"first_frame", "reference_image", "reference_image", "reference_video"}
		wantSources := []string{"previous_shot:6", "character:7", "scene:8", "previous_shot:6"}
		roles := make([]string, 0, len(got.References))
		sources := make([]string, 0, len(got.References))
		for _, reference := range got.References {
			roles = append(roles, reference.Role)
			sources = append(sources, reference.SourceType+":"+reference.SourceID)
		}
		if !reflect.DeepEqual(roles, wantRoles) || !reflect.DeepEqual(sources, wantSources) {
			t.Fatalf("roles=%#v sources=%#v", roles, sources)
		}
	})
}

func canonicalPromptLabels(references video.CanonicalReferences) []string {
	labels := make([]string, 0, len(references.References))
	for _, reference := range references.References {
		labels = append(labels, reference.Label)
	}
	return labels
}

func captureLegacyWorkflowState(t *testing.T, ctx context.Context, database *sql.DB, projectID int64) string {
	t.Helper()
	var state string
	err := database.QueryRowContext(ctx, `
		SELECT jsonb_build_object(
		  'characters', (SELECT jsonb_agg(jsonb_build_array(breakdown_item_key,required) ORDER BY id) FROM video_project_characters WHERE project_id=$1),
		  'scenes', (SELECT jsonb_agg(jsonb_build_array(breakdown_item_key,required) ORDER BY id) FROM video_project_scenes WHERE project_id=$1),
		  'candidates', (SELECT jsonb_agg(jsonb_build_array(target_type,target_id,source,image_url,status,selected) ORDER BY target_type,target_id) FROM video_project_asset_candidates WHERE project_id=$1),
		  'references', (SELECT jsonb_agg(jsonb_build_array(a.reference_role,a.source_type,a.source_id,a.sort_order) ORDER BY a.sort_order,a.id) FROM video_shot_assets a JOIN video_shots s ON s.id=a.shot_id WHERE s.project_id=$1),
		  'selected', (SELECT jsonb_agg(selected_generation_id ORDER BY order_num) FROM video_shots WHERE project_id=$1)
		)::text`, projectID).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(state, " ", "")
}

func stringID(value int64) string {
	return strconv.FormatInt(value, 10)
}
