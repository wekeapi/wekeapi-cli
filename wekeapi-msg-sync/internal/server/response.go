package server

import (
	"encoding/json"
	"net/http"
)

// 错误码。
const (
	CodeInvalidParams = "INVALID_PARAMS"
	CodeInternalError = "INTERNAL_ERROR"
)

type successBody struct {
	Success bool `json:"success"`
	Data    any  `json:"data"`
}

type errorBody struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeSuccess(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, successBody{Success: true, Data: data})
}

func writeMessage(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": message})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Success: false, Error: message, Code: code})
}
