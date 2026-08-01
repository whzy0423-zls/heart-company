package video

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestValidateGenerateRequestRejectsStaleCapabilityVersion(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	req := GenerateRequest{Model: caps.Model, Prompt: "雨夜车站", CapabilityVersion: "old-version", Duration: 15, AspectRatio: "16:9", TaskMode: "reference"}
	err := ValidateGenerateRequest(req, caps)
	validationErr := assertValidationCode(t, err, "capability_version_stale")
	if validationErr.LatestCapabilities == nil || validationErr.LatestCapabilities.CapabilityVersion != caps.CapabilityVersion {
		t.Fatalf("stale capability error must include latest capabilities, got %+v", validationErr.LatestCapabilities)
	}
}

func TestValidateGenerateRequestRequiresPrompt(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	req := validLegacyGenerateRequest(caps)
	req.Prompt = " \t\n "

	validationErr := assertValidationCode(t, ValidateGenerateRequest(req, caps), "prompt_required")
	if validationErr.Field != "prompt" || validationErr.Message == "" || validationErr.Fix == "" {
		t.Fatalf("prompt error is not actionable: %+v", validationErr)
	}
}

func TestValidateGenerateRequestRejectsUnsupportedParametersAndLimits(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	seed := 7
	cameraFixed := false
	generateAudio := false
	videoEightSeconds := 8.0
	audioSixteenSeconds := 16.0

	tests := []struct {
		name   string
		mutate func(*GenerateRequest)
		code   string
		field  string
	}{
		{
			name: "seed",
			mutate: func(req *GenerateRequest) {
				req.Seed = &seed
			},
			code:  "seed_unsupported",
			field: "seed",
		},
		{
			name: "camera fixed",
			mutate: func(req *GenerateRequest) {
				req.CameraFixed = &cameraFixed
			},
			code:  "camera_fixed_unsupported",
			field: "cameraFixed",
		},
		{
			name: "resolution",
			mutate: func(req *GenerateRequest) {
				req.Resolution = "1080P"
			},
			code:  "resolution_unsupported",
			field: "resolution",
		},
		{
			name: "generate audio",
			mutate: func(req *GenerateRequest) {
				req.GenerateAudio = &generateAudio
			},
			code:  "generate_audio_unsupported",
			field: "generateAudio",
		},
		{
			name: "duration",
			mutate: func(req *GenerateRequest) {
				req.Duration = 12
			},
			code:  "duration_unsupported",
			field: "duration",
		},
		{
			name: "aspect ratio",
			mutate: func(req *GenerateRequest) {
				req.AspectRatio = "4:3"
			},
			code:  "aspect_ratio_unsupported",
			field: "aspectRatio",
		},
		{
			name: "reference role",
			mutate: func(req *GenerateRequest) {
				req.References = []Reference{{ID: "1", Kind: "image", Role: "first_frame", URL: "https://cdn.example.com/i1.png"}}
			},
			code:  "reference_role_unsupported",
			field: "references[0].role",
		},
		{
			name: "image count",
			mutate: func(req *GenerateRequest) {
				req.References = makeReferences("image", "reference_image", caps.Limits.MaxImages+1, nil)
			},
			code:  "max_images_exceeded",
			field: "references",
		},
		{
			name: "video count",
			mutate: func(req *GenerateRequest) {
				req.References = makeReferences("video", "reference_video", caps.Limits.MaxVideos+1, nil)
			},
			code:  "max_videos_exceeded",
			field: "references",
		},
		{
			name: "audio count",
			mutate: func(req *GenerateRequest) {
				req.References = makeReferences("audio", "reference_audio", caps.Limits.MaxAudios+1, nil)
			},
			code:  "max_audios_exceeded",
			field: "references",
		},
		{
			name: "total video seconds",
			mutate: func(req *GenerateRequest) {
				req.References = makeReferences("video", "reference_video", 2, &videoEightSeconds)
			},
			code:  "max_video_seconds_total_exceeded",
			field: "references",
		},
		{
			name: "total audio seconds",
			mutate: func(req *GenerateRequest) {
				req.References = makeReferences("audio", "reference_audio", 1, &audioSixteenSeconds)
			},
			code:  "max_audio_seconds_total_exceeded",
			field: "references",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validLegacyGenerateRequest(caps)
			tt.mutate(&req)
			validationErr := assertValidationCode(t, ValidateGenerateRequest(req, caps), tt.code)
			if validationErr.Field != tt.field {
				t.Fatalf("validation field = %q, want %q", validationErr.Field, tt.field)
			}
			if validationErr.Message == "" || validationErr.Fix == "" {
				t.Fatalf("typed validation error must include message and fix: %+v", validationErr)
			}
		})
	}
}

