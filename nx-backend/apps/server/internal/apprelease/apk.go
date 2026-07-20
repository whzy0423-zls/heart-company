package apprelease

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/avast/apkverifier"
	"github.com/shogo82148/androidbinary/apk"
)

type APKInfo struct {
	PackageName       string
	VersionName       string
	VersionCode       int64
	CertificateSHA256 string
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

	return APKInfo{
		PackageName: packageName,
		VersionName: versionName,
		VersionCode: int64(versionCode),
	}, nil
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
