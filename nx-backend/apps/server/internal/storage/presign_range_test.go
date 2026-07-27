package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

func TestOSSPlaybackPresignLeavesRangeHeaderUnsignedForMediaSeeking(t *testing.T) {
	uploader, err := NewOSSUploader(OSSConfig{
		AccessKeyID:     "release-verification-key",
		AccessKeySecret: "release-verification-secret",
		Bucket:          "private-classroom",
		Endpoint:        "https://oss-cn-hangzhou.aliyuncs.com",
		Region:          "cn-hangzhou",
	})
	if err != nil {
		t.Fatal(err)
	}

	signedURL, err := uploader.PresignGetURL(context.Background(), "classroom/video/lesson.mp4", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatal(err)
	}
	presigned, err := uploader.client.Presign(context.Background(), &oss.GetObjectRequest{
		Bucket: oss.Ptr(uploader.bucket),
		Key:    oss.Ptr("classroom/video/lesson.mp4"),
	}, oss.PresignExpires(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for header := range presigned.SignedHeaders {
		if strings.EqualFold(header, "Range") {
			t.Fatalf("Range must remain caller-selectable so video/audio seeking can issue byte ranges: %+v", presigned.SignedHeaders)
		}
	}
	if parsed.Scheme != "https" || parsed.Query().Get("x-oss-expires") == "" || parsed.Query().Get("x-oss-signature") == "" {
		t.Fatalf("expected a short-lived HTTPS OSS GET URL, got %s", signedURL)
	}
}

func TestOSSClassroomPlaybackPresignedURLServesByteRange(t *testing.T) {
	const (
		bucket    = "private-classroom"
		objectKey = "classroom/private/content-21.mp4"
		media     = "0123456789abcdef"
	)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/"+bucket+"/"+objectKey {
			http.Error(w, "unexpected classroom playback object path", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("x-oss-signature") == "" {
			http.Error(w, "missing OSS signature", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("Range"); got != "bytes=4-8" {
			http.Error(w, "unexpected Range header", http.StatusBadRequest)
			return
		}
		http.ServeContent(w, r, objectKey, time.Unix(0, 0), bytes.NewReader([]byte(media)))
	}))
	t.Cleanup(origin.Close)

	uploader, err := NewOSSUploader(OSSConfig{
		AccessKeyID:     "release-verification-key",
		AccessKeySecret: "release-verification-secret",
		Bucket:          bucket,
		Endpoint:        origin.URL,
		Region:          "cn-hangzhou",
	})
	if err != nil {
		t.Fatal(err)
	}
	uploader.client = oss.NewClient(oss.LoadDefaultConfig().
		WithRegion("cn-hangzhou").
		WithEndpoint(origin.URL).
		WithUsePathStyle(true).
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider("release-verification-key", "release-verification-secret")))

	playbackURL, err := uploader.PresignGetURL(context.Background(), objectKey, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, playbackURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=4-8")
	response, err := origin.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("Range response status=%d body=%q", response.StatusCode, body)
	}
	if got := response.Header.Get("Content-Range"); got != "bytes 4-8/16" {
		t.Fatalf("Content-Range=%q", got)
	}
	if string(body) != "45678" {
		t.Fatalf("Range body=%q", body)
	}
}
