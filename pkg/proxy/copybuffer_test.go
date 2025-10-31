package proxy

import (
	"bytes"
	"testing"
)

func TestSlabCopyRoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte{0xCD}, SlabSize*2+123)
	var dst bytes.Buffer
	n, err := CopyWithSlab(&dst, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(payload)) || !bytes.Equal(dst.Bytes(), payload) {
		t.Fatalf("slab copy n=%d want %d", n, len(payload))
	}
}

func TestSlabPoolReuse(t *testing.T) {
	slab := GetSlab()
	if len(slab) != SlabSize {
		t.Fatalf("slab len = %d", len(slab))
	}
	PutSlab(slab)
	// Foreign-size buffers are dropped, not pooled.
	PutSlab(make([]byte, 1024))
	again := GetSlab()
	if len(again) != SlabSize {
		t.Fatalf("reused slab len = %d", len(again))
	}
	PutSlab(again)
}