func TestValidateGenerateRequestEnforcesTaskTargetPredicates(t *testing.T) {
	caps := targetCapableCapabilities()

	t.Run("reference permits pure text to video", func(t *testing.T) {
		req := validLegacyGenerateRequest(caps)
		if err := ValidateGenerateRequest(req, caps); err != nil {
			t.Fatalf("pure T2V must be valid in reference mode: %v", err)
		}
	})

	t.Run("edit requires exactly one edit target", func(t *testing.T) {
		req := validLegacyGenerateRequest(caps)
		req.TaskMode = "edit"
		req.References = []Reference{{ID: "1", Kind: "video", Role: "edit_target", URL: "https://cdn.example.com/v1.mp4"}}
		if err := ValidateGenerateRequest(req, caps); err != nil {
			t.Fatalf("one edit target must be valid: %v", err)
		}
	})

	t.Run("extend requires exactly one extend target", func(t *testing.T) {
		req := validLegacyGenerateRequest(caps)
		req.TaskMode = "extend"
		req.References = []Reference{{ID: "1", Kind: "video", Role: "extend_target", URL: "https://cdn.example.com/v1.mp4"}}
		if err := ValidateGenerateRequest(req, caps); err != nil {
			t.Fatalf("one extend target must be valid: %v", err)
		}
	})

	tests := []struct {
		name string
		mode string
		refs []Reference
		code string
	}{
		{
			name: "reference rejects a target role",
			mode: "reference",
			refs: []Reference{{ID: "1", Kind: "video", Role: "edit_target"}},
			code: "target_role_not_allowed",
		},
		{
			name: "edit target required when absent",
			mode: "edit",
			code: "edit_target_required",
		},
		{
			name: "edit with only extend target still needs edit target",
			mode: "edit",
			refs: []Reference{{ID: "1", Kind: "video", Role: "extend_target"}},
			code: "edit_target_required",
		},
		{
			name: "multiple edit targets",
			mode: "edit",
			refs: []Reference{
				{ID: "1", Kind: "video", Role: "edit_target"},
				{ID: "2", Kind: "video", Role: "edit_target"},
			},
			code: "multiple_edit_targets",
		},
		{
			name: "extend target required when absent",
			mode: "extend",
			code: "extend_target_required",
		},
		{
			name: "extend with only edit target still needs extend target",
			mode: "extend",
			refs: []Reference{{ID: "1", Kind: "video", Role: "edit_target"}},
			code: "extend_target_required",
		},
		{
			name: "multiple extend targets",
			mode: "extend",
			refs: []Reference{
				{ID: "1", Kind: "video", Role: "extend_target"},
				{ID: "2", Kind: "video", Role: "extend_target"},
			},
			code: "multiple_extend_targets",
		},
		{
			name: "mixed target roles",
			mode: "edit",
			refs: []Reference{
				{ID: "1", Kind: "video", Role: "edit_target"},
				{ID: "2", Kind: "video", Role: "extend_target"},
			},
			code: "mixed_target_roles",
		},
		{
			name: "unknown mode fails closed",
			mode: "future-mode",
			code: "task_mode_unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validLegacyGenerateRequest(caps)
			req.TaskMode = tt.mode
			req.References = referencesWithPublicURLs(tt.refs)
			assertValidationCode(t, ValidateGenerateRequest(req, caps), tt.code)
		})
	}
}

