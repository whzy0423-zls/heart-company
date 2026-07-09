package videoproject

import "testing"

func TestValidateGatewayReferenceURLsRejectsLocalUploadProxyURLs(t *testing.T) {
	err := validateGatewayReferenceURLs(
		[]string{"https://oss.example.com/shot.jpg"},
		[]string{"/api/upload-assets/12"},
		[]string{"https://oss.example.com/audio.mp3"},
	)

	if err == nil {
		t.Fatal("expected local upload proxy reference to be rejected before submitting to video gateway")
	}
	if got := err.Error(); got != "参考视频需要阿里云 OSS 文件桶公网 http(s) 地址，请重新上传到文件桶后再生成: /api/upload-assets/12" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestValidateGatewayReferenceURLsAcceptsPublicOSSURLs(t *testing.T) {
	if err := validateGatewayReferenceURLs(
		[]string{"https://oss.example.com/shot.jpg"},
		[]string{"https://oss.example.com/ref.mp4"},
		[]string{"https://oss.example.com/audio.mp3"},
	); err != nil {
		t.Fatalf("expected public OSS URLs to be accepted, got %v", err)
	}
}

func TestRequirePublicAssetObjectURLRejectsMissingOrLocalProxyURL(t *testing.T) {
	for _, raw := range []string{"", "/api/upload-assets/9"} {
		if _, err := requirePublicAssetObjectURL("视频抽帧", raw); err == nil {
			t.Fatalf("expected %q to be rejected as non-public objectUrl", raw)
		}
	}
}

func TestRequirePublicAssetObjectURLAcceptsPublicOSSURL(t *testing.T) {
	got, err := requirePublicAssetObjectURL("视频抽帧", "https://oss.example.com/video/frame.jpg")
	if err != nil {
		t.Fatalf("expected public objectUrl to be accepted: %v", err)
	}
	if got != "https://oss.example.com/video/frame.jpg" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeShotAssetInputRejectsLocalProxyObjectURL(t *testing.T) {
	input := ShotAssetInput{
		AssetType: "image",
		ObjectURL: "/api/upload-assets/12",
		Name:      "本地代理图片",
	}

	err := normalizeShotAssetInput(&input)

	if err == nil {
		t.Fatal("expected local upload proxy URL to be rejected for shot reference assets")
	}
	if got := err.Error(); got != "分镜参考素材需要阿里云 OSS 文件桶公网 objectUrl，请配置 OSS_PUBLIC_URL/文件桶公网访问后重新上传" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestNormalizeShotAssetInputAcceptsPublicObjectURL(t *testing.T) {
	input := ShotAssetInput{
		AssetType: "image",
		ObjectURL: "https://oss.example.com/shot.jpg",
		Name:      "公网图片",
	}

	if err := normalizeShotAssetInput(&input); err != nil {
		t.Fatalf("expected public objectUrl to be accepted: %v", err)
	}
}
