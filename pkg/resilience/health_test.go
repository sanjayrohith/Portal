package resilience

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/mux"
)

func TestHealthWatchdog_HealthySession(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	clientSession := mux.NewSession(c1, false)
	serverSession := mux.NewSession(c2, true)
	defer clientSession.Close()
	defer serverSession.Close()

	watchdog := NewHealthWatchdog(clientSession, 20*time.Millisecond, 50*time.Millisecond, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchdog.Start(ctx)
	time.Sleep(60 * time.Millisecond)

	if watchdog.Status() != StatusHealthy {
		t.Errorf("expected StatusHealthy, got %s", watchdog.Status())
	}

	watchdog.Stop()
}

func TestHealthWatchdog_InterfaceMigrationDetection(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	clientSession := mux.NewSession(c1, false)
	defer clientSession.Close()

	watchdog := NewHealthWatchdog(clientSession, 20*time.Millisecond, 50*time.Millisecond, 1)
	// Simulate bound to an old Wi-Fi IP that no longer exists on any interface
	watchdog.boundLocalIP = "198.51.100.254"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchdog.Start(ctx)

	select {
	case <-watchdog.DisruptionChan():
		if watchdog.Status() != StatusDead {
			t.Errorf("expected StatusDead after interface migration, got %s", watchdog.Status())
		}
		if !clientSession.IsClosed() {
			t.Errorf("expected clientSession to be closed on interface migration")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for interface migration disruption")
	}

	watchdog.Stop()
}

func TestHealthWatchdog_DisruptionDetectionOnDrop(t *testing.T) {
	c1, c2 := net.Pipe()

	clientSession := mux.NewSession(c1, false)
	serverSession := mux.NewSession(c2, true)

	watchdog := NewHealthWatchdog(clientSession, 20*time.Millisecond, 30*time.Millisecond, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchdog.Start(ctx)

	// Sever connection abruptly
	_ = c2.Close()
	_ = serverSession.Close()

	select {
	case <-watchdog.DisruptionChan():
		// Detected disruption successfully
		if watchdog.Status() == StatusHealthy {
			t.Errorf("expected non-healthy status after disruption, got %s", watchdog.Status())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for watchdog disruption notification")
	}

	watchdog.Stop()
	_ = clientSession.Close()
}
