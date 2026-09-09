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
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty output, got:\n%s", out)
	}
}
