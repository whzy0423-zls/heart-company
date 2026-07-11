package videoproject

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposerCopiesLocalUploadSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "video", "generated", "demo.mp4")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("local mp4")
	if err := os.WriteFile(source, want, 0o644); err != nil {
		t.Fatal(err)
	}

	composer := NewComposer(root)
	local, err := composer.downloadFile(context.Background(), "/api/uploads/video/generated/demo.mp4", "copy")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(local)
	got, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("copied bytes = %q, want %q", got, want)
	}
}

func TestComposerRejectsLocalUploadTraversal(t *testing.T) {
	composer := NewComposer(t.TempDir())
	_, err := composer.downloadFile(context.Background(), "/api/uploads/../secret.mp4", "copy")
	if err == nil || !strings.Contains(err.Error(), "upload root") {
		t.Fatalf("expected upload-root traversal error, got %v", err)
	}
}

func TestComposerRejectsLocalUploadSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.mp4")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "video", "generated", "linked.mp4")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	composer := NewComposer(root)
	_, err := composer.downloadFile(context.Background(), "/api/uploads/video/generated/linked.mp4", "copy")
	if err == nil || !strings.Contains(err.Error(), "upload root") {
		t.Fatalf("expected symlink escape error, got %v", err)
	}
}

func TestComposerReportsMissingLocalUpload(t *testing.T) {
	composer := NewComposer(t.TempDir())
	_, err := composer.downloadFile(context.Background(), "/api/uploads/video/generated/missing.mp4", "copy")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing local upload error, got %v", err)
	}
}

func TestComposerComposesRealLocalUploadVideos(t *testing.T) {
	root := t.TempDir()
	for i, color := range []string{"red", "blue"} {
		path := filepath.Join(root, "video", "generated", "demo-"+string(rune('1'+i))+".mp4")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		command := exec.Command("ffmpeg",
			"-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "color=c="+color+":s=320x180:r=24:d=1",
			"-c:v", "libx264", "-pix_fmt", "yuv420p", "-an", "-y", path,
		)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("create fixture MP4: %v: %s", err, output)
		}
	}

	composer := NewComposer(root)
	result, err := composer.ComposeVideos(context.Background(), []string{
		"/api/uploads/video/generated/demo-1.mp4",
		"/api/uploads/video/generated/demo-2.mp4",
	}, ComposeOptions{Transition: "none"})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(result.OutputPath)
	if result.FileSize <= 0 || result.Duration <= 0 {
		t.Fatalf("invalid composed result: %+v", result)
	}
	if output, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", result.OutputPath).CombinedOutput(); err != nil {
		t.Fatalf("ffprobe rejected composed MP4: %v: %s", err, output)
	}
}
