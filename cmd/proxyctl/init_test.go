package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	privateproxy "private-proxy"
	"private-proxy/internal/caddyfile"
	"private-proxy/internal/deploy"
	"private-proxy/internal/routes"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInitProjectFreshRoot(t *testing.T) {
	root := t.TempDir()
	if err := initProject(root); err != nil {
		t.Fatalf("initProject: %v", err)
	}

	if got := readTestFile(t, filepath.Join(root, composeFileName)); got != string(privateproxy.ComposeFile) {
		t.Fatalf("docker-compose.yml does not match the embedded copy:\n%s", got)
	}
	s, err := routes.Load(filepath.Join(root, routesFileName))
	if err != nil {
		t.Fatalf("load generated routes.yaml: %v", err)
	}
	if len(s.Routes) != 0 {
		t.Fatalf("expected no routes, got %v", s.Routes)
	}
	want, err := caddyfile.Render(&routes.Store{})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, filepath.Join(root, deploy.CaddyfileName)); got != want {
		t.Fatalf("Caddyfile = %q, want %q", got, want)
	}
}

func TestInitProjectKeepsUserFiles(t *testing.T) {
	root := t.TempDir()
	composePath := filepath.Join(root, composeFileName)
	routesPath := filepath.Join(root, routesFileName)
	caddyfilePath := filepath.Join(root, deploy.CaddyfileName)
	userRoutes := "routes:\n  - hostname: example.com\n    upstream: 10.0.0.5:8080\n    enabled: true\n"
	writeTestFile(t, composePath, "stale compose\n")
	writeTestFile(t, routesPath, userRoutes)
	writeTestFile(t, caddyfilePath, "# live Caddyfile\n")

	if err := initProject(root); err != nil {
		t.Fatalf("initProject: %v", err)
	}

	if got := readTestFile(t, composePath); got != string(privateproxy.ComposeFile) {
		t.Fatalf("docker-compose.yml was not refreshed:\n%s", got)
	}
	if got := readTestFile(t, routesPath); got != userRoutes {
		t.Fatalf("routes.yaml was overwritten:\n%s", got)
	}
	if got := readTestFile(t, caddyfilePath); got != "# live Caddyfile\n" {
		t.Fatalf("Caddyfile was overwritten:\n%s", got)
	}
}

func TestRunInitCreatesPrivateRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-proxy")
	if err := runInit(context.Background(), root, nil); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", root)
	}
	// Windows has no unix permission bits to check.
	if runtime.GOOS != "windows" && info.Mode().Perm() != rootDirMode {
		t.Fatalf("root mode = %v, want %v", info.Mode().Perm(), rootDirMode)
	}
	if _, err := os.Stat(filepath.Join(root, composeFileName)); err != nil {
		t.Fatalf("docker-compose.yml not written: %v", err)
	}
}

func TestRunInitRejectsArgs(t *testing.T) {
	if err := runInit(context.Background(), t.TempDir(), []string{"extra"}); err == nil {
		t.Fatal("expected a usage error")
	}
}
