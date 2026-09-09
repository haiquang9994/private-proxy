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

## Usage

```
proxyctl add <hostname> <ip:port>      Add a new hostname -> upstream route
proxyctl remove <hostname>             Remove a route
proxyctl edit <hostname> <ip:port>     Change the upstream of an existing route
proxyctl enable <hostname>             Re-enable a disabled route
proxyctl disable <hostname>            Disable a route without deleting it
proxyctl list                          List all routes and their status
proxyctl validate                      Render and validate the Caddyfile, without applying it
proxyctl apply                         Validate, then apply and reload Caddy
```

`add`, `remove`, `edit`, `enable`, and `disable` only edit `routes.yaml`.
Nothing is deployed until you run `proxyctl apply`, which regenerates the
Caddyfile, validates it inside the running container, backs up the previous
Caddyfile to `Caddyfile.bak`, and reloads Caddy with zero downtime. If
validation fails, the live Caddyfile is left untouched.

## Example

```
proxyctl add app.example.com 10.0.0.5:8080
proxyctl apply
```
