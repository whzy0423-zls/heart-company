package caresystem

import "time"

const EvaluationVersion = "care-v1"

type DataStatus string

const (
	DataStatusReady        DataStatus = "ready"
	DataStatusInsufficient DataStatus = "insufficient"
	DataStatusFailed       DataStatus = "failed"
)

type Trend string

const (
	TrendUp     Trend = "up"
	TrendDown   Trend = "down"
	TrendStable Trend = "stable"
)

type Signals struct {
	MessageCount       int     `json:"messageCount"`
	RecentMessageCount int     `json:"recentMessageCount"`
	DistressHits       int     `json:"distressHits"`
	SupportHits        int     `json:"supportHits"`
	LossOfControlHits  int     `json:"lossOfControlHits"`
	RecoveryHits       int     `json:"recoveryHits"`
	Intensity          float64 `json:"intensity"`
}

type Evidence struct {
	Source    string
	Role      string
	Content   string
	CreatedAt time.Time
}

type Baseline struct {
	Level       *int
	EvaluatedAt *time.Time
}

type Evaluation struct {
	AppUserID         int64
	Level             *int
	Label             string
	Summary           string
	Trend             Trend
	DataStatus        DataStatus
	Signals           Signals
	WindowStart       time.Time
	WindowEnd         time.Time
	KnowledgeVersion  string
	EvaluationVersion string
	ErrorMessage      string
}
