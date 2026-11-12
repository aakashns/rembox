#!/bin/sh
# Installs rembox (https://github.com/aakashns/rembox) on macOS or Linux:
#
#   curl -fsSL https://rembox.aakashns.com/install.sh | sh
#
# Downloads the latest release for this machine, checks it against the release's SHA256SUMS, and
# installs it at /usr/local/bin/rembox, with sudo if that folder needs it. Running it again
# upgrades (as does `rembox upgrade`). Settings, as environment variables:
#
#   REMBOX_VERSION      install this version instead of the latest (e.g. 0.1.0)
#   REMBOX_INSTALL_DIR  install here instead of /usr/local/bin
set -eu

# Everything is in main(), called on the last line, so a download cut short runs nothing.
main() {
  repo="https://github.com/aakashns/rembox"
  # Where releases are published. Tests point it at pretend ones (see cli/cmd/rembox/e2e_test.go).
  releases="${REMBOX_RELEASES_URL:-$repo/releases}"
  # Downloads are HTTPS only, even after redirects, unless the tests' pretend releases aren't.
  protocols='=https'
  case "$releases" in http://*) protocols='=http,https' ;; esac
  dir="${REMBOX_INSTALL_DIR:-/usr/local/bin}"

  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "there's no rembox build for $(uname -s); see $repo" ;;
  esac

  case "$(uname -m)" in
    arm64 | aarch64) arch=arm64 ;;
    x86_64 | amd64) arch=amd64 ;;
    *) fail "there's no rembox build for $(uname -m) processors; see $repo" ;;
  esac

  # A shell running under Rosetta reports x86_64 on Apple Silicon Macs.
  if [ "$os-$arch" = darwin-amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = 1 ]; then
    arch=arm64
  fi

  command -v curl >/dev/null || fail "curl is needed to download rembox"

  binary="rembox-$os-$arch"
  if [ -n "${REMBOX_VERSION:-}" ]; then
    version="${REMBOX_VERSION#v}"
    # It goes into a URL, so only a plain version will do.
    case "$version" in
      *[!0-9A-Za-z.-]* | "") fail "not a version: $REMBOX_VERSION" ;;
    esac
    url="$releases/download/v$version"
  else
    url="$releases/latest/download"
  fi

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  echo "Downloading $url/$binary..."
  download "$url/$binary" "$tmp/rembox" || fail "couldn't download $url/$binary"
  download "$url/SHA256SUMS" "$tmp/SHA256SUMS" || fail "couldn't download $url/SHA256SUMS"

  expected=$(awk -v name="$binary" '$2 == name || $2 == "*" name { print $1 }' "$tmp/SHA256SUMS")
  if command -v sha256sum >/dev/null; then
    actual=$(sha256sum "$tmp/rembox" | cut -d ' ' -f 1)
  elif command -v shasum >/dev/null; then
    actual=$(shasum -a 256 "$tmp/rembox" | cut -d ' ' -f 1)
  else
    fail "sha256sum or shasum is needed to check the download"
  fi
  if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
    fail "the download doesn't match its checksum; please try again"
  fi
  chmod 755 "$tmp/rembox"

  # /usr/local/bin usually belongs to root.
  sudo=
  if [ "$(id -u)" != 0 ] && ! { mkdir -p "$dir" 2>/dev/null && [ -w "$dir" ]; }; then
    command -v sudo >/dev/null || fail "can't write to $dir; run this as root"
    echo "Installing at $dir/rembox needs sudo"
    sudo=sudo
  fi

  # Copied next to the destination, then renamed over it, so an existing rembox is replaced in one
  # step and never left half-written.
  $sudo mkdir -p "$dir" || fail "couldn't create $dir"
  $sudo cp "$tmp/rembox" "$dir/.rembox.new" || fail "couldn't write to $dir"
  $sudo mv -f "$dir/.rembox.new" "$dir/rembox"

  echo "Installed $("$dir/rembox" version) at $dir/rembox"

  command=rembox
  found=$(command -v rembox || true)
  if [ -z "$found" ]; then
    echo "Note: $dir isn't on your PATH, so run it as $dir/rembox."
    command="$dir/rembox"
  elif [ "$found" != "$dir/rembox" ]; then
    echo "Note: running \`rembox\` starts $found, which comes first on your PATH."
    command="$dir/rembox"
  fi
  echo "Run \`$command help\` to get started."
}

fail() {
  printf 'rembox install: %s\n' "$*" >&2
  exit 1
}

download() {
  curl -fsSL --proto "$protocols" --proto-redir "$protocols" "$1" -o "$2"
}

main "$@"
