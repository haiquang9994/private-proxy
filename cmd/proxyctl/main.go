// Command proxyctl manages the proxied hostnames declared in routes.yaml
// and drives Caddy (running in Docker Compose) to apply them.
//
// Its project root (routes.yaml, Caddyfile, docker-compose.yml) is
// $PROXYCTL_ROOT if set, else the directory of a repo clone the binary was
// built into (<root>/bin/proxyctl), else /opt/private-proxy.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"private-proxy/internal/caddyfile"
	"private-proxy/internal/deploy"
	"private-proxy/internal/filelock"
	"private-proxy/internal/routes"
)

const (
	routesFileName  = "routes.yaml"
	composeFileName = "docker-compose.yml"
	lockFileName    = ".proxyctl.lock"

	// lockTimeout bounds how long a command waits for another proxyctl
	// (e.g. a slow `apply`) to finish before giving up.
	lockTimeout = 30 * time.Second
)

// version is stamped at release build time via -ldflags "-X main.version=...".
var version = "dev"

// projectCommands need root privileges and, except init, an initialized
// project root.
var projectCommands = []string{
	"init", "add", "remove", "edit", "enable", "disable", "list", "validate", "apply",
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	// These work without root privileges or an initialized project root.
	switch cmd {
	case "help", "-h", "--help":
		usage()
		return
	case "version":
		fmt.Println(version)
		return
	case "completion":
		exitOnError(runCompletion(args))
		return
	case completeCommand:
		// Best effort: an unresolvable root only means no route suggestions.
		root, _ := currentRoot()
		exitOnError(runComplete(root, args))
		return
	}

	if !slices.Contains(projectCommands, cmd) {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}
	exitOnError(requireRoot())

	// Ctrl+C / SIGTERM cancel ctx instead of killing proxyctl outright, so
	// an interrupted apply still rolls back the Caddyfile and releases the lock.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cmd == "init" {
		root, err := currentRoot()
		exitOnError(err)
		exitOnError(runInit(ctx, root, args))
		return
	}
	root, err := projectRoot()
	exitOnError(err)
	exitOnError(runProjectCommand(ctx, root, cmd, args))
}

// runProjectCommand runs a command against an initialized project root.
func runProjectCommand(ctx context.Context, root, cmd string, args []string) error {
	switch cmd {
	case "add":
		return withLock(ctx, root, func() error { return runAdd(root, args) })
	case "remove":
		return withLock(ctx, root, func() error { return runRemove(root, args) })
	case "edit":
		return withLock(ctx, root, func() error { return runEdit(root, args) })
	case "enable":
		return withLock(ctx, root, func() error { return runSetEnabled(root, args, true) })
	case "disable":
		return withLock(ctx, root, func() error { return runSetEnabled(root, args, false) })
	case "list":
		// Read-only, and routes.yaml is replaced atomically, so no lock needed.
		return runList(root, args)
	case "validate":
		return withLock(ctx, root, func() error { return runValidate(ctx, root, args) })
	case "apply":
		return withLock(ctx, root, func() error { return runApply(ctx, root, args) })
	}
	return fmt.Errorf("unknown command %q", cmd)
}

// exitOnError prints err and exits with status 1 if err is non-nil.
func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// withLock runs fn while holding the project-wide lock, so concurrent
// proxyctl invocations can't lose each other's routes.yaml edits or
// overwrite each other's Caddyfile.new mid-apply.
func withLock(ctx context.Context, root string, fn func() error) error {
	lock, err := filelock.Acquire(ctx, filepath.Join(root, lockFileName), lockTimeout)
	if err != nil {
		return fmt.Errorf("%w: is another proxyctl command still running?", err)
	}
	defer lock.Release()
	return fn()
}

func usage() {
	fmt.Fprint(os.Stderr, `proxyctl - manage proxied hostnames backed by Caddy

Usage:
  proxyctl init                                 Create the project root and its files
  proxyctl add <hostname>[/path] <ip:port>      Add a new route -> upstream
  proxyctl remove <hostname>[/path]             Remove a route
  proxyctl edit <hostname>[/path] <ip:port>     Change the upstream of an existing route
  proxyctl enable <hostname>[/path]             Re-enable a disabled route
  proxyctl disable <hostname>[/path]            Disable a route without deleting it
  proxyctl list                                 List all routes and their status
  proxyctl validate                             Render and validate the Caddyfile, without applying it
  proxyctl apply                                Validate, then apply and reload Caddy
  proxyctl completion bash|zsh                  Print the shell tab-completion script
  proxyctl version                              Print the proxyctl version

Commands other than "apply" only edit routes.yaml; run "apply" to deploy the change.
Every command except help, version and completion must be run as root.

The project root is $PROXYCTL_ROOT if set, else /opt/private-proxy
(or the repo clone proxyctl was built in).

A hostname with no path is its own route (the catch-all for that host); a
path route only matches that path and everything under it -- it doesn't
affect the hostname's other routes. "remove"/"disable" on a hostname only
removes/disables that hostname's catch-all route, not its path routes.
`)
}

