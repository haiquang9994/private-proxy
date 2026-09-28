package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"private-proxy/internal/routes"
)

func testStore() *routes.Store {
	return &routes.Store{Routes: []routes.Route{
		{Hostname: "example.com", Upstream: "10.10.10.1:8080", Enabled: true},
		{Hostname: "example.com", Path: "/ws", Upstream: "10.10.10.1:6001", Enabled: false},
		{Hostname: "app.example.com", Upstream: "10.0.0.5:8080", Enabled: true},
	}}
}

func TestCompletions(t *testing.T) {
	s := testStore()
	tests := []struct {
		name  string
		words []string
		want  []string
	}{
		{"all commands", []string{""}, commandNames},
		{"command prefix", []string{"e"}, []string{"edit", "enable"}},
		{"init and version are commands", []string{"v"}, []string{"validate", "version"}},
		{"remove offers all routes", []string{"remove", ""}, []string{"app.example.com", "example.com", "example.com/ws"}},
		{"route prefix", []string{"edit", "example.com/"}, []string{"example.com/ws"}},
		{"enable offers disabled routes", []string{"enable", ""}, []string{"example.com/ws"}},
		{"disable offers enabled routes", []string{"disable", ""}, []string{"app.example.com", "example.com"}},
		{"edit offers current upstream", []string{"edit", "example.com/ws", ""}, []string{"10.10.10.1:6001"}},
		{"edit unknown route", []string{"edit", "nope.com", ""}, nil},
		{"completion shells", []string{"completion", "z"}, []string{"zsh"}},
		{"add has no suggestions", []string{"add", ""}, nil},
		{"extra args", []string{"remove", "example.com", ""}, nil},
		{"unknown command", []string{"bogus", ""}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := completions(s, tt.words); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("completions(%q) = %q, want %q", tt.words, got, tt.want)
			}
		})
	}
}

func TestCompletionsWithoutStore(t *testing.T) {
	if got := completions(nil, []string{"remove", ""}); got != nil {
		t.Fatalf("expected no route suggestions without a store, got %q", got)
	}
	if got := completions(nil, []string{"edit", "example.com", ""}); got != nil {
		t.Fatalf("expected no upstream suggestions without a store, got %q", got)
	}
	if got := completions(nil, []string{"li"}); !reflect.DeepEqual(got, []string{"list"}) {
		t.Fatalf("expected command suggestions without a store, got %q", got)
	}
}

func TestShellQuote(t *testing.T) {
	if got, want := shellQuote(`/opt/it's/proxyctl`), `'/opt/it'\''s/proxyctl'`; got != want {
		t.Fatalf("shellQuote = %s, want %s", got, want)
	}
}

func TestRunCompleteWithoutRoutesFile(t *testing.T) {
	for _, root := range []string{"", filepath.Join(t.TempDir(), "missing")} {
		out := captureStdout(t, func() {
			if err := runComplete(root, []string{"li"}); err != nil {
				t.Fatalf("runComplete(%q): %v", root, err)
			}
		})
		if out != "list\n" {
			t.Fatalf("runComplete(%q) printed %q, want %q", root, out, "list\n")
		}
	}
}

// captureStdout returns what fn writes to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	w.Close()
	var b strings.Builder
	if _, err := io.Copy(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
