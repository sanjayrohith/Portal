package inspector

import (
	"fmt"
	"sync"
	"testing"
)

func TestRingBufferFIFOEviction(t *testing.T) {
	buffer := NewRingBuffer(3)
	for i := 1; i <= 4; i++ {
		buffer.Add(&CapturedTransaction{ID: fmt.Sprintf("tx-%d", i)})
	}

	if got, want := buffer.Len(), 3; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	transactions := buffer.List()
	if got, want := len(transactions), 3; got != want {
		t.Fatalf("List() returned %d transactions, want %d", got, want)
	}
	for i, wantID := range []string{"tx-2", "tx-3", "tx-4"} {
		if transactions[i].ID != wantID {
			t.Errorf("List()[%d].ID = %q, want %q", i, transactions[i].ID, wantID)
		}
	}

	if _, ok := buffer.Get("tx-1"); ok {
		t.Error("evicted transaction was still found")
	}
	if transaction, ok := buffer.Get("tx-3"); !ok || transaction.ID != "tx-3" {
		t.Errorf("Get(tx-3) = %#v, %v; want tx-3, true", transaction, ok)
	}
}

func TestRingBufferDefaultsAndClear(t *testing.T) {
	if got := NewRingBuffer(0).Capacity(); got != DefaultRingBufferCapacity {
		t.Fatalf("zero capacity = %d, want default %d", got, DefaultRingBufferCapacity)
	}
	if got := NewRingBuffer(-1).Capacity(); got != DefaultRingBufferCapacity {
		t.Fatalf("negative capacity = %d, want default %d", got, DefaultRingBufferCapacity)
	}

	buffer := NewRingBuffer(2)
	buffer.Add(&CapturedTransaction{ID: "tx-1"})
	buffer.Clear()
	if got := buffer.Len(); got != 0 {
		t.Fatalf("Len() after Clear() = %d, want 0", got)
	}
	if got := buffer.List(); len(got) != 0 {
		t.Fatalf("List() after Clear() = %#v, want empty", got)
	}
}

func TestRingBufferCopiesTransactions(t *testing.T) {
	transaction := &CapturedTransaction{
		ID: "tx-1",
		Request: CapturedRequest{
			Headers: HeaderValues{"X-Test": {"before"}},
			Body:    []byte("request"),
		},
		Response: &CapturedResponse{Body: []byte("response")},
	}
	buffer := NewRingBuffer(1)
	buffer.Add(transaction)

	transaction.Request.Headers["X-Test"][0] = "changed"
	transaction.Request.Body[0] = 'X'
	transaction.Response.Body[0] = 'X'

	first, ok := buffer.Get("tx-1")
	if !ok {
		t.Fatal("stored transaction was not found")
	}
	if got := first.Request.Headers["X-Test"][0]; got != "before" {
		t.Errorf("stored header = %q, want before", got)
	}
	if got := string(first.Request.Body); got != "request" {
		t.Errorf("stored request body = %q, want request", got)
	}
	if got := string(first.Response.Body); got != "response" {
		t.Errorf("stored response body = %q, want response", got)
	}

	first.Request.Headers["X-Test"][0] = "mutated snapshot"
	second, _ := buffer.Get("tx-1")
	if got := second.Request.Headers["X-Test"][0]; got != "before" {
		t.Errorf("stored header changed through returned snapshot: %q", got)
	}
}

func TestRingBufferConcurrentAccess(t *testing.T) {
	const (
		capacity  = 64
		writers   = 16
		perWriter = 250
	)

	buffer := NewRingBuffer(capacity)
	var wg sync.WaitGroup
	for writer := 0; writer < writers; writer++ {
		writer := writer
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sequence := 0; sequence < perWriter; sequence++ {
				buffer.Add(&CapturedTransaction{ID: fmt.Sprintf("writer-%d-%d", writer, sequence)})
				_ = buffer.Len()
				_, _ = buffer.Get("missing")
				_ = buffer.List()
			}
		}()
	}
	wg.Wait()

	if got := buffer.Len(); got != capacity {
		t.Fatalf("Len() after concurrent writes = %d, want capacity %d", got, capacity)
	}
	if got := len(buffer.List()); got != capacity {
		t.Fatalf("List() after concurrent writes returned %d, want %d", got, capacity)
	}
}

func TestRingBufferMemoryRecycling(t *testing.T) {
	buf := NewRingBuffer(5)
	payload := make([]byte, 1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	// Add 50 transactions to trigger recycling across slots
	for i := 0; i < 50; i++ {
		buf.Add(&CapturedTransaction{
			ID: fmt.Sprintf("tx-%d", i),
			Request: CapturedRequest{
				Body: payload,
			},
			Response: &CapturedResponse{
				Body: payload,
			},
		})
	}

	if buf.Len() != 5 {
		t.Fatalf("expected len 5, got %d", buf.Len())
	}

	list := buf.List()
	if len(list) != 5 {
		t.Fatalf("expected 5 transactions, got %d", len(list))
	}
	for _, tx := range list {
		if len(tx.Request.Body) != 1024 || len(tx.Response.Body) != 1024 {
			t.Fatalf("corrupted body in recycled transaction: %d, %d", len(tx.Request.Body), len(tx.Response.Body))
		}
	}

	buf.Clear()
	if buf.Len() != 0 {
		t.Fatalf("expected 0 after Clear, got %d", buf.Len())
	}
}

func BenchmarkRingBufferSustainedWebhookBursts(b *testing.B) {
	ring := NewRingBuffer(200)
	body := make([]byte, 8*1024) // 8KB webhook JSON payload

	tx := &CapturedTransaction{
		ID: "bench-tx",
		Request: CapturedRequest{
			Headers: HeaderValues{"Content-Type": {"application/json"}},
			Body:    body,
		},
		Response: &CapturedResponse{
			Headers: HeaderValues{"Content-Type": {"application/json"}},
			Body:    body,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ring.Add(tx)
	}
}
