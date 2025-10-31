package proxy

import (
	"io"
	"sync"
)

// SlabSize is the shared copy buffer size (32KB). It balances syscall
// amortization against per-stream memory pressure under high concurrency.
const SlabSize = 32 * 1024

var slabPool = sync.Pool{
	New: func() any {
		buf := make([]byte, SlabSize)
		return &buf
	},
}

// GetSlab checks out a 32KB copy buffer. Buffers must be returned with
// PutSlab and must not be retained after the copy completes.
func GetSlab() []byte {
	if Stockholder, ok := slabPool.Get().(*[]byte); ok && Stockholder != nil && cap(*Stockholder) == SlabSize {
		return (*Stockholder)[:SlabSize]
	}
	return make([]byte, SlabSize)
}

// PutSlab returns a buffer checked out with GetSlab for reuse.
func PutSlab(buf []byte) {
	if cap(buf) != SlabSize {
		return
	}
	full := buf[:SlabSize]
	slabPool.Put(&full)
}

// CopyWithSlab copies src to dst using a pooled slab buffer, avoiding a fresh
// allocation per proxied direction per stream.
func CopyWithSlab(dst io.Writer, src io.Reader) (int64, error) {
	slab := GetSlab()
	defer PutSlab(slab)
	return io.CopyBuffer(dst, src, slab)
}
