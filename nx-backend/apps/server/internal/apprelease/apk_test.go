package apprelease

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(info.CertificateSHA256) {
		t.Fatalf("CertificateSHA256 = %q, want 64 lowercase hexadecimal characters", info.CertificateSHA256)
	}
	if info.CertificateSHA256 != fixtureCertificateSHA256 {
		t.Fatalf("CertificateSHA256 = %q, want %q", info.CertificateSHA256, fixtureCertificateSHA256)
	}
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
