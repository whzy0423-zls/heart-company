package server

import (
	"context"
	"log"
	"time"
)

func (s *Server) enqueueCareEvaluation(appUserID int64) {
	if s == nil || s.careEvaluator == nil || appUserID <= 0 {
		return
	}
	go s.careEvaluator.EnqueueAndEvaluate(context.Background(), appUserID)
}

func (s *Server) runCareEvaluationSweep(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.careEvaluator == nil {
				continue
			}
			// Message hooks enqueue immediately; this periodic pass is intentionally
			// conservative and lets the queue implementation remain the source of truth.
			if err := s.careEvaluator.Sweep(ctx); err != nil {
				log.Printf("care evaluation sweep failed: %v", err)
			}
		}
	}
}
