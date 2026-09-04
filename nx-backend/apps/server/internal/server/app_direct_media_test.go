package server

import (
	"mime/multipart"
	"net/textproto"
	"testing"
)

func TestValidDirectMediaAcceptsExpectedTypes(t *testing.T) {
	imageHeader := &multipart.FileHeader{Header: textproto.MIMEHeader{"Content-Type": []string{"image/png"}}}
	if !validDirectMedia("image", imageHeader, []byte("not-inspected-when-header-is-image")) {
		t.Fatal("expected image upload to be accepted")
	}
	audioHeader := &multipart.FileHeader{Filename: "voice.aac", Header: textproto.MIMEHeader{"Content-Type": []string{"audio/aac"}}}
	if !validDirectMedia("voice", audioHeader, []byte("audio")) {
		t.Fatal("expected voice upload to be accepted")
	}
	if validDirectMedia("video", imageHeader, []byte("image")) {
		t.Fatal("unexpected media type should be rejected")
	}
}
