package deploy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubCaddy replaces runCaddy for the duration of the test and records
// the caddy subcommands it was asked to run.
func stubCaddy(t *testing.T, err error) *[]string {
	t.Helper()
	var calls []string
	orig := runCaddy
	runCaddy = func(root string, args ...string) ([]byte, error) {
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

	if err := Reload(root); err != nil {
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

	err := Reload(root)
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

	if err := Reload(root); err == nil {
		t.Fatal("expected reload error")
	}
	if _, err := os.Stat(filepath.Join(root, CaddyfileName)); !os.IsNotExist(err) {
		t.Fatalf("expected Caddyfile to be removed, stat err: %v", err)
	}
}

func TestValidateReportsCaddyOutput(t *testing.T) {
	root := t.TempDir()
	calls := stubCaddy(t, errors.New("exit status 1"))

	err := Validate(root)
	if err == nil || !strings.Contains(err.Error(), "stub output") {
		t.Fatalf("expected validate error with caddy output, got: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0] != "validate" {
		t.Fatalf("expected one validate call, got %v", *calls)
	}
}
