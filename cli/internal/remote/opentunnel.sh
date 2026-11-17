# opentunnel.sh: installs opentunnel, which gives a dev server on the server a
# public HTTPS link, so you can test it from your phone. Its own installer puts
# it in ~/.local/bin; running setup again asks before upgrading it.

# setup_opentunnel: installs the latest opentunnel, or upgrades it. Runs as
# your user. Its service starts when an agent first adds a route.
setup_opentunnel() {
  step "Install the latest opentunnel"
  ((EUID != 0)) || fail "Run this as your user, not root."
  local bin=$HOME/.local/bin/opentunnel have latest
  have=$(version_of "$bin")
  latest=$(npm_latest opentunnel)
  if wants_latest opentunnel "$have" "$latest"; then
    run_installer https://opentunnel.xyz/install sh SCRIPT ||
      warn "opentunnel couldn't install, as it says above. Run rembox setup again later."
  fi
  have=$(version_of "$bin")
  if [[ -z $have ]]; then
    checklist warn "opentunnel didn't install"
    return
  fi
  ok "opentunnel $have is installed."
  checklist ok "opentunnel $have, for dev server links"
}
