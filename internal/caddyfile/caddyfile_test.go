package caddyfile

import (
	"strings"
	"testing"

	"private-proxy/internal/routes"
)

func TestRenderSkipsDisabledRoutes(t *testing.T) {
	s := &routes.Store{
		Routes: []routes.Route{
			{Hostname: "app.example.com", Upstream: "10.0.0.5:8080", Enabled: true},
			{Hostname: "old.example.com", Upstream: "10.0.0.9:3000", Enabled: false},
		},
	}

	out, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "app.example.com") {
		t.Fatalf("expected enabled route in output, got:\n%s", out)
	}
	if strings.Contains(out, "old.example.com") {
		t.Fatalf("expected disabled route to be skipped, got:\n%s", out)
	}
	if !strings.Contains(out, "reverse_proxy 10.0.0.5:8080") {
		t.Fatalf("expected reverse_proxy directive, got:\n%s", out)
	}
}

func TestRenderEmptyStore(t *testing.T) {
	s := &routes.Store{}
	out, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out != header {
		t.Fatalf("expected only the header, got:\n%s", out)
	}
}

func TestRenderPathRouteUsesNamedMatcher(t *testing.T) {
	s := &routes.Store{
		Routes: []routes.Route{
			{Hostname: "example.com", Path: "/ws", Upstream: "10.10.10.1:6001", Enabled: true},
		},
	}

	out, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "@p0 path /ws /ws/*") {
		t.Fatalf("expected named matcher definition, got:\n%s", out)
	}
	if !strings.Contains(out, "reverse_proxy @p0 10.10.10.1:6001") {
		t.Fatalf("expected reverse_proxy to reference the named matcher, got:\n%s", out)
	}
}

func TestRenderGroupsSameHostnameIntoOneBlock(t *testing.T) {
	s := &routes.Store{
		Routes: []routes.Route{
			{Hostname: "example.com", Path: "", Upstream: "10.10.10.1:8080", Enabled: true},
			{Hostname: "example.com", Path: "/ws", Upstream: "10.10.10.1:6001", Enabled: true},
		},
	}

	out, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if n := strings.Count(out, "example.com {"); n != 1 {
		t.Fatalf("expected exactly one site block for example.com, got %d:\n%s", n, out)
	}
}

func TestRenderOrdersSpecificPathBeforeCatchAll(t *testing.T) {
	s := &routes.Store{
		Routes: []routes.Route{
			{Hostname: "example.com", Path: "", Upstream: "10.10.10.1:8080", Enabled: true},
			{Hostname: "example.com", Path: "/ws", Upstream: "10.10.10.1:6001", Enabled: true},
		},
	}

	out, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	wsIdx := strings.Index(out, "reverse_proxy @p0 10.10.10.1:6001")
	catchAllIdx := strings.Index(out, "reverse_proxy 10.10.10.1:8080")
	if wsIdx == -1 || catchAllIdx == -1 {
		t.Fatalf("expected both routes in output, got:\n%s", out)
	}
	if wsIdx > catchAllIdx {
		t.Fatalf("expected path-scoped route before catch-all, got:\n%s", out)
	}
}

func TestRenderOrdersDeeperPathsFirst(t *testing.T) {
	s := &routes.Store{
		Routes: []routes.Route{
			{Hostname: "example.com", Path: "", Upstream: "10.10.10.1:8080", Enabled: true},
			{Hostname: "example.com", Path: "/api", Upstream: "10.10.10.1:7000", Enabled: true},
			{Hostname: "example.com", Path: "/api/v2/ws", Upstream: "10.10.10.1:6001", Enabled: true},
		},
	}

	out, err := Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	deepIdx := strings.Index(out, "10.10.10.1:6001")
	apiIdx := strings.Index(out, "10.10.10.1:7000")
	catchAllIdx := strings.Index(out, "10.10.10.1:8080")
	if deepIdx == -1 || apiIdx == -1 || catchAllIdx == -1 {
		t.Fatalf("expected all three routes in output, got:\n%s", out)
	}
	if !(deepIdx < apiIdx && apiIdx < catchAllIdx) {
		t.Fatalf("expected order /api/v2/ws, /api, catch-all, got:\n%s", out)
	}
}
