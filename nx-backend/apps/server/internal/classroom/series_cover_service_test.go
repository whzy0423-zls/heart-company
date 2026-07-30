package classroom

import (
	"bytes"
	"context"
	"testing"
	"time"
)

type fakeSeriesCoverStore struct {
	series Series
}

func (f *fakeSeriesCoverStore) GetSeries(context.Context, int64) (Series, error) {
	return f.series, nil
}

func (f *fakeSeriesCoverStore) SetSeriesManualCover(_ context.Context, _ int64, key string, expected time.Time, _ *int64) (Series, error) {
	if !f.series.UpdatedAt.Equal(expected) {
		return Series{}, ErrConflict
	}
	f.series.ManualCoverObjectKey = key
	f.series.UpdatedAt = f.series.UpdatedAt.Add(time.Second)
	return f.series, nil
}

func (f *fakeSeriesCoverStore) SetSeriesCoverSettings(_ context.Context, _ int64, ratio CoverAspectRatio, expected time.Time, _ *int64) (Series, error) {
	if !f.series.UpdatedAt.Equal(expected) {
		return Series{}, ErrConflict
	}
	f.series.CoverAspectRatio = ratio
	f.series.UpdatedAt = f.series.UpdatedAt.Add(time.Second)
	return f.series, nil
}

func TestSeriesCoverServiceUploadsDeletesAndUpdatesRatio(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeSeriesCoverStore{series: Series{ID: 5, Title: "系列", Status: SeriesDraft, AccessLevel: AccessPublic, CoverAspectRatio: CoverAspectRatio16x9, UpdatedAt: now}}
	objects := &fakeCoverStorage{}
	svc := NewSeriesCoverService(store, objects, 1024)
	uploaded, err := svc.Upload(context.Background(), 5, now, nil, "cover.png", bytes.NewReader(testPNG))
	if err != nil || uploaded.ManualCoverObjectKey == "" || len(objects.uploads) != 1 {
		t.Fatalf("uploaded=%+v uploads=%v err=%v", uploaded, objects.uploads, err)
	}
	ratioUpdated, err := svc.UpdateSettings(context.Background(), 5, CoverAspectRatio9x16, uploaded.UpdatedAt, nil)
	if err != nil || ratioUpdated.CoverAspectRatio != CoverAspectRatio9x16 {
		t.Fatalf("ratio=%+v err=%v", ratioUpdated, err)
	}
	deleted, err := svc.Delete(context.Background(), 5, ratioUpdated.UpdatedAt, nil)
	if err != nil || deleted.ManualCoverObjectKey != "" || len(objects.deleted) != 1 {
		t.Fatalf("deleted=%+v deletes=%v err=%v", deleted, objects.deleted, err)
	}
}
