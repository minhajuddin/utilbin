#!/usr/bin/env bash
# utilbin installer — fetches the latest release tarball for the current
# OS/arch, extracts the binary into ~/.local/bin/, and runs `utilbin install`
# to create the per-command symlink farm.
#
# Usage:  curl -fsSL https://raw.githubusercontent.com/minhajuddin/utilbin/main/install.sh | bash
#         INSTALL_DIR=/usr/local/bin curl ... | sudo -E bash    # system-wide
#         UTILBIN_VERSION=v0.1.0 curl ... | bash                # pin a version

set -euo pipefail

REPO="${UTILBIN_REPO:-minhajuddin/utilbin}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${UTILBIN_VERSION:-latest}"

err() { echo "utilbin install: $*" >&2; exit 1; }
say() { echo "utilbin install: $*"; }

case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *)      err "unsupported OS: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64)        arch=amd64 ;;
  aarch64|arm64)       arch=arm64 ;;
  *)                   err "unsupported arch: $(uname -m)" ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep -m1 '"tag_name"' \
    | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
  [[ -n "$tag" ]] || err "could not determine latest tag from GitHub"
else
  tag="$VERSION"
fi

asset="utilbin_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$tag/$asset"

say "downloading $tag for $os/$arch"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -o "$tmp/$asset" "$url" || err "download failed: $url"
curl -fsSL -o "$tmp/checksums.txt" "https://github.com/$REPO/releases/download/$tag/checksums.txt" \
  || say "warning: no checksums.txt — skipping verification"

if [[ -f "$tmp/checksums.txt" ]]; then
  (cd "$tmp" && grep " $asset\$" checksums.txt | sha256sum -c -) || err "checksum mismatch"
fi

tar -C "$tmp" -xzf "$tmp/$asset"
[[ -f "$tmp/utilbin" ]] || err "tarball did not contain a utilbin binary"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/utilbin" "$INSTALL_DIR/utilbin"
say "installed $INSTALL_DIR/utilbin ($tag)"

"$INSTALL_DIR/utilbin" install --link-dir "$INSTALL_DIR"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) say "warning: $INSTALL_DIR is not on your PATH — add it to your shell rc" ;;
esac
