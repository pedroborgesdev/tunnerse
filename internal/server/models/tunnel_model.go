package models

import "net/http"

type SerializableRequest struct {
	Method    string      `json:"method"`
	Path      string      `json:"path"`
	Header    http.Header `json:"headers"`
	Body      string      `json:"body"`
	Host      string      `json:"host"`
	RequestID string      `json:"request_id"`
	Token     string      `json:"token"`
}

type ResponseData struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
	Token      string              `json:"token"`
}
