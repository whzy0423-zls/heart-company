package apprelease

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/shogo82148/androidbinary"
)

const fixtureCertificateSHA256 = "9fa7d5b8a76d2cb8869cf0613c0237e02b66ea95aa775a6ff386204aab8b0162"

func TestAPKInspectorExtractsManifestAndCertificate(t *testing.T) {
	info, err := NewAPKInspector().Inspect("testdata/signed-minimal.apk")
	if err != nil {
		t.Fatal(err)
	}

	if info.PackageName != "com.xinzhili.nine_xing_app" {
		t.Fatalf("PackageName = %q, want com.xinzhili.nine_xing_app", info.PackageName)
	}
	if info.VersionName != "1.2.3" {
		t.Fatalf("VersionName = %q, want 1.2.3", info.VersionName)
	}
	if info.VersionCode != 123 {
		t.Fatalf("VersionCode = %d, want 123", info.VersionCode)
	}
	if info.AppName != "九型芯之力测试包" {
		t.Fatalf("AppName = %q, want 九型芯之力测试包", info.AppName)
	}
	if len(info.IconPNG) != 0 {
		t.Fatalf("IconPNG length = %d, want no icon for minimal fixture", len(info.IconPNG))
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(info.CertificateSHA256) {
		t.Fatalf("CertificateSHA256 = %q, want 64 lowercase hexadecimal characters", info.CertificateSHA256)
	}
	if info.CertificateSHA256 != fixtureCertificateSHA256 {
		t.Fatalf("CertificateSHA256 = %q, want %q", info.CertificateSHA256, fixtureCertificateSHA256)
	}
}

func TestResolveAPKAppNamePrefersChineseLabel(t *testing.T) {
	resolver := &stubAPKLabelResolver{labels: map[[4]byte]labelResult{
		{'z', 'h', 'C', 'N'}: {value: "九星"},
		{}:                   {value: "Nine Xing"},
	}}

	if got := resolveAPKAppName(resolver, "com.example.app"); got != "九星" {
		t.Fatalf("resolveAPKAppName() = %q, want 九星", got)
	}
	if resolver.calls != 1 {
		t.Fatalf("Label() calls = %d, want 1", resolver.calls)
	}
}

func TestResolveAPKAppNameFallsBackToDefaultLabel(t *testing.T) {
	resolver := &stubAPKLabelResolver{labels: map[[4]byte]labelResult{
		{'z', 'h', 'C', 'N'}: {err: errors.New("localized label unavailable")},
		{}:                   {value: "Nine Xing"},
	}}

	if got := resolveAPKAppName(resolver, "com.example.app"); got != "Nine Xing" {
		t.Fatalf("resolveAPKAppName() = %q, want Nine Xing", got)
	}
	if resolver.calls != 2 {
		t.Fatalf("Label() calls = %d, want 2", resolver.calls)
	}
}

func TestResolveAPKAppNameFallsBackToPackageName(t *testing.T) {
	resolver := &stubAPKLabelResolver{labels: map[[4]byte]labelResult{
		{'z', 'h', 'C', 'N'}: {value: "  "},
		{}:                   {err: errors.New("default label unavailable")},
	}}

	if got := resolveAPKAppName(resolver, "com.example.app"); got != "com.example.app" {
		t.Fatalf("resolveAPKAppName() = %q, want com.example.app", got)
	}
}

func TestExtractAPKIconNormalizesPNGAndJPEG(t *testing.T) {
	tests := []struct {
		name       string
		entryName  string
		encodeIcon func(io.Writer) error
	}{
		{
			name:      "png",
			entryName: "res/mipmap/icon.png",
			encodeIcon: func(w io.Writer) error {
				return png.Encode(w, solidAPKIcon(color.RGBA{R: 0x33, G: 0x66, B: 0x99, A: 0xff}))
			},
		},
		{
			name:      "jpeg",
			entryName: "res/mipmap/icon.jpg",
			encodeIcon: func(w io.Writer) error {
				return jpeg.Encode(w, solidAPKIcon(color.RGBA{R: 0x99, G: 0x66, B: 0x33, A: 0xff}), nil)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var source bytes.Buffer
			if err := test.encodeIcon(&source); err != nil {
				t.Fatal(err)
			}
			apkPath := writeSyntheticAPK(t, map[string][]byte{test.entryName: source.Bytes()})

			got, err := extractAPKIcon(apkPath, test.entryName)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := png.Decode(bytes.NewReader(got)); err != nil {
				t.Fatalf("normalized icon is not PNG: %v", err)
			}
		})
	}
}

func TestExtractAPKIconUsesCleanExactZIPPath(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, solidAPKIcon(color.White)); err != nil {
		t.Fatal(err)
	}
	apkPath := writeSyntheticAPK(t, map[string][]byte{
		"res/mipmap/icon.png":        source.Bytes(),
		"res/mipmap/icon.png.backup": []byte("not an image"),
	})

	got, err := extractAPKIcon(apkPath, "res/mipmap-anydpi/../mipmap/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("extractAPKIcon() returned no icon")
	}
}

