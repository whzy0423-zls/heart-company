// Package problemfollowup owns durable, one-time follow-ups to successful
// problem-solving turns. It never starts a conversation or grants membership.
package problemfollowup

import (
	"context"
	"errors"
	"time"
)

const Delay = 30 * time.Minute
const Lease = 2 * time.Minute
const MaxAttempts = 3

var ErrStale = errors.New("problem followup: stale task")
var ErrInvalidOutput = errors.New("problem followup: invalid model output")

type Turn struct{ AppUserID, SessionID, CardID, Revision int64 }
type turnKey struct{}

func WithTurn(ctx context.Context, turn Turn) context.Context {
	return context.WithValue(ctx, turnKey{}, turn)
}
func turnFrom(ctx context.Context) Turn { turn, _ := ctx.Value(turnKey{}).(Turn); return turn }

type Job struct {
	ID, AppUserID, SessionID, CardID, AssistantMessageID, Revision int64
	Status, ClaimToken                                             string
	DueAt                                                          time.Time
	Attempts                                                       int
}
type Delivery struct {
	AppUserID, SessionID, CardID, MessageID, NotificationID int64
	DeepLink                                                string
}
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type Input struct {
	Messages []Message `json:"messages"`
	Question string    `json:"question"`
	Answer   string    `json:"answer"`
}
type Decision struct {
	ShouldFollowUp bool   `json:"shouldFollowUp"`
	ProblemSummary string `json:"problemSummary"`
	Message        string `json:"message"`
}
type Completer interface {
	CompleteJSON(context.Context, string, string, int) (string, error)
}
type CompleteFunc func(context.Context, string, string, int) (string, error)

func (f CompleteFunc) CompleteJSON(ctx context.Context, system, user string, tokens int) (string, error) {
	return f(ctx, system, user, tokens)
}
