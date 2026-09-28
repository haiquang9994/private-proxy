// Package filelock provides an exclusive, advisory, cross-process lock
// backed by a lock file. The OS releases the lock when the holding process
// exits, even if it crashes, so a lock can never be left stale.
package filelock

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// retryInterval is how often Acquire retries while another process holds the lock.
const retryInterval = 100 * time.Millisecond

// errBusy is returned by the platform tryLock when another handle holds the lock.
var errBusy = errors.New("lock is held by another process")

// Lock is a held lock; call Release to let other processes acquire it.
type Lock struct {
	f *os.File
}

// Acquire takes an exclusive lock on path (creating the file if needed),
// retrying until timeout if another process holds it.
func Acquire(path string, timeout time.Duration) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		err := tryLock(f)
		if err == nil {
			return &Lock{f: f}, nil
		}
		if !errors.Is(err, errBusy) {
			f.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("lock %s: %w (waited %s)", path, errBusy, timeout)
		}
		time.Sleep(retryInterval)
	}
}

// Release unlocks and closes the lock file. The file itself is left in
// place: deleting it would race with another process opening it.
func (l *Lock) Release() error {
	unlockErr := unlock(l.f)
	closeErr := l.f.Close()
	return errors.Join(unlockErr, closeErr)
}