func solidAPKIcon(fill color.Color) image.Image {
	icon := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			icon.Set(x, y, fill)
		}
	}
	return icon
}

func TestExtractAPKIconSkipsResourceIDsXMLAndAdaptiveIcons(t *testing.T) {
	apkPath := writeSyntheticAPK(t, map[string][]byte{
		"res/mipmap/icon.xml": []byte(`<adaptive-icon/>`),
	})

	for _, iconPath := range []string{"@0x7f010001", "res/mipmap/icon.xml", "res/mipmap/icon.webp"} {
		t.Run(iconPath, func(t *testing.T) {
			got, err := extractAPKIcon(apkPath, iconPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 0 {
				t.Fatalf("extractAPKIcon() returned %d bytes, want none", len(got))
			}
		})
	}
}

func TestExtractAPKIconRejectsOversizedSource(t *testing.T) {
	apkPath := writeSyntheticAPK(t, map[string][]byte{
		"res/mipmap/icon.png": make([]byte, maxAPKIconBytes+1),
	})

	if _, err := extractAPKIcon(apkPath, "res/mipmap/icon.png"); err == nil {
		t.Fatal("extractAPKIcon() error = nil, want oversized source rejection")
	}
}

func TestNormalizeAPKIconRejectsOversizedDimensionsAndPixels(t *testing.T) {
	tests := []struct {
		name          string
		width, height uint32
	}{
		{name: "width", width: maxAPKIconDimension + 1, height: 1},
		{name: "height", width: 1, height: maxAPKIconDimension + 1},
		{name: "pixels", width: 2001, height: 2000},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := normalizeAPKIcon(pngHeader(test.width, test.height)); err == nil {
				t.Fatal("normalizeAPKIcon() error = nil, want dimension rejection")
			}
		})
	}
}

type labelResult struct {
	value string
	err   error
}

type stubAPKLabelResolver struct {
	labels map[[4]byte]labelResult
	calls  int
}

func (r *stubAPKLabelResolver) Label(config *androidbinary.ResTableConfig) (string, error) {
	r.calls++
	key := [4]byte{config.Language[0], config.Language[1], config.Country[0], config.Country[1]}
	result := r.labels[key]
	return result.value, result.err
}

func writeSyntheticAPK(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.apk")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, data := range entries {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func pngHeader(width, height uint32) []byte {
	var result bytes.Buffer
	result.Write([]byte("\x89PNG\r\n\x1a\n"))
	var ihdr bytes.Buffer
	_ = binary.Write(&ihdr, binary.BigEndian, width)
	_ = binary.Write(&ihdr, binary.BigEndian, height)
	ihdr.Write([]byte{8, 6, 0, 0, 0})
	writePNGChunk(&result, "IHDR", ihdr.Bytes())
	writePNGChunk(&result, "IEND", nil)
	return result.Bytes()
}

