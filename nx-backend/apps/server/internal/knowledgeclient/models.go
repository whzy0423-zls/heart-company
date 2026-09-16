package knowledgeclient

import "encoding/json"

type Scope struct {
	Public              bool    `json:"public"`
	TheoryReleaseIDs    []int64 `json:"theoryReleaseIds,omitempty"`
	EnneagramReleaseIDs []int64 `json:"enneagramReleaseIds,omitempty"`
	SkillReleaseID      *int64  `json:"skillReleaseId"`
}

type Profile struct {
	MainType *int `json:"mainType,omitempty"`
	WingType *int `json:"wingType,omitempty"`
}

type RetrievalOptions struct {
	TopK            int `json:"topK,omitempty"`
	VectorK         int `json:"vectorK,omitempty"`
	LexicalK        int `json:"lexicalK,omitempty"`
	RerankK         int `json:"rerankK,omitempty"`
	MaxContextRunes int `json:"maxContextRunes,omitempty"`
}

type RetrievalRequest struct {
	RequestID string           `json:"requestId"`
	Query     string           `json:"query"`
	Scene     string           `json:"scene"`
	Scope     Scope            `json:"scope"`
	Profile   Profile          `json:"profile"`
	Retrieval RetrievalOptions `json:"retrieval"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
}

type Document struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Library   string         `json:"library"`
	ReleaseID *int64         `json:"releaseId"`
	Score     float64        `json:"score"`
	Source    string         `json:"source"`
	Locator   map[string]any `json:"locator"`
}

type RetrievalTrace struct {
	RetrievalMethod string `json:"retrievalMethod"`
	CandidateCount  int    `json:"candidateCount"`
	ReturnedCount   int    `json:"returnedCount"`
}

type RetrievalResponse struct {
	RequestID string         `json:"requestId"`
	Documents []Document     `json:"documents"`
	Trace     RetrievalTrace `json:"trace"`
}

type AnswerRequest struct {
	RequestID string           `json:"requestId"`
	Query     string           `json:"query,omitempty"`
	Scene     string           `json:"scene,omitempty"`
	Scope     Scope            `json:"scope"`
	Profile   Profile          `json:"profile"`
	Retrieval RetrievalOptions `json:"retrieval"`
	Messages  []map[string]any `json:"messages,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
}

type Citation struct {
	DocumentID string         `json:"documentId"`
	Source     string         `json:"source"`
	Locator    map[string]any `json:"locator"`
}

type AnswerResponse struct {
	RequestID string     `json:"requestId"`
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
	TraceID   string     `json:"traceId"`
}

type StreamEvent struct {
	Type string
	Data json.RawMessage
}
