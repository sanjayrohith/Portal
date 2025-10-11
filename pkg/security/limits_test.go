package security

import (
	"errors"
	"testing"

	portalErr "github.com/sanjayrohith/portal/pkg/errors"
)

func TestQuotaManager_Tunnels(t *testing.T) {
	qm := NewQuotaManager(2)

	// Acquire 1
	if err := qm.AcquireTunnel("tok-1", 2); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if qm.ActiveTunnels("tok-1") != 1 {
		t.Errorf("expected 1, got %d", qm.ActiveTunnels("tok-1"))
	}

	// Acquire 2
	if err := qm.AcquireTunnel("tok-1", 2); err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}

	// Acquire 3 -> reject
	err := qm.AcquireTunnel("tok-1", 2)
	if err == nil {
		t.Fatalf("expected quota exceeded error")
	}
	if !errors.Is(err, portalErr.ErrQuotaExceeded) {
		t.Errorf("expected ErrQuotaExceeded, got %v", err)
	}

	// Release 1
	qm.ReleaseTunnel("tok-1")
	if qm.ActiveTunnels("tok-1") != 1 {
		t.Errorf("expected 1 active after release, got %d", qm.ActiveTunnels("tok-1"))
	}

	// Can now acquire again
	if err := qm.AcquireTunnel("tok-1", 2); err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
}

func TestQuotaManager_Streams(t *testing.T) {
	qm := NewQuotaManager(5)

	if err := qm.AcquireStream("tok-1", 1); err != nil {
		t.Fatalf("acquire stream failed: %v", err)
	}

	if err := qm.AcquireStream("tok-1", 1); err != ErrMaxStreamsExceeded {
		t.Fatalf("expected ErrMaxStreamsExceeded, got %v", err)
	}

	qm.ReleaseStream("tok-1")
	if qm.ActiveStreams("tok-1") != 0 {
		t.Errorf("expected 0 active streams after release")
	}
}
