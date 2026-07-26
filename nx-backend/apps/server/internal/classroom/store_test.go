package classroom

import (
	"testing"
	"time"
)

func TestSeriesValidationAndTransitions(t *testing.T) {
	valid := Series{Title: "九型入门", Status: SeriesDraft, AccessLevel: AccessPublic}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid series rejected: %v", err)
	}
	if !CanTransitionSeries(SeriesDraft, SeriesPublished) || !CanTransitionSeries(SeriesPublished, SeriesOffline) {
		t.Fatal("expected documented series transitions to be allowed")
	}
	if CanTransitionSeries(SeriesDraft, SeriesOffline) || CanTransitionSeries(SeriesOffline, SeriesPublished) {
		t.Fatal("unexpected series transition allowed")
	}
	for _, tc := range []Series{
		{Title: "x", Status: "unknown", AccessLevel: AccessPublic},
		{Title: "x", Status: SeriesDraft, AccessLevel: AccessInherit},
		{Title: "x", Status: SeriesDraft, AccessLevel: AccessPaid},
		{Title: "x", Status: SeriesDraft, AccessLevel: AccessPublic, PriceCents: 1},
	} {
		if err := tc.Validate(); err == nil {
			t.Fatalf("invalid series accepted: %+v", tc)
		}
	}
	paid := Series{Title: "付费系列", Status: SeriesDraft, AccessLevel: AccessPaid, PriceCents: 9900}
	if err := paid.Validate(); err != nil {
		t.Fatalf("valid paid series rejected: %v", err)
	}
	paid.PlaybackBlocked = true
	if err := paid.Validate(); err != nil {
		t.Fatalf("playback_blocked must be independently valid: %v", err)
	}
}

func TestContentValidationAndTransitions(t *testing.T) {
	seriesID := int64(7)
	valid := Content{SeriesID: &seriesID, ShowAsStandalone: true, Title: "第一课", ContentType: ContentVideo, Status: ContentReady, AccessLevel: AccessInherit}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid series content rejected: %v", err)
	}
	if !CanTransitionContent(ContentDraft, ContentProcessing) || !CanTransitionContent(ContentProcessing, ContentReady) || !CanTransitionContent(ContentReady, ContentPublished) || !CanTransitionContent(ContentPublished, ContentOffline) || !CanTransitionContent(ContentFailed, ContentDraft) {
		t.Fatal("expected documented content transitions to be allowed")
	}
	if CanTransitionContent(ContentDraft, ContentPublished) || CanTransitionContent(ContentFailed, ContentPublished) {
		t.Fatal("unexpected content transition allowed")
	}
	standalone := Content{Title: "独立音频", ContentType: ContentAudio, Status: ContentDraft, AccessLevel: AccessInherit}
	if err := standalone.Validate(); err == nil {
		t.Fatal("standalone content must not inherit access")
	}
	standalone.AccessLevel = AccessLogin
	standalone.ShowAsStandalone = true
	if err := standalone.Validate(); err == nil {
		t.Fatal("show_as_standalone requires a parent series")
	}
	standalone.ShowAsStandalone = false
	standalone.AccessLevel = AccessPaid
	if err := standalone.Validate(); err == nil {
		t.Fatal("paid content requires a positive price")
	}
	standalone.PriceCents = 1990
	standalone.PlaybackBlocked = true
	if err := standalone.Validate(); err != nil {
		t.Fatalf("valid paid content rejected: %v", err)
	}
}

