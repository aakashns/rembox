# opentunnel.sh: installs opentunnel, which gives a dev server on the server a
# public HTTPS link, so you can test it from your phone. Its own installer puts
# it in ~/.local/bin; running setup again asks before upgrading it.

# setup_opentunnel: installs the latest opentunnel, or upgrades it, then has it
# create the tunnel, with its certificate, which can take a few minutes, and
# run its service, which systemd starts at boot. Then an agent's first link is
# ready at once. Runs as your user.
setup_opentunnel() {
  step "Install the latest opentunnel, and connect its tunnel"
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
  if ! "$bin" status 2>/dev/null | grep -q '^Service: connected'; then
    if ! run "$bin" up; then
      warn "opentunnel couldn't connect, as it says above. To try again: opentunnel up"
      checklist warn "opentunnel $have, not connected"
      return
    fi
  fi
  ok "opentunnel $have is installed, and its tunnel is connected."
  checklist ok "opentunnel $have, for dev server links"
}
