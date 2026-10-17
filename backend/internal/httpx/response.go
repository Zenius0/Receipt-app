package httpx

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Error   string            `json:"error"`
	Message string            `json:"message,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorBody{Error: code, Message: message})
}
