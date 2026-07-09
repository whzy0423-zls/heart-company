package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
)

func TestAppReassessmentLatestPassesAuthenticatedUserAndCard(t *testing.T) {
	service := &fakeReassessmentService{latest: map[string]any{"id": int64(66), "cardId": int64(123), "status": "generated"}}
	s := &Server{appReassessment: service}

	response := performAppCalibrationRequest(t, s.appReassessmentLatest, http.MethodGet, "/api/app/reassessment/latest?cardId=123", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	if service.latestUserID != 7 || service.latestCardID != 123 {
		t.Fatalf("expected service to receive user=7 card=123, got user=%d card=%d", service.latestUserID, service.latestCardID)
	}
}

func TestAppReassessmentDetailMapsForeignReportToNotFound(t *testing.T) {
	service := &fakeReassessmentService{detailErr: sql.ErrNoRows}
	s := &Server{appReassessment: service}

	response := performAppCalibrationRequest(t, s.appReassessmentRouter, http.MethodGet, "/api/app/reassessment/66", nil)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign/missing report, got %d body=%s", response.Code, response.Body.String())
	}
	if service.detailUserID != 7 || service.detailID != 66 {
		t.Fatalf("expected service to receive user=7 report=66, got user=%d report=%d", service.detailUserID, service.detailID)
	}
}

func TestAppReassessmentAcceptPassesAuthenticatedUserAndID(t *testing.T) {
	service := &fakeReassessmentService{accept: map[string]any{"accepted": true}}
	s := &Server{appReassessment: service}

	response := performAppCalibrationRequest(t, s.appReassessmentRouter, http.MethodPost, "/api/app/reassessment/66/accept", nil)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	if service.acceptUserID != 7 || service.acceptID != 66 {
		t.Fatalf("expected service to receive user=7 report=66, got user=%d report=%d", service.acceptUserID, service.acceptID)
	}
}

func TestAppReassessmentRejectRejectsInvalidID(t *testing.T) {
	service := &fakeReassessmentService{}
	s := &Server{appReassessment: service}

	response := performAppCalibrationRequest(t, s.appReassessmentRouter, http.MethodPost, "/api/app/reassessment/0/reject", nil)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid reassessment id, got %d body=%s", response.Code, response.Body.String())
	}
	if service.rejectID != 0 {
		t.Fatalf("service should not be called for invalid id, got reject id=%d", service.rejectID)
	}
}

type fakeReassessmentService struct {
	latest       any
	latestErr    error
	latestUserID int64
	latestCardID int64

	detail       any
	detailErr    error
	detailUserID int64
	detailID     int64

	accept       any
	acceptErr    error
	acceptUserID int64
	acceptID     int64

	reject       any
	rejectErr    error
	rejectUserID int64
	rejectID     int64
}

func (f *fakeReassessmentService) Latest(_ context.Context, appUserID, cardID int64) (any, error) {
	f.latestUserID = appUserID
	f.latestCardID = cardID
	return f.latest, f.latestErr
}

func (f *fakeReassessmentService) Detail(_ context.Context, appUserID, id int64) (any, error) {
	f.detailUserID = appUserID
	f.detailID = id
	return f.detail, f.detailErr
}

func (f *fakeReassessmentService) Accept(_ context.Context, appUserID, id int64) (any, error) {
	f.acceptUserID = appUserID
	f.acceptID = id
	return f.accept, f.acceptErr
}

func (f *fakeReassessmentService) Reject(_ context.Context, appUserID, id int64) (any, error) {
	f.rejectUserID = appUserID
	f.rejectID = id
	return f.reject, f.rejectErr
}
