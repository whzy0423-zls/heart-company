package signup

import "testing"

func TestNormalizePhone(t *testing.T) {
	got, ok := normalizePhone(" 138-0000 0000 ")
	if !ok {
		t.Fatal("expected phone to be valid")
	}
	if got != "13800000000" {
		t.Fatalf("expected normalized phone, got %q", got)
	}
}

func TestNormalizePhoneRejectsInvalidPhone(t *testing.T) {
	for _, input := range []string{"", "123456", "23800000000", "1380000000a"} {
		if got, ok := normalizePhone(input); ok {
			t.Fatalf("expected %q to be invalid, got %q", input, got)
		}
	}
}

func TestNormalizeContactAllowsWechatWithoutPhoneValidation(t *testing.T) {
	contactType, contact, err := normalizeContact("wechat", "  wx_11111  ")
	if err != nil {
		t.Fatalf("expected wechat to be valid, got %v", err)
	}
	if contactType != ContactTypeWechat {
		t.Fatalf("expected contact type %q, got %q", ContactTypeWechat, contactType)
	}
	if contact != "wx_11111" {
		t.Fatalf("expected trimmed wechat id, got %q", contact)
	}
}

func TestNormalizeContactValidatesPhoneOnlyForPhoneType(t *testing.T) {
	_, _, err := normalizeContact("phone", "11111")
	if err == nil || err.Error() != "请输入正确的手机号" {
		t.Fatalf("expected phone validation error, got %v", err)
	}
}

func TestNormalizeAttributionKeepsUsefulSourceFields(t *testing.T) {
	input := AttributionInput{
		VisitorID:   " visitor-1 ",
		SourcePath:  "/game?from=share#signup",
		LandingPage: "https://example.com/?utm_source=douyin",
		Referrer:    "https://referrer.example.com/page",
		UTMSource:   " douyin ",
		UTMMedium:   " video ",
		UTMCampaign: " summer ",
		UTMContent:  " card ",
		UTMTerm:     " enneagram ",
	}

	got := normalizeAttribution(input)

	if got.VisitorID != "visitor-1" {
		t.Fatalf("expected visitor id to be trimmed, got %q", got.VisitorID)
	}
	if got.SourcePath != "/game?from=share#signup" || got.UTMSource != "douyin" {
		t.Fatalf("expected source fields to be preserved, got %+v", got)
	}
}

func TestNormalizeAttributionDefaultsSourcePath(t *testing.T) {
	got := normalizeAttribution(AttributionInput{VisitorID: "v1"})
	if got.SourcePath != "/" {
		t.Fatalf("expected empty source path to default to '/', got %q", got.SourcePath)
	}
}
