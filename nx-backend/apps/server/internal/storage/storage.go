package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

type ObjectUploader interface {
	Upload(ctx context.Context, input UploadInput) (UploadResult, error)
}

type ObjectSigner interface {
	PresignGetURL(ctx context.Context, objectKey string, expires time.Duration) (string, error)
}

// MultipartStorage is the narrow boundary used by long-running browser uploads.
// It intentionally coexists with ObjectUploader so existing small-file callers remain unchanged.
var ErrAlreadyGone = errors.New("storage object already gone")

func IsAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAlreadyGone) {
		return true
	}
	var service *oss.ServiceError
	if errors.As(err, &service) {
		return service.StatusCode == 404 || service.Code == "NoSuchUpload" || service.Code == "NoSuchKey" || service.Code == "NoSuchObject"
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "nosuchupload") || strings.Contains(message, "no such upload") || strings.Contains(message, "not found") || strings.Contains(message, "already gone")
}

type MultipartStorage interface {
	InitiateMultipart(context.Context, InitiateMultipartInput) (InitiateMultipartResult, error)
	SignMultipartPart(context.Context, SignPartInput) (SignPartResult, error)
	CompleteMultipart(context.Context, CompleteMultipartInput) (CompleteMultipartResult, error)
	AbortMultipart(context.Context, AbortMultipartInput) error
	ListMultipartParts(context.Context, ListPartsInput) ([]MultipartPart, error)
	HeadObject(context.Context, string) (ObjectMetadata, error)
	DeleteObject(context.Context, string) error
}

type InitiateMultipartInput struct{ ObjectKey, ContentType, Checksum string }
type InitiateMultipartResult struct{ UploadID string }
type SignPartInput struct {
	ObjectKey, UploadID string
	PartNumber          int
	Expires             time.Duration
}
type SignPartResult struct {
	URL        string
	PartNumber int
	ExpiresAt  time.Time
}
type CompletedPart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}
type CompleteMultipartInput struct {
	ObjectKey, UploadID string
	Parts               []CompletedPart
}
type CompleteMultipartResult struct{ ETag, Checksum string }
type AbortMultipartInput struct{ ObjectKey, UploadID string }
type ListPartsInput struct {
	ObjectKey, UploadID string
	MaxParts            int
}
type MultipartPart struct {
	PartNumber int
	ETag       string
	Size       int64
}
type ObjectMetadata struct {
	ObjectKey, ETag, Checksum, ContentType string
	Size                                   int64
}

type UploadInput struct {
	ContentType string
	Dir         string
	Filename    string
	Reader      io.Reader
	Size        int64
}

type UploadResult struct {
	AssetID     int64  `json:"assetId,omitempty"`
	AssetKey    string `json:"assetKey,omitempty"`
	ContentType string `json:"contentType"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	ObjectKey   string `json:"objectKey,omitempty"`
	ObjectURL   string `json:"objectUrl,omitempty"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
}

type OSSConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	Bucket          string
	Endpoint        string
	PublicURL       string
	Region          string
	Prefix          string
}

type OSSUploader struct {
	bucket    string
	client    *oss.Client
	endpoint  string
	publicURL string
	prefix    string
}

type LocalUploader struct {
	publicPrefix string
	root         string
}

func NewLocalUploader(root string, publicPrefix string) *LocalUploader {
	if publicPrefix == "" {
		publicPrefix = "/api/uploads"
	}
	return &LocalUploader{
		publicPrefix: "/" + strings.Trim(publicPrefix, "/"),
		root:         root,
	}
}

func NewOSSUploader(config OSSConfig) (*OSSUploader, error) {
	missing := []string{}
	if config.AccessKeyID == "" {
		missing = append(missing, "OSS_ACCESS_KEY_ID")
	}
	if config.AccessKeySecret == "" {
		missing = append(missing, "OSS_ACCESS_KEY_SECRET")
	}
	if config.Bucket == "" {
		missing = append(missing, "OSS_BUCKET")
	}
	if config.Region == "" {
		missing = append(missing, "OSS_REGION")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("OSS 配置不完整，缺少：%s；请补齐 OSS 配置，或清空所有 OSS_* 使用服务器本地上传", strings.Join(missing, ", "))
	}

	cfg := oss.LoadDefaultConfig().
		WithRegion(config.Region).
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.AccessKeySecret))
	if config.Endpoint != "" {
		cfg = cfg.WithEndpoint(config.Endpoint)
	}

	return &OSSUploader{
		bucket:    config.Bucket,
		client:    oss.NewClient(cfg),
		endpoint:  strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(config.Endpoint, "https://"), "http://"), "/"),
		publicURL: strings.TrimRight(config.PublicURL, "/"),
		prefix:    cleanDir(config.Prefix),
	}, nil
}

