package classroom

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingCoverSigner struct {
	url     string
	err     error
	keys    []string
	expires []time.Duration
}

func (s *recordingCoverSigner) PresignGetURL(_ context.Context, key string, expires time.Duration) (string, error) {
	s.keys = append(s.keys, key)
	s.expires = append(s.expires, expires)
	return s.url, s.err
}

func TestResolveEffectiveCoverPriorityAndSigning(t *testing.T) {
	ttl := 30 * time.Minute
	tests := []struct {
		name       string
		input      CoverInput
		wantURL    string
		wantSource CoverSource
		wantKey    string
	}{
		{"manual wins", CoverInput{ContentType: ContentVideo, ManualObjectKey: "manual/key.jpg", GeneratedObjectKey: "generated/key.jpg", LegacyURL: "https://legacy.example/cover.jpg"}, "https://signed.example/cover", CoverSourceManual, "manual/key.jpg"},
		{"generated video wins over legacy", CoverInput{ContentType: ContentVideo, GeneratedObjectKey: "generated/key.jpg", LegacyURL: "https://legacy.example/cover.jpg"}, "https://signed.example/cover", CoverSourceGenerated, "generated/key.jpg"},
		{"legacy remains unchanged", CoverInput{ContentType: ContentVideo, LegacyURL: "/legacy/cover.jpg"}, "/legacy/cover.jpg", CoverSourceLegacy, ""},
		{"audio gets stable default", CoverInput{ContentType: ContentAudio}, "/api/public/classroom/audio-cover.svg", CoverSourceAudioDefault, ""},
		{"video without cover stays empty", CoverInput{ContentType: ContentVideo}, "", CoverSourceNone, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signer := &recordingCoverSigner{url: "https://signed.example/cover"}
			got, err := ResolveEffectiveCover(context.Background(), test.input, signer, ttl, "/api/public/classroom/audio-cover.svg")
			if err != nil {
				t.Fatal(err)
			}
			if got.URL != test.wantURL || got.Source != test.wantSource {
				t.Fatalf("got=%+v want url=%q source=%q", got, test.wantURL, test.wantSource)
			}
			if test.wantKey == "" {
				if len(signer.keys) != 0 {
					t.Fatalf("unexpected signing calls: %v", signer.keys)
				}
				return
			}
			if len(signer.keys) != 1 || signer.keys[0] != test.wantKey || signer.expires[0] != ttl {
				t.Fatalf("sign calls keys=%v expires=%v", signer.keys, signer.expires)
			}
			if strings.Contains(got.URL, test.wantKey) {
				t.Fatalf("raw object key leaked in resolved URL: %q", got.URL)
			}
		})
	}
}

func TestResolveEffectiveCoverDoesNotUseGeneratedCoverForAudio(t *testing.T) {
	signer := &recordingCoverSigner{url: "https://signed.example/generated"}
	got, err := ResolveEffectiveCover(context.Background(), CoverInput{ContentType: ContentAudio, GeneratedObjectKey: "video-only/key.jpg"}, signer, time.Minute, "/audio.svg")
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "/audio.svg" || got.Source != CoverSourceAudioDefault || len(signer.keys) != 0 {
		t.Fatalf("got=%+v sign calls=%v", got, signer.keys)
	}
}

func TestResolveEffectiveCoverReturnsSigningFailureWithoutLeakingKey(t *testing.T) {
	key := "private/manual/secret.jpg"
	signer := &recordingCoverSigner{err: errors.New("signer unavailable")}
	got, err := ResolveEffectiveCover(context.Background(), CoverInput{ContentType: ContentVideo, ManualObjectKey: key}, signer, time.Minute, "/audio.svg")
	if err == nil {
		t.Fatal("expected signing error")
	}
	if got.URL != "" || strings.Contains(err.Error(), key) {
		t.Fatalf("key leaked on signing failure: got=%+v err=%v", got, err)
	}
}
