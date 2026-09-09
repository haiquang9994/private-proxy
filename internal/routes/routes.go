// Package routes manages the declarative list of proxied hostnames
// stored in routes.yaml, the source of truth for the Caddyfile generator.
package routes

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

var hostnameRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// Route is a single hostname -> upstream mapping.
type Route struct {
	Hostname string `yaml:"hostname"`
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

	var s Store
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &s, nil
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

// Find returns the route with the given hostname, if present.
func (s *Store) Find(hostname string) (*Route, bool) {
	for i := range s.Routes {
		if s.Routes[i].Hostname == hostname {
			return &s.Routes[i], true
		}
	}
	return nil, false
}

// Add appends a new route. It fails if the hostname or upstream is invalid,
// or if the hostname already exists.
func (s *Store) Add(hostname, upstream string) error {
	if err := ValidateHostname(hostname); err != nil {
		return err
	}
	if err := ValidateUpstream(upstream); err != nil {
		return err
	}
	if _, ok := s.Find(hostname); ok {
		return fmt.Errorf("hostname %q already exists", hostname)
	}
	s.Routes = append(s.Routes, Route{Hostname: hostname, Upstream: upstream, Enabled: true})
	return nil
}

// Remove deletes the route with the given hostname. It fails if not found.
func (s *Store) Remove(hostname string) error {
	for i := range s.Routes {
		if s.Routes[i].Hostname == hostname {
			s.Routes = append(s.Routes[:i], s.Routes[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("hostname %q not found", hostname)
}

// Edit updates the upstream of an existing route. It fails if not found or upstream is invalid.
func (s *Store) Edit(hostname, upstream string) error {
	if err := ValidateUpstream(upstream); err != nil {
		return err
	}
	r, ok := s.Find(hostname)
	if !ok {
		return fmt.Errorf("hostname %q not found", hostname)
	}
	r.Upstream = upstream
	return nil
}

// SetEnabled toggles whether a route is active. It fails if not found.
func (s *Store) SetEnabled(hostname string, enabled bool) error {
	r, ok := s.Find(hostname)
	if !ok {
		return fmt.Errorf("hostname %q not found", hostname)
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

// ValidateUpstream reports whether upstream is a syntactically valid "host:port" address.
func ValidateUpstream(upstream string) error {
	host, portStr, err := net.SplitHostPort(upstream)
	if err != nil {
		return fmt.Errorf("invalid upstream %q: expected host:port", upstream)
	}
	if host == "" {
		return fmt.Errorf("invalid upstream %q: missing host", upstream)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid upstream %q: port must be 1-65535", upstream)
	}
	return nil
}
