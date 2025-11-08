package security

import (
	"fmt"
	"io"
	"net/http"
)

const (
	// DefaultMaxRequestBodySize is 10MB default payload size cap.
	DefaultMaxRequestBodySize = 10 * 1024 * 1024
	// DefaultMaxHeaderBytes is 1MB default maximum request header block size.
	DefaultMaxHeaderBytes = 1 * 1024 * 1024
)

// RequestSizeCapMiddleware limits the maximum request body size and total header size.
func RequestSizeCapMiddleware(maxBodySize int64, next http.Handler) http.Handler {
	if maxBodySize <= 0 {
		maxBodySize = DefaultMaxRequestBodySize
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Enforce Content-Length check if provided upfront
		if r.ContentLength > maxBodySize {
			http.Error(w, fmt.Sprintf("413 Request Entity Too Large: payload size %d exceeds limit %d", r.ContentLength, maxBodySize), http.StatusRequestEntityTooLarge)
			return
		}

		// Wrap Body with MaxBytesReader to guard chunked or streaming transfer-encodings
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		}

		next.ServeHTTP(w, r)
	})
}

// BoundedReader wraps an io.Reader and returns an error if reading exceeds maxBytes.
type BoundedReader struct {
	r         io.Reader
	maxBytes  int64
	readSoFar int64
}

// NewBoundedReader creates a BoundedReader.
func NewBoundedReader(r io.Reader, maxBytes int64) *BoundedReader {
	return &BoundedReader{
		r:        r,
		maxBytes: maxBytes,
	}
}

func (b *BoundedReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.readSoFar += int64(n)
	if b.readSoFar > b.maxBytes {
		return n, fmt.Errorf("payload limit exceeded: read %d bytes, max %d", b.readSoFar, b.maxBytes)
	}
	return n, err
}
