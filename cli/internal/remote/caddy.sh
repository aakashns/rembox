# caddy.sh: has apt install Caddy from Caddy's own apt repo.

# Where Caddy's docs, at caddyserver.com/docs/install, put them.
CADDY_KEYRING=/usr/share/keyrings/caddy-stable-archive-keyring.gpg
CADDY_SOURCES=/etc/apt/sources.list.d/caddy-stable.list
CADDY_REPO=https://dl.cloudsmith.io/public/caddy/stable

# add_caddy_repo: has apt install Caddy from Caddy's stable apt repo, on
# Cloudsmith, as the distros' lags behind (2.6 on Debian 12 and Ubuntu 24.04),
# the way Caddy's docs do: its key, dearmored with gpg, and its sources list,
# as it is. It goes before update, and install_basics installs caddy from it.
add_caddy_repo() {
  step "Install Caddy from Caddy's apt repo, which has its latest version"
  local dir changed=no
  if ! command -v gpg >/dev/null; then
    run "${APT[@]}" update
    install_packages gpg
  fi
  dir=$(mktemp -d)
  # The key and list may change, so they're downloaded again each time.
  if ! run curl -fsSL -o "$dir/key.asc" "$CADDY_REPO/gpg.key" ||
    ! run curl -fsSL -o "$dir/caddy.list" "$CADDY_REPO/debian.deb.txt"; then
    rm -rf "$dir"
    warn "Couldn't download the key or sources list for Caddy's apt repo, so Caddy comes from the distro, or stays as it is. Run rembox setup again later."
    return
  fi
  run gpg --dearmor -o "$dir/key.gpg" "$dir/key.asc"
  if ! cmp -s "$dir/key.gpg" "$CADDY_KEYRING"; then
    [[ -d ${CADDY_KEYRING%/*} ]] || run install -d -m 755 "${CADDY_KEYRING%/*}"
    run install -m 644 "$dir/key.gpg" "$CADDY_KEYRING"
    changed=yes
  fi
  if ! cmp -s "$dir/caddy.list" "$CADDY_SOURCES"; then
    run install -m 644 "$dir/caddy.list" "$CADDY_SOURCES"
    changed=yes
  fi
  rm -rf "$dir"
  if [[ $changed == yes ]]; then
    ok "apt installs Caddy from Caddy's apt repo, and update upgrades it."
  else
    ok "apt installs Caddy from Caddy's apt repo already."
  fi
}