func TestValidateGenerateRequestPrioritizesReferenceBasicsBeforeTargets(t *testing.T) {
	caps := targetCapableCapabilities()
	tests := []struct {
		name  string
		first Reference
		code  string
		field string
	}{
		{
			name:  "kind before mixed targets",
			first: Reference{ID: "bad-kind", Kind: "document", Role: "typo_target", URL: "https://cdn.example.com/bad.bin"},
			code:  "reference_kind_unsupported",
			field: "references[0].kind",
		},
		{
			name:  "role before mixed targets",
			first: Reference{ID: "bad-role", Kind: "video", Role: "typo_target", URL: "https://cdn.example.com/bad.mp4"},
			code:  "reference_role_unsupported",
			field: "references[0].role",
		},
		{
			name:  "kind role mismatch before mixed targets",
			first: Reference{ID: "bad-pair", Kind: "image", Role: "reference_video", URL: "https://cdn.example.com/bad.png"},
			code:  "reference_kind_role_mismatch",
			field: "references[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validLegacyGenerateRequest(caps)
			req.TaskMode = "edit"
			req.References = []Reference{
				tt.first,
				{ID: "edit", Kind: "video", Role: "edit_target", URL: "https://cdn.example.com/edit.mp4"},
				{ID: "extend", Kind: "video", Role: "extend_target", URL: "https://cdn.example.com/extend.mp4"},
			}

			validationErr := assertValidationCode(t, ValidateGenerateRequest(req, caps), tt.code)
			if validationErr.Field != tt.field {
				t.Fatalf("field = %q, want %q", validationErr.Field, tt.field)
			}
		})
	}
}

func TestValidateGenerateRequestValidatesReferenceURLsBeforeTargets(t *testing.T) {
	caps := targetCapableCapabilities()
	tests := []struct {
		name string
		url  string
		code string
	}{
		{name: "empty", url: "", code: "reference_url_required"},
		{name: "blank", url: " \t ", code: "reference_url_required"},
		{name: "file scheme", url: "file:///tmp/ref.mp4", code: "reference_url_invalid"},
		{name: "data scheme", url: "data:video/mp4;base64,AAAA", code: "reference_url_invalid"},
		{name: "ftp scheme", url: "ftp://cdn.example.com/ref.mp4", code: "reference_url_invalid"},
		{name: "scheme relative", url: "//cdn.example.com/ref.mp4", code: "reference_url_invalid"},
		{name: "unix file path", url: "/tmp/ref.mp4", code: "reference_url_invalid"},
		{name: "windows file path", url: `C:\tmp\ref.mp4`, code: "reference_url_invalid"},
		{name: "temporary path traversal", url: "/pg/assets/../secret.mp4", code: "reference_url_invalid"},
		{name: "encoded temporary path traversal", url: "/pg/assets/%2e%2e/secret.mp4", code: "reference_url_invalid"},
		{name: "temporary path without asset", url: "/pg/assets/", code: "reference_url_invalid"},
		{name: "userinfo", url: "https://user:password@cdn.example.com/ref.mp4", code: "reference_url_invalid"},
		{name: "localhost", url: "http://localhost/ref.mp4", code: "reference_url_invalid"},
		{name: "local domain", url: "https://storage.service.local/ref.mp4", code: "reference_url_invalid"},
		{name: "loopback ipv4", url: "http://127.0.0.1/ref.mp4", code: "reference_url_invalid"},
		{name: "loopback ipv6", url: "http://[::1]/ref.mp4", code: "reference_url_invalid"},
		{name: "rfc1918 10", url: "http://10.1.2.3/ref.mp4", code: "reference_url_invalid"},
		{name: "rfc1918 172", url: "http://172.16.2.3/ref.mp4", code: "reference_url_invalid"},
		{name: "rfc1918 192", url: "http://192.168.2.3/ref.mp4", code: "reference_url_invalid"},
		{name: "link local", url: "http://169.254.169.254/ref.mp4", code: "reference_url_invalid"},
		{name: "single label host", url: "https://cdn/ref.mp4", code: "reference_url_invalid"},
		{name: "invalid numeric host", url: "https://999.999.999.999/ref.mp4", code: "reference_url_invalid"},
		{name: "invalid domain label", url: "https://-cdn.example.com/ref.mp4", code: "reference_url_invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validLegacyGenerateRequest(caps)
			req.TaskMode = "edit"
			req.References = []Reference{
				{ID: "edit", Kind: "video", Role: "edit_target", URL: tt.url},
				{ID: "extend", Kind: "video", Role: "extend_target", URL: "https://cdn.example.com/extend.mp4"},
			}

			validationErr := assertValidationCode(t, ValidateGenerateRequest(req, caps), tt.code)
			if validationErr.Field != "references[0].url" {
				t.Fatalf("field = %q", validationErr.Field)
			}
		})
	}
}

