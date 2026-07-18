#!/bin/sh
# VARA installer — downloads a prebuilt `vara` binary from GitHub Releases and
# installs it onto your PATH. Linux and macOS (amd64/arm64).
#
#   curl -fsSL https://raw.githubusercontent.com/thulasiramk-2310/vara/main/scripts/install.sh | sh
#
# Environment overrides:
#   VARA_VERSION   release tag to install (default: latest, e.g. v0.3.0)
#   VARA_INSTALL   install directory   (default: /usr/local/bin, else ~/.local/bin)
set -eu

REPO="thulasiramk-2310/vara"
BIN="vara"

err() { echo "install: $*" >&2; exit 1; }

# --- detect platform ---------------------------------------------------------
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac
case "$os" in
  linux | darwin) ;;
  *) err "unsupported OS: $os — on Windows download the .zip from the releases page" ;;
esac

# --- resolve version ---------------------------------------------------------
version="${VARA_VERSION:-latest}"
if [ "$version" = "latest" ]; then
  version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -n1 | cut -d '"' -f4)
  [ -n "$version" ] || err "could not determine the latest release — set VARA_VERSION"
fi
num="${version#v}" # archive names omit the leading v (e.g. 0.3.0)

archive="${BIN}_${num}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$version/$archive"

# --- choose install dir ------------------------------------------------------
if [ -n "${VARA_INSTALL:-}" ]; then
  dest="$VARA_INSTALL"
elif [ -w /usr/local/bin ] 2>/dev/null; then
  dest=/usr/local/bin
else
  dest="$HOME/.local/bin"
fi
mkdir -p "$dest"

# --- download & install ------------------------------------------------------
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "install: fetching $version ($os/$arch)"
curl -fsSL "$url" -o "$tmp/$archive" || err "download failed: $url"
tar -xzf "$tmp/$archive" -C "$tmp" || err "extract failed"
install -m 0755 "$tmp/$BIN" "$dest/$BIN" 2>/dev/null || {
  mv "$tmp/$BIN" "$dest/$BIN" && chmod 0755 "$dest/$BIN"
}

echo "install: installed $BIN $version -> $dest/$BIN"
case ":$PATH:" in
  *":$dest:"*) ;;
  *) echo "install: note: $dest is not on your PATH — add it:"
     echo "         export PATH=\"$dest:\$PATH\"" ;;
esac
"$dest/$BIN" --version
