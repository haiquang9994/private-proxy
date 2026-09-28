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

	"private-proxy/internal/fsutil"
)

const (
	CaddyfileName    = "Caddyfile"
	CaddyfileNewName = "Caddyfile.new"
	CaddyfileBakName = "Caddyfile.bak"

	containerService = "caddy"
	containerRoot    = "/srv"
)

// runCaddy runs `caddy <args...>` inside the running container, with root
// as the working directory so docker compose finds the right
// docker-compose.yml. It is a variable so tests can stub out Docker.
var runCaddy = func(root string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"compose", "exec", "-T", containerService, "caddy"}, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Dir = root
	return cmd.CombinedOutput()
}

// WriteNewCaddyfile atomically writes content to <root>/Caddyfile.new.
func WriteNewCaddyfile(root, content string) error {
	return fsutil.AtomicWrite(filepath.Join(root, CaddyfileNewName), []byte(content))
}

// Validate runs `caddy validate` inside the running container against
// <root>/Caddyfile.new (visible inside the container at /srv/Caddyfile.new,
// since docker-compose.yml mounts root at /srv). It does not modify the
// live Caddyfile.
func Validate(root string) error {
	out, err := runCaddy(root, "validate", "--config", containerRoot+"/"+CaddyfileNewName, "--adapter", "caddyfile")
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
		if err := fsutil.AtomicWrite(bakPath, prev); err != nil {
			return fmt.Errorf("backup current Caddyfile: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read current Caddyfile: %w", err)
	}

	if err := os.Rename(newPath, caddyfilePath); err != nil {
		return fmt.Errorf("promote Caddyfile.new: %w", err)
	}

	out, err := runCaddy(root, "reload", "--config", containerRoot+"/"+CaddyfileName, "--adapter", "caddyfile")
	if err != nil {
		reloadErr := fmt.Errorf("caddy reload failed: %w\n%s", err, out)
		// Caddy is still running the previous config, so put the previous
		// Caddyfile back; otherwise the next container restart would load
		// the config that just failed to apply.
		if rbErr := restoreCaddyfile(caddyfilePath, prev, hadPrev); rbErr != nil {
			return fmt.Errorf("%w\nrollback of %s also failed: %v", reloadErr, CaddyfileName, rbErr)
		}
		return fmt.Errorf("%w\n%s was rolled back to the previous version", reloadErr, CaddyfileName)
	}
	return nil
}

// restoreCaddyfile puts back the Caddyfile that was live before Reload
// promoted Caddyfile.new, or removes it if there was none.
func restoreCaddyfile(caddyfilePath string, prev []byte, hadPrev bool) error {
	if !hadPrev {
		if err := os.Remove(caddyfilePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return fsutil.AtomicWrite(caddyfilePath, prev)
}
