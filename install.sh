#!/bin/sh
set -eu

fail() { printf '%s\n' "tsp install: $*" >&2; exit 1; }
[ "$(uname -s)" = Linux ] || fail 'Linux only; alternatively: go install github.com/alesierraalta/tsp/cmd/tsp@latest'
arch=$(uname -m)
case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) fail "unsupported architecture: $arch" ;;
esac
if command -v curl >/dev/null 2>&1; then
    download() { curl -fsSL "$1" -o "$2"; }
    latest() { curl -fsSL -o /dev/null -w '%{url_effective}' https://github.com/alesierraalta/tsp/releases/latest; }
elif command -v wget >/dev/null 2>&1; then
    download() { wget -q -O "$2" "$1"; }
    latest() { wget --spider --server-response https://github.com/alesierraalta/tsp/releases/latest 2>&1 | awk '/^[[:space:]]*Location:/ {url=$2} END {print url}'; }
else
    fail 'curl or wget is required'
fi
version=${TSP_VERSION:-}
if [ -z "$version" ]; then
    url=$(latest)
    version=${url##*/}
fi
case "$version" in v[0-9]*) ;; *) fail "invalid release version: $version" ;; esac
case "$version" in *[!a-zA-Z0-9._-]*) fail "invalid release version: $version" ;; esac
archive=tsp_${version#v}_linux_${arch}.tar.gz
tmp=$(mktemp -d)
staged=
trap 'rm -rf "$tmp"; if [ -n "$staged" ]; then rm -f "$staged"; fi' 0
trap 'exit 1' HUP INT TERM
base=${TSP_RELEASE_BASE_URL:-https://github.com/alesierraalta/tsp/releases/download}
for file in "$archive" checksums.txt; do
    download "$base/$version/$file" "$tmp/$file" || fail "download failed: $base/$version/$file (is $version published for linux/$arch?)"
done
expected=$(awk -v name="$archive" '$2 == name {print $1; n++} END {if(n != 1) exit 1}' "$tmp/checksums.txt") || fail 'missing or duplicate checksum'
if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$tmp/$archive")
elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$tmp/$archive")
else
    fail 'sha256sum or shasum is required'
fi
actual=${actual%% *}
[ "$expected" = "$actual" ] || fail 'SHA-256 checksum mismatch'
tar -xzf "$tmp/$archive" -C "$tmp" tsp
if [ ! -f "$tmp/tsp" ] || [ -L "$tmp/tsp" ]; then
    fail 'archive has no regular tsp binary'
fi
dest=${TSP_INSTALL_DIR:-$HOME/.local/bin}
mkdir -p "$dest"
dest=$(cd "$dest" && pwd -P)
staged=$(mktemp "$dest/.tsp-install.XXXXXX")
cp "$tmp/tsp" "$staged"
chmod 755 "$staged"
mv -f "$staged" "$dest/tsp"
staged=
case ":${PATH:-}:" in
    *:"$dest":*) ;;
    *) quoted=$(printf '%s' "$dest" | sed "s/'/'\\\\''/g")
       printf "Add this to ~/.profile:\nexport PATH='%s':\"\$PATH\"\n" "$quoted" ;;
esac
"$dest/tsp" version
if [ "${TSP_NO_SETUP:-0}" != 1 ]; then
    "$dest/tsp" setup
fi
