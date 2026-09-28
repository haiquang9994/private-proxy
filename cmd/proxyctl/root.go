package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// rootEnvVar overrides where proxyctl keeps routes.yaml and the Caddyfile.
	rootEnvVar = "PROXYCTL_ROOT"
	// defaultRoot is where install.sh sets up the project.
	defaultRoot = "/opt/private-proxy"
)

// executablePath returns the running binary's path with symlinks resolved.
func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate proxyctl executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve proxyctl executable path: %w", err)
	}
	return exe, nil
}

// resolveRoot picks the project root, first match wins:
//  1. envRoot ($PROXYCTL_ROOT), if non-empty, made absolute because docker
//     compose runs with the root as its working directory;
//  2. the legacy layout of a repo clone, where exe lives in the project
//     root or in its bin/ directory, next to a docker-compose.yml;
//  3. defaultRoot.
//
// It does not check that the chosen root is initialized.
func resolveRoot(exe, envRoot string) (string, error) {
	if envRoot != "" {
		root, err := filepath.Abs(envRoot)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", rootEnvVar, err)
		}
		return root, nil
	}
	if exe != "" {
		dir := filepath.Dir(exe)
		if filepath.Base(dir) == "bin" {
			dir = filepath.Dir(dir)
		}
		if _, err := os.Stat(filepath.Join(dir, composeFileName)); err == nil {
			return dir, nil
		}
	}
	return defaultRoot, nil
}

// currentRoot resolves the project root for this process.
func currentRoot() (string, error) {
	exe, err := executablePath()
	if err != nil {
		return "", err
	}
	return resolveRoot(exe, os.Getenv(rootEnvVar))
}

// checkInitialized fails unless root has a docker-compose.yml, so commands
// can't silently create routes.yaml in a directory Caddy never reads.
func checkInitialized(root string) error {
	if _, err := os.Stat(filepath.Join(root, composeFileName)); err != nil {
		return fmt.Errorf("%s not found in %s: run \"proxyctl init\" first, or set %s", composeFileName, root, rootEnvVar)
	}
	return nil
}

// projectRoot resolves the project root and checks it is initialized.
func projectRoot() (string, error) {
	root, err := currentRoot()
	if err != nil {
		return "", err
	}
	if err := checkInitialized(root); err != nil {
		return "", err
	}
	return root, nil
}
