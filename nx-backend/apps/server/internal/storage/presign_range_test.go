package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

func expectedOSSQueryV1(r *http.Request, accessKey, secret, bucket, objectKey string, now time.Time) ([]byte, bool) {
	query := r.URL.Query()
	if query.Get("OSSAccessKeyId") != accessKey {
		return nil, false
	}
	expires, err := strconv.ParseInt(query.Get("Expires"), 10, 64)
	if err != nil {
		return nil, false
	}
	if expires <= now.Unix() {
		return nil, false
	}
	canonicalResource := "/" + bucket + "/" + objectKey
	stringToSign := strings.Join([]string{r.Method, "", "", query.Get("Expires"), canonicalResource}, "\n")
	h := hmac.New(sha1.New, []byte(secret))
	_, _ = io.WriteString(h, stringToSign)
	return h.Sum(nil), true
}

func verifyOSSQueryV1(r *http.Request, accessKey, secret, bucket, objectKey string, now time.Time) bool {
	expected, ok := expectedOSSQueryV1(r, accessKey, secret, bucket, objectKey, now)
	if !ok {
		return false
	}
	actual, err := base64.StdEncoding.DecodeString(r.URL.Query().Get("Signature"))
	return err == nil && hmac.Equal(actual, expected)
}

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
	if parsed.Scheme != "https" || parsed.Query().Get("Expires") == "" || parsed.Query().Get("Signature") == "" || parsed.Query().Get("OSSAccessKeyId") == "" {
		t.Fatalf("expected a short-lived HTTPS OSS GET URL, got %s", signedURL)
	}
}

func TestOSSClassroomPlaybackPresignedURLServesByteRange(t *testing.T) {
	const (
		accessKey = "release-verification-key"
		secret    = "release-verification-secret"
		bucket    = "private-classroom"
		objectKey = "classroom/private/content-21.mp4"
		media     = "0123456789abcdef"
	)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/"+bucket+"/"+objectKey {
			http.Error(w, "unexpected classroom playback object path", http.StatusBadRequest)
			return
		}
		if !verifyOSSQueryV1(r, accessKey, secret, bucket, objectKey, time.Now()) {
			http.Error(w, "invalid OSS signature", http.StatusForbidden)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Range"), "bytes=") {
			http.Error(w, "missing Range header", http.StatusBadRequest)
			return
		}
		http.ServeContent(w, r, objectKey, time.Unix(0, 0), bytes.NewReader([]byte(media)))
	}))
	t.Cleanup(origin.Close)

	uploader, err := NewOSSUploader(OSSConfig{
		AccessKeyID:     accessKey,
		AccessKeySecret: secret,
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
		WithSignatureVersion(oss.SignatureVersionV1).
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secret)))

	playbackURL, err := uploader.PresignGetURL(context.Background(), objectKey, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	fetchRange := func(rawURL, byteRange string) (*http.Response, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Range", byteRange)
		response, err := origin.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return response, body
	}

	response, body := fetchRange(playbackURL, "bytes=4-8")
	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("Range response status=%d body=%q", response.StatusCode, body)
	}
	if got := response.Header.Get("Content-Range"); got != "bytes 4-8/16" {
		t.Fatalf("Content-Range=%q", got)
	}
	if string(body) != "45678" {
		t.Fatalf("Range body=%q", body)
	}

	dynamic, dynamicBody := fetchRange(playbackURL, "bytes=9-11")
	if dynamic.StatusCode != http.StatusPartialContent || dynamic.Header.Get("Content-Range") != "bytes 9-11/16" || string(dynamicBody) != "9ab" {
		t.Fatalf("same signed URL must accept a dynamic Range: status=%d content-range=%q body=%q", dynamic.StatusCode, dynamic.Header.Get("Content-Range"), dynamicBody)
	}

	tampered, err := url.Parse(playbackURL)
	if err != nil {
		t.Fatal(err)
	}
	tamperedQuery := tampered.Query()
	signature := tamperedQuery.Get("Signature")
	replacement := "0"
	if strings.HasSuffix(signature, replacement) {
		replacement = "1"
	}
	tamperedQuery.Set("Signature", signature[:len(signature)-1]+replacement)
	tampered.RawQuery = tamperedQuery.Encode()
	tamperedResponse, _ := fetchRange(tampered.String(), "bytes=4-8")
	if tamperedResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered OSS signature status=%d", tamperedResponse.StatusCode)
	}

}
