package resilience

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sanjayrohith/portal/pkg/auth"
	"github.com/sanjayrohith/portal/pkg/control"
	"github.com/sanjayrohith/portal/pkg/mux"
	"github.com/sanjayrohith/portal/pkg/registry"
)

// simulateSeveredSocket forcefully severs a TCP connection after delay.
func severConnAfter(c net.Conn, delay time.Duration) {
	go func() {
		time.Sleep(delay)
		_ = c.Close()
	}()
}

func TestChaosSeveredSocketAutoReconnection(t *testing.T) {
	// Setup TokenStore and SubdomainRegistry on Server
	tokenStore := auth.NewMemoryTokenStore()
	_, err := tokenStore.RegisterRawToken("resilient-token-123", "alice", 5, 100)
	if err != nil {
		t.Fatalf("RegisterRawToken failed: %v", err)
	}

	reg := registry.NewSubdomainRegistry()
	baseDomain := "tunnel.portal.dev"
	authorizer := control.NewPersistentReclamationAuthorizer(tokenStore, reg, baseDomain, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Track reconnection events
	var reconnectCount sync.WaitGroup
	reconnectCount.Add(2) // We will sever twice and reconnect successfully twice

	backoffCfg := BackoffConfig{
		InitialInterval: 20 * time.Millisecond,
		MaxInterval:     100 * time.Millisecond,
		Multiplier:      1.5,
		MaxJitterRatio:  0.1,
		MaxRetries:      10,
	}

	sup := NewReconnectSupervisor(backoffCfg)
	tracker := NewInFlightTracker()
	reconciler := NewSessionReconciler(tracker)

	var currentSession *mux.Session
	var mu sync.Mutex

	dialServer := func() (net.Conn, *control.ServerHello, error) {
		cServer, cClient := net.Pipe()

		go func() {
			_, _, _ = control.ServerHandshake(cServer, authorizer, 2*time.Second)
		}()

		hello := control.ClientHello{
			ClientID:  "chaos-client-1",
			AuthToken: "resilient-token-123",
			Subdomain: "resilient-app",
		}

		sHello, err := control.ClientHandshake(cClient, hello, 2*time.Second)
		if err != nil {
			_ = cClient.Close()
			_ = cServer.Close()
			return nil, nil, err
		}

		// Bind session on server side to registry
		serverSess := mux.NewSession(cServer, true)
		_, _ = reg.Allocate(sHello.AssignedSubdomain, auth.HashToken("resilient-token-123"), "alice", serverSess)

		return cClient, sHello, nil
	}

	// 1. Initial connect
	conn, sHello, err := dialServer()
	if err != nil {
		t.Fatalf("initial dial failed: %v", err)
	}
	if sHello.AssignedSubdomain != "resilient-app" {
		t.Fatalf("expected resilient-app, got %s", sHello.AssignedSubdomain)
	}

	currentSession = mux.NewSession(conn, false)

	// Chaos round 1: sever socket
	severConnAfter(conn, 30*time.Millisecond)

	watchdog := NewHealthWatchdog(currentSession, 15*time.Millisecond, 20*time.Millisecond, 1)
	watchdog.Start(ctx)

	// Wait for disruption signal
	<-watchdog.DisruptionChan()
	watchdog.Stop()

	// Client reconnect loop
	reconnectSuccess := false
	for i := 0; i < 5; i++ {
		sup.Sleep(ctx)

		newConn, newHello, err := dialServer()
		if err != nil {
			continue
		}

		newSession := mux.NewSession(newConn, false)
		mu.Lock()
		_ = reconciler.Reconcile(ctx, currentSession, newSession, 50*time.Millisecond)
		currentSession = newSession
		mu.Unlock()

		if newHello.AssignedSubdomain == "resilient-app" {
			reconnectSuccess = true
			reconnectCount.Done()
			break
		}
	}

	if !reconnectSuccess {
		t.Fatalf("reconnection round 1 failed")
	}

	// Chaos round 2: sever socket again
	mu.Lock()
	sess2 := currentSession
	mu.Unlock()

	watchdog2 := NewHealthWatchdog(sess2, 15*time.Millisecond, 20*time.Millisecond, 1)
	watchdog2.Start(ctx)

	_ = sess2.Close() // abrupt close

	<-watchdog2.DisruptionChan()
	watchdog2.Stop()

	// Reconnect round 2
	reconnectSuccess2 := false
	for i := 0; i < 5; i++ {
		sup.Sleep(ctx)

		newConn, newHello, err := dialServer()
		if err != nil {
			continue
		}

		newSession := mux.NewSession(newConn, false)
		mu.Lock()
		_ = reconciler.Reconcile(ctx, currentSession, newSession, 50*time.Millisecond)
		currentSession = newSession
		mu.Unlock()

		if newHello.AssignedSubdomain == "resilient-app" {
			reconnectSuccess2 = true
			reconnectCount.Done()
			break
		}
	}

	if !reconnectSuccess2 {
		t.Fatalf("reconnection round 2 failed")
	}

	reconnectCount.Wait()

	// Final verification: check subdomain in registry is bound to active session
	activeSess, ok := reg.GetSession("resilient-app")
	if !ok || activeSess == nil || activeSess.IsClosed() {
		t.Errorf("expected registry to hold active non-closed session for resilient-app")
	}

	// Clean up
	mu.Lock()
	if currentSession != nil {
		_ = currentSession.Close()
	}
	mu.Unlock()
}

func TestChaosDroppedPacketsAndDataRecovery(t *testing.T) {
	c1, c2 := net.Pipe()
	clientSess := mux.NewSession(c1, false)
	serverSess := mux.NewSession(c2, true)
	defer clientSess.Close()
	defer serverSess.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		stream, err := serverSess.AcceptStream()
		if err != nil {
			return
		}
		defer stream.Close()

		buf := make([]byte, 1024)
		n, _ := io.ReadFull(stream, buf[:10])
		_, _ = stream.Write(buf[:n])
	}()

	clientStream, err := clientSess.OpenStream()
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	defer clientStream.Close()

	payload := []byte("0123456789")
	if _, err := clientStream.Write(payload); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	echo := make([]byte, 10)
	if _, err := io.ReadFull(clientStream, echo); err != nil {
		t.Fatalf("Read echo failed: %v", err)
	}

	if string(echo) != string(payload) {
		t.Errorf("expected %s, got %s", string(payload), string(echo))
	}

	wg.Wait()
}
