package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubCaddy replaces runCaddy for the duration of the test and records
// the caddy subcommands it was asked to run.
func stubCaddy(t *testing.T, err error) *[]string {
	t.Helper()
	var calls []string
	orig := runCaddy
	runCaddy = func(ctx context.Context, root string, args ...string) ([]byte, error) {
		calls = append(calls, args[0])
		return []byte("stub output"), err
	}
	t.Cleanup(func() { runCaddy = orig })
	return &calls
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReloadPromotesAndBacksUp(t *testing.T) {
	root := t.TempDir()
	calls := stubCaddy(t, nil)
	writeFile(t, filepath.Join(root, CaddyfileName), "old")
	writeFile(t, filepath.Join(root, CaddyfileNewName), "new")

	if err := Reload(context.Background(), root); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := readFile(t, filepath.Join(root, CaddyfileName)); got != "new" {
		t.Fatalf("expected promoted Caddyfile %q, got %q", "new", got)
	}
	if got := readFile(t, filepath.Join(root, CaddyfileBakName)); got != "old" {
		t.Fatalf("expected backup %q, got %q", "old", got)
	}
	if _, err := os.Stat(filepath.Join(root, CaddyfileNewName)); !os.IsNotExist(err) {
		t.Fatalf("expected Caddyfile.new to be consumed, stat err: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0] != "reload" {
		t.Fatalf("expected one reload call, got %v", *calls)
	}
}

func TestReloadFailureRestoresPrevious(t *testing.T) {
	root := t.TempDir()
	stubCaddy(t, errors.New("exit status 1"))
	writeFile(t, filepath.Join(root, CaddyfileName), "old")
	writeFile(t, filepath.Join(root, CaddyfileNewName), "new")

	err := Reload(context.Background(), root)
	if err == nil {
		t.Fatal("expected reload error")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback message, got: %v", err)
	}
	if got := readFile(t, filepath.Join(root, CaddyfileName)); got != "old" {
		t.Fatalf("expected Caddyfile restored to %q, got %q", "old", got)
	}
}

func TestReloadFailureWithoutPreviousRemovesCaddyfile(t *testing.T) {
	root := t.TempDir()
	stubCaddy(t, errors.New("exit status 1"))
	writeFile(t, filepath.Join(root, CaddyfileNewName), "new")

	if err := Reload(context.Background(), root); err == nil {
		t.Fatal("expected reload error")
	}
	if _, err := os.Stat(filepath.Join(root, CaddyfileName)); !os.IsNotExist(err) {
		t.Fatalf("expected Caddyfile to be removed, stat err: %v", err)
	}
}

func TestValidateReportsCaddyOutput(t *testing.T) {
	root := t.TempDir()
	calls := stubCaddy(t, errors.New("exit status 1"))

	err := Validate(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "stub output") {
		t.Fatalf("expected validate error with caddy output, got: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0] != "validate" {
		t.Fatalf("expected one validate call, got %v", *calls)
	}
}

// stubHangingCaddy makes runCaddy block until its context is done, like a
// Caddy stuck mid-reload, and shortens caddyTimeout to timeout.
func stubHangingCaddy(t *testing.T, timeout time.Duration) {
	t.Helper()
	origRun, origTimeout := runCaddy, caddyTimeout
	runCaddy = func(ctx context.Context, root string, args ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, errors.New("signal: killed")
	}
	caddyTimeout = timeout
	t.Cleanup(func() { runCaddy, caddyTimeout = origRun, origTimeout })
}

func TestReloadTimeoutRestoresPrevious(t *testing.T) {
	root := t.TempDir()
	stubHangingCaddy(t, 100*time.Millisecond)
	writeFile(t, filepath.Join(root, CaddyfileName), "old")
	writeFile(t, filepath.Join(root, CaddyfileNewName), "new")

	err := Reload(context.Background(), root)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got: %v", err)
	}
	for _, want := range []string{"did not finish within", "rolled back", "docker compose restart caddy"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to mention %q, got: %v", want, err)
		}
	}
	if got := readFile(t, filepath.Join(root, CaddyfileName)); got != "old" {
		t.Fatalf("expected Caddyfile restored to %q, got %q", "old", got)
	}
}

func TestReloadInterruptRestoresPrevious(t *testing.T) {
	root := t.TempDir()
	stubHangingCaddy(t, time.Minute)
	writeFile(t, filepath.Join(root, CaddyfileName), "old")
	writeFile(t, filepath.Join(root, CaddyfileNewName), "new")

	// Simulate Ctrl+C arriving while the reload hangs.
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	err := Reload(ctx, root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got: %v", err)
	}
	if !strings.Contains(err.Error(), "interrupted") || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected interrupt + rollback message, got: %v", err)
	}
	if got := readFile(t, filepath.Join(root, CaddyfileName)); got != "old" {
		t.Fatalf("expected Caddyfile restored to %q, got %q", "old", got)
	}
}

// TestHelperHang is not a real test: runCommand tests re-exec the test
// binary with this test selected to get a child process that never exits.
func TestHelperHang(t *testing.T) {
	if os.Getenv("DEPLOY_TEST_HANG") != "1" {
		t.Skip("helper process only")
	}
	time.Sleep(time.Hour)
}

func TestRunCommandKillsProcessOnTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	t.Setenv("DEPLOY_TEST_HANG", "1")

	// Absolute path: runCommand runs the child in another directory.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	out, err := runCommand(ctx, t.TempDir(), self, "-test.run=^TestHelperHang$")
	if err == nil {
		t.Fatal("expected error from killed process")
	}
	// The child must have been running until the timeout, otherwise it
	// failed on its own and this test proves nothing about killing it.
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("child exited after %s, before the timeout: %v\n%s", elapsed, err, out)
	}
	if elapsed := time.Since(start); elapsed > killWaitDelay+5*time.Second {
		t.Fatalf("runCommand took %s; hanging child was not killed", elapsed)
	}
}
