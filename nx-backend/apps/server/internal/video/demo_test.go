package video

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestFFmpegDemoRendererCreatesValidMP4(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	renderer := NewFFmpegDemoRenderer(t.TempDir())

	result, err := renderer.Render(ctx, DemoRenderInput{AspectRatio: "16:9", Seconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(result.Path)

	if result.Width != 640 || result.Height != 360 {
		t.Fatalf("dimensions=%dx%d, want 640x360", result.Width, result.Height)
	}
	if result.Duration < 0.9 || result.Duration > 1.2 {
		t.Fatalf("duration=%f, want approximately 1 second", result.Duration)
	}
	if result.FPS != 24 {
		t.Fatalf("fps=%f, want 24", result.FPS)
	}
	info, err := os.Stat(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("demo MP4 is empty")
	}
}

func TestFFmpegDemoRendererUsesAspectAwareDimensions(t *testing.T) {
	cases := []struct {
		ratio         string
		width, height int
	}{
		{ratio: "16:9", width: 640, height: 360},
		{ratio: "9:16", width: 360, height: 640},
		{ratio: "1:1", width: 512, height: 512},
	}
	for _, tc := range cases {
		t.Run(tc.ratio, func(t *testing.T) {
			width, height := demoDimensions(tc.ratio)
			if width != tc.width || height != tc.height {
				t.Fatalf("demoDimensions(%q)=%dx%d, want %dx%d", tc.ratio, width, height, tc.width, tc.height)
			}
		})
	}
}
