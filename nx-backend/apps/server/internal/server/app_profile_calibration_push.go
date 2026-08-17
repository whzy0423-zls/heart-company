package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
	"nine-xing/nx-backend/apps/server/internal/push"
)

const defaultProfileCalibrationPushInterval = time.Minute

type appDailyQuizReminderService interface {
	ListDailyReminderCandidates(ctx context.Context, date string, limit int) ([]profilecalibration.DailyReminderCandidate, error)
	TodayBatchForDate(ctx context.Context, appUserID, cardID int64, date string) (profilecalibration.Batch, error)
	EnsureDailyQuizSet(ctx context.Context, date string) (profilecalibration.DailyQuizSet, error)
	ClaimBatchPush(ctx context.Context, batchID int64) (bool, error)
	MarkBatchPushSent(ctx context.Context, batchID int64) error
	MarkDailyQuizSetPushed(ctx context.Context, date string) error
	ListGeneratedReassessmentPushCandidates(ctx context.Context, limit int) ([]profilecalibration.ReassessmentPushCandidate, error)
	ClaimReassessmentPush(ctx context.Context, id int64) (bool, error)
	MarkReassessmentPushSent(ctx context.Context, id int64) error
}

type appCalibrationPushResult struct {
	Candidates  int
	SentUsers   int
	SentDevices int
}

func (s *Server) runProfileCalibrationPushLoop(ctx context.Context) {
	s.runProfileCalibrationScheduledTasks(ctx, time.Now())
	ticker := time.NewTicker(s.profileCalibrationPushInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runProfileCalibrationScheduledTasks(ctx, time.Now())
		}
	}
}

func (s *Server) runProfileCalibrationPushOnce(ctx context.Context) {
	s.runProfileCalibrationScheduledTasks(ctx, time.Now())
}

func (s *Server) runProfileCalibrationScheduledTasks(ctx context.Context, now time.Time) {
	if _, err := s.sendGeneratedReassessmentReminders(ctx); err != nil {
		log.Printf("profile calibration reassessment reminder failed: %v", err)
	}
	date := appCalibrationBusinessDate(now)
	if shouldPreGenerateDailyQuiz(now) {
		if _, err := s.ensureDailyQuizSet(ctx, date); err != nil {
			log.Printf("profile calibration daily quiz pre-generate failed: %v", err)
		}
		return
	}
	if shouldSendDailyQuiz(now) {
		if _, err := s.ensureDailyQuizSet(ctx, date); err != nil {
			log.Printf("profile calibration daily quiz noon ensure failed: %v", err)
		}
		if _, err := s.sendDailyQuizReminders(ctx, now); err != nil {
			log.Printf("profile calibration daily quiz reminder failed: %v", err)
			return
		}
		if err := s.markDailyQuizSetPushed(ctx, date); err != nil {
			log.Printf("profile calibration daily quiz set mark pushed failed: %v", err)
		}
	}
}

func (s *Server) profileCalibrationPushInterval() time.Duration {
	return defaultProfileCalibrationPushInterval
}

func (s *Server) ensureDailyQuizSet(ctx context.Context, date string) (profilecalibration.DailyQuizSet, error) {
	if s == nil || s.appDailyQuizReminders == nil {
		return profilecalibration.DailyQuizSet{}, nil
	}
	return s.appDailyQuizReminders.EnsureDailyQuizSet(ctx, date)
}

func (s *Server) markDailyQuizSetPushed(ctx context.Context, date string) error {
	if s == nil || s.appDailyQuizReminders == nil {
		return nil
	}
	return s.appDailyQuizReminders.MarkDailyQuizSetPushed(ctx, date)
}

