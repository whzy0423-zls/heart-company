package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

func ossV4EscapePath(path string) string {
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~/"
	var out strings.Builder
	for i := 0; i < len(path); i++ {
		if strings.ContainsRune(unreserved, rune(path[i])) {
			out.WriteByte(path[i])
		} else {
			fmt.Fprintf(&out, "%%%02X", path[i])
		}
	}
	return out.String()
}

func ossV4HMAC(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = io.WriteString(h, value)
	return h.Sum(nil)
}

func expectedOSSQueryV4(r *http.Request, accessKey, secret, region string, now time.Time) ([]byte, bool) {
	query := r.URL.Query()
	credential := strings.Split(query.Get("x-oss-credential"), "/")
	if query.Get("x-oss-signature-version") != "OSS4-HMAC-SHA256" || len(credential) != 5 || credential[0] != accessKey || credential[2] != region || credential[3] != "oss" || credential[4] != "aliyun_v4_request" {
		return nil, false
	}
	signedAt, err := time.Parse("20060102T150405Z", query.Get("x-oss-date"))
	if err != nil || credential[1] != signedAt.UTC().Format("20060102") {
		return nil, false
	}
	expires, err := strconv.ParseInt(query.Get("x-oss-expires"), 10, 64)
	if err != nil || expires <= 0 || now.Before(signedAt.Add(-time.Minute)) || now.After(signedAt.Add(time.Duration(expires)*time.Second)) {
		return nil, false
	}

	additional := strings.FieldsFunc(strings.ToLower(query.Get("x-oss-additional-headers")), func(r rune) bool { return r == ';' })
	sort.Strings(additional)
	var canonicalHeaders strings.Builder
	for _, name := range additional {
		values := r.Header.Values(name)
		if len(values) == 0 {
			return nil, false
		}
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(strings.Join(values, ","))
		canonicalHeaders.WriteByte('\n')
	}

	query.Del("x-oss-signature")
	canonicalQuery := strings.ReplaceAll(query.Encode(), "+", "%20")
	canonicalRequest := strings.Join([]string{
		r.Method,
		ossV4EscapePath(r.URL.Path),
		canonicalQuery,
		canonicalHeaders.String(),
		strings.Join(additional, ";"),
		"UNSIGNED-PAYLOAD",
	}, "\n")
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	scope := strings.Join(credential[1:], "/")
	stringToSign := strings.Join([]string{"OSS4-HMAC-SHA256", query.Get("x-oss-date"), scope, hex.EncodeToString(canonicalHash[:])}, "\n")
	dateKey := ossV4HMAC([]byte("aliyun_v4"+secret), credential[1])
	regionKey := ossV4HMAC(dateKey, credential[2])
	productKey := ossV4HMAC(regionKey, credential[3])
	signingKey := ossV4HMAC(productKey, credential[4])
	return ossV4HMAC(signingKey, stringToSign), true
}

func verifyOSSQueryV4(r *http.Request, accessKey, secret, region string, now time.Time) bool {
	expected, ok := expectedOSSQueryV4(r, accessKey, secret, region, now)
	if !ok {
		return false
	}
	actual, err := hex.DecodeString(r.URL.Query().Get("x-oss-signature"))
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
	if parsed.Scheme != "https" || parsed.Query().Get("x-oss-expires") == "" || parsed.Query().Get("x-oss-signature") == "" {
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
		if !verifyOSSQueryV4(r, accessKey, secret, "cn-hangzhou", time.Now()) {
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
	signature := tamperedQuery.Get("x-oss-signature")
	replacement := "0"
	if strings.HasSuffix(signature, replacement) {
		replacement = "1"
	}
	tamperedQuery.Set("x-oss-signature", signature[:len(signature)-1]+replacement)
	tampered.RawQuery = tamperedQuery.Encode()
	tamperedResponse, _ := fetchRange(tampered.String(), "bytes=4-8")
	if tamperedResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered OSS signature status=%d", tamperedResponse.StatusCode)
	}

	constrained, err := url.Parse(playbackURL)
	if err != nil {
		t.Fatal(err)
	}
	constrainedQuery := constrained.Query()
	constrainedQuery.Set("x-oss-additional-headers", "range")
	constrained.RawQuery = constrainedQuery.Encode()
	constrainedRequest, err := http.NewRequest(http.MethodGet, constrained.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	constrainedRequest.Header.Set("Range", "bytes=4-8")
	constrainedSignature, ok := expectedOSSQueryV4(constrainedRequest, accessKey, secret, "cn-hangzhou", time.Now())
	if !ok {
		t.Fatal("build deliberately Range-constrained signature")
	}
	constrainedQuery.Set("x-oss-signature", hex.EncodeToString(constrainedSignature))
	constrained.RawQuery = constrainedQuery.Encode()
	constrainedOK, _ := fetchRange(constrained.String(), "bytes=4-8")
	if constrainedOK.StatusCode != http.StatusPartialContent {
		t.Fatalf("Range-constrained control request status=%d", constrainedOK.StatusCode)
	}
	constrainedChanged, _ := fetchRange(constrained.String(), "bytes=9-11")
	if constrainedChanged.StatusCode != http.StatusForbidden {
		t.Fatalf("changing a mistakenly signed Range must fail verification, status=%d", constrainedChanged.StatusCode)
	}
}
