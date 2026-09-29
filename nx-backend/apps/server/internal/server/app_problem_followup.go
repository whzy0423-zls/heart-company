package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/problemfollowup"
	"nine-xing/nx-backend/apps/server/internal/push"
)

const (
	problemFollowupPollInterval = 15 * time.Second
	problemFollowupWorkTimeout  = 25 * time.Second
)

type appProblemFollowupStore interface {
	BeginTurn(context.Context, int64, int64, time.Time) (problemfollowup.Turn, error)
	Claim(context.Context, time.Time) (*problemfollowup.Job, error)
	LoadContext(context.Context, problemfollowup.Job) (problemfollowup.Input, error)
	SaveDecision(context.Context, problemfollowup.Job, problemfollowup.Decision, time.Time) error
	Fail(context.Context, problemfollowup.Job, time.Time) error
	Deliver(context.Context, problemfollowup.Job, time.Time) (*problemfollowup.Delivery, error)
}

// Record accepted activity before generation, including failed/slow answers.
// A turn revision is carried into the chat store's existing save transaction.
func (s *Server) beginProblemFollowupTurn(r *http.Request, userID, sessionID int64) (*http.Request, error) {
	if s.problemFollowups == nil || chat.EnneagramType(r.Context()) > 0 {
		return r, nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	turn, err := s.problemFollowups.BeginTurn(ctx, userID, sessionID, time.Now())
	if err != nil {
		return r, err
	}
	return r.WithContext(problemfollowup.WithTurn(r.Context(), turn)), nil
}

func (s *Server) startProblemFollowups() {
	s.problemFollowups = problemfollowup.NewStore(s.db)
	ctx, cancel := context.WithCancel(context.Background())
	s.problemFollowupCancel = cancel
	for range 2 {
		s.problemFollowupWorkers.Add(1)
		go func() {
			defer s.problemFollowupWorkers.Done()
			ticker := time.NewTicker(problemFollowupPollInterval)
			defer ticker.Stop()
			for {
				if err := s.runProblemFollowupOnce(ctx, time.Now()); err != nil && ctx.Err() == nil {
					// Model/provider errors may contain conversation text or credentials.
					log.Print("problem followup worker pass failed; task will be rechecked")
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
}

func (s *Server) runProblemFollowupOnce(ctx context.Context, now time.Time) error {
	if s.problemFollowups == nil {
		return nil
	}
	for iteration := 0; iteration < 8; iteration++ {
		if iteration > 0 {
			now = time.Now()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		claimCtx, cancelClaim := context.WithTimeout(ctx, 5*time.Second)
		job, err := s.problemFollowups.Claim(claimCtx, now)
		cancelClaim()
		if err != nil {
			return err
		}
		if job == nil {
			return nil
		}
		workCtx, cancelWork := context.WithTimeout(ctx, problemFollowupWorkTimeout)
		err = s.processProblemFollowupJob(workCtx, *job, now)
		cancelWork()
		if err != nil && !errors.Is(err, problemfollowup.ErrStale) {
			// Use a fresh bounded context so a model timeout can still release its
			// durable claim. Cancellation/shutdown leaves recovery to the lease.
			if ctx.Err() == nil {
				failCtx, cancelFail := context.WithTimeout(ctx, 5*time.Second)
				failErr := s.problemFollowups.Fail(failCtx, *job, time.Now())
				cancelFail()
				if failErr != nil {
					return failErr
				}
			}
			return err
		}
	}
	return nil
}

func (s *Server) processProblemFollowupJob(ctx context.Context, job problemfollowup.Job, now time.Time) error {
	switch job.Status {
	case "classifying":
		input, err := s.problemFollowups.LoadContext(ctx, job)
		if err != nil {
			return err
		}
		evaluate := s.problemFollowupEvaluate
		if evaluate == nil {
			evaluate = problemfollowup.NewGenerator(problemfollowup.CompleteFunc(s.completePreferenceJSON)).Evaluate
		}
		decision, err := evaluate(ctx, input)
		if err != nil {
			return err
		}
		return s.problemFollowups.SaveDecision(ctx, job, decision, time.Now())
	case "delivering":
		delivery, err := s.problemFollowups.Deliver(ctx, job, time.Now())
		if err != nil || delivery == nil {
			return err
		}
		publish := s.problemFollowupPush
		if publish == nil {
			publish = s.pushProblemFollowup
		}
		// Chat + inbox were committed atomically. Push is one best-effort attempt:
		// retrying the durable task after a push error would duplicate delivery.
		if err := publish(ctx, *delivery); err != nil && ctx.Err() == nil {
			log.Print("problem followup device push failed; durable inbox is retained")
		}
		return nil
	default:
		return errors.New("invalid problem followup claim state")
	}
}

func (s *Server) pushProblemFollowup(ctx context.Context, delivery problemfollowup.Delivery) error {
	if s.pushStore == nil || s.pushStore.Pusher() == nil {
		return nil
	}
	pushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	registrationIDs, err := s.pushStore.GetRegistrationIDsByUserIDs(pushCtx, []int64{delivery.AppUserID})
	if err != nil || len(registrationIDs) == 0 {
		return err
	}
	_, err = s.pushStore.Pusher().Push(pushCtx, registrationIDs, push.Message{
		Title:    "有一条问题跟进",
		Content:  "关于刚才聊到的事情，想再关心一下你的进展。",
		DeepLink: delivery.DeepLink,
	})
	return err
}
