package inspector

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// RequestSummary is the compact representation used by the transaction list
// endpoint. Full request and response bodies are available from the detail
// endpoint.
type RequestSummary struct {
	ID         string        `json:"id"`
	Method     string        `json:"method"`
	Path       string        `json:"path"`
	StatusCode int           `json:"status_code,omitempty"`
	StartedAt  time.Time     `json:"started_at"`
	Duration   time.Duration `json:"duration"`
	ClientIP   string        `json:"client_ip,omitempty"`
}

func newAPIHandler(buffer *RingBuffer, replayTarget string, events *eventHub) http.Handler {
	static := staticHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/static/") {
			static.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/events" {
			handleEvents(w, r, events)
			return
		}
		const prefix = "/api/requests"
		if r.URL.Path == prefix || r.URL.Path == prefix+"/" {
			if r.Method != http.MethodGet {
				methodNotAllowed(w, http.MethodGet)
				return
			}
			handleRequestList(w, buffer)
			return
		}

		if strings.HasPrefix(r.URL.Path, prefix+"/") {
			id := strings.TrimPrefix(r.URL.Path, prefix+"/")
			if strings.HasSuffix(id, "/replay") {
				if r.Method != http.MethodPost {
					methodNotAllowed(w, http.MethodPost)
					return
				}
				id = strings.TrimSuffix(id, "/replay")
				if id == "" || strings.Contains(id, "/") {
					writeJSONError(w, http.StatusNotFound, "request not found")
					return
				}
				handleRequestReplay(w, buffer, id, replayTarget)
				return
			}

			if r.Method != http.MethodGet {
				methodNotAllowed(w, http.MethodGet)
				return
			}
			if id == "" || strings.Contains(id, "/") {
				writeJSONError(w, http.StatusNotFound, "request not found")
				return
			}
			handleRequestDetail(w, buffer, id)
			return
		}

		http.NotFound(w, r)
	})
}

func handleRequestReplay(w http.ResponseWriter, buffer *RingBuffer, id, target string) {
	transaction, ok := buffer.Get(id)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "request not found")
		return
	}
	result, err := replayTransaction(transaction, target)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleRequestList(w http.ResponseWriter, buffer *RingBuffer) {
	transactions := buffer.List()
	summaries := make([]RequestSummary, 0, len(transactions))
	for _, transaction := range transactions {
		summary := RequestSummary{
			ID:        transaction.ID,
			Method:    transaction.Request.Method,
			Path:      transaction.Request.Path,
			StartedAt: transaction.StartedAt,
			Duration:  transaction.Duration,
			ClientIP:  transaction.ClientIP,
		}
		if transaction.Response != nil {
			summary.StatusCode = transaction.Response.StatusCode
		}
		summaries = append(summaries, summary)
	}
	writeJSON(w, http.StatusOK, summaries)
}

func handleRequestDetail(w http.ResponseWriter, buffer *RingBuffer, id string) {
	transaction, ok := buffer.Get(id)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "request not found")
		return
	}
	writeJSON(w, http.StatusOK, transaction)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}
