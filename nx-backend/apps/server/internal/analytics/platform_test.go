package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPlatformOverviewRejectsInvalidDays(t *testing.T) {
	_, err := (&Store{}).PlatformOverview(context.Background(), 8, time.Now())
	if !errors.Is(err, ErrInvalidDays) {
		t.Fatalf("expected ErrInvalidDays, got %v", err)
	}
}

func TestPlatformOverviewJSONContractUsesLowerCamelCase(t *testing.T) {
	raw, err := json.Marshal(PlatformOverview{Website: WebsiteOverview{TotalUsers: 1}, Series: []PlatformSeriesPoint{{WebsiteActiveUsers: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, key := range []string{`"totalUsers":1`, `"websiteActiveUsers":2`, `"recentActivities":null`} {
		if !strings.Contains(text, key) {
			t.Fatalf("missing %s in %s", key, text)
		}
	}
	if strings.Contains(text, `"TotalUsers"`) {
		t.Fatalf("unexpected Go field name in %s", text)
	}
}
