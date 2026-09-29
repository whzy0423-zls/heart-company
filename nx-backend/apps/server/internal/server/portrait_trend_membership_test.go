package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/quiz"
	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestPortraitRedactionAlsoRemovesCareContent(t *testing.T) {
	level := 7
	portrait := portraitResp{MainType: 4, UpdatedAt: "2026/09/30 10:00:00", NextUpdateAt: "2026/10/07 10:00:00", CareLevel: &level, CareLabel: "care label", CareSummary: "private care", CareTrend: "up", CareDataStatus: "ready", CareEvaluatedAt: "2026/09/30 09:00:00"}
	redactPortraitContent(&portrait, membershipResourceMetadata{State: resourceAccessReadOnlyOverLimit, RequiredPlanLevel: "vip", UpgradeRequired: true})
	if portrait.CareLevel != nil || portrait.CareLabel != "" || portrait.CareSummary != "" || portrait.CareTrend != "" || portrait.CareDataStatus != "" || portrait.CareEvaluatedAt != "" {
		t.Fatalf("care leaked through locked preview: %+v", portrait)
	}
	if portrait.MainType != 4 || portrait.UpdatedAt == "" || portrait.NextUpdateAt == "" {
		t.Fatalf("preview identity/timing missing: %+v", portrait)
	}
}

func TestPortraitTrendMembershipPostgresRequiresVIPAndPreservesStricterCardGate(t *testing.T) {
	database, _ := testdb.OpenEnvIsolatedSchema(t, "portrait_trend_gate")
	initializePortraitTrendMembershipSchema(t, database)
	server := &Server{db: database, quiz: quiz.NewStore(database), appUsers: appuser.NewStore(database)}
	for _, tc := range []struct {
		name, level, cardType, required string
		expired, locked                 bool
	}{
		{"free primary", "free", "primary", "vip", false, true},
		{"vip primary", "vip", "primary", "vip", false, false},
		{"svip primary", "svip", "primary", "vip", false, false},
		{"expired vip", "vip_month", "primary", "vip", true, true},
		{"expired svip", "svip_year", "primary", "vip", true, true},
		{"vip overflow secondary", "vip", "secondary", "svip", false, true},
		{"svip restored secondary", "svip", "secondary", "svip", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userID, cardID := seedPortraitTrendMembershipUser(t, database, tc.level, tc.cardType, tc.required, tc.expired)
			portrait := httptest.NewRecorder()
			server.appCardPortrait(portrait, httptest.NewRequest(http.MethodGet, "/portrait", nil), userID, fmt.Sprint(cardID))
			if portrait.Code != http.StatusOK {
				t.Fatalf("portrait status=%d body=%s", portrait.Code, portrait.Body.String())
			}
			var p struct {
				Data portraitResp `json:"data"`
			}
			if err := json.Unmarshal(portrait.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if p.Data.UpgradeRequired != tc.locked || p.Data.RequiredPlanLevel != tc.required {
				t.Fatalf("portrait access=%+v", p.Data.membershipResourceMetadata)
			}
			if p.Data.MainType != 4 || p.Data.UpdatedAt == "" || p.Data.NextUpdateAt == "" {
				t.Fatalf("portrait preview lost identity/time: %+v", p.Data)
			}
			if tc.locked {
				if p.Data.HasEnoughData || p.Data.Summary != "" || p.Data.StateLabel != "" || len(p.Data.Strengths) != 0 || len(p.Data.GrowthAdvice) != 0 || p.Data.CareLevel != nil || p.Data.CareLabel != "" || p.Data.CareSummary != "" || p.Data.CareTrend != "" || p.Data.CareDataStatus != "" || p.Data.CareEvaluatedAt != "" {
					t.Fatalf("locked portrait leaked detail: %+v", p.Data)
				}
			} else if !p.Data.HasEnoughData || p.Data.Summary == "" || len(p.Data.Strengths) == 0 || p.Data.CareLevel == nil || p.Data.CareSummary == "" {
				t.Fatalf("paid portrait lost detail: %+v", p.Data)
			}
			trend := httptest.NewRecorder()
			server.appCardTrend(trend, httptest.NewRequest(http.MethodGet, "/trend?days=7", nil), userID, fmt.Sprint(cardID))
			if trend.Code != http.StatusOK {
				t.Fatalf("trend status=%d body=%s", trend.Code, trend.Body.String())
			}
			var tr struct {
				Data []appTrendSeries `json:"data"`
			}
			if err := json.Unmarshal(trend.Body.Bytes(), &tr); err != nil {
				t.Fatal(err)
			}
			if len(tr.Data) != 4 {
				t.Fatalf("trend dimensions=%d", len(tr.Data))
			}
			for _, series := range tr.Data {
				if series.Label == "" || series.Dimension == "" || series.UpgradeRequired != tc.locked || series.RequiredPlanLevel != tc.required {
					t.Fatalf("trend access=%+v", series)
				}
				want := 7
				if tc.locked {
					want = 0
				}
				if len(series.Points) != want || series.Points == nil {
					t.Fatalf("trend points=%+v want length=%d", series.Points, want)
				}
			}
			// The new paid detail floor does not change the free primary-card gate.
			if tc.cardType == "primary" {
				access, err := server.cardMembershipResourceMetadata(httptest.NewRequest(http.MethodGet, "/", nil).Context(), userID, cardID, "basic card")
				if err != nil || access.UpgradeRequired || access.RequiredPlanLevel != "free" {
					t.Fatalf("basic primary access changed: %+v %v", access, err)
				}
			}
		})
	}
}

