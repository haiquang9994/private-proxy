package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeProject creates dir with a docker-compose.yml, like a repo clone.
func fakeProject(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, composeFileName), "services: {}\n")
}

func mustResolveRoot(t *testing.T, exe, envRoot string) string {
	t.Helper()
	root, err := resolveRoot(exe, envRoot)
	if err != nil {
		t.Fatalf("resolveRoot: %v", err)
	}
	return root
}

func TestResolveRootPrefersEnv(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "proj")
	fakeProject(t, proj)
	envRoot := t.TempDir()
	if got := mustResolveRoot(t, filepath.Join(proj, "bin", "proxyctl"), envRoot); got != envRoot {
		t.Fatalf("root = %s, want %s", got, envRoot)
	}
}

func TestResolveRootMakesEnvAbsolute(t *testing.T) {
	want, err := filepath.Abs("data")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustResolveRoot(t, "", "data"); got != want {
		t.Fatalf("root = %s, want %s", got, want)
	}
}

func TestResolveRootIgnoresEmptyEnv(t *testing.T) {
	if got := mustResolveRoot(t, filepath.Join(t.TempDir(), "proxyctl"), ""); got != defaultRoot {
		t.Fatalf("root = %s, want %s", got, defaultRoot)
	}
}

func TestResolveRootLegacyBinLayout(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "proj")
	fakeProject(t, proj)
	if got := mustResolveRoot(t, filepath.Join(proj, "bin", "proxyctl"), ""); got != proj {
		t.Fatalf("root = %s, want %s", got, proj)
	}
}

func TestResolveRootLegacyFlatLayout(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "proj")
	fakeProject(t, proj)
	if got := mustResolveRoot(t, filepath.Join(proj, "proxyctl"), ""); got != proj {
		t.Fatalf("root = %s, want %s", got, proj)
	}
}

func TestResolveRootDefaultsWhenNoComposeNearBinary(t *testing.T) {
	// Mirrors /usr/local/bin/proxyctl: bin's parent has no docker-compose.yml.
	exe := filepath.Join(t.TempDir(), "usr", "local", "bin", "proxyctl")
	if got := mustResolveRoot(t, exe, ""); got != defaultRoot {
		t.Fatalf("root = %s, want %s", got, defaultRoot)
	}
}

func TestCheckInitializedHintsInit(t *testing.T) {
	err := checkInitialized(t.TempDir())
	if err == nil {
		t.Fatal("expected an error for an uninitialized root")
	}
	if !strings.Contains(err.Error(), `run "proxyctl init"`) {
		t.Fatalf("error does not mention proxyctl init: %v", err)
	}
}

func TestCheckInitializedAcceptsProject(t *testing.T) {
	root := t.TempDir()
	fakeProject(t, root)
	if err := checkInitialized(root); err != nil {
		t.Fatalf("checkInitialized: %v", err)
	}
}
