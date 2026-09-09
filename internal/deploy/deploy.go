// Package deploy writes the generated Caddyfile to disk and drives the
// running Caddy container (via `docker compose exec`) to validate and
// reload it. Every function takes the absolute project root explicitly,
// so behavior does not depend on the caller's current working directory.
package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	CaddyfileName    = "Caddyfile"
	CaddyfileNewName = "Caddyfile.new"
	CaddyfileBakName = "Caddyfile.bak"

	containerService = "caddy"
	containerRoot    = "/srv"
)

// WriteNewCaddyfile atomically writes content to <root>/Caddyfile.new.
func WriteNewCaddyfile(root, content string) error {
	return atomicWrite(root, filepath.Join(root, CaddyfileNewName), content)
}

func atomicWrite(root, path, content string) error {
	tmp, err := os.CreateTemp(root, ".caddyfile-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(content); err != nil {
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

// Validate runs `caddy validate` inside the running container against
// <root>/Caddyfile.new (visible inside the container at /srv/Caddyfile.new,
// since docker-compose.yml mounts root at /srv). It does not modify the
// live Caddyfile. The docker compose command is run with root as its
// working directory so it finds the right docker-compose.yml.
func Validate(root string) error {
	cmd := exec.Command("docker", "compose", "exec", "-T", containerService,
		"caddy", "validate", "--config", containerRoot+"/"+CaddyfileNewName, "--adapter", "caddyfile")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("caddy validate failed: %w\n%s", err, out)
	}
	return nil
}

// Reload backs up the live Caddyfile to Caddyfile.bak, promotes Caddyfile.new
// to Caddyfile, and tells the running container to reload with zero downtime.
// The caller is expected to have already validated Caddyfile.new.
func Reload(root string) error {
	caddyfilePath := filepath.Join(root, CaddyfileName)
	newPath := filepath.Join(root, CaddyfileNewName)
	bakPath := filepath.Join(root, CaddyfileBakName)

	if data, err := os.ReadFile(caddyfilePath); err == nil {
		if err := atomicWrite(root, bakPath, string(data)); err != nil {
			return fmt.Errorf("backup current Caddyfile: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read current Caddyfile: %w", err)
	}

	if err := os.Rename(newPath, caddyfilePath); err != nil {
		return fmt.Errorf("promote Caddyfile.new: %w", err)
	}

	cmd := exec.Command("docker", "compose", "exec", "-T", containerService,
		"caddy", "reload", "--config", containerRoot+"/"+CaddyfileName, "--adapter", "caddyfile")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("caddy reload failed: %w\n%s", err, out)
	}
	return nil
}