func TestPortraitTrendMembershipPostgresPlanFailureClosedAndOwnershipChecked(t *testing.T) {
	database, _ := testdb.OpenEnvIsolatedSchema(t, "portrait_trend_errors")
	initializePortraitTrendMembershipSchema(t, database)
	server := &Server{db: database, quiz: quiz.NewStore(database), appUsers: appuser.NewStore(database)}
	userID, cardID := seedPortraitTrendMembershipUser(t, database, "vip", "primary", "vip", false)
	otherID, _ := seedPortraitTrendMembershipUser(t, database, "svip", "primary", "vip", false)
	portrait := httptest.NewRecorder()
	server.appCardPortrait(portrait, httptest.NewRequest(http.MethodGet, "/portrait", nil), otherID, fmt.Sprint(cardID))
	if portrait.Code != http.StatusNotFound {
		t.Fatalf("portrait cross account=%d", portrait.Code)
	}
	trend := httptest.NewRecorder()
	server.appCardTrend(trend, httptest.NewRequest(http.MethodGet, "/trend", nil), otherID, fmt.Sprint(cardID))
	if trend.Code != http.StatusForbidden {
		t.Fatalf("trend cross account=%d", trend.Code)
	}
	if _, err := database.Exec(`ALTER TABLE app_users RENAME COLUMN member_expires_at TO unavailable_member_expires_at`); err != nil {
		t.Fatal(err)
	}
	portrait = httptest.NewRecorder()
	server.appCardPortrait(portrait, httptest.NewRequest(http.MethodGet, "/portrait", nil), userID, fmt.Sprint(cardID))
	if portrait.Code != http.StatusInternalServerError {
		t.Fatalf("portrait unknown membership status=%d body=%s", portrait.Code, portrait.Body.String())
	}
	trend = httptest.NewRecorder()
	server.appCardTrend(trend, httptest.NewRequest(http.MethodGet, "/trend", nil), userID, fmt.Sprint(cardID))
	if trend.Code != http.StatusInternalServerError {
		t.Fatalf("trend unknown membership status=%d body=%s", trend.Code, trend.Body.String())
	}
}

func initializePortraitTrendMembershipSchema(t *testing.T, database *sql.DB) {
	t.Helper()
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(string(schema)); err != nil {
		t.Fatalf("initialize full schema: %v", err)
	}
}

func seedPortraitTrendMembershipUser(t *testing.T, database *sql.DB, level, cardType, required string, expired bool) (int64, int64) {
	t.Helper()
	var expiry any
	if expired {
		expiry = time.Now().Add(-time.Hour)
	} else if level == "vip_month" || level == "svip_year" {
		expiry = time.Now().Add(time.Hour)
	}
	var userID, cardID int64
	if err := database.QueryRow(`INSERT INTO app_users(phone,member_level,member_expires_at,care_level,care_label,care_summary,care_trend,care_data_status,care_evaluated_at) VALUES($1,$2,$3,7,'care label','private care summary','up','ready',now()) RETURNING id`, fmt.Sprintf("portrait-gate-%d", time.Now().UnixNano()), level, expiry).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`INSERT INTO app_user_cards(app_user_id,card_type,name,relation,enneagram) VALUES($1,$2,'fixture','self',4) RETURNING id`, userID, cardType).Scan(&cardID); err != nil {
		t.Fatal(err)
	}
	if cardType == "secondary" {
		if _, err := database.Exec(`INSERT INTO app_membership_resource_access(app_user_id,resource_type,resource_id,state,required_plan_level,priority_rank,reason) VALUES($1,'cards',$2,'read_only_over_limit',$3,4,'超出当前人物卡额度')`, userID, cardID, required); err != nil {
			t.Fatal(err)
		}
	}
	return userID, cardID
}
