# private-proxy

Manages a Caddy reverse proxy via a declarative `routes.yaml` and a `proxyctl`
CLI. Intended to run at the root of a server: Caddy runs in Docker Compose,
`proxyctl` runs directly on the host.

## Install

On a Linux server (amd64 or arm64) with Docker and the compose plugin:

```bash
curl -fsSL https://raw.githubusercontent.com/haiquang9994/private-proxy/master/install.sh | sudo sh
```

This installs `proxyctl` to `/usr/local/bin`, creates `/opt/private-proxy`
(`docker-compose.yml`, `routes.yaml`, `Caddyfile`, readable by root only) and
starts Caddy. Pin a release with `... | sudo VERSION=v0.1.0 sh`.

The script is for fresh installs only: it refuses to run if
`/opt/private-proxy` already exists. It does not install Docker.

`proxyctl` must be run as root (except `help`, `version` and `completion`).

Caddy's admin API listens on a unix socket inside the container
(`CADDY_ADMIN` in `docker-compose.yml`), not on `localhost:2019`: with host
networking, a TCP admin port would let any local process on the server
rewrite the proxy config. `proxyctl` reaches it through `docker compose exec`,
so nothing else needs configuring.

### Project root

`proxyctl` looks for its files in, first match wins:

1. `$PROXYCTL_ROOT`, if set;
2. the repo clone it was built into (`<clone>/bin/proxyctl`, next to
   `docker-compose.yml`) -- see [Building from source](#building-from-source);
3. `/opt/private-proxy`.

`proxyctl init` creates the root and its files. It always refreshes
`docker-compose.yml` and never overwrites an existing `routes.yaml` or
`Caddyfile`.

## Building from source

Needs Go. In a clone of this repo:

```
docker compose up -d
go build -o bin/proxyctl ./cmd/proxyctl
```

Built to `bin/proxyctl`, the binary uses the clone as its project root. To
build for a Linux server from another OS, cross-compile:

```sh
GOOS=linux GOARCH=amd64 go build -o bin/proxyctl ./cmd/proxyctl
```

## Releasing

Push a `v*` tag; the `release` workflow tests, builds `proxyctl-linux-amd64`
and `proxyctl-linux-arm64`, and publishes them with `checksums.txt`:

```
git tag v0.1.0
git push origin v0.1.0
```

## Usage

```
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
```

### Tab completion

Add one line to your shell's rc file, then open a new shell:

```bash
# ~/.bashrc
source <(proxyctl completion bash)

# ~/.zshrc
source <(proxyctl completion zsh)
```

Tab then completes command names, existing routes for `remove`/`edit`, only
disabled routes for `enable`, only enabled routes for `disable`, and the
current upstream as the second argument of `edit`. Suggestions are read live
from `routes.yaml`, so they always match what's configured.

Route suggestions need read access to `routes.yaml`, so they only work in a
root shell (`sudo -i`); `sudo proxyctl <TAB>` from a normal user completes
command names only.

`add`, `remove`, `edit`, `enable`, and `disable` only edit `routes.yaml`.
Nothing is deployed until you run `proxyctl apply`, which regenerates the
Caddyfile, validates it inside the running container, backs up the previous
Caddyfile to `Caddyfile.bak`, and reloads Caddy with zero downtime. If
validation fails, the live Caddyfile is left untouched.

A route can optionally be scoped to a path: `example.com` is the catch-all
for that hostname, while `example.com/ws` only matches `/ws` and everything
under it (e.g. `/ws/socket`), leaving the rest of the hostname's traffic to
its catch-all route. Routes on the same hostname are independent: `remove`,
`enable`, and `disable` on `example.com` only affect that hostname's
catch-all route, not its path routes.

## Example

```
proxyctl add app.example.com 10.0.0.5:8080
proxyctl apply
```

## Example: path-based routing

```
proxyctl add example.com 10.10.10.1:8080
proxyctl add example.com/ws 10.10.10.1:6001
proxyctl apply
```
