package routes

import (
	"path/filepath"
	"testing"
)

func TestAddAndFind(t *testing.T) {
	s := &Store{}
	if err := s.Add("app.example.com", "10.0.0.5:8080"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	r, ok := s.Find("app.example.com")
	if !ok {
		t.Fatal("expected route to be found")
	}
	if r.Upstream != "10.0.0.5:8080" || !r.Enabled {
		t.Fatalf("unexpected route: %+v", r)
	}
}

func TestAddDuplicateFails(t *testing.T) {
	s := &Store{}
	if err := s.Add("app.example.com", "10.0.0.5:8080"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Add("app.example.com", "10.0.0.6:8080"); err == nil {
		t.Fatal("expected error adding duplicate hostname")
	}
}

func TestAddInvalidHostname(t *testing.T) {
	s := &Store{}
	if err := s.Add("not a hostname!", "10.0.0.5:8080"); err == nil {
		t.Fatal("expected error for invalid hostname")
	}
}

func TestAddInvalidUpstream(t *testing.T) {
	s := &Store{}
	if err := s.Add("app.example.com", "10.0.0.5"); err == nil {
		t.Fatal("expected error for upstream missing port")
	}
	if err := s.Add("app.example.com", "10.0.0.5:99999"); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
}

func TestRemove(t *testing.T) {
	s := &Store{}
	_ = s.Add("app.example.com", "10.0.0.5:8080")
	if err := s.Remove("app.example.com"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := s.Find("app.example.com"); ok {
		t.Fatal("expected route to be removed")
	}
}

func TestRemoveNotFound(t *testing.T) {
	s := &Store{}
	if err := s.Remove("missing.example.com"); err == nil {
		t.Fatal("expected error removing missing hostname")
	}
}

func TestEdit(t *testing.T) {
	s := &Store{}
	_ = s.Add("app.example.com", "10.0.0.5:8080")
	if err := s.Edit("app.example.com", "10.0.0.9:9090"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	r, _ := s.Find("app.example.com")
	if r.Upstream != "10.0.0.9:9090" {
		t.Fatalf("expected upstream to be updated, got %q", r.Upstream)
	}
}

func TestEditNotFound(t *testing.T) {
	s := &Store{}
	if err := s.Edit("missing.example.com", "10.0.0.9:9090"); err == nil {
		t.Fatal("expected error editing missing hostname")
	}
}

func TestSetEnabled(t *testing.T) {
	s := &Store{}
	_ = s.Add("app.example.com", "10.0.0.5:8080")
	if err := s.SetEnabled("app.example.com", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	r, _ := s.Find("app.example.com")
	if r.Enabled {
		t.Fatal("expected route to be disabled")
	}
}

func TestSetEnabledNotFound(t *testing.T) {
	s := &Store{}
	if err := s.SetEnabled("missing.example.com", false); err == nil {
		t.Fatal("expected error toggling missing hostname")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.yaml")

	s := &Store{}
	_ = s.Add("app.example.com", "10.0.0.5:8080")
	_ = s.SetEnabled("app.example.com", false)

	if err := Save(path, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, ok := loaded.Find("app.example.com")
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
