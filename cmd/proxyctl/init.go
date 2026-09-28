package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	privateproxy "private-proxy"
	"private-proxy/internal/caddyfile"
	"private-proxy/internal/deploy"
	"private-proxy/internal/fsutil"
	"private-proxy/internal/routes"
)

// rootDirMode keeps the project root private to root: routes.yaml and the
// Caddyfile are only meant to change through proxyctl.
const rootDirMode fs.FileMode = 0o700

func runInit(ctx context.Context, root string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: proxyctl init")
	}
	// The lock file lives inside root, so root must exist before locking.
	if err := os.MkdirAll(root, rootDirMode); err != nil {
		return fmt.Errorf("create %s: %w", root, err)
	}
	if err := withLock(ctx, root, func() error { return initProject(root) }); err != nil {
		return err
	}
	fmt.Printf("initialized %s\n", root)
	return nil
}

// initProject writes the files Caddy and proxyctl need into an existing
// root. docker-compose.yml is owned by proxyctl and always rewritten;
// routes.yaml and the Caddyfile hold user state, so they are only created
// when missing.
func initProject(root string) error {
	if err := fsutil.AtomicWrite(filepath.Join(root, composeFileName), privateproxy.ComposeFile); err != nil {
		return err
	}

	routesPath := filepath.Join(root, routesFileName)
	missing, err := isMissing(routesPath)
	if err != nil {
		return err
	}
	if missing {
		if err := routes.Save(routesPath, &routes.Store{}); err != nil {
			return err
		}
	}

	// Caddy refuses to start without a Caddyfile, so render one from
	// whatever routes.yaml now holds.
	caddyfilePath := filepath.Join(root, deploy.CaddyfileName)
	if missing, err = isMissing(caddyfilePath); err != nil || !missing {
		return err
	}
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	content, err := caddyfile.Render(s)
	if err != nil {
		return fmt.Errorf("render Caddyfile: %w", err)
	}
	return fsutil.AtomicWrite(caddyfilePath, []byte(content))
}

// isMissing reports whether path does not exist.
func isMissing(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return false, nil
}
