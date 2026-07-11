package videoproject

import (
	"os"
	"strings"
	"testing"
)

func TestProjectScriptRevision(t *testing.T) {
	cases := []struct {
		name    string
		before  string
		after   string
		changed bool
	}{
		{name: "same", before: "第一幕\n\n第二幕", after: "第一幕\n\n第二幕", changed: false},
		{name: "line endings", before: "第一幕\r\n\r\n第二幕", after: "第一幕\n\n第二幕", changed: false},
		{name: "surrounding whitespace", before: " 第一幕 \n\n 第二幕 ", after: "第一幕\n\n第二幕", changed: false},
		{name: "content", before: "第一幕", after: "第一幕改", changed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scriptContentChanged(tc.before, tc.after); got != tc.changed {
				t.Fatalf("scriptContentChanged=%v, want %v", got, tc.changed)
			}
		})
	}
}

func TestShotGenerationRevisionTriggers(t *testing.T) {
	base := ShotInput{
		ActionDescription:       "人物走进房间",
		AspectRatio:             "16:9",
		CameraMovement:          "固定",
		CharacterIDs:            []string{"2", "1"},
		Duration:                15,
		DynamicDescription:      "衣摆轻动",
		ImageReferenceModes:     []string{"character_ref", "scene_ref"},
		Name:                    "镜头一",
		SceneID:                 "3",
		ScriptOriginalContent:   "第一幕",
		SoundAndPictureTogether: "together",
		VideoModel:              "model-a",
		VideoReferenceMode:      "none",
		VideoResolution:         "1080p",
	}
	cases := []struct {
		name    string
		mutate  func(*ShotInput)
		changed bool
	}{
		{name: "action", mutate: func(v *ShotInput) { v.ActionDescription = "人物跑进房间" }, changed: true},
		{name: "dynamic", mutate: func(v *ShotInput) { v.DynamicDescription = "窗帘飘动" }, changed: true},
		{name: "characters", mutate: func(v *ShotInput) { v.CharacterIDs = []string{"1", "4"} }, changed: true},
		{name: "character order normalized", mutate: func(v *ShotInput) { v.CharacterIDs = []string{"1", "2"} }, changed: false},
		{name: "scene", mutate: func(v *ShotInput) { v.SceneID = "4" }, changed: true},
		{name: "duration", mutate: func(v *ShotInput) { v.Duration = 10 }, changed: true},
		{name: "aspect", mutate: func(v *ShotInput) { v.AspectRatio = "9:16" }, changed: true},
		{name: "model", mutate: func(v *ShotInput) { v.VideoModel = "model-b" }, changed: true},
		{name: "resolution", mutate: func(v *ShotInput) { v.VideoResolution = "720p" }, changed: true},
		{name: "sound", mutate: func(v *ShotInput) { v.SoundAndPictureTogether = "separate" }, changed: true},
		{name: "image modes", mutate: func(v *ShotInput) { v.ImageReferenceModes = []string{"prev_frame"} }, changed: true},
		{name: "video mode", mutate: func(v *ShotInput) { v.VideoReferenceMode = "prev_video" }, changed: true},
		{name: "camera", mutate: func(v *ShotInput) { v.CameraMovement = "推进" }, changed: true},
		{name: "cosmetic name", mutate: func(v *ShotInput) { v.Name = "新名称" }, changed: false},
		{name: "whitespace", mutate: func(v *ShotInput) { v.ActionDescription = " 人物走进房间 " }, changed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after := base
			after.CharacterIDs = append([]string(nil), base.CharacterIDs...)
			after.ImageReferenceModes = append([]string(nil), base.ImageReferenceModes...)
			tc.mutate(&after)
			if got := shotGenerationInputChanged(base, after); got != tc.changed {
				t.Fatalf("shotGenerationInputChanged=%v, want %v", got, tc.changed)
			}
		})
	}
}

func TestSelectedVersionValidity(t *testing.T) {
	cases := []struct {
		name       string
		shotID     string
		ownerID    string
		status     string
		videoURL   string
		selectable bool
	}{
		{name: "completed", shotID: "1", ownerID: "1", status: "completed", videoURL: "https://cdn/video.mp4", selectable: true},
		{name: "succeeded", shotID: "1", ownerID: "1", status: "succeeded", videoURL: "https://cdn/video.mp4", selectable: true},
		{name: "foreign", shotID: "1", ownerID: "2", status: "completed", videoURL: "https://cdn/video.mp4", selectable: false},
		{name: "failed", shotID: "1", ownerID: "1", status: "failed", videoURL: "https://cdn/video.mp4", selectable: false},
		{name: "missing url", shotID: "1", ownerID: "1", status: "completed", selectable: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canSelectGeneration(tc.shotID, tc.ownerID, tc.status, tc.videoURL); got != tc.selectable {
				t.Fatalf("canSelectGeneration=%v, want %v", got, tc.selectable)
			}
		})
	}
}

func TestWorkflowPersistenceSourceContract(t *testing.T) {
	raw, err := os.ReadFile("videoproject.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, fragment := range []string{
		"ScriptContent",
		"ScriptRevision",
		"FinalVideoInputHash",
		"GenerationRevision",
		"SelectedGenerationID",
		"SourceKey",
		"SourceScriptRevision",
		"SortOrder",
		"script_revision",
		"selected_generation_id",
		"generation_revision",
		"ORDER BY sort_order ASC, id ASC",
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("videoproject persistence is missing %q", fragment)
		}
	}
}
