package videoproject

import (
	"os"
	"strings"
	"testing"
)

func TestComposeInputHash(t *testing.T) {
	shots := []ComposeShotFacts{
		{ShotID: "2", OrderNum: 2, GenerationRevision: 4, SelectedGenerationID: "22", SelectedRevision: 4, SelectedStatus: "completed", SelectedVideoURL: "https://cdn/2.mp4"},
		{ShotID: "1", OrderNum: 1, GenerationRevision: 3, SelectedGenerationID: "11", SelectedRevision: 3, SelectedStatus: "completed", SelectedVideoURL: "https://cdn/1.mp4"},
	}
	first, err := BuildComposeInputSnapshot(shots, ComposeProjectInput{Transition: " fade ", MusicURL: " https://cdn/music.mp3 ", EnableSubtitles: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildComposeInputSnapshot([]ComposeShotFacts{shots[1], shots[0]}, ComposeProjectInput{Transition: "fade", MusicURL: "https://cdn/music.mp3", EnableSubtitles: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.InputHash != second.InputHash {
		t.Fatalf("shot input order changed deterministic hash: %s != %s", first.InputHash, second.InputHash)
	}
	if first.Included[0].ShotID != "1" || first.Included[1].ShotID != "2" {
		t.Fatalf("participants not normalized by order/id: %+v", first.Included)
	}
	partial, err := BuildComposeInputSnapshot(shots, ComposeProjectInput{
		Transition: "fade", MusicURL: "https://cdn/music.mp3", EnableSubtitles: true,
		PartialAcknowledged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.InputHash != first.InputHash {
		t.Fatal("partial acknowledgement UI state must not change content hash")
	}
}

func TestComposeRequiresSelectedVersions(t *testing.T) {
	shots := []ComposeShotFacts{
		{ShotID: "1", OrderNum: 1, GenerationRevision: 3, SelectedGenerationID: "11", SelectedRevision: 3, SelectedStatus: "completed", SelectedVideoURL: "https://cdn/1.mp4"},
		{ShotID: "2", OrderNum: 2, GenerationRevision: 2, LatestGenerationID: "latest-2", LegacyVideoURL: "https://cdn/legacy.mp4"},
	}
	_, err := BuildComposeInputSnapshot(shots, ComposeProjectInput{})
	if err == nil || !strings.Contains(err.Error(), "shot 2") {
		t.Fatalf("default compose must reject missing explicit selection, got %v", err)
	}
}

func TestPartialComposeValidation(t *testing.T) {
	shots := []ComposeShotFacts{
		{ShotID: "1", OrderNum: 1, GenerationRevision: 3, SelectedGenerationID: "11", SelectedRevision: 3, SelectedStatus: "completed", SelectedVideoURL: "https://cdn/1.mp4"},
		{ShotID: "2", OrderNum: 2, GenerationRevision: 2},
	}
	_, err := BuildComposeInputSnapshot(shots, ComposeProjectInput{ExcludedShotIDs: []string{"2"}})
	if err == nil || !strings.Contains(err.Error(), "acknowledgement") {
		t.Fatalf("partial compose without acknowledgement should fail, got %v", err)
	}
	snapshot, err := BuildComposeInputSnapshot(shots, ComposeProjectInput{ExcludedShotIDs: []string{"2"}, PartialAcknowledged: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Included) != 1 || snapshot.Included[0].ShotID != "1" || len(snapshot.ExcludedShotIDs) != 1 || snapshot.ExcludedShotIDs[0] != "2" {
		t.Fatalf("unexpected server-computed partial snapshot: %+v", snapshot)
	}
	_, err = BuildComposeInputSnapshot(shots, ComposeProjectInput{ExcludedShotIDs: []string{"foreign"}, PartialAcknowledged: true})
	if err == nil || !strings.Contains(err.Error(), "unknown excluded shot") {
		t.Fatalf("forged exclusion should fail, got %v", err)
	}
}

func TestComposeStatusStale(t *testing.T) {
	shots := []ComposeShotFacts{{ShotID: "1", OrderNum: 1, GenerationRevision: 3, SelectedGenerationID: "11", SelectedRevision: 3, SelectedStatus: "completed", SelectedVideoURL: "url"}}
	snapshot, err := BuildComposeInputSnapshot(shots, ComposeProjectInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !ComposeResultIsCurrent(snapshot.InputHash, snapshot.InputHash) {
		t.Fatal("matching input hashes must be current")
	}
	shots[0].GenerationRevision++
	changed, err := BuildComposeInputSnapshot(shots, ComposeProjectInput{})
	if err == nil || changed.InputHash == snapshot.InputHash {
		t.Fatal("stale selection should be rejected and cannot remain current")
	}
	if ComposeResultIsCurrent(snapshot.InputHash, "") {
		t.Fatal("empty current input hash cannot be current")
	}
}

func TestComposeJobSourceContracts(t *testing.T) {
	raw, err := os.ReadFile("projectcomposer.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, fragment := range []string{
		"StartCompose",
		"GetComposeJob",
		"idx_video_compose_jobs_active_project",
		"10", "30", "70", "90", "100",
		"compose_input_snapshot",
		"final_video_input_hash",
		"queued", "processing", "completed", "failed",
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("compose job implementation missing %q", fragment)
		}
	}
}
