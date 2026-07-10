package video

import (
	"errors"
	"testing"
)

func TestValidateGenerateRequestRejectsStaleCapabilityVersion(t *testing.T) {
	caps := ResolveCapabilities(CapabilityConfig{Model: "video-ds-2.0", GatewayContract: LegacyFlatContract()})
	req := GenerateRequest{Model: caps.Model, CapabilityVersion: "old-version", Duration: 15, AspectRatio: "16:9", TaskMode: "reference"}
	err := ValidateGenerateRequest(req, caps)
	validationErr := assertValidationCode(t, err, "capability_version_stale")
	if validationErr.LatestCapabilities == nil || validationErr.LatestCapabilities.CapabilityVersion != caps.CapabilityVersion {
		t.Fatalf("stale capability error must include latest capabilities, got %+v", validationErr.LatestCapabilities)
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
			req.References = tt.refs
			assertValidationCode(t, ValidateGenerateRequest(req, caps), tt.code)
		})
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

func validLegacyGenerateRequest(caps Capabilities) GenerateRequest {
	return GenerateRequest{
		Model:             caps.Model,
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
