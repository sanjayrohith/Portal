package resilience

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestShutdownCoordinatorOrder(t *testing.T) {
	var calls []string
	coordinator := NewShutdownCoordinator(func(context.Context) error { calls = append(calls, "teardown"); return nil }, func(context.Context) error { calls = append(calls, "release"); return nil }, time.Second)
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	if err := coordinator.Wait(context.Background(), signals); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"teardown", "release"}) {
		t.Fatalf("calls = %v, want teardown then release", calls)
	}
}

func TestShutdownCoordinatorJoinsErrors(t *testing.T) {
	teardownErr := errors.New("teardown failed")
	releaseErr := errors.New("release failed")
	coordinator := NewShutdownCoordinator(func(context.Context) error { return teardownErr }, func(context.Context) error { return releaseErr }, time.Second)
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	err := coordinator.Wait(context.Background(), signals)
	if !errors.Is(err, teardownErr) || !errors.Is(err, releaseErr) {
		t.Fatalf("error = %v, want both causes", err)
	}
}