// splitHostPath splits a CLI token like "example.com" or "example.com/ws"
// into its hostname and path parts. path is "" when the token has no "/".
func splitHostPath(token string) (hostname, path string) {
	if i := strings.IndexByte(token, '/'); i >= 0 {
		return token[:i], token[i:]
	}
	return token, ""
}

// mutateRoutes loads routes.yaml, applies fn to the store, and saves it back.
func mutateRoutes(root string, fn func(s *routes.Store) error) error {
	routesPath := filepath.Join(root, routesFileName)
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if err := fn(s); err != nil {
		return err
	}
	return routes.Save(routesPath, s)
}

func runAdd(root string, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: proxyctl add <hostname>[/path] <ip:port>")
	}
	hostname, path := splitHostPath(args[0])
	if err := mutateRoutes(root, func(s *routes.Store) error {
		return s.Add(hostname, path, args[1])
	}); err != nil {
		return err
	}
	fmt.Printf("added %s -> %s (run \"proxyctl apply\" to deploy)\n", args[0], args[1])
	return nil
}

func runRemove(root string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: proxyctl remove <hostname>[/path]")
	}
	hostname, path := splitHostPath(args[0])
	if err := mutateRoutes(root, func(s *routes.Store) error {
		return s.Remove(hostname, path)
	}); err != nil {
		return err
	}
	fmt.Printf("removed %s (run \"proxyctl apply\" to deploy)\n", args[0])
	return nil
}

func runEdit(root string, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: proxyctl edit <hostname>[/path] <ip:port>")
	}
	hostname, path := splitHostPath(args[0])
	if err := mutateRoutes(root, func(s *routes.Store) error {
		return s.Edit(hostname, path, args[1])
	}); err != nil {
		return err
	}
	fmt.Printf("updated %s -> %s (run \"proxyctl apply\" to deploy)\n", args[0], args[1])
	return nil
}

func runSetEnabled(root string, args []string, enabled bool) error {
	if len(args) != 1 {
		verb := "enable"
		if !enabled {
			verb = "disable"
		}
		return fmt.Errorf("usage: proxyctl %s <hostname>[/path]", verb)
	}
	hostname, path := splitHostPath(args[0])
	if err := mutateRoutes(root, func(s *routes.Store) error {
		return s.SetEnabled(hostname, path, enabled)
	}); err != nil {
		return err
	}
	state := "enabled"
	if !enabled {
		state = "disabled"
	}
	fmt.Printf("%s %s (run \"proxyctl apply\" to deploy)\n", state, args[0])
	return nil
}

func runList(root string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: proxyctl list")
	}
	routesPath := filepath.Join(root, routesFileName)
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if len(s.Routes) == 0 {
		fmt.Println("no routes configured")
		return nil
	}

	// Display order only: group routes by hostname so they show up next to
	// each other, without touching s.Routes (its order can affect how Caddy
	// matches paths on apply).
	sorted := make([]routes.Route, len(s.Routes))
	copy(sorted, s.Routes)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Hostname < sorted[j].Hostname
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "HOSTNAME\tPATH\tUPSTREAM\tSTATUS")
	for _, r := range sorted {
		status := "enabled"
		if !r.Enabled {
			status = "disabled"
		}
		path := r.Path
		if path == "" {
			path = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Hostname, path, r.Upstream, status)
	}
	return w.Flush()
}

func runValidate(ctx context.Context, root string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: proxyctl validate")
	}
	routesPath := filepath.Join(root, routesFileName)
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	content, err := caddyfile.Render(s)
	if err != nil {
		return fmt.Errorf("render Caddyfile: %w", err)
	}
	if err := deploy.WriteNewCaddyfile(root, content); err != nil {
		return err
	}
	if err := deploy.Validate(ctx, root); err != nil {
		return err
	}
	fmt.Println("Caddyfile is valid")
	return nil
}

func runApply(ctx context.Context, root string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: proxyctl apply")
	}
	if err := runValidate(ctx, root, nil); err != nil {
		return err
	}
	if err := deploy.Reload(ctx, root); err != nil {
		return err
	}
	fmt.Println("applied and reloaded Caddy")
	return nil
}
