package lifestory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGenerationPayloadHashIsCanonicalAndVersionSensitive(t *testing.T) {
	base := GenerationInput{
		RequestKey:      "request-1",
		FactsVersion:    3,
		OutlineVersion:  4,
		SourceVersionID: 7,
		Instruction:     "调整结尾",
	}
	first, err := GenerationPayloadHash(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerationPayloadHash(base)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("hash must be stable: %q != %q", first, second)
	}
	changed := base
	changed.OutlineVersion++
	other, err := GenerationPayloadHash(changed)
	if err != nil {
		t.Fatal(err)
	}
	if first == other {
		t.Fatal("outline version must participate in the payload hash")
	}
}

func TestSnapshotPayloadHashIgnoresJSONBFormattingAndKeyOrder(t *testing.T) {
	first := []byte(`{"storyId":1,"materials":[],"outline":{"tone":"warm","chapters":[]}}`)
	jsonbStyle := []byte(`{ "outline": {"chapters": [], "tone": "warm"}, "materials": [], "storyId": 1 }`)
	if got, want := snapshotPayloadHash(first), snapshotPayloadHash(jsonbStyle); got != want {
		t.Fatalf("canonical snapshot hashes differ: %s != %s", got, want)
	}
}

func TestSnapshotPayloadHashChangesWithStoryStyle(t *testing.T) {
	base := confirmedSnapshot()
	base.Outline.StoryStyle = StoryStyleRealistic
	other := base
	other.Outline = base.Outline
	other.Outline.StoryStyle = StoryStyleMyth

	baseRaw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	otherRaw, err := json.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := snapshotPayloadHash(baseRaw), snapshotPayloadHash(otherRaw); got == want {
		t.Fatalf("story style did not change snapshot hash: %s", got)
	}
}

func TestSnapshotPayloadHMACDistinguishesOriginalPrivateValues(t *testing.T) {
	first := confirmedSnapshot()
	first.Materials[0].Text = "请联系 first@example.com"
	second := first
	second.Materials = append([]Material(nil), first.Materials...)
	second.Materials[0].Text = "请联系 second@example.com"

	firstSafe, _ := TokenizeSnapshot(first)
	secondSafe, _ := TokenizeSnapshot(second)
	firstSafeRaw, err := json.Marshal(firstSafe)
	if err != nil {
		t.Fatal(err)
	}
	secondSafeRaw, err := json.Marshal(secondSafe)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := snapshotPayloadHash(firstSafeRaw), snapshotPayloadHash(secondSafeRaw); got != want {
		t.Fatalf("fixture must reproduce tokenized snapshot collision: %s != %s", got, want)
	}

	firstRaw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	key := tokenKeyBytes("application-secret")
	if got, want := snapshotPayloadHMAC(firstRaw, key), snapshotPayloadHMAC(secondRaw, key); got == want {
		t.Fatalf("original private values produced the same keyed payload hash: %s", got)
	}
	if got, want := snapshotPayloadHMAC(firstRaw, key), snapshotPayloadHMAC(firstRaw, tokenKeyBytes("other-secret")); got == want {
		t.Fatalf("payload hash did not depend on the application key: %s", got)
	}
}

func TestJobJSONDoesNotExposeIntegrityHashes(t *testing.T) {
	raw, err := json.Marshal(Job{
		ID: 1, RequestKey: "request-1", Status: JobQueued,
		PayloadHash: "private-payload-hmac", SnapshotHash: "private-snapshot-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-payload-hmac", "private-snapshot-hash", "payloadHash", "snapshotHash"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("job JSON exposed internal hash %q: %s", secret, raw)
		}
	}
}

func TestTokenizeSnapshotRemovesPrivateNamesFromModelInput(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Materials[0].Text = "真实姓名在北京大学工作。"
	snapshot.FactCard.Characters[0].PrivacyMode = "pseudonym"
	snapshot.FactCard.Events = []FactEvent{{
		Location:    "北京大学",
		Description: "真实姓名在那里完成了一次转变",
		Confirmed:   true,
	}}

	safe, tokenMap := TokenizeSnapshot(snapshot)
	raw, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"真实姓名", "北京大学"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("tokenized snapshot leaked %q: %s", private, raw)
		}
	}
	if len(tokenMap) < 2 {
		t.Fatalf("expected person and location tokens, got %#v", tokenMap)
	}
}

func TestValidateSnapshotUsesConfirmedOutlineValidation(t *testing.T) {
	snapshot := confirmedSnapshot()
	snapshot.Outline.Chapters[0].Title = ""

	err := ValidateSnapshot(snapshot)
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("ValidateSnapshot error=%v, want outline title validation", err)
	}
}

func TestWorkerDefaultsMatchDurableContract(t *testing.T) {
	worker := NewWorker(WorkerConfig{})
	if worker.lease != 2*time.Minute {
		t.Fatalf("lease=%s, want 2m", worker.lease)
	}
	if worker.generationTimeout != 90*time.Second {
		t.Fatalf("timeout=%s, want 90s", worker.generationTimeout)
	}
	if worker.pollInterval != 5*time.Second {
		t.Fatalf("poll=%s, want 5s", worker.pollInterval)
	}
	if worker.concurrency != 2 {
		t.Fatalf("concurrency=%d, want 2", worker.concurrency)
	}
}

func TestReadingProgressJSONUsesZeroBasedChapterIndex(t *testing.T) {
	clientUpdatedAt := "2026-08-30T10:01:00Z"
	raw, err := json.Marshal(ReadingProgress{
		StoryID:         1,
		VersionID:       2,
		ChapterIndex:    0,
		CharacterOffset: 8,
		ClientUpdatedAt: clientUpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"chapterIndex":0`) {
		t.Fatalf("progress JSON must expose chapterIndex: %s", raw)
	}
	var decoded ReadingProgress
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ClientUpdatedAt != clientUpdatedAt {
		t.Fatalf("clientUpdatedAt=%q, want %q", decoded.ClientUpdatedAt, clientUpdatedAt)
	}
}