func TestValidateGenerateRequestAcceptsDocumentedReferenceURLs(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	for _, rawURL := range []string{
		"http://cdn.example.com/ref.png",
		"https://cdn.example.com/ref.png?signature=ok",
		"https://8.8.8.8/ref.png",
		"/pg/assets/session-42/ref.png?token=temporary",
	} {
		t.Run(rawURL, func(t *testing.T) {
			req := validLegacyGenerateRequest(caps)
			req.References = []Reference{{ID: "1", Kind: "image", Role: "reference_image", URL: rawURL}}
			if err := ValidateGenerateRequest(req, caps); err != nil {
				t.Fatalf("documented reference URL %q was rejected: %v", rawURL, err)
			}
		})
	}
}

func TestValidateGenerateRequestRejectsInvalidMediaDurationsBeforeTargets(t *testing.T) {
	caps := targetCapableCapabilities()
	tests := []struct {
		name     string
		duration float64
	}{
		{name: "zero", duration: 0},
		{name: "negative", duration: -0.01},
		{name: "nan", duration: math.NaN()},
		{name: "positive infinity", duration: math.Inf(1)},
		{name: "negative infinity", duration: math.Inf(-1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validLegacyGenerateRequest(caps)
			req.TaskMode = "edit"
			req.References = []Reference{
				{ID: "edit", Kind: "video", Role: "edit_target", URL: "https://cdn.example.com/edit.mp4", DurationSeconds: &tt.duration},
				{ID: "extend", Kind: "video", Role: "extend_target", URL: "https://cdn.example.com/extend.mp4"},
			}

			validationErr := assertValidationCode(t, ValidateGenerateRequest(req, caps), "media_duration_invalid")
			if validationErr.Field != "references[0].durationSeconds" || validationErr.Message == "" || validationErr.Fix == "" {
				t.Fatalf("invalid duration error is not actionable: %+v", validationErr)
			}
		})
	}
}

