# private-proxy

Manages a Caddy reverse proxy via a declarative `routes.yaml` and a `proxyctl`
CLI. Intended to run at the root of a server: Caddy runs in Docker Compose,
`proxyctl` runs directly on the host.

## Setup

```
docker compose up -d
go build -o bin/proxyctl ./cmd/proxyctl
```

`proxyctl` must be built to `bin/proxyctl` under the project root, exactly as
above -- at startup it resolves the project root from its own binary path
(parent of the `bin/` directory it lives in), so it works correctly even when
invoked from another directory or with `bin/` added to `PATH`.

Caddy's admin API listens on a unix socket inside the container
(`CADDY_ADMIN` in `docker-compose.yml`), not on `localhost:2019`: with host
networking, a TCP admin port would let any local process on the server
rewrite the proxy config. `proxyctl` reaches it through `docker compose exec`,
so nothing else needs configuring.

When upgrading an existing server to this setup, recreate the container once
so it picks up the new environment (`docker compose restart` is not enough):

```
docker compose up -d
```

## Building locally and deploying to the server

The server runs Linux, so if you build on a different OS you need to
cross-compile. Run this from the project root.

macOS/Linux (bash/zsh):

```sh
GOOS=linux GOARCH=amd64 go build -o bin/proxyctl ./cmd/proxyctl
scp bin/proxyctl user@server:/opt/private-proxy/bin/proxyctl
```

Windows (PowerShell):

```powershell
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -o bin/proxyctl .\cmd\proxyctl
scp bin/proxyctl user@server:/opt/private-proxy/bin/proxyctl
```

If you're building directly on the server (Linux on Linux), a plain
`go build -o bin/proxyctl ./cmd/proxyctl` is enough -- no `GOOS`/`GOARCH`
needed.

## Usage

```
proxyctl add <hostname>[/path] <ip:port>      Add a new route -> upstream
proxyctl remove <hostname>[/path]             Remove a route
proxyctl edit <hostname>[/path] <ip:port>     Change the upstream of an existing route
proxyctl enable <hostname>[/path]             Re-enable a disabled route
proxyctl disable <hostname>[/path]            Disable a route without deleting it
proxyctl list                                 List all routes and their status
proxyctl validate                             Render and validate the Caddyfile, without applying it
proxyctl apply                                Validate, then apply and reload Caddy
proxyctl completion bash|zsh                  Print the shell tab-completion script
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
