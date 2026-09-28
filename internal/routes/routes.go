// Package routes manages the declarative list of proxied hostnames
// stored in routes.yaml, the source of truth for the Caddyfile generator.
package routes

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

var hostnameRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// pathRE matches a non-root URL path: a leading slash followed by one or
// more non-empty segments, e.g. "/ws" or "/api/v2/ws". No trailing slash,
// no empty segments.
var pathRE = regexp.MustCompile(`^/[a-zA-Z0-9._~-]+(/[a-zA-Z0-9._~-]+)*$`)

// Route is a single hostname[+path] -> upstream mapping. An empty Path
// means the route is the catch-all for its hostname.
type Route struct {
	Hostname string `yaml:"hostname"`
	Path     string `yaml:"path,omitempty"`
	Upstream string `yaml:"upstream"`
	Enabled  bool   `yaml:"enabled"`
}

// Store is the full routes.yaml document.
type Store struct {
	Routes []Route `yaml:"routes"`
}

// Load reads and parses routes.yaml from path. A missing file yields an empty Store.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Store{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// KnownFields rejects typos like "enable:" instead of silently dropping them.
	var s Store
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return &s, nil
}

// Validate checks every route with the same rules Add enforces, and rejects
// duplicate (hostname, path) pairs, so a hand-edited routes.yaml can't feed
// unchecked values into the Caddyfile.
func (s *Store) Validate() error {
	seen := make(map[string]bool, len(s.Routes))
	for i, r := range s.Routes {
		if err := ValidateHostname(r.Hostname); err != nil {
			return fmt.Errorf("route #%d: %w", i+1, err)
		}
		if err := ValidatePath(r.Path); err != nil {
			return fmt.Errorf("route #%d: %w", i+1, err)
		}
		if err := ValidateUpstream(r.Upstream); err != nil {
			return fmt.Errorf("route #%d: %w", i+1, err)
		}
		key := displayName(r.Hostname, r.Path)
		if seen[key] {
			return fmt.Errorf("route #%d: duplicate route %q", i+1, key)
		}
		seen[key] = true
	}
	return nil
}

// Save writes the Store back to path atomically (write to temp file, then rename).
func Save(path string, s *Store) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode routes: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".routes-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// displayName formats a hostname+path pair the way it's identified on the CLI.
func displayName(hostname, path string) string {
	return hostname + path
}

// Find returns the route with the given hostname and path, if present.
func (s *Store) Find(hostname, path string) (*Route, bool) {
	for i := range s.Routes {
		if s.Routes[i].Hostname == hostname && s.Routes[i].Path == path {
			return &s.Routes[i], true
		}
	}
	return nil, false
}

// Add appends a new route. It fails if the hostname, path, or upstream is
// invalid, or if the (hostname, path) pair already exists.
func (s *Store) Add(hostname, path, upstream string) error {
	if err := ValidateHostname(hostname); err != nil {
		return err
	}
	if err := ValidatePath(path); err != nil {
		return err
	}
	if err := ValidateUpstream(upstream); err != nil {
		return err
	}
	if _, ok := s.Find(hostname, path); ok {
		return fmt.Errorf("route %q already exists", displayName(hostname, path))
	}
	s.Routes = append(s.Routes, Route{Hostname: hostname, Path: path, Upstream: upstream, Enabled: true})
	return nil
}

// Remove deletes the route with the given hostname and path. It fails if not found.
func (s *Store) Remove(hostname, path string) error {
	for i := range s.Routes {
		if s.Routes[i].Hostname == hostname && s.Routes[i].Path == path {
			s.Routes = append(s.Routes[:i], s.Routes[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("route %q not found", displayName(hostname, path))
}

// Edit updates the upstream of an existing route. It fails if not found or upstream is invalid.
func (s *Store) Edit(hostname, path, upstream string) error {
	if err := ValidateUpstream(upstream); err != nil {
		return err
	}
	r, ok := s.Find(hostname, path)
	if !ok {
		return fmt.Errorf("route %q not found", displayName(hostname, path))
	}
	r.Upstream = upstream
	return nil
}

// SetEnabled toggles whether a route is active. It fails if not found.
func (s *Store) SetEnabled(hostname, path string, enabled bool) error {
	r, ok := s.Find(hostname, path)
	if !ok {
		return fmt.Errorf("route %q not found", displayName(hostname, path))
	}
	r.Enabled = enabled
	return nil
}

// ValidateHostname reports whether hostname is a syntactically valid domain name.
func ValidateHostname(hostname string) error {
	if hostname == "" || !hostnameRE.MatchString(hostname) {
		return fmt.Errorf("invalid hostname %q", hostname)
	}
	return nil
}

// ValidatePath reports whether path is a syntactically valid route path.
// An empty path is valid and denotes the catch-all route for a hostname.
func ValidatePath(path string) error {
	if path == "" {
		return nil
	}
	if !pathRE.MatchString(path) {
		return fmt.Errorf("invalid path %q: must start with \"/\", contain no empty segments, and have no trailing slash", path)
	}
	return nil
}

// ValidateUpstream reports whether upstream is a syntactically valid "host:port" address.
func ValidateUpstream(upstream string) error {
	host, portStr, err := net.SplitHostPort(upstream)
	if err != nil {
		return fmt.Errorf("invalid upstream %q: expected host:port", upstream)
	}
	if host == "" {
		return fmt.Errorf("invalid upstream %q: missing host", upstream)
	}
	// The host is rendered verbatim into the Caddyfile, so it must be a plain
	// IP or hostname -- anything else (spaces, braces, newlines) could inject
	// extra Caddyfile directives.
	if net.ParseIP(host) == nil && !hostnameRE.MatchString(host) {
		return fmt.Errorf("invalid upstream %q: host must be an IP address or hostname", upstream)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid upstream %q: port must be 1-65535", upstream)
	}
	return nil
}
