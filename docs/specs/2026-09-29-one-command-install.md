# Spec: One-Command Install

- **Date:** 2026-09-29
- **Status:** Approved

## Goal

On a Linux server that already has Docker, a user runs exactly one command:

```bash
curl -fsSL https://raw.githubusercontent.com/haiquang9994/private-proxy/master/install.sh | sudo sh
```

Afterwards:
- `proxyctl` is on `PATH` (`/usr/local/bin/proxyctl`);
- project data lives in `/opt/private-proxy` (`docker-compose.yml`, `routes.yaml`, `Caddyfile`);
- the Caddy container is running;
- the script prints the line to add to `~/.bashrc` / `~/.zshrc` to enable tab completion.

No Go toolchain and no repo clone are needed on the server. The script is for
fresh installs only: if `/opt/private-proxy` already exists it fails immediately.

## Non-Goals

- Installing Docker. A missing `docker` or `docker compose` is an error.
- Configuring tab completion automatically. `proxyctl completion bash|zsh` stays
  as it is; the script only prints instructions.
- Upgrading an existing install (the script refuses to run when
  `/opt/private-proxy` exists).
- `.deb`/`.rpm` packages, automated uninstall, non-Linux servers.

## Decisions

| Topic | Decision |
|---|---|
| Data directory | `/opt/private-proxy` |
| Architectures | `linux/amd64` and `linux/arm64` |
| Docker missing | Fail with an error, do not install it |
| Who may use proxyctl | root only |
| Tab completion | Keep current mechanism, print setup instructions after install |
| Existing `/opt/private-proxy` | Fail immediately, before downloading anything |

## Design

### 1. Release binaries (GitHub Actions)

`.github/workflows/release.yml`, triggered by pushing a `v*` tag:
1. `go test ./...`
2. Build with `CGO_ENABLED=0 GOOS=linux` for `amd64` and `arm64`, stamping the
   version via `-ldflags "-s -w -X main.version=<tag>"`.
3. Generate `checksums.txt` with `sha256sum`.
4. Create a GitHub Release with assets `proxyctl-linux-amd64`,
   `proxyctl-linux-arm64`, `checksums.txt`.

Asset names are fixed, so the installer downloads them from
`https://github.com/haiquang9994/private-proxy/releases/latest/download/<asset>`
(or `.../releases/download/<tag>/<asset>`) without calling the GitHub API.

### 2. `proxyctl` changes

**a. Embedded compose file.** A new root-level package (`assets.go`, package
`privateproxy`) embeds `docker-compose.yml` with `//go:embed`. This is the
repo's own file, not a copy. `routes.yaml` is **not** embedded (the repo copy may
hold a developer's real routes); `init` generates it with `routes.Save` on an
empty store instead.

**b. Project root resolution**, first match wins:
1. `PROXYCTL_ROOT` environment variable, if non-empty (made absolute).
2. Legacy layout: the binary's directory (or its parent, if the directory is
   named `bin`) contains `docker-compose.yml`. Keeps source builds and existing
   clone-based servers working.
3. Default `/opt/private-proxy`.

For `/usr/local/bin/proxyctl`, step 2 does not match (`/usr/local` has no
`docker-compose.yml`), so step 3 applies. Every command except `init` fails if
the resolved root has no `docker-compose.yml`, with a hint to run
`proxyctl init`.

**c. Commands that do not need a project root.** `help`, `version`,
`completion` and `__complete` work without an initialized root and without root
privileges. `__complete` uses the resolved root without checking it; if
`routes.yaml` is missing or unreadable it silently suggests no routes (current
behavior).

**d. New command `proxyctl init`:**
- Creates the root directory with mode `0700` if missing.
- `docker-compose.yml`: always (re)written from the embedded copy -- the file is
  owned by proxyctl.
- `routes.yaml`: created only if missing, never overwritten.
- `Caddyfile`: created only if missing, rendered from `routes.yaml` (Caddy needs
  it to start).
- Runs under the same project-wide lock as other mutating commands.
- Does not call Docker; `install.sh` runs `docker compose up -d`.

**e. New command `proxyctl version`:** prints the stamped version (`dev` for
local builds).

**f. Root only.** Every command except those in 2c checks `os.Geteuid() == 0`
(skipped on Windows) and otherwise fails with
`proxyctl must be run as root (try sudo)`. The `0700` root directory also keeps
non-root users from reading or editing the files directly.

Consequence: route suggestions in tab completion only work in a root shell
(`sudo -i`). A normal user typing `sudo proxyctl <TAB>` gets command names only,
because `__complete` runs as that user and cannot read `routes.yaml`.

### 3. `install.sh` (POSIX sh, repo root)

1. Fail immediately, before downloading anything, if: not root, not Linux,
   `/opt/private-proxy` exists (file, directory or symlink), neither `curl` nor
   `wget` is available, `sha256sum` is missing, `docker` is missing, or
   `docker compose version` fails. Message for an existing install:
   `/opt/private-proxy already exists; refusing to overwrite an existing install`.
2. Map `uname -m`: `x86_64`/`amd64` -> `amd64`, `aarch64`/`arm64` -> `arm64`,
   anything else -> error.
3. Version: `VERSION` env var (e.g. `VERSION=v1.2.0`), default latest.
4. Download the binary and `checksums.txt` into a `mktemp -d` directory and
   verify with `sha256sum -c`. Cleanup removes the known files and `rmdir`s the
   directory -- no `rm -rf`.
5. `install -m 0755` to `/usr/local/bin/proxyctl`.
6. `PROXYCTL_ROOT=/opt/private-proxy proxyctl init`.
7. `docker compose up -d` in `/opt/private-proxy`. On failure, print how to retry
   that single step (the directory now exists, so re-running the script would be
   refused).
8. Print the installed version and:
   ```
   Tab completion: add one line to root's shell rc file, then open a new shell:
     bash: echo 'source <(proxyctl completion bash)' >> ~/.bashrc
     zsh:  echo 'source <(proxyctl completion zsh)'  >> ~/.zshrc
   ```

The script uses `set -eu` and literal paths for every filesystem write.

### 4. Existing clone-based servers

Such servers already have `/opt/private-proxy`, so `install.sh` refuses to run.
They keep using `bin/proxyctl` (root resolution step 2b). Migrating them is out
of scope.

### 5. README

Replace Setup with the one-line install; move building from source to a
secondary section; document releasing (`git tag v0.1.0 && git push origin v0.1.0`)
and the root-only rule.

## Testing

- Go unit tests: root resolution (env / legacy / default), missing-root error
  hint, `init` (preserves `routes.yaml` and `Caddyfile`, always writes
  `docker-compose.yml`), root privilege check.
- `install.sh`: `shellcheck`; prerequisite checks in a plain `ubuntu` container;
  full run on a test server against a real release.
