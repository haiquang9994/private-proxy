// Package deploy writes the generated Caddyfile to disk and drives the
// running Caddy container (via `docker compose exec`) to validate and
// reload it. It assumes the current working directory is the project
// root, matching the docker-compose.yml volume mount of "." to "/srv"
// inside the container.
package deploy

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	CaddyfilePath    = "Caddyfile"
	CaddyfileNewPath = "Caddyfile.new"
	CaddyfileBakPath = "Caddyfile.bak"

	containerService = "caddy"
	containerRoot    = "/srv"
)

// WriteNewCaddyfile atomically writes content to Caddyfile.new.
func WriteNewCaddyfile(content string) error {
	return atomicWrite(CaddyfileNewPath, content)
}

func atomicWrite(path, content string) error {
	tmp, err := os.CreateTemp(".", ".caddyfile-*.tmp")
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

// Validate runs `caddy validate` inside the running container against relPath
// (a file under the project root, visible inside the container at /srv/relPath).
// It does not modify the live Caddyfile.
func Validate(relPath string) error {
	cmd := exec.Command("docker", "compose", "exec", "-T", containerService,
		"caddy", "validate", "--config", containerRoot+"/"+relPath, "--adapter", "caddyfile")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("caddy validate failed: %w\n%s", err, out)
	}
	return nil
}

// Reload backs up the live Caddyfile to Caddyfile.bak, promotes Caddyfile.new
// to Caddyfile, and tells the running container to reload with zero downtime.
// The caller is expected to have already validated Caddyfile.new.
func Reload() error {
	if data, err := os.ReadFile(CaddyfilePath); err == nil {
		if err := atomicWrite(CaddyfileBakPath, string(data)); err != nil {
			return fmt.Errorf("backup current Caddyfile: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read current Caddyfile: %w", err)
	}

	if err := os.Rename(CaddyfileNewPath, CaddyfilePath); err != nil {
		return fmt.Errorf("promote Caddyfile.new: %w", err)
	}

	cmd := exec.Command("docker", "compose", "exec", "-T", containerService,
		"caddy", "reload", "--config", containerRoot+"/"+CaddyfilePath, "--adapter", "caddyfile")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("caddy reload failed: %w\n%s", err, out)
	}
	return nil
}
