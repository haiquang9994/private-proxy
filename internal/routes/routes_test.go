package routes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddAndFind(t *testing.T) {
	s := &Store{}
	if err := s.Add("app.example.com", "", "10.0.0.5:8080"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	r, ok := s.Find("app.example.com", "")
	if !ok {
		t.Fatal("expected route to be found")
	}
	if r.Upstream != "10.0.0.5:8080" || !r.Enabled {
		t.Fatalf("unexpected route: %+v", r)
	}
}

func TestAddDuplicateFails(t *testing.T) {
	s := &Store{}
	if err := s.Add("app.example.com", "", "10.0.0.5:8080"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Add("app.example.com", "", "10.0.0.6:8080"); err == nil {
		t.Fatal("expected error adding duplicate hostname+path")
	}
}

func TestAddInvalidHostname(t *testing.T) {
	s := &Store{}
	if err := s.Add("not a hostname!", "", "10.0.0.5:8080"); err == nil {
		t.Fatal("expected error for invalid hostname")
	}
}

func TestAddInvalidUpstream(t *testing.T) {
	s := &Store{}
	if err := s.Add("app.example.com", "", "10.0.0.5"); err == nil {
		t.Fatal("expected error for upstream missing port")
	}
	if err := s.Add("app.example.com", "", "10.0.0.5:99999"); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
}

func TestAddPathScopedRouteAlongsideCatchAll(t *testing.T) {
	s := &Store{}
	if err := s.Add("example.com", "", "10.10.10.1:8080"); err != nil {
		t.Fatalf("Add catch-all: %v", err)
	}
	if err := s.Add("example.com", "/ws", "10.10.10.1:6001"); err != nil {
		t.Fatalf("Add path route: %v", err)
	}

	root, ok := s.Find("example.com", "")
	if !ok || root.Upstream != "10.10.10.1:8080" {
		t.Fatalf("unexpected catch-all route: %+v", root)
	}
	ws, ok := s.Find("example.com", "/ws")
	if !ok || ws.Upstream != "10.10.10.1:6001" {
		t.Fatalf("unexpected path route: %+v", ws)
	}
}

func TestAddNestedPath(t *testing.T) {
	s := &Store{}
	if err := s.Add("example.com", "/api/v2/ws", "10.10.10.1:6001"); err != nil {
		t.Fatalf("Add nested path: %v", err)
	}
}

func TestAddInvalidPath(t *testing.T) {
	cases := []string{"ws", "/ws/", "/ws//x", "/ws x"}
	for _, path := range cases {
		s := &Store{}
		if err := s.Add("example.com", path, "10.10.10.1:6001"); err == nil {
			t.Fatalf("expected error for invalid path %q", path)
		}
	}
}

func TestRemove(t *testing.T) {
	s := &Store{}
	_ = s.Add("app.example.com", "", "10.0.0.5:8080")
	if err := s.Remove("app.example.com", ""); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := s.Find("app.example.com", ""); ok {
		t.Fatal("expected route to be removed")
	}
}

func TestRemoveNotFound(t *testing.T) {
	s := &Store{}
	if err := s.Remove("missing.example.com", ""); err == nil {
		t.Fatal("expected error removing missing hostname")
	}
}

func TestRemoveCatchAllLeavesPathRoute(t *testing.T) {
	s := &Store{}
	_ = s.Add("example.com", "", "10.10.10.1:8080")
	_ = s.Add("example.com", "/ws", "10.10.10.1:6001")

	if err := s.Remove("example.com", ""); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := s.Find("example.com", "/ws"); !ok {
		t.Fatal("expected path route to survive removing the catch-all")
	}
}

func TestEdit(t *testing.T) {
	s := &Store{}
	_ = s.Add("app.example.com", "", "10.0.0.5:8080")
	if err := s.Edit("app.example.com", "", "10.0.0.9:9090"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	r, _ := s.Find("app.example.com", "")
	if r.Upstream != "10.0.0.9:9090" {
		t.Fatalf("expected upstream to be updated, got %q", r.Upstream)
	}
}

func TestEditNotFound(t *testing.T) {
	s := &Store{}
	if err := s.Edit("missing.example.com", "", "10.0.0.9:9090"); err == nil {
		t.Fatal("expected error editing missing hostname")
	}
}

func TestSetEnabled(t *testing.T) {
	s := &Store{}
	_ = s.Add("app.example.com", "", "10.0.0.5:8080")
	if err := s.SetEnabled("app.example.com", "", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	r, _ := s.Find("app.example.com", "")
	if r.Enabled {
		t.Fatal("expected route to be disabled")
	}
}

func TestSetEnabledNotFound(t *testing.T) {
	s := &Store{}
	if err := s.SetEnabled("missing.example.com", "", false); err == nil {
		t.Fatal("expected error toggling missing hostname")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.yaml")

	s := &Store{}
	_ = s.Add("app.example.com", "", "10.0.0.5:8080")
	_ = s.SetEnabled("app.example.com", "", false)

	if err := Save(path, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, ok := loaded.Find("app.example.com", "")
	if !ok {
		t.Fatal("expected loaded route to be found")
	}
	if r.Upstream != "10.0.0.5:8080" || r.Enabled {
		t.Fatalf("unexpected loaded route: %+v", r)
	}
}

func TestLoadMissingFileReturnsEmptyStore(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Routes) != 0 {
		t.Fatalf("expected empty store, got %+v", s.Routes)
	}
}

func TestAddRejectsUpstreamInjection(t *testing.T) {
	s := &Store{}
	for _, upstream := range []string{
		"a b:80",
		"x {\n}\nevil.com {\n}\ny:80",
		"host{:80",
	} {
		if err := s.Add("app.example.com", "", upstream); err == nil {
			t.Fatalf("expected error for upstream %q", upstream)
		}
	}
}

func TestAddAcceptsHostnameAndIPv6Upstreams(t *testing.T) {
	s := &Store{}
	if err := s.Add("a.example.com", "", "backend.internal:8080"); err != nil {
		t.Fatalf("hostname upstream: %v", err)
	}
	if err := s.Add("b.example.com", "", "[::1]:8080"); err != nil {
		t.Fatalf("IPv6 upstream: %v", err)
	}
}

func writeRoutesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := writeRoutesFile(t, "routes:\n  - hostname: app.example.com\n    upstream: 10.0.0.5:8080\n    enable: true\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown field \"enable\"")
	}
}

func TestLoadRejectsInvalidRoute(t *testing.T) {
	path := writeRoutesFile(t, "routes:\n  - hostname: app.example.com\n    upstream: \"x {:80\"\n    enabled: true\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid upstream")
	}
}

func TestLoadRejectsDuplicateRoute(t *testing.T) {
	path := writeRoutesFile(t, "routes:\n"+
		"  - hostname: app.example.com\n    upstream: 10.0.0.5:8080\n    enabled: true\n"+
		"  - hostname: app.example.com\n    upstream: 10.0.0.6:8080\n    enabled: true\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for duplicate route")
	}
}

func TestLoadEmptyFileReturnsEmptyStore(t *testing.T) {
	s, err := Load(writeRoutesFile(t, ""))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Routes) != 0 {
		t.Fatalf("expected empty store, got %+v", s.Routes)
	}
}

