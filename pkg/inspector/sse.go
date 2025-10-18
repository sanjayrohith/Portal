package inspector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type eventHub struct {
	mu          sync.RWMutex
	subscribers map[chan *CapturedTransaction]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{subscribers: make(map[chan *CapturedTransaction]struct{})}
}

func (h *eventHub) Subscribe() (<-chan *CapturedTransaction, func()) {
	channel := make(chan *CapturedTransaction, 16)
	h.mu.Lock()
	h.subscribers[channel] = struct{}{}
	h.mu.Unlock()
	return channel, func() {
		h.mu.Lock()
		if _, ok := h.subscribers[channel]; ok {
			delete(h.subscribers, channel)
			close(channel)
		}
		h.mu.Unlock()
	}
}

func (h *eventHub) Publish(transaction *CapturedTransaction) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range h.subscribers {
		select {
		case subscriber <- cloneTransaction(transaction):
		default:
		}
	}
}

func handleEvents(w http.ResponseWriter, r *http.Request, events *eventHub) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	transactions, unsubscribe := events.Subscribe()
	defer unsubscribe()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case transaction := <-transactions:
			if transaction == nil {
				return
			}
			payload, err := json.Marshal(transaction)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: request\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