func (u *OSSUploader) Upload(ctx context.Context, input UploadInput) (UploadResult, error) {
	key := u.objectKey(input.Dir, input.Filename)
	contentType := input.ContentType
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(input.Filename))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err := u.client.PutObject(ctx, &oss.PutObjectRequest{
		Bucket:        oss.Ptr(u.bucket),
		Key:           oss.Ptr(key),
		Body:          input.Reader,
		ContentLength: oss.Ptr(input.Size),
		ContentType:   oss.Ptr(contentType),
	})
	if err != nil {
		return UploadResult{}, err
	}

	return UploadResult{
		ContentType: contentType,
		Key:         key,
		Name:        safeFilename(input.Filename),
		Size:        input.Size,
		URL:         u.publicObjectURL(key),
	}, nil
}

func (u *OSSUploader) PresignGetURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	objectKey = strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if objectKey == "" {
		return "", fmt.Errorf("object key is required")
	}
	if expires <= 0 {
		expires = 30 * time.Minute
	}
	result, err := u.client.Presign(ctx, &oss.GetObjectRequest{
		Bucket: oss.Ptr(u.bucket),
		Key:    oss.Ptr(objectKey),
	}, oss.PresignExpires(expires))
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

func (u *OSSUploader) objectKey(dir string, filename string) string {
	parts := []string{}
	if u.prefix != "" {
		parts = append(parts, u.prefix)
	}
	if cleanedDir := cleanDir(dir); cleanedDir != "" {
		parts = append(parts, cleanedDir)
	}
	parts = append(parts, time.Now().UTC().Format("20060102"), uniqueFilename(filename))
	return strings.Join(parts, "/")
}

func (u *OSSUploader) publicObjectURL(key string) string {
	if u.publicURL != "" {
		return u.publicURL + "/" + strings.TrimLeft(key, "/")
	}
	if u.endpoint != "" {
		return fmt.Sprintf("https://%s.%s/%s", u.bucket, u.endpoint, key)
	}
	return "/" + strings.TrimLeft(key, "/")
}

func (u *OSSUploader) InitiateMultipart(ctx context.Context, input InitiateMultipartInput) (InitiateMultipartResult, error) {
	result, err := u.client.InitiateMultipartUpload(ctx, &oss.InitiateMultipartUploadRequest{Bucket: oss.Ptr(u.bucket), Key: oss.Ptr(strings.TrimLeft(input.ObjectKey, "/")), ContentType: oss.Ptr(input.ContentType), ForbidOverwrite: oss.Ptr("true"), Metadata: map[string]string{"checksum": input.Checksum}})
	if err != nil {
		return InitiateMultipartResult{}, err
	}
	return InitiateMultipartResult{UploadID: ptrString(result.UploadId)}, nil
}

func (u *OSSUploader) SignMultipartPart(ctx context.Context, input SignPartInput) (SignPartResult, error) {
	if input.PartNumber < 1 || input.PartNumber > 10000 {
		return SignPartResult{}, fmt.Errorf("invalid part number")
	}
	expires := input.Expires
	if expires <= 0 {
		expires = 15 * time.Minute
	}
	result, err := u.client.Presign(ctx, &oss.UploadPartRequest{Bucket: oss.Ptr(u.bucket), Key: oss.Ptr(strings.TrimLeft(input.ObjectKey, "/")), UploadId: oss.Ptr(input.UploadID), PartNumber: int32(input.PartNumber)}, oss.PresignExpires(expires))
	if err != nil {
		return SignPartResult{}, err
	}
	return SignPartResult{URL: result.URL, PartNumber: input.PartNumber, ExpiresAt: time.Now().Add(expires)}, nil
}

func (u *OSSUploader) CompleteMultipart(ctx context.Context, input CompleteMultipartInput) (CompleteMultipartResult, error) {
	parts := make([]oss.UploadPart, 0, len(input.Parts))
	for _, part := range input.Parts {
		parts = append(parts, oss.UploadPart{PartNumber: int32(part.PartNumber), ETag: oss.Ptr(part.ETag)})
	}
	result, err := u.client.CompleteMultipartUpload(ctx, &oss.CompleteMultipartUploadRequest{Bucket: oss.Ptr(u.bucket), Key: oss.Ptr(strings.TrimLeft(input.ObjectKey, "/")), UploadId: oss.Ptr(input.UploadID), ForbidOverwrite: oss.Ptr("true"), CompleteMultipartUpload: &oss.CompleteMultipartUpload{Parts: parts}})
	if err != nil {
		return CompleteMultipartResult{}, err
	}
	return CompleteMultipartResult{ETag: ptrString(result.ETag), Checksum: crc64Value(result.HashCRC64)}, nil
}