func (s *Server) sendDailyQuizReminders(ctx context.Context, now time.Time) (appCalibrationPushResult, error) {
	var result appCalibrationPushResult
	if s == nil || s.appDailyQuizReminders == nil {
		return result, nil
	}
	date := appCalibrationBusinessDate(now)
	candidates, err := s.appDailyQuizReminders.ListDailyReminderCandidates(ctx, date, 1000)
	if err != nil {
		return result, err
	}
	result.Candidates = len(candidates)
	for _, candidate := range candidates {
		batch, err := s.appDailyQuizReminders.TodayBatchForDate(ctx, candidate.AppUserID, candidate.CardID, date)
		if err != nil {
			return result, err
		}
		claimed, err := s.appDailyQuizReminders.ClaimBatchPush(ctx, batch.ID)
		if err != nil {
			return result, err
		}
		if !claimed {
			continue
		}
		message := push.DailyQuizReminder()
		if s.appNotifications != nil {
			if _, err := s.appNotifications.CreateForUser(
				ctx, candidate.AppUserID, "growth", message.Title, message.Content,
				message.DeepLink, fmt.Sprintf("daily-quiz:%d", batch.ID),
			); err != nil {
				return result, err
			}
		}
		if s.pushStore == nil || s.pushStore.Pusher() == nil {
			continue
		}
		registrationIDs, err := s.pushStore.GetRegistrationIDsByUserIDs(ctx, []int64{candidate.AppUserID})
		if err != nil {
			return result, err
		}
		if len(registrationIDs) == 0 {
			continue
		}
		pushResult, err := s.pushStore.Pusher().Push(ctx, registrationIDs, message)
		if err != nil {
			return result, err
		}
		if pushResult.Sent <= 0 {
			continue
		}
		if err := s.appDailyQuizReminders.MarkBatchPushSent(ctx, batch.ID); err != nil {
			return result, err
		}
		result.SentUsers++
		result.SentDevices += pushResult.Sent
	}
	return result, nil
}

func (s *Server) sendGeneratedReassessmentReminders(ctx context.Context) (appCalibrationPushResult, error) {
	var result appCalibrationPushResult
	if s == nil || s.appDailyQuizReminders == nil {
		return result, nil
	}
	candidates, err := s.appDailyQuizReminders.ListGeneratedReassessmentPushCandidates(ctx, 1000)
	if err != nil {
		return result, err
	}
	result.Candidates = len(candidates)
	for _, candidate := range candidates {
		claimed, err := s.appDailyQuizReminders.ClaimReassessmentPush(ctx, candidate.ID)
		if err != nil {
			return result, err
		}
		if !claimed {
			continue
		}
		message := push.ReassessmentReady(candidate.ID)
		if s.appNotifications != nil {
			if _, err := s.appNotifications.CreateForUser(
				ctx, candidate.AppUserID, "growth", message.Title, message.Content,
				message.DeepLink, fmt.Sprintf("reassessment:%d", candidate.ID),
			); err != nil {
				return result, err
			}
		}
		if s.pushStore == nil || s.pushStore.Pusher() == nil {
			continue
		}
		registrationIDs, err := s.pushStore.GetRegistrationIDsByUserIDs(ctx, []int64{candidate.AppUserID})
		if err != nil {
			return result, err
		}
		if len(registrationIDs) == 0 {
			continue
		}
		pushResult, err := s.pushStore.Pusher().Push(ctx, registrationIDs, message)
		if err != nil {
			return result, err
		}
		if pushResult.Sent <= 0 {
			continue
		}
		if err := s.appDailyQuizReminders.MarkReassessmentPushSent(ctx, candidate.ID); err != nil {
			return result, err
		}
		result.SentUsers++
		result.SentDevices += pushResult.Sent
	}
	return result, nil
}

func appCalibrationBusinessDate(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return now.In(loc).Format("2006-01-02")
}

func shouldPreGenerateDailyQuiz(now time.Time) bool {
	local := appCalibrationLocalTime(now)
	if local.Hour() != 11 {
		return false
	}
	switch local.Minute() {
	case 30, 40, 50:
		return true
	default:
		return false
	}
}

func shouldSendDailyQuiz(now time.Time) bool {
	local := appCalibrationLocalTime(now)
	return local.Hour() == 12 && local.Minute() >= 0 && local.Minute() <= 10
}

func appCalibrationLocalTime(now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return now.In(loc)
}
