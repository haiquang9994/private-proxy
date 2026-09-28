package main

import (
	"errors"
	"os"
	"runtime"
)

// requireRoot fails unless proxyctl runs as root.
func requireRoot() error {
	return checkRoot(runtime.GOOS, os.Geteuid())
}

// checkRoot is requireRoot's logic with the platform inputs passed in, so
// tests can cover every case. Windows has no euid (Geteuid returns -1) and
// is only used for development, so the check is skipped there.
func checkRoot(goos string, euid int) error {
	if goos == "windows" || euid == 0 {
		return nil
	}
	return errors.New("proxyctl must be run as root (try sudo)")
}
