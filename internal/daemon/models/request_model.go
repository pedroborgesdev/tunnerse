package models

import (
	"encoding/base64"
	"encoding/json"
)

type RequestData struct {
	Method    string              `json:"method"`
	Path      string              `json:"path"`
	Headers   map[string][]string `json:"headers"`
	Body      []byte              `json:"-"`
	Host      string              `json:"host"`
	RequestID string              `json:"request_id"`
	Token     string              `json:"token"`
}

func (r *RequestData) UnmarshalJSON(data []byte) error {
	type Alias struct {
		Method    string              `json:"method"`
		Path      string              `json:"path"`
		Headers   map[string][]string `json:"headers"`
		Body      string              `json:"body"`
		Host      string              `json:"host"`
		RequestID string              `json:"request_id"`
		Token     string              `json:"token"`
	}

	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	body, err := base64.StdEncoding.DecodeString(alias.Body)
	if err != nil {
		body = []byte(alias.Body)
	}

	*r = RequestData{
		Method:    alias.Method,
		Path:      alias.Path,
		Headers:   alias.Headers,
		Body:      body,
		Host:      alias.Host,
		RequestID: alias.RequestID,
		Token:     alias.Token,
	}
	return nil
}

type ResponseData struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       []byte              `json:"-"`
	Token      string              `json:"token"`
}

func (r *ResponseData) MarshalJSON() ([]byte, error) {
	type Alias struct {
		StatusCode int                 `json:"status_code"`
		Headers    map[string][]string `json:"headers"`
		Body       string              `json:"body"`
		Token      string              `json:"token"`
	}

	return json.Marshal(&Alias{
		StatusCode: r.StatusCode,
		Headers:    r.Headers,
		Body:       base64.StdEncoding.EncodeToString(r.Body),
		Token:      r.Token,
	})
}

type RegisterResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Message   string `json:"message"`
		Subdomain bool   `json:"subdomain"`
		Tunnel    string `json:"tunnel"`
	} `json:"data"`
	Status int `json:"status"`
}
