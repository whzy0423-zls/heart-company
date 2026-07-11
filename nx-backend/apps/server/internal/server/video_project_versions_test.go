package server

import (
	"os"
	"strings"
	"testing"
)

func sourceBetweenForVideoProjectVersionsTest(t *testing.T, source, start, end string) string {
	t.Helper()
	startIndex := strings.Index(source, start)
	if startIndex < 0 {
		t.Fatalf("expected source to contain start marker %q", start)
	}
	endIndex := strings.Index(source[startIndex+len(start):], end)
	if endIndex < 0 {
		t.Fatalf("expected source to contain end marker %q after %q", end, start)
	}
	return source[startIndex : startIndex+len(start)+endIndex]
}

func TestVideoProjectShotVersionRefreshRouteUsesGenerationRefreshAndSyncsShot(t *testing.T) {
	routes, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	routeSource := string(routes)
	handlerSource := string(handlers)

	for _, want := range []string{
		`"/api/video/shots-video-versions/refresh/"`,
		"s.refreshShotVideoVersions",
	} {
		if !strings.Contains(routeSource, want) {
			t.Fatalf("expected video project routes to include %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Server) refreshShotVideoVersions",
		"s.videoStore().Refresh",
		"SetShotVideoVersion",
		"ListShotVideoVersions",
	} {
		if !strings.Contains(handlerSource, want) {
			t.Fatalf("expected refresh handler to include %q", want)
		}
	}
}

func TestVideoProjectShotVersionRefreshRouteContinuesAfterSingleGenerationRefreshError(t *testing.T) {
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	handlerSource := string(handlers)
	start := strings.Index(handlerSource, "func (s *Server) refreshShotVideoVersions")
	if start < 0 {
		t.Fatal("refreshShotVideoVersions handler not found")
	}
	end := strings.Index(handlerSource[start:], "func (s *Server) copyShotVideoVersion")
	if end < 0 {
		t.Fatal("copyShotVideoVersion handler not found after refresh handler")
	}
	refreshSource := handlerSource[start : start+end]

	for _, want := range []string{
		"refreshErrors",
		"continue",
		"strings.Join(refreshErrors",
	} {
		if !strings.Contains(refreshSource, want) {
			t.Fatalf("expected refresh handler to tolerate individual generation refresh errors with %q", want)
		}
	}
	if strings.Contains(refreshSource, "httpx.Fail(w, http.StatusBadRequest, err.Error())") {
		t.Fatal("refresh handler must not fail the entire version list when one generation refresh fails")
	}
}

func TestVideoProjectShotVersionCopyRouteCopiesVersionToAnotherShot(t *testing.T) {
	routes, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}
	routeSource := string(routes)
	handlerSource := string(handlers)
	storeSource := string(store)

	for _, want := range []string{
		`"/api/video/shots-video-versions/copy/"`,
		"s.copyShotVideoVersion",
	} {
		if !strings.Contains(routeSource, want) {
			t.Fatalf("expected video project routes to include %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Server) copyShotVideoVersion",
		"CopyShotVideoVersion",
	} {
		if !strings.Contains(handlerSource, want) {
			t.Fatalf("expected copy handler to include %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Store) CopyShotVideoVersion",
		"INSERT INTO video_generations",
		"project_id=$1",
		"generation_id=$1",
	} {
		if !strings.Contains(storeSource, want) {
			t.Fatalf("expected store copy implementation to include %q", want)
		}
	}
}

func TestVideoProjectBatchGenerateRouteCanLimitToSelectedShots(t *testing.T) {
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	batchGenerator, err := os.ReadFile("../videoproject/batchgenerator.go")
	if err != nil {
		t.Fatalf("read batchgenerator.go: %v", err)
	}
	handlerSource := string(handlers)
	batchSource := string(batchGenerator)

	for _, want := range []string{
		"batchGenerateInput",
		"Items []videoproject.SafeBatchGenerateItem",
		"json.NewDecoder(r.Body).Decode",
		"GenerateSafe",
		"validRequestKey(item.RequestKey)",
	} {
		if !strings.Contains(handlerSource, want) {
			t.Fatalf("expected batch generate handler to include %q", want)
		}
	}

	for _, want := range []string{
		"func (bg *BatchGenerator) GenerateSafe",
		"FilterGeneratableShotIDs",
		"SafeBatchGenerateItem",
	} {
		if !strings.Contains(batchSource, want) {
			t.Fatalf("expected batch generator to include %q", want)
		}
	}
}

func TestVideoProjectShotVersionViewedRoutePersistsUnviewedBadgeState(t *testing.T) {
	routes, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	for _, want := range []string{
		`"/api/video/shots-video-versions/viewed/"`,
		"s.markShotVideoVersionViewed",
	} {
		if !strings.Contains(string(routes), want) {
			t.Fatalf("expected route registration to include %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Server) markShotVideoVersionViewed",
		"MarkShotVideoVersionViewed",
	} {
		if !strings.Contains(string(handlers), want) {
			t.Fatalf("expected handler to include %q", want)
		}
	}

	for _, want := range []string{
		"ViewedFlag",
		"viewed_flag",
		"MarkShotVideoVersionViewed",
	} {
		if !strings.Contains(string(store), want) {
			t.Fatalf("expected store to include %q", want)
		}
	}

	if !strings.Contains(string(schema), "viewed_flag") {
		t.Fatalf("expected video_generations schema to include viewed_flag")
	}
}

func TestVideoProjectShotVersionExtractFrameRouteCreatesShotImageAsset(t *testing.T) {
	routes, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}
	generator, err := os.ReadFile("../videoproject/generator.go")
	if err != nil {
		t.Fatalf("read generator.go: %v", err)
	}
	frameExtractor, err := os.ReadFile("../videoproject/frameextractor.go")
	if err != nil {
		t.Fatalf("read frameextractor.go: %v", err)
	}

	for _, want := range []string{
		`"/api/video/shots-video-versions/extract-frame/"`,
		"s.extractShotVideoFrame",
	} {
		if !strings.Contains(string(routes), want) {
			t.Fatalf("expected route registration to include %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Server) extractShotVideoFrame",
		"ExtractShotVideoFrame",
	} {
		if !strings.Contains(string(handlers), want) {
			t.Fatalf("expected extract-frame handler to include %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Store) GetShotVideoVersion",
		"CreateShotAsset",
		"assetType",
	} {
		if !strings.Contains(string(store), want) {
			t.Fatalf("expected store to support frame asset creation with %q", want)
		}
	}

	for _, want := range []string{
		"func (g *Generator) ExtractShotVideoFrame",
		"FrameExtractor",
		"ExtractFrameAtTime",
		"video/frames",
		"CreateShotAsset",
	} {
		if !strings.Contains(string(generator), want) {
			t.Fatalf("expected generator extract-frame implementation to include %q", want)
		}
	}

	if !strings.Contains(string(frameExtractor), "func (e *FrameExtractor) ExtractFrameAtTime") {
		t.Fatalf("expected frame extractor to expose ExtractFrameAtTime")
	}
}

func TestVideoProjectShotVersionBackupRoutePersistsLiuguangBackupFlag(t *testing.T) {
	routes, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	for _, want := range []string{
		`"/api/video/shots-video-versions/backup/"`,
		"s.setShotVideoVersionBackup",
	} {
		if !strings.Contains(string(routes), want) {
			t.Fatalf("expected route registration to include %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Server) setShotVideoVersionBackup",
		"SetShotVideoVersionBackup",
		"backupFlag",
	} {
		if !strings.Contains(string(handlers), want) {
			t.Fatalf("expected backup handler to include %q", want)
		}
	}

	for _, want := range []string{
		"BackupFlag",
		"backup_flag",
		"SetShotVideoVersionBackup",
	} {
		if !strings.Contains(string(store), want) {
			t.Fatalf("expected store to persist backup flag with %q", want)
		}
	}

	if !strings.Contains(string(schema), "backup_flag") {
		t.Fatalf("expected video_generations schema to include backup_flag")
	}
}

func TestVideoProjectShotVersionDetailRouteReturnsLiuguangDetailData(t *testing.T) {
	routes, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}

	for _, want := range []string{
		`"/api/video/shots-video-versions/detail/"`,
		"s.videoShotVideoVersionDetail",
	} {
		if !strings.Contains(string(routes), want) {
			t.Fatalf("expected detail route registration to include %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Server) videoShotVideoVersionDetail",
		"GetShotVideoVersionDetail",
	} {
		if !strings.Contains(string(handlers), want) {
			t.Fatalf("expected detail handler to include %q", want)
		}
	}

	for _, want := range []string{
		"type ShotVideoVersionDetail struct",
		"GetShotVideoVersionDetail",
		"ShotVideoVersionDetailReference",
		"ShotAssets",
		"UsedImages",
		"UsedVideos",
	} {
		if !strings.Contains(string(store), want) {
			t.Fatalf("expected store detail implementation to include %q", want)
		}
	}
}

func TestVideoProjectShotVersionDetailPersistsUsedAudioReferences(t *testing.T) {
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}
	generator, err := os.ReadFile("../videoproject/generator.go")
	if err != nil {
		t.Fatalf("read generator.go: %v", err)
	}
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	storeSource := string(store)
	generatorSource := string(generator)
	schemaSource := string(schema)

	for _, want := range []string{
		"used_audios",
		"UsedAudios",
		"pushReference(\"audio\", audio",
		"MarkShotGenerating(ctx context.Context, shotID, generationID, prompt string, images, videos, audios []string)",
	} {
		if !strings.Contains(storeSource, want) {
			t.Fatalf("expected video project store to persist used audio references with %q", want)
		}
	}
	if !strings.Contains(generatorSource, "preview.Audios") {
		t.Fatalf("expected generator to pass preview audio references into MarkShotGenerating")
	}
	if !strings.Contains(schemaSource, "used_audios") {
		t.Fatalf("expected video_shots schema to include used_audios")
	}
}

func TestVideoProjectShotVersionDetailUsesGenerationScopedReferences(t *testing.T) {
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	storeSource := string(store)
	schemaSource := string(schema)

	generationTable := sourceBetweenForVideoProjectVersionsTest(
		t,
		schemaSource,
		"CREATE TABLE IF NOT EXISTS video_generations (",
		");\n\nALTER TABLE video_generations",
	)
	for _, want := range []string{"used_images", "used_videos", "used_audios"} {
		if !strings.Contains(generationTable, want) {
			t.Fatalf("expected video_generations schema to persist generation-scoped reference field %q", want)
		}
	}

	detailSource := sourceBetweenForVideoProjectVersionsTest(
		t,
		storeSource,
		"func (s *Store) GetShotVideoVersionDetail",
		"func (s *Store) SetShotVideoVersionBackup",
	)
	for _, want := range []string{
		"getShotVideoVersionUsedReferences",
		"g.used_images",
		"g.used_videos",
		"g.used_audios",
		"fallbackToCurrentShotUsedReferences",
	} {
		if !strings.Contains(detailSource, want) {
			t.Fatalf("expected detail source to read generation-scoped references with %q", want)
		}
	}

	markSource := sourceBetweenForVideoProjectVersionsTest(
		t,
		storeSource,
		"func (s *Store) MarkShotGenerating",
		"// MarkShotCompleted",
	)
	for _, want := range []string{
		"used_images=",
		"used_videos=",
		"used_audios=",
		"toJSONArray(images)",
		"toJSONArray(videos)",
		"toJSONArray(audios)",
	} {
		if !strings.Contains(markSource, want) {
			t.Fatalf("expected MarkShotGenerating to persist refs on video_generations with %q", want)
		}
	}

	setCurrentSource := sourceBetweenForVideoProjectVersionsTest(
		t,
		storeSource,
		"func (s *Store) SetShotVideoVersion",
		"func (s *Store) MarkShotVideoVersionViewed",
	)
	for _, want := range []string{
		"genUsedImages",
		"genUsedVideos",
		"genUsedAudios",
		"used_images=$",
		"used_videos=$",
		"used_audios=$",
	} {
		if !strings.Contains(setCurrentSource, want) {
			t.Fatalf("expected SetShotVideoVersion to sync the selected generation refs back to the shot with %q", want)
		}
	}
}

func TestVideoProjectShotVersionDerivedVersionsCopyGenerationScopedReferences(t *testing.T) {
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}
	storeSource := string(store)

	for _, tc := range []struct {
		name  string
		start string
		end   string
	}{
		{
			name:  "subtitle removed version",
			start: "func (s *Store) CreateSubtitleRemovedShotVideoVersion",
			end:   "func (s *Store) CreateUpscaledShotVideoVersion",
		},
		{
			name:  "upscaled version",
			start: "func (s *Store) CreateUpscaledShotVideoVersion",
			end:   "func (s *Store) SetShotVideoVersion",
		},
		{
			name:  "copied version",
			start: "func (s *Store) CopyShotVideoVersion",
			end:   "func (s *Store) DeleteShotVideoVersion",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			section := sourceBetweenForVideoProjectVersionsTest(t, storeSource, tc.start, tc.end)
			for _, want := range []string{"used_images", "used_videos", "used_audios"} {
				if !strings.Contains(section, want) {
					t.Fatalf("expected %s to copy generation-scoped refs with %q", tc.name, want)
				}
			}
		})
	}
}

func TestVideoProjectShotVersionRemoveSubtitleRouteCreatesLiuguangSubtitleRemovedVersion(t *testing.T) {
	routes, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}
	generator, err := os.ReadFile("../videoproject/generator.go")
	if err != nil {
		t.Fatalf("read generator.go: %v", err)
	}
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	for _, want := range []string{
		`"/api/video/shots-video-versions/remove-subtitle/"`,
		"s.removeShotVideoVersionSubtitle",
	} {
		if !strings.Contains(string(routes), want) {
			t.Fatalf("expected remove-subtitle route registration to include %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Server) removeShotVideoVersionSubtitle",
		"RemoveShotVideoVersionSubtitle",
	} {
		if !strings.Contains(string(handlers), want) {
			t.Fatalf("expected remove-subtitle handler to include %q", want)
		}
	}

	for _, want := range []string{
		"SubtitleRemove",
		"subtitle_remove",
		"CreateSubtitleRemovedShotVideoVersion",
	} {
		if !strings.Contains(string(store), want) {
			t.Fatalf("expected store to persist subtitle removal with %q", want)
		}
	}

	for _, want := range []string{
		"func (g *Generator) RemoveShotVideoVersionSubtitle",
		"stripSubtitleTracks",
		"video/subtitle-removed",
		"CreateSubtitleRemovedShotVideoVersion",
	} {
		if !strings.Contains(string(generator), want) {
			t.Fatalf("expected generator remove-subtitle implementation to include %q", want)
		}
	}

	if !strings.Contains(string(schema), "subtitle_remove") {
		t.Fatalf("expected video_generations schema to include subtitle_remove")
	}
}

func TestVideoProjectShotVersionUpscaleRouteCreatesLiuguangUpscaledVersion(t *testing.T) {
	routes, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	handlers, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatalf("read videoproject_routes.go: %v", err)
	}
	store, err := os.ReadFile("../videoproject/videoproject.go")
	if err != nil {
		t.Fatalf("read videoproject store: %v", err)
	}
	generator, err := os.ReadFile("../videoproject/generator.go")
	if err != nil {
		t.Fatalf("read generator.go: %v", err)
	}
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	for _, want := range []string{
		`"/api/video/shots-video-versions/upscale/"`,
		"s.upscaleShotVideoVersion",
	} {
		if !strings.Contains(string(routes), want) {
			t.Fatalf("expected upscale route registration to include %q", want)
		}
	}

	for _, want := range []string{
		"upscaleShotVideoVersionInput",
		"func (s *Server) upscaleShotVideoVersion",
		"UpscaleShotVideoVersion",
		"resolution",
	} {
		if !strings.Contains(string(handlers), want) {
			t.Fatalf("expected upscale handler to include %q", want)
		}
	}

	for _, want := range []string{
		"UpscaledFlag",
		"UpscaledResolution",
		"upscaled_flag",
		"upscaled_resolution",
		"CreateUpscaledShotVideoVersion",
	} {
		if !strings.Contains(string(store), want) {
			t.Fatalf("expected store to persist upscale metadata with %q", want)
		}
	}

	for _, want := range []string{
		"func (g *Generator) UpscaleShotVideoVersion",
		"parseUpscaleResolution",
		"upscaleVideoFile",
		"video/upscaled",
		"CreateUpscaledShotVideoVersion",
	} {
		if !strings.Contains(string(generator), want) {
			t.Fatalf("expected generator upscale implementation to include %q", want)
		}
	}

	for _, want := range []string{"upscaled_flag", "upscaled_resolution"} {
		if !strings.Contains(string(schema), want) {
			t.Fatalf("expected video_generations schema to include %q", want)
		}
	}
}

func TestVideoProjectShotAssetsMigrationBackfillsExistingTables(t *testing.T) {
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	source := string(schema)

	for _, want := range []string{
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS asset_type",
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS object_url",
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS name",
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS mime_type",
		"ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS size_bytes",
		"ALTER TABLE video_shot_assets ALTER COLUMN project_asset_id DROP NOT NULL",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("expected schema migration to include %q", want)
		}
	}
}
