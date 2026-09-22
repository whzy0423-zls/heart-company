package server

import (
	"testing"
)

func TestParseMiniappAnalyticsDateUsesShanghaiDay(t *testing.T) {
	day, err := parseMiniappAnalyticsDate("2026-09-22")
	if err != nil {
		t.Fatal(err)
	}
	if day.Format("2006-01-02 15:04:05 -0700 MST") != "2026-09-22 00:00:00 +0800 CST" {
		t.Fatalf("unexpected analytics day: %s", day.Format("2006-01-02 15:04:05 -0700 MST"))
	}
	if _, err := parseMiniappAnalyticsDate("2026/09/22"); err == nil {
		t.Fatal("expected invalid date format to fail")
	}
}
