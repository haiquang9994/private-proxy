package filelock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	first, err := Acquire(context.Background(), path, time.Second)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	// A second handle on the same file conflicts even within one process,
	// both for flock (per open file description) and LockFileEx (per handle).
	start := time.Now()
	_, err = Acquire(context.Background(), path, 300*time.Millisecond)
	if !errors.Is(err, errBusy) {
		t.Fatalf("expected busy error, got %v", err)
	}
	if waited := time.Since(start); waited < 300*time.Millisecond {
		t.Fatalf("expected Acquire to retry until timeout, returned after %s", waited)
	}
}

func TestAcquireAfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	first, err := Acquire(context.Background(), path, time.Second)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	second, err := Acquire(context.Background(), path, time.Second)
	if err != nil {
		t.Fatalf("second Acquire after Release: %v", err)
	}
	second.Release()
}

func TestAcquireWaitsForRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	first, err := Acquire(context.Background(), path, time.Second)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		first.Release()
	}()

	second, err := Acquire(context.Background(), path, 5*time.Second)
	if err != nil {
		t.Fatalf("expected second Acquire to succeed once released: %v", err)
	}
	second.Release()
}

func TestAcquireStopsWhenContextCanceled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	first, err := Acquire(context.Background(), path, time.Second)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	_, err = Acquire(ctx, path, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error, got %v", err)
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("expected Acquire to stop soon after cancel, took %s", waited)
	}
}
