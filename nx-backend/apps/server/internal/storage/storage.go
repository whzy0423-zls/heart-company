package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
