// Command proxyctl manages the proxied hostnames declared in routes.yaml
// and drives Caddy (running in Docker Compose) to apply them.
//
// It can be invoked from any working directory (e.g. with its bin/
// directory added to PATH): the project root is always resolved from the
// location of the binary itself, which must live at <project_root>/bin/proxyctl.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"private-proxy/internal/caddyfile"
	"private-proxy/internal/deploy"
	"private-proxy/internal/routes"
)

const routesFileName = "routes.yaml"

func main() {
	root, err := projectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "add":
		err = runAdd(root, args)
	case "remove":
		err = runRemove(root, args)
	case "edit":
		err = runEdit(root, args)
	case "enable":
		err = runSetEnabled(root, args, true)
	case "disable":
		err = runSetEnabled(root, args, false)
	case "list":
		err = runList(root, args)
	case "validate":
		err = runValidate(root, args)
	case "apply":
		err = runApply(root, args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// projectRoot resolves the project root from the location of the running
// binary, so proxyctl behaves the same regardless of the caller's current
// working directory. The binary is expected to live at <project_root>/bin/proxyctl.
func projectRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate proxyctl executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve proxyctl executable path: %w", err)
	}

	binDir := filepath.Dir(exe)
	if filepath.Base(binDir) == "bin" {
		return filepath.Dir(binDir), nil
	}
	return binDir, nil
}

func usage() {
	fmt.Fprint(os.Stderr, `proxyctl - manage proxied hostnames backed by Caddy

Usage:
  proxyctl add <hostname>[/path] <ip:port>      Add a new route -> upstream
  proxyctl remove <hostname>[/path]             Remove a route
  proxyctl edit <hostname>[/path] <ip:port>     Change the upstream of an existing route
  proxyctl enable <hostname>[/path]             Re-enable a disabled route
  proxyctl disable <hostname>[/path]            Disable a route without deleting it
  proxyctl list                                 List all routes and their status
  proxyctl validate                             Render and validate the Caddyfile, without applying it
  proxyctl apply                                Validate, then apply and reload Caddy

Commands other than "apply" only edit routes.yaml; run "apply" to deploy the change.

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

func runAdd(root string, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: proxyctl add <hostname>[/path] <ip:port>")
	}
	hostname, path := splitHostPath(args[0])
	routesPath := filepath.Join(root, routesFileName)
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if err := s.Add(hostname, path, args[1]); err != nil {
		return err
	}
	if err := routes.Save(routesPath, s); err != nil {
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
	routesPath := filepath.Join(root, routesFileName)
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if err := s.Remove(hostname, path); err != nil {
		return err
	}
	if err := routes.Save(routesPath, s); err != nil {
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
	routesPath := filepath.Join(root, routesFileName)
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if err := s.Edit(hostname, path, args[1]); err != nil {
		return err
	}
	if err := routes.Save(routesPath, s); err != nil {
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
	routesPath := filepath.Join(root, routesFileName)
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if err := s.SetEnabled(hostname, path, enabled); err != nil {
		return err
	}
	if err := routes.Save(routesPath, s); err != nil {
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

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "HOSTNAME\tPATH\tUPSTREAM\tSTATUS")
	for _, r := range s.Routes {
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

func runValidate(root string, args []string) error {
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
	if err := deploy.Validate(root); err != nil {
		return err
	}
	fmt.Println("Caddyfile is valid")
	return nil
}

func runApply(root string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: proxyctl apply")
	}
	if err := runValidate(root, nil); err != nil {
		return err
	}
	if err := deploy.Reload(root); err != nil {
		return err
	}
	fmt.Println("applied and reloaded Caddy")
	return nil
}
