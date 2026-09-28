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
// The caller is expected to have already validated Caddyfile.new. If the
// reload fails, the previous Caddyfile is restored.
func Reload(root string) error {
	caddyfilePath := filepath.Join(root, CaddyfileName)
	newPath := filepath.Join(root, CaddyfileNewName)
	bakPath := filepath.Join(root, CaddyfileBakName)

	prev, err := os.ReadFile(caddyfilePath)
	hadPrev := err == nil
	if hadPrev {
		if err := atomicWrite(root, bakPath, string(prev)); err != nil {
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
		reloadErr := fmt.Errorf("caddy reload failed: %w\n%s", err, out)
		// Caddy is still running the previous config, so put the previous
		// Caddyfile back; otherwise the next container restart would load
		// the config that just failed to apply.
		if rbErr := restoreCaddyfile(root, caddyfilePath, prev, hadPrev); rbErr != nil {
			return fmt.Errorf("%w\nrollback of %s also failed: %v", reloadErr, CaddyfileName, rbErr)
		}
		return fmt.Errorf("%w\n%s was rolled back to the previous version", reloadErr, CaddyfileName)
	}
	return nil
}

// restoreCaddyfile puts back the Caddyfile that was live before Reload
// promoted Caddyfile.new, or removes it if there was none.
func restoreCaddyfile(root, caddyfilePath string, prev []byte, hadPrev bool) error {
	if !hadPrev {
		if err := os.Remove(caddyfilePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return atomicWrite(root, caddyfilePath, string(prev))
}