func TestAddNormalizesHostnameCase(t *testing.T) {
	s := &Store{}
	if err := s.Add("App.Example.COM", "", "10.0.0.5:8080"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := s.Routes[0].Hostname; got != "app.example.com" {
		t.Fatalf("expected lowercased hostname, got %q", got)
	}
	if err := s.Add("app.example.com", "", "10.0.0.6:8080"); err == nil {
		t.Fatal("expected duplicate error for same hostname in different case")
	}
}

func TestAddRejectsPathDifferingOnlyByCase(t *testing.T) {
	s := &Store{}
	if err := s.Add("example.com", "/ws", "10.0.0.5:6001"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Add("example.com", "/WS", "10.0.0.5:6002"); err == nil {
		t.Fatal("expected duplicate error for path differing only by case")
	}
}

func TestFindAndRemoveIgnoreCase(t *testing.T) {
	s := &Store{}
	_ = s.Add("example.com", "/ws", "10.0.0.5:6001")
	if _, ok := s.Find("EXAMPLE.com", "/WS"); !ok {
		t.Fatal("expected case-insensitive Find to match")
	}
	if err := s.Remove("Example.com", "/Ws"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(s.Routes) != 0 {
		t.Fatalf("expected route removed, got %+v", s.Routes)
	}
}

func TestLoadNormalizesHostnameAndRejectsCaseDuplicates(t *testing.T) {
	path := writeRoutesFile(t, "routes:\n  - hostname: App.Example.com\n    upstream: 10.0.0.5:8080\n    enabled: true\n")
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := s.Routes[0].Hostname; got != "app.example.com" {
		t.Fatalf("expected lowercased hostname, got %q", got)
	}

	dupPath := writeRoutesFile(t, "routes:\n"+
		"  - hostname: Example.com\n    upstream: 10.0.0.5:8080\n    enabled: true\n"+
		"  - hostname: example.com\n    upstream: 10.0.0.6:8080\n    enabled: true\n")
	if _, err := Load(dupPath); err == nil {
		t.Fatal("expected duplicate error for hostnames differing only by case")
	}
}

func TestSaveWritesHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.yaml")
	if err := Save(path, &Store{}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// An empty store must save byte-for-byte as the committed routes.yaml,
	// so running proxyctl doesn't leave a spurious diff in the repo.
	committed, err := os.ReadFile("../../routes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(committed) {
		t.Fatalf("saved file differs from committed routes.yaml:\ngot:\n%s\nwant:\n%s", got, committed)
	}
}

func TestSaveHeaderSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.yaml")
	s := &Store{}
	_ = s.Add("app.example.com", "", "10.0.0.5:8080")
	if err := Save(path, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Save(path, loaded); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(got), "# Source of truth"); n != 1 {
		t.Fatalf("expected header exactly once after load+save, got %d:\n%s", n, got)
	}
}
