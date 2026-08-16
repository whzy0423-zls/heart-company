package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
	"nine-xing/nx-backend/apps/server/internal/push"
)

func TestSendDailyQuizRemindersCreatesBatchSendsDeepLinkAndMarksPushSent(t *testing.T) {
	registerAppCalibrationPushTestDriver()
	database, err := sql.Open(appCalibrationPushTestDriverName, "daily")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := &fakeAppCalibrationReminderService{
		dailyCandidates: []profilecalibration.DailyReminderCandidate{{AppUserID: 7, CardID: 123}},
		dailyBatch:      profilecalibration.Batch{ID: 88, CardID: 123},
	}
	pusher := &recordingAppCalibrationPusher{}
	inbox := &fakeAppNotificationService{}
	s := &Server{appDailyQuizReminders: service, pushStore: push.NewStore(database, pusher), appNotifications: inbox}

	result, err := s.sendDailyQuizReminders(context.Background(), time.Date(2026, 7, 9, 9, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	if err != nil {
		t.Fatalf("send daily quiz reminders: %v", err)
	}

	if service.dailyDate != "2026-07-09" || service.todayDate != "2026-07-09" {
		t.Fatalf("expected business date 2026-07-09, got list=%q today=%q", service.dailyDate, service.todayDate)
	}
	if service.todayUserID != 7 || service.todayCardID != 123 {
		t.Fatalf("expected batch for user/card 7/123, got %d/%d", service.todayUserID, service.todayCardID)
	}
	if service.markedBatchID != 88 {
		t.Fatalf("expected batch 88 marked push sent, got %d", service.markedBatchID)
	}
	if result.Candidates != 1 || result.SentUsers != 1 || result.SentDevices != 1 {
		t.Fatalf("unexpected daily reminder result: %+v", result)
	}
	if len(pusher.messages) != 1 || pusher.messages[0].DeepLink != "/daily-quiz" {
		t.Fatalf("expected one /daily-quiz push, messages=%+v", pusher.messages)
	}
	if len(inbox.createdUsers) != 1 || inbox.createdUsers[0].source != "daily-quiz:88" || inbox.createdUsers[0].userID != 7 {
		t.Fatalf("expected one idempotent daily inbox item, items=%+v", inbox.createdUsers)
	}
}

func TestSendGeneratedReassessmentRemindersSendsReportDeepLinkAndMarksPushSent(t *testing.T) {
	registerAppCalibrationPushTestDriver()
	database, err := sql.Open(appCalibrationPushTestDriverName, "reassessment")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := &fakeAppCalibrationReminderService{
		reassessmentCandidates: []profilecalibration.ReassessmentPushCandidate{{ID: 66, AppUserID: 7, CardID: 123}},
	}
	pusher := &recordingAppCalibrationPusher{}
	inbox := &fakeAppNotificationService{}
	s := &Server{appDailyQuizReminders: service, pushStore: push.NewStore(database, pusher), appNotifications: inbox}

	result, err := s.sendGeneratedReassessmentReminders(context.Background())
	if err != nil {
		t.Fatalf("send reassessment reminders: %v", err)
	}

	if service.markedReassessmentID != 66 {
		t.Fatalf("expected reassessment 66 marked push sent, got %d", service.markedReassessmentID)
	}
	if result.Candidates != 1 || result.SentUsers != 1 || result.SentDevices != 1 {
		t.Fatalf("unexpected reassessment reminder result: %+v", result)
	}
	if len(pusher.messages) != 1 || pusher.messages[0].DeepLink != "/reassessment/66" {
		t.Fatalf("expected one report push, messages=%+v", pusher.messages)
	}
	if len(inbox.createdUsers) != 1 || inbox.createdUsers[0].source != "reassessment:66" || inbox.createdUsers[0].userID != 7 {
		t.Fatalf("expected one idempotent reassessment inbox item, items=%+v", inbox.createdUsers)
	}
}

func TestProfileCalibrationScheduledTasksPreGeneratesBeforeNoonAndSendsAtNoon(t *testing.T) {
	registerAppCalibrationPushTestDriver()
	database, err := sql.Open(appCalibrationPushTestDriverName, "scheduled")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := &fakeAppCalibrationReminderService{
		dailyCandidates: []profilecalibration.DailyReminderCandidate{{AppUserID: 7, CardID: 123}},
		dailyBatch:      profilecalibration.Batch{ID: 88, CardID: 123},
	}
	pusher := &recordingAppCalibrationPusher{}
	s := &Server{appDailyQuizReminders: service, pushStore: push.NewStore(database, pusher)}
	loc := time.FixedZone("CST", 8*3600)

	s.runProfileCalibrationScheduledTasks(context.Background(), time.Date(2026, 7, 9, 11, 30, 0, 0, loc))
	if service.ensureSetDate != "2026-07-09" {
		t.Fatalf("expected 11:30 to pre-generate quiz set for 2026-07-09, got %q", service.ensureSetDate)
	}
	if len(pusher.messages) != 0 {
		t.Fatalf("11:30 must not send daily quiz push, messages=%+v", pusher.messages)
	}

	s.runProfileCalibrationScheduledTasks(context.Background(), time.Date(2026, 7, 9, 12, 0, 0, 0, loc))
	if len(pusher.messages) != 1 || pusher.messages[0].DeepLink != "/daily-quiz" {
		t.Fatalf("12:00 should send daily quiz push, messages=%+v", pusher.messages)
	}
	if service.pushedSetDate != "2026-07-09" {
		t.Fatalf("expected daily quiz set pushed date to be marked, got %q", service.pushedSetDate)
	}
}

func TestShouldSendDailyQuizUsesNoonCompensationWindow(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	for _, minute := range []int{0, 1, 5, 10} {
		when := time.Date(2026, 7, 9, 12, minute, 30, 0, loc)
		if !shouldSendDailyQuiz(when) {
			t.Fatalf("expected daily quiz push window to include %s", when.Format("15:04:05"))
		}
	}
	for _, when := range []time.Time{
		time.Date(2026, 7, 9, 11, 59, 59, 0, loc),
		time.Date(2026, 7, 9, 12, 11, 0, 0, loc),
		time.Date(2026, 7, 9, 13, 0, 0, 0, loc),
	} {
		if shouldSendDailyQuiz(when) {
			t.Fatalf("expected daily quiz push window to exclude %s", when.Format("15:04:05"))
		}
	}
}

func TestSendDailyQuizRemindersSkipsWhenBatchClaimFails(t *testing.T) {
	registerAppCalibrationPushTestDriver()
	database, err := sql.Open(appCalibrationPushTestDriverName, "daily-claim-false")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := &fakeAppCalibrationReminderService{
		dailyCandidates: []profilecalibration.DailyReminderCandidate{{AppUserID: 7, CardID: 123}},
		dailyBatch:      profilecalibration.Batch{ID: 88, CardID: 123},
		denyBatchClaim:  true,
	}
	pusher := &recordingAppCalibrationPusher{}
	s := &Server{appDailyQuizReminders: service, pushStore: push.NewStore(database, pusher)}

	result, err := s.sendDailyQuizReminders(context.Background(), time.Date(2026, 7, 9, 9, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	if err != nil {
		t.Fatalf("send daily quiz reminders: %v", err)
	}
	if result.SentUsers != 0 || len(pusher.messages) != 0 || service.markedBatchID != 0 {
		t.Fatalf("claim=false must skip sending and marking, result=%+v messages=%+v marked=%d", result, pusher.messages, service.markedBatchID)
	}
}

func TestSendGeneratedReassessmentRemindersSkipsWhenClaimFails(t *testing.T) {
	registerAppCalibrationPushTestDriver()
	database, err := sql.Open(appCalibrationPushTestDriverName, "reassessment-claim-false")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := &fakeAppCalibrationReminderService{
		reassessmentCandidates: []profilecalibration.ReassessmentPushCandidate{{ID: 66, AppUserID: 7, CardID: 123}},
		denyReassessmentClaim:  true,
	}
	pusher := &recordingAppCalibrationPusher{}
	s := &Server{appDailyQuizReminders: service, pushStore: push.NewStore(database, pusher)}

	result, err := s.sendGeneratedReassessmentReminders(context.Background())
	if err != nil {
		t.Fatalf("send reassessment reminders: %v", err)
	}
	if result.SentUsers != 0 || len(pusher.messages) != 0 || service.markedReassessmentID != 0 {
		t.Fatalf("claim=false must skip sending and marking, result=%+v messages=%+v marked=%d", result, pusher.messages, service.markedReassessmentID)
	}
}

type fakeAppCalibrationReminderService struct {
	dailyCandidates []profilecalibration.DailyReminderCandidate
	dailyDate       string
	dailyLimit      int

	dailyBatch  profilecalibration.Batch
	todayUserID int64
	todayCardID int64
	todayDate   string

	markedBatchID  int64
	denyBatchClaim bool
	ensureSetDate  string
	pushedSetDate  string

	reassessmentCandidates []profilecalibration.ReassessmentPushCandidate
	reassessmentLimit      int
	markedReassessmentID   int64
	denyReassessmentClaim  bool
}

func (f *fakeAppCalibrationReminderService) ListDailyReminderCandidates(_ context.Context, date string, limit int) ([]profilecalibration.DailyReminderCandidate, error) {
	f.dailyDate = date
	f.dailyLimit = limit
	return f.dailyCandidates, nil
}

func (f *fakeAppCalibrationReminderService) TodayBatchForDate(_ context.Context, appUserID, cardID int64, date string) (profilecalibration.Batch, error) {
	f.todayUserID = appUserID
	f.todayCardID = cardID
	f.todayDate = date
	return f.dailyBatch, nil
}

func (f *fakeAppCalibrationReminderService) ClaimBatchPush(_ context.Context, batchID int64) (bool, error) {
	return !f.denyBatchClaim, nil
}

func (f *fakeAppCalibrationReminderService) MarkBatchPushSent(_ context.Context, batchID int64) error {
	f.markedBatchID = batchID
	return nil
}

func (f *fakeAppCalibrationReminderService) EnsureDailyQuizSet(_ context.Context, date string) (profilecalibration.DailyQuizSet, error) {
	f.ensureSetDate = date
	return profilecalibration.DailyQuizSet{Date: date, Status: "generated"}, nil
}

func (f *fakeAppCalibrationReminderService) MarkDailyQuizSetPushed(_ context.Context, date string) error {
	f.pushedSetDate = date
	return nil
}

func (f *fakeAppCalibrationReminderService) ListGeneratedReassessmentPushCandidates(_ context.Context, limit int) ([]profilecalibration.ReassessmentPushCandidate, error) {
	f.reassessmentLimit = limit
	return f.reassessmentCandidates, nil
}

func (f *fakeAppCalibrationReminderService) ClaimReassessmentPush(_ context.Context, id int64) (bool, error) {
	return !f.denyReassessmentClaim, nil
}

func (f *fakeAppCalibrationReminderService) MarkReassessmentPushSent(_ context.Context, id int64) error {
	f.markedReassessmentID = id
	return nil
}

type recordingAppCalibrationPusher struct {
	messages []push.Message
}

func (p *recordingAppCalibrationPusher) Push(_ context.Context, registrationIDs []string, msg push.Message) (push.PushResult, error) {
	p.messages = append(p.messages, msg)
	return push.PushResult{MsgID: "test", Sent: len(registrationIDs)}, nil
}

const appCalibrationPushTestDriverName = "app_calibration_push_test"

var appCalibrationPushTestDriverRegistered bool

func registerAppCalibrationPushTestDriver() {
	if appCalibrationPushTestDriverRegistered {
		return
	}
	appCalibrationPushTestDriverRegistered = true
	sql.Register(appCalibrationPushTestDriverName, appCalibrationPushTestDriver{})
}

type appCalibrationPushTestDriver struct{}

func (appCalibrationPushTestDriver) Open(string) (driver.Conn, error) {
	return appCalibrationPushTestConn{}, nil
}

type appCalibrationPushTestConn struct{}

func (appCalibrationPushTestConn) Prepare(string) (driver.Stmt, error)      { return nil, nil }
func (appCalibrationPushTestConn) Close() error                             { return nil }
func (appCalibrationPushTestConn) Begin() (driver.Tx, error)                { return nil, nil }
func (appCalibrationPushTestConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (appCalibrationPushTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "ANY($1)") {
		return &appCalibrationPushRows{columns: []string{"registration_id"}, rows: [][]driver.Value{{"reg-1"}}}, nil
	}
	return &appCalibrationPushRows{columns: nil}, nil
}

type appCalibrationPushRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r appCalibrationPushRows) Columns() []string { return r.columns }
func (r appCalibrationPushRows) Close() error      { return nil }
func (r *appCalibrationPushRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

var _ driver.QueryerContext = appCalibrationPushTestConn{}
var _ driver.NamedValueChecker = appCalibrationPushTestConn{}