func TestContentPublishRequiresReadyMediaAndPublishedParent(t *testing.T) {
	seriesID := int64(3)
	content := Content{SeriesID: &seriesID, Title: "第一课", ContentType: ContentVideo, Status: ContentReady, AccessLevel: AccessInherit, MediaAssetID: ptrInt64(9)}
	media := MediaAsset{ID: 9, ContentType: ContentVideo, StorageStatus: MediaReady, Bucket: "private", ObjectKey: "classroom/video/9.mp4", ETag: "etag", Checksum: "sha256", SizeBytes: 1024, DurationSeconds: 60}
	publishedParent := Series{ID: 3, Title: "系列", Status: SeriesPublished, AccessLevel: AccessPublic}
	if err := ValidateContentPublish(content, media, &publishedParent); err != nil {
		t.Fatalf("publishable content rejected: %v", err)
	}
	media.StorageStatus = MediaProcessing
	if err := ValidateContentPublish(content, media, &publishedParent); err == nil {
		t.Fatal("content published before media ready")
	}
	media.StorageStatus = MediaReady
	publishedParent.Status = SeriesDraft
	if err := ValidateContentPublish(content, media, &publishedParent); err == nil {
		t.Fatal("content published before parent series")
	}
	if err := ValidateContentPublish(content, media, nil); err == nil {
		t.Fatal("series content published without parent snapshot")
	}
}

func TestMediaUploadEntitlementAndProgressValidation(t *testing.T) {
	media := MediaAsset{Bucket: "private", ObjectKey: "classroom/audio/a.mp3", ETag: "etag", Checksum: "sha256", ContentType: ContentAudio, SizeBytes: 10, DurationSeconds: 2, StorageStatus: MediaReady}
	if err := media.Validate(); err != nil {
		t.Fatalf("valid media rejected: %v", err)
	}
	media.ObjectKey = ""
	if err := media.Validate(); err == nil {
		t.Fatal("ready media without object metadata accepted")
	}
	task := UploadTask{ContentID: 4, CreatorID: 8, OSSUploadID: "upload", ObjectKey: "classroom/video/4.mp4", ExpectedSize: 100, Checksum: "sum", PartSize: 10, MaxParts: 10, Status: UploadInitiated, ExpiresAt: time.Now().Add(time.Hour)}
	if err := task.Validate(); err != nil {
		t.Fatalf("valid upload task rejected: %v", err)
	}
	task.ContentID = 0
	if err := task.Validate(); err == nil {
		t.Fatal("upload task without unique draft binding accepted")
	}
	seriesID, contentID := int64(1), int64(2)
	for _, entitlement := range []Entitlement{{WXUserID: 1, SeriesID: &seriesID, Source: EntitlementPurchase}, {WXUserID: 1, ContentID: &contentID, Source: EntitlementManual}} {
		if err := entitlement.Validate(); err != nil {
			t.Fatalf("valid entitlement rejected: %v", err)
		}
	}
	if err := (Entitlement{WXUserID: 1, SeriesID: &seriesID, ContentID: &contentID, Source: EntitlementPurchase}).Validate(); err == nil {
		t.Fatal("entitlement targeting both series and content accepted")
	}
	if err := (Entitlement{WXUserID: 1, Source: EntitlementPurchase}).Validate(); err == nil {
		t.Fatal("entitlement without target accepted")
	}
}

func ptrInt64(value int64) *int64 { return &value }

func TestUploadTaskBindsExactlyOneDraft(t *testing.T) {
	task := UploadTask{ContentID: 4, CreatorID: 8, OSSUploadID: "upload", ObjectKey: "classroom/video/4.mp4", ExpectedSize: 100, Checksum: "sum", PartSize: 10, MaxParts: 10, Status: UploadInitiated, ExpiresAt: time.Now().Add(time.Hour)}
	draft := Content{ID: 4, Title: "待上传", ContentType: ContentVideo, Status: ContentDraft, AccessLevel: AccessPublic}
	if err := ValidateUploadDraftBinding(task, draft); err != nil {
		t.Fatalf("valid draft binding rejected: %v", err)
	}
	draft.Status = ContentReady
	if err := ValidateUploadDraftBinding(task, draft); err == nil {
		t.Fatal("upload task bound to non-draft content")
	}
	draft.Status = ContentDraft
	draft.ID = 5
	if err := ValidateUploadDraftBinding(task, draft); err == nil {
		t.Fatal("upload task bound to a different draft")
	}
}
