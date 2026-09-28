#!/bin/sh
# Installs proxyctl and starts the Caddy proxy on a fresh Linux server.
#
#   curl -fsSL https://raw.githubusercontent.com/haiquang9994/private-proxy/master/install.sh | sudo sh
#
# Pin a release with: ... | sudo VERSION=v1.2.0 sh
set -eu

REPO="haiquang9994/private-proxy"

die() {
	echo "error: $*" >&2
	exit 1
}

# Everything runs inside main, called on the last line, so a download cut
# short by `curl | sh` runs nothing instead of a partial script.
main() {
	# --- Prerequisites: fail before downloading anything. ---

	[ "$(id -u)" -eq 0 ] || die "must be run as root (pipe into 'sudo sh')"
	[ "$(uname -s)" = "Linux" ] || die "only Linux is supported"
	if [ -e /opt/private-proxy ] || [ -L /opt/private-proxy ]; then
		die "/opt/private-proxy already exists; refusing to overwrite an existing install"
	fi

	if command -v curl >/dev/null 2>&1; then
		fetch() { curl -fsSL -o "$2" "$1"; }
	elif command -v wget >/dev/null 2>&1; then
		fetch() { wget -qO "$2" "$1"; }
	else
		die "curl or wget is required"
	fi
	command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required"
	command -v docker >/dev/null 2>&1 || die "docker is required: install Docker first (https://docs.docker.com/engine/install/)"
	docker compose version >/dev/null 2>&1 || die "the docker compose plugin is required"

	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "unsupported architecture: $(uname -m)" ;;
	esac

	version="${VERSION:-latest}"
	if [ "$version" = "latest" ]; then
		base_url="https://github.com/$REPO/releases/latest/download"
	else
		base_url="https://github.com/$REPO/releases/download/$version"
	fi
	asset="proxyctl-linux-$arch"

	# --- Download and verify. ---

	tmp_dir="$(mktemp -d)"
	cleanup() {
		rm -f "$tmp_dir/$asset" "$tmp_dir/checksums.txt"
		rmdir "$tmp_dir"
	}
	trap cleanup EXIT

	echo "Downloading $asset ($version)..."
	fetch "$base_url/$asset" "$tmp_dir/$asset" || die "download failed: $base_url/$asset"
	fetch "$base_url/checksums.txt" "$tmp_dir/checksums.txt" || die "download failed: $base_url/checksums.txt"
	(cd "$tmp_dir" && grep " $asset\$" checksums.txt | sha256sum -c -) >/dev/null ||
		die "checksum mismatch for $asset"

	# --- Install and start. ---

	install -m 0755 "$tmp_dir/$asset" /usr/local/bin/proxyctl
	PROXYCTL_ROOT=/opt/private-proxy /usr/local/bin/proxyctl init

	if ! (cd /opt/private-proxy && docker compose up -d); then
		die "starting Caddy failed; fix the problem above, then run: cd /opt/private-proxy && docker compose up -d"
	fi

	cat <<-EOF

	Installed proxyctl $(/usr/local/bin/proxyctl version); Caddy is running from /opt/private-proxy.

	Tab completion: add one line to root's shell rc file, then open a new shell:
	  bash: echo 'source <(proxyctl completion bash)' >> ~/.bashrc
	  zsh:  echo 'source <(proxyctl completion zsh)'  >> ~/.zshrc

	Get started:
	  proxyctl add app.example.com 127.0.0.1:8080
	  proxyctl apply
	EOF
}

main "$@"