func TestValidateGenerateRequestUsesStableDurationPrecisionAtLimit(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	durations := []float64{0.01, 4.03, 10.96}
	req := validLegacyGenerateRequest(caps)
	req.References = []Reference{
		{ID: "1", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/1.mp4", DurationSeconds: &durations[0]},
		{ID: "2", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/2.mp4", DurationSeconds: &durations[1]},
		{ID: "3", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/3.mp4", DurationSeconds: &durations[2]},
	}

	if err := ValidateGenerateRequest(req, caps); err != nil {
		t.Fatalf("0.01 + 4.03 + 10.96 must equal the 15 second limit: %v", err)
	}

	durations[2] += 0.000000001
	assertValidationCode(t, ValidateGenerateRequest(req, caps), "max_video_seconds_total_exceeded")
}

func TestValidateGenerateRequestAcceptsAnyFinitePositiveMediaDuration(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	duration := 0.0000000001
	req := validLegacyGenerateRequest(caps)
	req.References = []Reference{{
		ID:              "video-1",
		Kind:            "video",
		Role:            "reference_video",
		URL:             "https://cdn.example.com/1.mp4",
		DurationSeconds: &duration,
	}}

	if err := ValidateGenerateRequest(req, caps); err != nil {
		t.Fatalf("finite positive duration must be accepted even below nanosecond precision: %v", err)
	}
}

func TestDurationNanosecondsRejectsFloatBoundaryOverflow(t *testing.T) {
	const maxInt64 = int64(^uint64(0) >> 1)
	boundary := float64(maxInt64) / float64(time.Second)
	below := math.Nextafter(boundary, 0)
	above := math.Nextafter(boundary, math.Inf(1))

	nanoseconds, ok := durationNanoseconds(below)
	if !ok || nanoseconds <= 0 {
		t.Fatalf("next float below boundary must remain representable: seconds=%v nanos=%d ok=%v", below, nanoseconds, ok)
	}

	for _, tt := range []struct {
		name    string
		seconds float64
	}{
		{name: "rounded max int64 boundary", seconds: boundary},
		{name: "next float above boundary", seconds: above},
		{name: "largest finite float", seconds: math.MaxFloat64},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nanoseconds, ok := durationNanoseconds(tt.seconds)
			if ok || nanoseconds != 0 {
				t.Fatalf("unrepresentable seconds must be rejected before int64 conversion: seconds=%v nanos=%d ok=%v", tt.seconds, nanoseconds, ok)
			}
		})
	}
}

func TestValidateGenerateRequestRejectsUnrepresentableMediaDuration(t *testing.T) {
	const maxInt64 = int64(^uint64(0) >> 1)
	boundary := float64(maxInt64) / float64(time.Second)
	values := []float64{
		boundary,
		math.Nextafter(boundary, math.Inf(1)),
		math.MaxFloat64,
	}
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})

	for _, duration := range values {
		req := validLegacyGenerateRequest(caps)
		req.References = []Reference{{
			ID:              "video-1",
			Kind:            "video",
			Role:            "reference_video",
			URL:             "https://cdn.example.com/1.mp4",
			DurationSeconds: &duration,
		}}

		validationErr := assertValidationCode(t, ValidateGenerateRequest(req, caps), "media_duration_invalid")
		if validationErr.Field != "references[0].durationSeconds" {
			t.Fatalf("field = %q", validationErr.Field)
		}
	}
}

