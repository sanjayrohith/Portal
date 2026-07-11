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

var (
	// inspectorBodyPool recycles payload byte slices for captured transactions
	// to avoid GC pressure during sustained high-throughput webhook bursts.
	inspectorBodyPool = sync.Pool{
		New: func() any {
			buf := make([]byte, 0, 64*1024)
			return &buf
		},
	}
)

func acquireBodyBuffer(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	if cap(src) <= 64*1024 {
		if ptr, ok := inspectorBodyPool.Get().(*[]byte); ok && ptr != nil {
			buf := (*ptr)[:0]
			buf = append(buf, src...)
			return buf
		}
	}
	return append([]byte(nil), src...)
}

func releaseBodyBuffer(buf []byte) {
	if cap(buf) == 64*1024 {
		buf = buf[:0]
		inspectorBodyPool.Put(&buf)
	}
}

func releaseTransactionBuffers(tx *CapturedTransaction) {
	if tx == nil {
		return
	}
	if len(tx.Request.Body) > 0 {
		releaseBodyBuffer(tx.Request.Body)
		tx.Request.Body = nil
	}
	if tx.Response != nil && len(tx.Response.Body) > 0 {
		releaseBodyBuffer(tx.Response.Body)
		tx.Response.Body = nil
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

	// Recycle buffers of evicted transaction if slot is being overwritten
	if old := b.entries[b.next]; old != nil {
		releaseTransactionBuffers(old)
	}

	b.entries[b.next] = cloneTransactionInternal(transaction)
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
			return cloneTransactionExport(entry), true
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
			transactions = append(transactions, cloneTransactionExport(entry))
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

	for i, entry := range b.entries {
		if entry != nil {
			releaseTransactionBuffers(entry)
			b.entries[i] = nil
		}
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

// cloneTransactionInternal creates an internally retained copy using pooled body slices.
func cloneTransactionInternal(transaction *CapturedTransaction) *CapturedTransaction {
	clone := *transaction
	clone.Request.Headers = cloneHeaders(transaction.Request.Headers)
	clone.Request.Body = acquireBodyBuffer(transaction.Request.Body)
	if transaction.Response != nil {
		response := *transaction.Response
		response.Headers = cloneHeaders(transaction.Response.Headers)
		response.Body = acquireBodyBuffer(transaction.Response.Body)
		clone.Response = &response
	}
	return &clone
}

// cloneTransaction creates an unpooled defensive copy for external callers.
func cloneTransaction(transaction *CapturedTransaction) *CapturedTransaction {
	return cloneTransactionExport(transaction)
}

// cloneTransactionExport creates an unpooled defensive copy for external callers.
func cloneTransactionExport(transaction *CapturedTransaction) *CapturedTransaction {
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
