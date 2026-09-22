package server

import (
	"net/http"
	"testing"
)

func TestParseTeacherUploadPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantAction string
		contentID  int64
		taskID     int64
		part       int
		ok         bool
	}{
		{name: "initiate", path: "7/uploads/initiate", wantAction: "initiate", contentID: 7, ok: true},
		{name: "sign", path: "7/uploads/19/parts/2/sign", wantAction: "sign", contentID: 7, taskID: 19, part: 2, ok: true},
		{name: "complete", path: "7/uploads/19/complete", wantAction: "complete", contentID: 7, taskID: 19, ok: true},
		{name: "progress", path: "7/uploads/19/progress", wantAction: "progress", contentID: 7, taskID: 19, ok: true},
		{name: "abort", path: "7/uploads/19/abort", wantAction: "abort", contentID: 7, taskID: 19, ok: true},
		{name: "bad part", path: "7/uploads/19/parts/no/sign", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTeacherUploadPath(tt.path)
			if got.OK != tt.ok || got.Action != tt.wantAction || got.ContentID != tt.contentID || got.TaskID != tt.taskID || got.PartNumber != tt.part {
				t.Fatalf("parseTeacherUploadPath(%q)=%+v", tt.path, got)
			}
		})
	}
}

func TestTeacherUploadActionRequiresPost(t *testing.T) {
	if got := teacherUploadMethodAllowed(http.MethodGet); got {
		t.Fatal("GET must not be accepted for upload mutations")
	}
	if got := teacherUploadMethodAllowed(http.MethodPost); !got {
		t.Fatal("POST should be accepted for upload mutations")
	}
}
