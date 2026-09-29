package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
)

func TestNormalizeTeacherEnrollmentInputMatchesMiniappBookingRules(t *testing.T) {
	got, err := normalizeTeacherEnrollmentInput(teacherEnrollmentInput{
		Kind: " course ", ContactName: " 张三 ", Phone: " 138-1234 5678 ",
		Intent: " 深入了解 ", PreferredTime: " 周六下午 ", Message: " 请联系我 ",
	})
	if err != nil {
		t.Fatalf("normalizeTeacherEnrollmentInput() error = %v", err)
	}
	if got.Kind != "course" || got.ContactName != "张三" || got.Phone != "13812345678" || got.Intent != "深入了解" || got.PreferredTime != "周六下午" || got.Message != "请联系我" {
		t.Fatalf("normalized input = %+v", got)
	}
}

func TestNormalizeTeacherEnrollmentInputRejectsInvalidPayload(t *testing.T) {
	cases := []teacherEnrollmentInput{
		{ContactName: "张三", Phone: "13812345678", Kind: "unknown"},
		{ContactName: "张三", Phone: "18012345678\n90"},
		{ContactName: "张三", Phone: "1801234567"},
		{ContactName: "", Phone: "18012345678"},
	}
	for _, in := range cases {
		if _, err := normalizeTeacherEnrollmentInput(in); !errors.Is(err, errInvalidTeacherEnrollment) {
			t.Errorf("input %+v error = %v, want errInvalidTeacherEnrollment", in, err)
		}
	}
}

func TestAppTeacherEnrollmentRejectsMissingAppUserContext(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/app/teachers/han/enrollments", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.appTeacherEnrollment(rec, req, "han")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s, want 401", rec.Code, rec.Body.String())
	}
}

func TestAppTeacherEnrollmentRequiresService(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/app/teachers/han/enrollments", strings.NewReader(`{"contactName":"张三","phone":"13812345678"}`))
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	rec := httptest.NewRecorder()
	s.appTeacherEnrollment(rec, req, "han")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s, want 503", rec.Code, rec.Body.String())
	}
}