func writePNGChunk(dst *bytes.Buffer, chunkType string, data []byte) {
	_ = binary.Write(dst, binary.BigEndian, uint32(len(data)))
	dst.WriteString(chunkType)
	dst.Write(data)
	checksum := crc32.NewIEEE()
	_, _ = checksum.Write([]byte(chunkType))
	_, _ = checksum.Write(data)
	_ = binary.Write(dst, binary.BigEndian, checksum.Sum32())
}

func TestAPKInspectorRejectsInvalidArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.apk")
	if err := os.WriteFile(path, []byte("not a zip archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewAPKInspector().Inspect(path)
	if !errors.Is(err, ErrInvalidAPK) {
		t.Fatalf("Inspect() error = %v, want ErrInvalidAPK", err)
	}
}

func TestAPKInspectorRejectsUnsignedAPK(t *testing.T) {
	_, err := NewAPKInspector().Inspect("testdata/unsigned-minimal.apk")
	if !errors.Is(err, ErrUnsignedAPK) {
		t.Fatalf("Inspect() error = %v, want ErrUnsignedAPK", err)
	}
}

func TestValidateUploadAPKRejectsWrongPackage(t *testing.T) {
	info := validAPKInfo()
	info.PackageName = "example.wrong"

	err := ValidateUploadAPK(info, "com.xinzhili.nine_xing_app")
	if !errors.Is(err, ErrPackageMismatch) {
		t.Fatalf("ValidateUploadAPK() error = %v, want ErrPackageMismatch", err)
	}
}

func TestValidateUploadAPKAllowsDraftWithoutConfiguredCertificate(t *testing.T) {
	if err := ValidateUploadAPK(validAPKInfo(), "com.xinzhili.nine_xing_app"); err != nil {
		t.Fatalf("ValidateUploadAPK() error = %v, want success", err)
	}
}

func TestValidateUploadAPKRejectsMissingVersionMetadata(t *testing.T) {
	tests := []struct {
		name string
		edit func(*APKInfo)
	}{
		{
			name: "empty version name",
			edit: func(info *APKInfo) { info.VersionName = "" },
		},
		{
			name: "blank version name",
			edit: func(info *APKInfo) { info.VersionName = " \t" },
		},
		{
			name: "zero version code",
			edit: func(info *APKInfo) { info.VersionCode = 0 },
		},
		{
			name: "negative version code",
			edit: func(info *APKInfo) { info.VersionCode = -1 },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := validAPKInfo()
			test.edit(&info)
			err := ValidateUploadAPK(info, "com.xinzhili.nine_xing_app")
			if !errors.Is(err, ErrInvalidVersion) {
				t.Fatalf("ValidateUploadAPK() error = %v, want ErrInvalidVersion", err)
			}
		})
	}
}

func TestValidatePublishAPKRejectsMismatchedCertificate(t *testing.T) {
	info := validAPKInfo()
	err := ValidatePublishAPK(info, strings.Repeat("b", 64))
	if !errors.Is(err, ErrCertificateMismatch) {
		t.Fatalf("ValidatePublishAPK() error = %v, want ErrCertificateMismatch", err)
	}
}

func TestValidatePublishAPKAcceptsMatchingNormalizedCertificate(t *testing.T) {
	info := validAPKInfo()
	if err := ValidatePublishAPK(info, info.CertificateSHA256); err != nil {
		t.Fatalf("ValidatePublishAPK() error = %v, want success", err)
	}
}

func validAPKInfo() APKInfo {
	return APKInfo{
		PackageName:       "com.xinzhili.nine_xing_app",
		VersionName:       "1.2.3",
		VersionCode:       123,
		CertificateSHA256: strings.Repeat("a", 64),
	}
}
