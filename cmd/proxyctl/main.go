// Command proxyctl manages the proxied hostnames declared in routes.yaml
// and drives Caddy (running in Docker Compose) to apply them.
// It must be run from the project root, alongside routes.yaml and docker-compose.yml.
package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"private-proxy/internal/caddyfile"
	"private-proxy/internal/deploy"
	"private-proxy/internal/routes"
)

const routesPath = "routes.yaml"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "add":
		err = runAdd(args)
	case "remove":
		err = runRemove(args)
	case "edit":
		err = runEdit(args)
	case "enable":
		err = runSetEnabled(args, true)
	case "disable":
		err = runSetEnabled(args, false)
	case "list":
		err = runList(args)
	case "validate":
		err = runValidate(args)
	case "apply":
		err = runApply(args)
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

func usage() {
	fmt.Fprint(os.Stderr, `proxyctl - manage proxied hostnames backed by Caddy

Usage:
  proxyctl add <hostname> <ip:port>      Add a new hostname -> upstream route
  proxyctl remove <hostname>             Remove a route
  proxyctl edit <hostname> <ip:port>     Change the upstream of an existing route
  proxyctl enable <hostname>             Re-enable a disabled route
  proxyctl disable <hostname>            Disable a route without deleting it
  proxyctl list                          List all routes and their status
  proxyctl validate                      Render and validate the Caddyfile, without applying it
  proxyctl apply                         Validate, then apply and reload Caddy

Commands other than "apply" only edit routes.yaml; run "apply" to deploy the change.
`)
}

func runAdd(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: proxyctl add <hostname> <ip:port>")
	}
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if err := s.Add(args[0], args[1]); err != nil {
		return err
	}
	if err := routes.Save(routesPath, s); err != nil {
		return err
	}
	fmt.Printf("added %s -> %s (run \"proxyctl apply\" to deploy)\n", args[0], args[1])
	return nil
}

func runRemove(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: proxyctl remove <hostname>")
	}
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if err := s.Remove(args[0]); err != nil {
		return err
	}
	if err := routes.Save(routesPath, s); err != nil {
		return err
	}
	fmt.Printf("removed %s (run \"proxyctl apply\" to deploy)\n", args[0])
	return nil
}

func runEdit(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: proxyctl edit <hostname> <ip:port>")
	}
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if err := s.Edit(args[0], args[1]); err != nil {
		return err
	}
	if err := routes.Save(routesPath, s); err != nil {
		return err
	}
	fmt.Printf("updated %s -> %s (run \"proxyctl apply\" to deploy)\n", args[0], args[1])
	return nil
}

func runSetEnabled(args []string, enabled bool) error {
	if len(args) != 1 {
		verb := "enable"
		if !enabled {
			verb = "disable"
		}
		return fmt.Errorf("usage: proxyctl %s <hostname>", verb)
	}
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if err := s.SetEnabled(args[0], enabled); err != nil {
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

func runList(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: proxyctl list")
	}
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	if len(s.Routes) == 0 {
		fmt.Println("no routes configured")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "HOSTNAME\tUPSTREAM\tSTATUS")
	for _, r := range s.Routes {
		status := "enabled"
		if !r.Enabled {
			status = "disabled"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Hostname, r.Upstream, status)
	}
	return w.Flush()
}

func runValidate(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: proxyctl validate")
	}
	s, err := routes.Load(routesPath)
	if err != nil {
		return err
	}
	content, err := caddyfile.Render(s)
	if err != nil {
		return fmt.Errorf("render Caddyfile: %w", err)
	}
	if err := deploy.WriteNewCaddyfile(content); err != nil {
		return err
	}
	if err := deploy.Validate(deploy.CaddyfileNewPath); err != nil {
		return err
	}
	fmt.Println("Caddyfile is valid")
	return nil
}

func runApply(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: proxyctl apply")
	}
	if err := runValidate(nil); err != nil {
		return err
	}
	if err := deploy.Reload(); err != nil {
		return err
	}
	fmt.Println("applied and reloaded Caddy")
	return nil
}
