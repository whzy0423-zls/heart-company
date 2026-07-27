package storage

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
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
