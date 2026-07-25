package apprelease

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io"
	"path"
	"strings"

	"github.com/avast/apkverifier"
	"github.com/shogo82148/androidbinary"
	"github.com/shogo82148/androidbinary/apk"

	_ "image/jpeg"
)

const (
	maxAPKIconBytes     = 8 << 20
	maxAPKIconDimension = 2048
	maxAPKIconPixels    = 4_000_000
)

type APKInfo struct {
	PackageName       string
	VersionName       string
	VersionCode       int64
	CertificateSHA256 string
	AppName           string
	IconPNG           []byte
}

type APKInspector struct{}

func NewAPKInspector() *APKInspector {
	return &APKInspector{}
}

func (i *APKInspector) Inspect(path string) (APKInfo, error) {
	info, err := inspectAPKManifest(path)
	if err != nil {
		return APKInfo{}, err
	}

	result, err := apkverifier.Verify(path, nil)
	if err != nil {
		if len(result.SignerCerts) == 0 {
			return APKInfo{}, fmt.Errorf("%w: no verified signer certificate", ErrUnsignedAPK)
		}
		return APKInfo{}, fmt.Errorf("%w: signature verification failed", ErrInvalidAPK)
	}
	_, certificate := apkverifier.PickBestApkCert(result.SignerCerts)
	if certificate == nil {
		return APKInfo{}, fmt.Errorf("%w: no signer certificate", ErrUnsignedAPK)
	}

	digest := sha256.Sum256(certificate.Raw)
	info.CertificateSHA256 = hex.EncodeToString(digest[:])
	return info, nil
}

func inspectAPKManifest(path string) (info APKInfo, err error) {
	parsed, openErr := apk.OpenFile(path)
	if openErr != nil {
		return APKInfo{}, fmt.Errorf("%w: manifest could not be read", ErrInvalidAPK)
	}
	defer func() {
		if closeErr := parsed.Close(); closeErr != nil && err == nil {
			info = APKInfo{}
			err = fmt.Errorf("%w: manifest could not be closed", ErrInvalidAPK)
		}
	}()

	manifest := parsed.Manifest()
	packageName, err := manifest.Package.String()
	if err != nil {
		return APKInfo{}, fmt.Errorf("%w: package name could not be read", ErrInvalidAPK)
	}
	versionName, err := manifest.VersionName.String()
	if err != nil {
		return APKInfo{}, fmt.Errorf("%w: version name could not be read", ErrInvalidAPK)
	}
	versionCode, err := manifest.VersionCode.Int32()
	if err != nil {
		return APKInfo{}, fmt.Errorf("%w: version code could not be read", ErrInvalidAPK)
	}
	appName := resolveAPKAppName(parsed, packageName)

	var iconPNG []byte
	iconPath, iconErr := parsed.Manifest().App.Icon.WithResTableConfig(&androidbinary.ResTableConfig{}).String()
	if iconErr == nil {
		iconPNG, _ = extractAPKIcon(path, iconPath)
	}

	return APKInfo{
		PackageName: packageName,
		VersionName: versionName,
		VersionCode: int64(versionCode),
		AppName:     appName,
		IconPNG:     iconPNG,
	}, nil
}

type apkLabelResolver interface {
	Label(*androidbinary.ResTableConfig) (string, error)
}

func resolveAPKAppName(parsed apkLabelResolver, packageName string) string {
	configs := []*androidbinary.ResTableConfig{
		{Language: [2]uint8{'z', 'h'}, Country: [2]uint8{'C', 'N'}},
		{},
	}
	for _, config := range configs {
		label, err := parsed.Label(config)
		if err == nil && strings.TrimSpace(label) != "" {
			return label
		}
	}
	return packageName
}

func extractAPKIcon(apkPath, iconPath string) ([]byte, error) {
	if androidbinary.IsResID(iconPath) {
		return nil, nil
	}

	cleanedPath := path.Clean(iconPath)
	if cleanedPath == "." || path.IsAbs(cleanedPath) || cleanedPath == ".." || strings.HasPrefix(cleanedPath, "../") {
		return nil, fmt.Errorf("invalid icon path")
	}
	switch strings.ToLower(path.Ext(cleanedPath)) {
	case ".png", ".jpg", ".jpeg":
	default:
		return nil, nil
	}

	archive, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, err
	}
	defer archive.Close()

	for _, file := range archive.File {
		if file.Name != cleanedPath {
			continue
		}
		if file.UncompressedSize64 > maxAPKIconBytes {
			return nil, fmt.Errorf("icon source exceeds %d bytes", maxAPKIconBytes)
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, maxAPKIconBytes+1))
		closeErr := reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(data) > maxAPKIconBytes {
			return nil, fmt.Errorf("icon source exceeds %d bytes", maxAPKIconBytes)
		}
		return normalizeAPKIcon(data)
	}
	return nil, fmt.Errorf("icon %q not found", cleanedPath)
}

func normalizeAPKIcon(data []byte) ([]byte, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := validateAPKIconDimensions(config.Width, config.Height); err != nil {
		return nil, err
	}

	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := decoded.Bounds()
	if err := validateAPKIconDimensions(bounds.Dx(), bounds.Dy()); err != nil {
		return nil, err
	}

	output := &limitedAPKIconBuffer{remaining: maxAPKIconBytes}
	if err := png.Encode(output, decoded); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func validateAPKIconDimensions(width, height int) error {
	if width <= 0 || height <= 0 || width > maxAPKIconDimension || height > maxAPKIconDimension {
		return fmt.Errorf("icon dimensions %dx%d exceed limits", width, height)
	}
	if int64(width)*int64(height) > maxAPKIconPixels {
		return fmt.Errorf("icon pixel count exceeds %d", maxAPKIconPixels)
	}
	return nil
}

type limitedAPKIconBuffer struct {
	bytes.Buffer
	remaining int
}

func (b *limitedAPKIconBuffer) Write(data []byte) (int, error) {
	if len(data) > b.remaining {
		return 0, fmt.Errorf("normalized icon exceeds %d bytes", maxAPKIconBytes)
	}
	n, err := b.Buffer.Write(data)
	b.remaining -= n
	return n, err
}

func ValidateUploadAPK(info APKInfo, expectedPackageName string) error {
	if strings.TrimSpace(info.VersionName) == "" || info.VersionCode <= 0 {
		return ErrInvalidVersion
	}
	if info.PackageName != expectedPackageName {
		return ErrPackageMismatch
	}
	return nil
}

func ValidatePublishAPK(info APKInfo, expectedCertificateSHA256 string) error {
	if info.CertificateSHA256 != expectedCertificateSHA256 {
		return ErrCertificateMismatch
	}
	return nil
}