func (u *OSSUploader) AbortMultipart(ctx context.Context, input AbortMultipartInput) error {
	_, err := u.client.AbortMultipartUpload(ctx, &oss.AbortMultipartUploadRequest{Bucket: oss.Ptr(u.bucket), Key: oss.Ptr(strings.TrimLeft(input.ObjectKey, "/")), UploadId: oss.Ptr(input.UploadID)})
	return err
}

func (u *OSSUploader) ListMultipartParts(ctx context.Context, input ListPartsInput) ([]MultipartPart, error) {
	maxParts := input.MaxParts
	if maxParts <= 0 || maxParts > 1000 {
		maxParts = 1000
	}
	marker := int32(0)
	parts := []MultipartPart{}
	for {
		result, err := u.client.ListParts(ctx, &oss.ListPartsRequest{Bucket: oss.Ptr(u.bucket), Key: oss.Ptr(strings.TrimLeft(input.ObjectKey, "/")), UploadId: oss.Ptr(input.UploadID), MaxParts: int32(maxParts), PartNumberMarker: marker})
		if err != nil {
			return nil, err
		}
		for _, part := range result.Parts {
			parts = append(parts, MultipartPart{PartNumber: int(part.PartNumber), ETag: ptrString(part.ETag), Size: part.Size})
		}
		if !result.IsTruncated {
			return parts, nil
		}
		marker = result.NextPartNumberMarker
	}
}

func (u *OSSUploader) HeadObject(ctx context.Context, objectKey string) (ObjectMetadata, error) {
	key := strings.TrimLeft(objectKey, "/")
	result, err := u.client.HeadObject(ctx, &oss.HeadObjectRequest{Bucket: oss.Ptr(u.bucket), Key: oss.Ptr(key)})
	if err != nil {
		return ObjectMetadata{}, err
	}
	checksum := ptrString(result.ContentMD5)
	if value := crc64Value(result.HashCRC64); value != "" {
		checksum = value
	}
	if value := result.Metadata["checksum"]; value != "" && checksum == "" {
		checksum = value
	}
	return ObjectMetadata{ObjectKey: key, ETag: ptrString(result.ETag), Checksum: checksum, ContentType: ptrString(result.ContentType), Size: result.ContentLength}, nil
}

func (u *OSSUploader) DeleteObject(ctx context.Context, objectKey string) error {
	_, err := u.client.DeleteObject(ctx, &oss.DeleteObjectRequest{Bucket: oss.Ptr(u.bucket), Key: oss.Ptr(strings.TrimLeft(objectKey, "/"))})
	return err
}

func crc64Value(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return ""
	}
	return "crc64:" + strings.TrimSpace(*value)
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.Trim(*value, `"`)
}

func (u *LocalUploader) Upload(ctx context.Context, input UploadInput) (UploadResult, error) {
	key := strings.TrimLeft(strings.Join([]string{cleanDir(input.Dir), uniqueFilename(input.Filename)}, "/"), "/")
	if key == "" {
		key = uniqueFilename(input.Filename)
	}
	target := filepath.Join(u.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return UploadResult{}, err
	}

	file, err := os.Create(target)
	if err != nil {
		return UploadResult{}, err
	}
	defer file.Close()

	if _, err := io.Copy(file, input.Reader); err != nil {
		return UploadResult{}, err
	}

	contentType := input.ContentType
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(input.Filename))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return UploadResult{
		ContentType: contentType,
		Key:         key,
		Name:        safeFilename(input.Filename),
		Size:        input.Size,
		URL:         u.publicPrefix + "/" + strings.TrimLeft(key, "/"),
	}, ctx.Err()
}

func cleanDir(dir string) string {
	dir = strings.TrimSpace(strings.ReplaceAll(dir, "\\", "/"))
	segments := strings.Split(dir, "/")
	cleaned := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			continue
		}
		cleaned = append(cleaned, safePathSegment(segment))
	}
	return strings.Join(cleaned, "/")
}

func safeFilename(filename string) string {
	name := filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "." || name == "/" || name == "" {
		return "file"
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	ext := strings.ToLower(filepath.Ext(name))
	stem = safePathSegment(stem)
	if stem == "" {
		stem = "file"
	}
	return stem + ext
}

func uniqueFilename(filename string) string {
	name := safeFilename(filename)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s-%s%s", stem, randomHex(8), ext)
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	return strings.Trim(builder.String(), ".-_")
}

func randomHex(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
