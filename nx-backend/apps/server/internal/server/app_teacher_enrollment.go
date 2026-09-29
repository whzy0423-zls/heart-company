package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/signup"
	"nine-xing/nx-backend/apps/server/internal/teacher"
)

var errInvalidTeacherEnrollment = errors.New("invalid teacher enrollment")

type teacherEnrollmentInput struct {
	Kind          string `json:"kind"`
	ContactName   string `json:"contactName"`
	Phone         string `json:"phone"`
	Intent        string `json:"intent"`
	PreferredTime string `json:"preferredTime"`
	Message       string `json:"message"`
}

const (
	maxTeacherEnrollmentNameRunes    = 80
	maxTeacherEnrollmentIntentRunes  = 120
	maxTeacherEnrollmentTimeRunes    = 80
	maxTeacherEnrollmentMessageRunes = 1000
)

func normalizeTeacherEnrollmentInput(in teacherEnrollmentInput) (teacherEnrollmentInput, error) {
	in.Kind = strings.TrimSpace(in.Kind)
	if in.Kind == "" {
		in.Kind = "consult"
	}
	switch in.Kind {
	case "consult", "course", "enterprise":
	default:
		return teacherEnrollmentInput{}, errInvalidTeacherEnrollment
	}
	in.ContactName = strings.TrimSpace(in.ContactName)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Intent = strings.TrimSpace(in.Intent)
	in.PreferredTime = strings.TrimSpace(in.PreferredTime)
	in.Message = strings.TrimSpace(in.Message)
	for _, field := range []string{in.ContactName, in.Phone, in.Intent, in.PreferredTime, in.Message} {
		if strings.ContainsFunc(field, unicode.IsControl) {
			return teacherEnrollmentInput{}, errInvalidTeacherEnrollment
		}
	}
	if in.ContactName == "" || utf8.RuneCountInString(in.ContactName) > maxTeacherEnrollmentNameRunes ||
		utf8.RuneCountInString(in.Intent) > maxTeacherEnrollmentIntentRunes ||
		utf8.RuneCountInString(in.PreferredTime) > maxTeacherEnrollmentTimeRunes ||
		utf8.RuneCountInString(in.Message) > maxTeacherEnrollmentMessageRunes {
		return teacherEnrollmentInput{}, errInvalidTeacherEnrollment
	}
	in.Phone = strings.NewReplacer(" ", "", "　", "", "-", "", "－", "").Replace(in.Phone)
	if !isMainlandPhone(in.Phone) {
		return teacherEnrollmentInput{}, errInvalidTeacherEnrollment
	}
	return in, nil
}

func (s *Server) appTeacherEnrollment(w http.ResponseWriter, r *http.Request, key string) {
	user, ok := appUserFromContext(r)
	if !ok || user.ID <= 0 {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if s == nil || s.teachers == nil || s.signupService == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "teacher enrollment service unavailable")
		return
	}
	key = strings.TrimSpace(key)
	profile, err := s.teachers.Get(r.Context(), key)
	if errors.Is(err, teacher.ErrNotFound) || errors.Is(err, io.EOF) {
		httpx.Fail(w, http.StatusNotFound, "teacher not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "get teacher failed")
		return
	}
	if !profile.Enabled {
		httpx.Fail(w, http.StatusNotFound, "teacher not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var body teacherEnrollmentInput
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "报名信息格式不正确")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpx.Fail(w, http.StatusBadRequest, "报名信息格式不正确")
		return
	}
	in, err := normalizeTeacherEnrollmentInput(body)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "报名信息格式不正确")
		return
	}

	teacherLabel := fmt.Sprintf("%s（%s）", strings.TrimSpace(profile.Name), key)
	interest := fmt.Sprintf("老师：%s | 报名类型：%s | 意向方向：%s", teacherLabel, in.Kind, in.Intent)
	message := fmt.Sprintf("老师：%s\n报名类型：%s\n意向方向：%s\n期望时间：%s\n留言：%s", teacherLabel, in.Kind, in.Intent, in.PreferredTime, in.Message)
	leadInput := signup.LeadInput{
		Name:        in.ContactName,
		Contact:     in.Phone,
		ContactType: signup.ContactTypePhone,
		Interest:    interest,
		Message:     message,
		AttributionInput: signup.AttributionInput{
			SourcePath:  r.URL.Path,
			LandingPage: r.URL.Path,
			VisitorID:   fmt.Sprintf("app-user:%d", user.ID),
		},
	}
	var lead signup.Lead
	if teacherService, ok := s.signupService.(teacherSignupCreator); ok {
		lead, err = teacherService.CreateTeacherSignup(r.Context(), leadInput, profile.Name, key, in.Kind, r)
	} else {
		// Keep compatibility with test fixtures and older deployments that only
		// expose the original website signup service. The persisted fields and
		// broadcast contract remain identical; the concrete current service uses
		// the teacher-specific notification above.
		lead, err = s.signupService.CreateWebsiteSignup(r.Context(), leadInput, r)
	}
	if err != nil {
		if errors.Is(err, signup.ErrServiceNotConfigured) {
			httpx.Fail(w, http.StatusServiceUnavailable, "报名服务暂不可用")
			return
		}
		httpx.Fail(w, http.StatusBadRequest, "报名提交失败")
		return
	}
	s.broadcastSignup(lead)
	httpx.OK(w, map[string]any{"signupId": lead.ID, "teacherKey": key, "status": "submitted"})
}
