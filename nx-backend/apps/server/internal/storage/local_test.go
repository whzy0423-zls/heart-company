package storage

import (
	"context"
	"strings"
	"testing"
)

func TestLocalUploaderStoresFileAndReturnsPreviewURL(t *testing.T) {
	root := t.TempDir()
	uploader := NewLocalUploader(root, "/api/uploads")

	result, err := uploader.Upload(context.Background(), UploadInput{
		ContentType: "image/png",
		Dir:         "../site-logo",
		Filename:    "九型 Logo.png",
		Reader:      strings.NewReader("image"),
		Size:        5,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(result.Key, "site-logo/") {
		t.Fatalf("expected sanitized key under site-logo, got %q", result.Key)
	}
	if !strings.HasPrefix(result.URL, "/api/uploads/site-logo/") {
		t.Fatalf("expected preview URL under /api/uploads, got %q", result.URL)
	}
	if result.Name != "Logo.png" || result.ContentType != "image/png" || result.Size != 5 {
		t.Fatalf("unexpected upload result: %+v", result)
	}
}
