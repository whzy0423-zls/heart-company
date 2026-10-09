package growthinsight

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const DisclosureVersion = "growth-analysis-2026-10-09"

var (
	ErrNotFound      = errors.New("growth insight not found")
	ErrDisabled      = errors.New("growth analysis disabled")
	ErrInvalid       = errors.New("invalid growth insight input")
	ErrStale         = errors.New("growth insight source changed")
	ErrNoNewEvidence = errors.New("no new growth insight evidence")
)

type CompleteFunc func(context.Context, string) (string, error)

type Claim struct {
	Text         string   `json:"text"`
	EvidenceRefs []string `json:"evidenceRefs"`
}

type SuggestedAction struct {
	Title        string   `json:"title"`
	Detail       string   `json:"detail"`
	EvidenceRefs []string `json:"evidenceRefs"`
}

type Analysis struct {
	Summary          Claim             `json:"summary"`
	WeeklyReview     Claim             `json:"weeklyReview"`
	TrendExplanation Claim             `json:"trendExplanation"`
	Observations     []Claim           `json:"observations"`
	Patterns         []Claim           `json:"patterns"`
	Goals            []Claim           `json:"goals"`
	Changes          []Claim           `json:"changes"`
	Uncertainties    []Claim           `json:"uncertainties"`
	Recommendations  []Claim           `json:"recommendations"`
	Actions          []SuggestedAction `json:"actions"`
	Strengths        []Claim           `json:"strengths"`
	StressPoints     []Claim           `json:"stressPoints"`
	AwarenessPrompts []Claim           `json:"awarenessPrompts"`
}

type Evidence struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Text       string    `json:"text"`
	OccurredAt time.Time `json:"occurredAt"`
}

type Coverage struct {
	EligibleCount  int  `json:"eligibleCount"`
	IncludedCount  int  `json:"includedCount"`
	OmittedCount   int  `json:"omittedCount"`
	TruncatedCount int  `json:"truncatedCount"`
	WindowDays     int  `json:"windowDays"`
	Truncated      bool `json:"truncated"`
}

type Report struct {
	ID            int64      `json:"id"`
	AppUserID     int64      `json:"appUserId"`
	CardID        int64      `json:"cardId"`
	Version       int        `json:"version"`
	GeneratedAt   time.Time  `json:"generatedAt"`
	SourceFrom    time.Time  `json:"sourceFrom"`
	SourceThrough time.Time  `json:"sourceThrough"`
	PeriodStart   time.Time  `json:"periodStart"`
	PeriodEnd     time.Time  `json:"periodEnd"`
	Timezone      string     `json:"timezone"`
	EvidenceCount int        `json:"evidenceCount"`
	Coverage      Coverage   `json:"coverage"`
	Published     bool       `json:"published"`
	Analysis      Analysis   `json:"analysis"`
	Evidence      []Evidence `json:"evidence"`
}

type Feedback struct {
	Status  string `json:"status"`
	Outcome string `json:"outcome"`
	Note    string `json:"note"`
}

