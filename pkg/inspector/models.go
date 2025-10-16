// Package inspector contains the local request inspection data model and storage.
package inspector

import "time"

// HeaderValues stores HTTP header values without exposing a mutable http.Header
// map through the inspector API.
type HeaderValues map[string][]string

// CapturedRequest is the request portion of an inspected HTTP transaction.
type CapturedRequest struct {
	Method    string       `json:"method"`
	URL       string       `json:"url"`
	Path      string       `json:"path"`
	Headers   HeaderValues `json:"headers,omitempty"`
	Body      []byte       `json:"body,omitempty"`
	Timestamp time.Time    `json:"timestamp"`
	ClientIP  string       `json:"client_ip,omitempty"`
}

// CapturedResponse is the response portion of an inspected HTTP transaction.
type CapturedResponse struct {
	StatusCode int          `json:"status_code"`
	Status     string       `json:"status"`
	Headers    HeaderValues `json:"headers,omitempty"`
	Body       []byte       `json:"body,omitempty"`
	Timestamp  time.Time    `json:"timestamp"`
}

// CapturedTransaction records one complete request/response exchange.
type CapturedTransaction struct {
	ID          string            `json:"id"`
	Request     CapturedRequest   `json:"request"`
	Response    *CapturedResponse `json:"response,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
	Duration    time.Duration     `json:"duration"`
	ClientIP    string            `json:"client_ip,omitempty"`
}
