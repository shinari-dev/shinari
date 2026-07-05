#!/bin/sh
# SPDX-FileCopyrightText: 2026 The Shinari Authors
# SPDX-License-Identifier: Apache-2.0
#
# Install script for shinari.
#
#   curl -sSL https://shinari.dev/install.sh | sh
#
# Environment overrides:
#   SHINARI_VERSION  install a specific release tag (e.g. v0.2.0); default: latest
#   BINDIR           install directory; default: /usr/local/bin
set -eu

REPO="shinari-dev/shinari"
BINARY="shinari"
BINDIR="${BINDIR:-/usr/local/bin}"

fail() {
  echo "install: $1" >&2
  exit 1
}

# --- detect OS ---
os=$(uname -s)
case "$os" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)      fail "unsupported OS '$os' — download manually from https://github.com/$REPO/releases" ;;
esac

# --- detect arch ---
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64)  arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *)               fail "unsupported architecture '$arch' — download manually from https://github.com/$REPO/releases" ;;
esac

# --- resolve version ---
version="${SHINARI_VERSION:-}"
if [ -z "$version" ]; then
  version=$(curl -sSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name" *: *"([^"]+)".*/\1/')
  [ -n "$version" ] || fail "could not resolve latest version from the GitHub API"
fi

# Normalize to the 'v'-prefixed tag form so SHINARI_VERSION=0.2.0 and v0.2.0 both work.
case "$version" in
  v*) ;;
  *)  version="v$version" ;;
esac

# GoReleaser archive names use the plain version without a leading 'v'.
plain_version=$(echo "$version" | sed 's/^v//')
archive="${BINARY}_${plain_version}_${os}_${arch}.tar.gz"
base_url="https://github.com/$REPO/releases/download/$version"

echo "install: downloading $archive ($version)"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -sSLf "$base_url/$archive"      -o "$tmp/$archive"      || fail "download failed: $base_url/$archive"
curl -sSLf "$base_url/checksums.txt" -o "$tmp/checksums.txt" || fail "checksums download failed"

# --- verify checksum ---
expected=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
[ -n "$expected" ] || fail "no checksum entry for $archive"

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
else
  fail "no sha256 tool (sha256sum or shasum) available"
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive (expected $expected, got $actual)"

# --- extract & install ---
tar -xzf "$tmp/$archive" -C "$tmp" "$BINARY" || fail "failed to extract $BINARY from $archive"

if [ -w "$BINDIR" ]; then
  install -m 0755 "$tmp/$BINARY" "$BINDIR/$BINARY"
else
  echo "install: $BINDIR is not writable, retrying with sudo"
  sudo install -m 0755 "$tmp/$BINARY" "$BINDIR/$BINARY"
fi

echo "install: installed $BINARY $version to $BINDIR/$BINARY"
