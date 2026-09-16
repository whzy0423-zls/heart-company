package knowledgeclient

import (
	"errors"
	"fmt"
)

var (
	ErrResponseTooLarge  = errors.New("knowledge service response too large")
	ErrStreamInterrupted = errors.New("knowledge service stream interrupted before done")
)

type ResponseError struct {
	StatusCode int
	Body       string
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("knowledge service returned HTTP %d: %s", e.StatusCode, e.Body)
}

type StreamError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *StreamError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("knowledge service stream error %s: %s", e.Code, e.Message)
}