type Action struct {
	ID        int64  `json:"id"`
	ReportID  int64  `json:"reportId"`
	CardID    int64  `json:"cardId"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Status    string `json:"status"`
	Outcome   string `json:"outcome"`
	Note      string `json:"note"`
	UpdatedAt string `json:"updatedAt"`
}

type UserView struct {
	Enabled          bool     `json:"enabled"`
	Status           string   `json:"status"`
	CardID           int64    `json:"cardId"`
	ReportID         int64    `json:"reportId"`
	Version          int      `json:"version"`
	GeneratedAt      string   `json:"generatedAt"`
	NextUpdateAt     string   `json:"nextUpdateAt"`
	SourceThrough    string   `json:"sourceThrough"`
	PeriodStart      string   `json:"periodStart"`
	PeriodEnd        string   `json:"periodEnd"`
	Timezone         string   `json:"timezone"`
	Summary          string   `json:"summary"`
	WeeklyReview     string   `json:"weeklyReview"`
	TrendExplanation string   `json:"trendExplanation"`
	Strengths        []string `json:"strengths"`
	StressPoints     []string `json:"stressPoints"`
	GrowthAdvice     []string `json:"growthAdvice"`
	AwarenessPrompts []string `json:"awarenessPrompts"`
	Actions          []Action `json:"actions"`
}

type ConsentState struct {
	Enabled           bool   `json:"enabled"`
	ReportID          int64  `json:"reportId"`
	DisclosureVersion string `json:"disclosureVersion"`
}

type ReportSummary struct {
	ID            int64     `json:"id"`
	Version       int       `json:"version"`
	GeneratedAt   time.Time `json:"generatedAt"`
	SourceThrough time.Time `json:"sourceThrough"`
	EvidenceCount int       `json:"evidenceCount"`
}

type AdminList struct {
	Enabled bool            `json:"enabled"`
	Status  string          `json:"status"`
	Reports []ReportSummary `json:"reports"`
}

func validateFeedback(f Feedback) error {
	if f.Status != "attempted" && f.Status != "skipped" {
		return ErrInvalid
	}
	if f.Outcome != "unknown" && f.Outcome != "helpful" && f.Outcome != "not_helpful" && f.Outcome != "worse" {
		return ErrInvalid
	}
	if f.Status == "skipped" && f.Outcome != "unknown" {
		return ErrInvalid
	}
	if utf8.RuneCountInString(f.Note) > 500 {
		return ErrInvalid
	}
	return nil
}

func validateAnalysis(a Analysis, evidence []Evidence, weekStart time.Time) error {
	refs := make(map[string]time.Time, len(evidence))
	for _, e := range evidence {
		refs[e.ID] = e.OccurredAt
	}
	check := func(c Claim, optional, weekly bool) error {
		if optional && c.Text == "" && len(c.EvidenceRefs) == 0 {
			return nil
		}
		if strings.TrimSpace(c.Text) == "" || utf8.RuneCountInString(c.Text) > 2000 || len(c.EvidenceRefs) == 0 || len(c.EvidenceRefs) > 20 {
			return ErrInvalid
		}
		for _, id := range c.EvidenceRefs {
			at, ok := refs[id]
			if !ok || (weekly && at.Before(weekStart)) {
				return ErrInvalid
			}
		}
		return nil
	}
	if check(a.Summary, false, false) != nil || check(a.WeeklyReview, true, true) != nil || check(a.TrendExplanation, true, false) != nil {
		return ErrInvalid
	}
	for _, section := range [][]Claim{a.Observations, a.Patterns, a.Goals, a.Changes, a.Uncertainties, a.Recommendations, a.Strengths, a.StressPoints, a.AwarenessPrompts} {
		if len(section) > 10 {
			return ErrInvalid
		}
		for _, c := range section {
			if check(c, false, false) != nil {
				return ErrInvalid
			}
		}
	}
	if len(a.Actions) > 3 {
		return ErrInvalid
	}
	for _, a := range a.Actions {
		if strings.TrimSpace(a.Title) == "" || utf8.RuneCountInString(a.Title) > 80 || utf8.RuneCountInString(a.Detail) > 500 || check(Claim{Text: a.Detail, EvidenceRefs: a.EvidenceRefs}, false, false) != nil {
			return ErrInvalid
		}
	}
	return nil
}

func emptyView(cardID int64) UserView {
	return UserView{Status: "disabled", CardID: cardID, Timezone: "Asia/Shanghai", Actions: []Action{}, Strengths: []string{}, StressPoints: []string{}, GrowthAdvice: []string{}, AwarenessPrompts: []string{}}
}

func project(r Report, actions []Action) UserView {
	v := emptyView(r.CardID)
	v.Enabled = true
	v.Status = "ready"
	v.ReportID = r.ID
	v.Version = r.Version
	v.GeneratedAt = stamp(r.GeneratedAt)
	v.NextUpdateAt = stamp(r.GeneratedAt.Add(7 * 24 * time.Hour))
	v.SourceThrough = stamp(r.SourceThrough)
	v.PeriodStart = stamp(r.PeriodStart)
	v.PeriodEnd = stamp(r.PeriodEnd)
	v.Summary = r.Analysis.Summary.Text
	v.WeeklyReview = r.Analysis.WeeklyReview.Text
	v.TrendExplanation = r.Analysis.TrendExplanation.Text
	for _, c := range r.Analysis.Recommendations {
		v.GrowthAdvice = append(v.GrowthAdvice, c.Text)
	}
	for _, c := range r.Analysis.Strengths {
		v.Strengths = append(v.Strengths, c.Text)
	}
	for _, c := range r.Analysis.StressPoints {
		v.StressPoints = append(v.StressPoints, c.Text)
	}
	for _, c := range r.Analysis.AwarenessPrompts {
		v.AwarenessPrompts = append(v.AwarenessPrompts, c.Text)
	}
	if actions != nil {
		v.Actions = actions
	}
	return v
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func weeklyPeriod(now time.Time) (time.Time, time.Time) {
	local := now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	start := day.AddDate(0, 0, -(int(day.Weekday())+6)%7).UTC()
	return start, start.Add(7 * 24 * time.Hour)
}