func TestValidateGenerateRequestReportsMissingMediaDurationAsWarnings(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	req := validLegacyGenerateRequest(caps)
	req.References = []Reference{
		{ID: "video-1", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/v1.mp4"},
		{ID: "audio-1", Kind: "audio", Role: "reference_audio", URL: "https://cdn.example.com/a1.mp3"},
	}

	report, err := ValidateGenerateRequestWithWarnings(req, caps)
	if err != nil {
		t.Fatalf("missing durations are warnings, not validation errors: %v", err)
	}
	if len(report.Warnings) != 2 {
		t.Fatalf("warnings = %+v, want one warning per missing duration", report.Warnings)
	}
	for index, warning := range report.Warnings {
		if warning.Code != "media_duration_missing" {
			t.Fatalf("warning %d code = %q", index, warning.Code)
		}
		if warning.Field == "" || warning.Message == "" || warning.Fix == "" {
			t.Fatalf("warning %d is not structured: %+v", index, warning)
		}
	}
}

func TestValidateGenerateRequestWarnsAboveRecommendedReferenceCountInStableOrder(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	req := validLegacyGenerateRequest(caps)
	req.References = []Reference{
		{ID: "image-1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/1.png"},
		{ID: "image-2", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/2.png"},
		{ID: "video-1", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/1.mp4"},
		{ID: "video-2", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/2.mp4"},
		{ID: "video-3", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/3.mp4"},
		{ID: "audio-1", Kind: "audio", Role: "reference_audio", URL: "https://cdn.example.com/1.mp3"},
	}
	wantCodes := []string{
		"reference_count_above_recommended",
		"media_duration_missing",
		"media_duration_missing",
		"media_duration_missing",
		"media_duration_missing",
	}
	wantFields := []string{
		"references",
		"references[2].durationSeconds",
		"references[3].durationSeconds",
		"references[4].durationSeconds",
		"references[5].durationSeconds",
	}

	var first ValidationReport
	for attempt := 0; attempt < 5; attempt++ {
		report, err := ValidateGenerateRequestWithWarnings(req, caps)
		if err != nil {
			t.Fatalf("recommended-count warning must not block generation: %v", err)
		}
		codes := make([]string, len(report.Warnings))
		fields := make([]string, len(report.Warnings))
		for index, warning := range report.Warnings {
			codes[index] = warning.Code
			fields[index] = warning.Field
		}
		if !reflect.DeepEqual(codes, wantCodes) || !reflect.DeepEqual(fields, wantFields) {
			t.Fatalf("warning order is not deterministic:\n codes=%#v\nfields=%#v", codes, fields)
		}
		if attempt == 0 {
			first = report
		} else if !reflect.DeepEqual(report, first) {
			t.Fatalf("attempt %d changed warning output:\nfirst=%+v\n  got=%+v", attempt, first, report)
		}
	}
}

func TestValidateGenerateRequestDoesNotWarnAtRecommendedReferenceCount(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	req := validLegacyGenerateRequest(caps)
	req.References = []Reference{
		{ID: "image-1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/1.png"},
		{ID: "video-1", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/1.mp4"},
		{ID: "video-2", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/2.mp4"},
		{ID: "video-3", Kind: "video", Role: "reference_video", URL: "https://cdn.example.com/3.mp4"},
		{ID: "audio-1", Kind: "audio", Role: "reference_audio", URL: "https://cdn.example.com/1.mp3"},
	}

	report, err := ValidateGenerateRequestWithWarnings(req, caps)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range report.Warnings {
		if warning.Code == "reference_count_above_recommended" {
			t.Fatalf("exactly five references must not trigger the recommendation warning: %+v", report.Warnings)
		}
	}
}

func validLegacyGenerateRequest(caps Capabilities) GenerateRequest {
	return GenerateRequest{
		Model:             caps.Model,
		Prompt:            "雨夜车站，一名旅人转身望向镜头",
		CapabilityVersion: caps.CapabilityVersion,
		Duration:          15,
		AspectRatio:       "16:9",
		TaskMode:          "reference",
	}
}

func targetCapableCapabilities() Capabilities {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	caps.TaskModes = []string{"reference", "edit", "extend"}
	caps.ReferenceRoles = append(caps.ReferenceRoles, "edit_target", "extend_target")
	caps.SupportsEdit = true
	caps.SupportsExtend = true
	return caps
}

func makeReferences(kind, role string, count int, duration *float64) []Reference {
	refs := make([]Reference, 0, count)
	for index := 0; index < count; index++ {
		refs = append(refs, Reference{
			ID:              role,
			Kind:            kind,
			Role:            role,
			URL:             "https://cdn.example.com/reference",
			DurationSeconds: duration,
		})
	}
	return refs
}

func referencesWithPublicURLs(refs []Reference) []Reference {
	result := append([]Reference(nil), refs...)
	for index := range result {
		if result[index].URL == "" {
			result[index].URL = "https://cdn.example.com/reference.mp4"
		}
	}
	return result
}

func assertValidationCode(t *testing.T, err error, want string) *ValidationError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation code %q, got nil", want)
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if validationErr.Code != want {
		t.Fatalf("validation code = %q, want %q (error=%+v)", validationErr.Code, want, validationErr)
	}
	return validationErr
}
