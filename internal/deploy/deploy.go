// Package deploy writes the generated Caddyfile to disk and drives the
// running Caddy container (via `docker compose exec`) to validate and
// reload it. Every function takes the absolute project root explicitly,
// so behavior does not depend on the caller's current working directory.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"private-proxy/internal/fsutil"
)

const (
	CaddyfileName    = "Caddyfile"
	CaddyfileNewName = "Caddyfile.new"
	CaddyfileBakName = "Caddyfile.bak"

	containerService = "caddy"
	containerRoot    = "/srv"

	// killWaitDelay bounds how long we wait for the docker CLI's output
	// pipes to close after it has been killed on timeout or interrupt.
	killWaitDelay = 5 * time.Second
)

// caddyTimeout bounds a single `caddy validate`/`caddy reload` run, so a
// Caddy that is stuck mid-reload can't hang proxyctl forever. It is a
// variable so tests can shorten it.
var caddyTimeout = 60 * time.Second

// runCaddy runs `caddy <args...>` inside the running container, with root
// as the working directory so docker compose finds the right
// docker-compose.yml. It is a variable so tests can stub out Docker.
var runCaddy = func(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"compose", "exec", "-T", containerService, "caddy"}, args...)
	return runCommand(ctx, root, "docker", cmdArgs...)
}

// runCommand runs name in dir and returns its combined output. The process
// is killed when ctx is done.
func runCommand(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.WaitDelay = killWaitDelay
	isolateFromTerminalSignals(cmd)
	return cmd.CombinedOutput()
}

// caddyCmd runs `caddy <subcmd> <args...>` in the container, bounded by
// caddyTimeout. A timeout or interrupt is reported as an error wrapping
// context.DeadlineExceeded or context.Canceled.
func caddyCmd(ctx context.Context, root, subcmd string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, caddyTimeout)
	defer cancel()

	out, err := runCaddy(ctx, root, append([]string{subcmd}, args...)...)
	if err == nil {
		return nil
	}
	switch ctxErr := ctx.Err(); {
	case errors.Is(ctxErr, context.DeadlineExceeded):
		return fmt.Errorf("caddy %s did not finish within %s: %w", subcmd, caddyTimeout, ctxErr)
	case errors.Is(ctxErr, context.Canceled):
		return fmt.Errorf("caddy %s interrupted: %w", subcmd, ctxErr)
	}
	return fmt.Errorf("caddy %s failed: %w\n%s", subcmd, err, out)
}

// WriteNewCaddyfile atomically writes content to <root>/Caddyfile.new.
func WriteNewCaddyfile(root, content string) error {
	return fsutil.AtomicWrite(filepath.Join(root, CaddyfileNewName), []byte(content))
}

// Validate runs `caddy validate` inside the running container against
// <root>/Caddyfile.new (visible inside the container at /srv/Caddyfile.new,
// since docker-compose.yml mounts root at /srv). It does not modify the
// live Caddyfile.
func Validate(ctx context.Context, root string) error {
	return caddyCmd(ctx, root, "validate", "--config", containerRoot+"/"+CaddyfileNewName, "--adapter", "caddyfile")
}

// Reload backs up the live Caddyfile to Caddyfile.bak, promotes Caddyfile.new
// to Caddyfile, and tells the running container to reload with zero downtime.
// The caller is expected to have already validated Caddyfile.new. If the
// reload fails, times out, or ctx is canceled (e.g. Ctrl+C), the previous
// Caddyfile is restored.
func Reload(ctx context.Context, root string) error {
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

	reloadErr := caddyCmd(ctx, root, "reload", "--config", containerRoot+"/"+CaddyfileName, "--adapter", "caddyfile")
	if reloadErr == nil {
		return nil
	}

	// Caddy is (most likely) still running the previous config, so put the
	// previous Caddyfile back; otherwise the next container restart would
	// load the config that didn't apply.
	if rbErr := restoreCaddyfile(caddyfilePath, prev, hadPrev); rbErr != nil {
		return fmt.Errorf("%w\nrollback of %s also failed: %v", reloadErr, CaddyfileName, rbErr)
	}
	msg := fmt.Sprintf("%s was rolled back to the previous version", CaddyfileName)
	if errors.Is(reloadErr, context.DeadlineExceeded) || errors.Is(reloadErr, context.Canceled) {
		// Killing the docker CLI doesn't abort the reload inside Caddy, which
		// holds its config lock until the reload finishes, so later reloads
		// would hang too.
		msg += "\nCaddy may still be stuck mid-reload; restart it with `docker compose restart caddy`, then run `proxyctl apply` again"
	}
	return fmt.Errorf("%w\n%s", reloadErr, msg)
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
