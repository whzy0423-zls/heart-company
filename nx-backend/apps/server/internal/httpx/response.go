package httpx

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Code    int         `json:"code"`
	Data    any         `json:"data"`
	Error   any         `json:"error"`
	Message string      `json:"message"`
}

func OK(w http.ResponseWriter, data any) {
	JSON(w, http.StatusOK, Response{
		Code:    0,
		Data:    data,
		Error:   nil,
		Message: "ok",
	})
}

func Fail(w http.ResponseWriter, status int, message string) {
	JSON(w, status, Response{
		Code:    -1,
		Data:    nil,
		Error:   message,
		Message: message,
	})
}

func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
