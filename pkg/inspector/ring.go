package inspector

import "sync"

const (
	// DefaultRingBufferCapacity is the number of transactions retained when no
	// explicit capacity is configured.
	DefaultRingBufferCapacity = 200
)

// RingBuffer keeps the most recent captured transactions in memory. It never
// writes captured request data to disk.
type RingBuffer struct {
	mu       sync.RWMutex
	entries  []*CapturedTransaction
	next     int
	size     int
	capacity int
}

// NewRingBuffer creates a fixed-capacity transaction buffer. Non-positive
// capacities use DefaultRingBufferCapacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultRingBufferCapacity
	}

	return &RingBuffer{
		entries:  make([]*CapturedTransaction, capacity),
		capacity: capacity,
	}
}

// Add stores a transaction, evicting the oldest transaction when the buffer
// is full. A deep copy is retained so callers can safely reuse their value and
// its byte slices after this method returns.
func (b *RingBuffer) Add(transaction *CapturedTransaction) {
	if transaction == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries[b.next] = cloneTransaction(transaction)
	b.next = (b.next + 1) % b.capacity
	if b.size < b.capacity {
		b.size++
	}
}

// Get returns a defensive copy of the transaction with the requested ID.
func (b *RingBuffer) Get(id string) (*CapturedTransaction, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for i := 0; i < b.size; i++ {
		entry := b.entries[b.oldestIndex(i)]
		if entry != nil && entry.ID == id {
			return cloneTransaction(entry), true
		}
	}
	return nil, false
}

// List returns transactions from oldest to newest, each as a defensive copy.
func (b *RingBuffer) List() []*CapturedTransaction {
	b.mu.RLock()
	defer b.mu.RUnlock()

	transactions := make([]*CapturedTransaction, 0, b.size)
	for i := 0; i < b.size; i++ {
		if entry := b.entries[b.oldestIndex(i)]; entry != nil {
			transactions = append(transactions, cloneTransaction(entry))
		}
	}
	return transactions
}

// Len returns the number of transactions currently retained.
func (b *RingBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.size
}

// Capacity returns the maximum number of transactions retained.
func (b *RingBuffer) Capacity() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.capacity
}

// Clear removes all retained transactions and releases their references.
func (b *RingBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.entries {
		b.entries[i] = nil
	}
	b.next = 0
	b.size = 0
}

func (b *RingBuffer) oldestIndex(offset int) int {
	start := b.next - b.size
	if start < 0 {
		start += b.capacity
	}
	return (start + offset) % b.capacity
}

func cloneTransaction(transaction *CapturedTransaction) *CapturedTransaction {
	clone := *transaction
	clone.Request.Headers = cloneHeaders(transaction.Request.Headers)
	clone.Request.Body = append([]byte(nil), transaction.Request.Body...)
	if transaction.Response != nil {
		response := *transaction.Response
		response.Headers = cloneHeaders(transaction.Response.Headers)
		response.Body = append([]byte(nil), transaction.Response.Body...)
		clone.Response = &response
	}
	return &clone
}

func cloneHeaders(headers HeaderValues) HeaderValues {
	if headers == nil {
		return nil
	}

	clone := make(HeaderValues, len(headers))
	for key, values := range headers {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}
