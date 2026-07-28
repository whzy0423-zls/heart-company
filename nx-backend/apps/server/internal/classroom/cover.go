package classroom

import (
	"context"
	"errors"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/storage"
)

type CoverSource string

const (
	CoverSourceManual       CoverSource = "manual"
	CoverSourceGenerated    CoverSource = "generated"
	CoverSourceLegacy       CoverSource = "legacy"
	CoverSourceAudioDefault CoverSource = "audio-default"
	CoverSourceNone         CoverSource = "none"
)

type CoverInput struct {
	ContentType        ContentType
	ManualObjectKey    string
	GeneratedObjectKey string
	LegacyURL          string
}

type ResolvedCover struct {
	URL    string
	Source CoverSource
	Signed bool
}

func ResolveEffectiveCover(ctx context.Context, input CoverInput, signer storage.ObjectSigner, ttl time.Duration, audioDefaultURL string) (ResolvedCover, error) {
	key, source := strings.TrimSpace(input.ManualObjectKey), CoverSourceManual
	if key == "" && input.ContentType == ContentVideo {
		key, source = strings.TrimSpace(input.GeneratedObjectKey), CoverSourceGenerated
	}
	if key != "" {
		if signer == nil {
			return ResolvedCover{}, errors.New("classroom cover signer unavailable")
		}
		url, err := signer.PresignGetURL(ctx, key, ttl)
		if err != nil {
			return ResolvedCover{}, errors.New("sign classroom cover: signer unavailable")
		}
		return ResolvedCover{URL: url, Source: source, Signed: true}, nil
	}
	if legacy := strings.TrimSpace(input.LegacyURL); legacy != "" {
		return ResolvedCover{URL: legacy, Source: CoverSourceLegacy}, nil
	}
	if input.ContentType == ContentAudio {
		return ResolvedCover{URL: strings.TrimSpace(audioDefaultURL), Source: CoverSourceAudioDefault}, nil
	}
	return ResolvedCover{Source: CoverSourceNone}, nil
}
