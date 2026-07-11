package video

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type DemoRenderInput struct {
	AspectRatio string
	Seconds     int
}

type DemoVideo struct {
	Duration float64
	FPS      float64
	Height   int
	Path     string
	Width    int
}

type DemoRenderer interface {
	Render(context.Context, DemoRenderInput) (DemoVideo, error)
}

type FFmpegDemoRenderer struct {
	tempDir string
}

func NewFFmpegDemoRenderer(tempDir string) *FFmpegDemoRenderer {
	if strings.TrimSpace(tempDir) == "" {
		tempDir = os.TempDir()
	}
	return &FFmpegDemoRenderer{tempDir: tempDir}
}

func demoDimensions(aspectRatio string) (int, int) {
	switch strings.TrimSpace(aspectRatio) {
	case "9:16":
		return 360, 640
	case "1:1":
		return 512, 512
	default:
		return 640, 360
	}
}

func (r *FFmpegDemoRenderer) Render(ctx context.Context, input DemoRenderInput) (DemoVideo, error) {
	if input.Seconds <= 0 {
		return DemoVideo{}, fmt.Errorf("demo video duration must be positive")
	}
	if err := os.MkdirAll(r.tempDir, 0o755); err != nil {
		return DemoVideo{}, err
	}
	file, err := os.CreateTemp(r.tempDir, "video-demo-*.mp4")
	if err != nil {
		return DemoVideo{}, err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return DemoVideo{}, err
	}
	_ = os.Remove(path)

	width, height := demoDimensions(input.AspectRatio)
	source := fmt.Sprintf("color=c=0x1d4ed8:s=%dx%d:r=24:d=%d", width, height, input.Seconds)
	command := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", source,
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "28",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart", "-an", "-y", path,
	)
	if output, err := command.CombinedOutput(); err != nil {
		_ = os.Remove(path)
		return DemoVideo{}, fmt.Errorf("render demo video with ffmpeg: %w: %s", err, strings.TrimSpace(string(output)))
	}
	result, err := probeDemoVideo(ctx, path)
	if err != nil {
		_ = os.Remove(path)
		return DemoVideo{}, err
	}
	result.Path = filepath.Clean(path)
	return result, nil
}

func probeDemoVideo(ctx context.Context, path string) (DemoVideo, error) {
	command := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,duration",
		"-of", "json", path,
	)
	output, err := command.Output()
	if err != nil {
		return DemoVideo{}, fmt.Errorf("inspect demo video with ffprobe: %w", err)
	}
	var payload struct {
		Streams []struct {
			Duration  string `json:"duration"`
			FrameRate string `json:"r_frame_rate"`
			Height    int    `json:"height"`
			Width     int    `json:"width"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &payload); err != nil || len(payload.Streams) != 1 {
		return DemoVideo{}, fmt.Errorf("invalid ffprobe response")
	}
	stream := payload.Streams[0]
	duration, err := strconv.ParseFloat(stream.Duration, 64)
	if err != nil {
		return DemoVideo{}, fmt.Errorf("invalid demo duration %q", stream.Duration)
	}
	fps, err := parseFrameRate(stream.FrameRate)
	if err != nil {
		return DemoVideo{}, err
	}
	return DemoVideo{Duration: duration, FPS: fps, Height: stream.Height, Width: stream.Width}, nil
}

func parseFrameRate(raw string) (float64, error) {
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid frame rate %q", raw)
	}
	numerator, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	denominator, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || denominator == 0 {
		return 0, fmt.Errorf("invalid frame rate %q", raw)
	}
	return numerator / denominator, nil
}
