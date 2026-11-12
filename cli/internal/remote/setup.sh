# setup.sh: gets the server ready for development. It checks that rembox can
# set the server up, installs the latest version of every package with apt,
# then, if an update needs it, asks to restart the server. rembox runs it as
# root. Running it again is harmless: each step changes only what needs
# changing.

# check: checks that rembox can set up the server, and sums it up.
check() {
  step "Check that rembox can set up this server"
  local ID='' PRETTY_NAME='an unknown system'
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
  fi
  local arch
  if command -v dpkg >/dev/null; then
    arch=$(dpkg --print-architecture) # Debian's names, like arm64 and amd64
  else
    arch=$(uname -m)
  fi

  local problems=()
  [[ $ID == ubuntu || $ID == debian ]] ||
    problems+=("rembox sets up Ubuntu and Debian servers, and this one runs $PRETTY_NAME.")
  command -v apt-get >/dev/null || problems+=("It has no apt-get, which rembox installs packages with.")
  [[ -d /run/systemd/system ]] || problems+=("It doesn't run systemd, which rembox runs services with.")
  [[ $arch == arm64 || $arch == amd64 ]] || problems+=("Its processor is $arch; rembox supports arm64 and amd64.")
  if ((${#problems[@]} > 0)); then
    fail "${problems[@]}"
  fi

  local memory disk
  memory=$(awk '/^MemTotal:/ { printf "%.1f", $2 / 1048576 }' /proc/meminfo)
  disk=$(df -Pk / | awk 'NR == 2 { printf "%.1f", $4 / 1048576 }')
  ok "$PRETTY_NAME, $arch, $(plural "$(nproc)" core), $memory GiB memory, $disk GiB disk free"
}

# update: installs the latest version of every package.
update() {
  export DEBIAN_FRONTEND=noninteractive # apt never stops to ask a question
  # Wait up to 5 minutes for another apt (like unattended-upgrades) to finish,
  # instead of failing because it holds the lock.
  local apt=(apt-get -y -o DPkg::Lock::Timeout=300)

  # On a new cloud server, cloud-init may still be setting it up, and using apt.
  if command -v cloud-init >/dev/null; then
    step "Wait for cloud-init to finish starting up the server"
    run cloud-init status --wait ||
      warn "cloud-init reported a problem, shown above. Carrying on, since it doesn't stop the updates."
  fi

  step "Refresh the package lists"
  run "${apt[@]}" update

  local plan upgrades
  plan=$(apt-get -s full-upgrade) # -s: only says what it would do
  upgrades=$(grep -c '^Inst ' <<<"$plan" || true)
  if [[ $upgrades == 0 ]]; then
    ok "Every package is already up to date."
  else
    # Where a package comes with a new default config file, keep the server's.
    step "Upgrade $(plural "$upgrades" package), keeping the server's config files"
    run "${apt[@]}" -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold full-upgrade
    ok "Upgraded $(plural "$upgrades" package)."
  fi

  step "Remove packages that are no longer needed"
  run "${apt[@]}" autoremove
}

# restart: restarts the server, if an update needs it and you say to.
restart() {
  if [[ ! -e /var/run/reboot-required ]]; then
    ok "No restart needed."
  elif confirm "The server needs a restart to finish the updates. Restart it now?"; then
    step "Restart the server in 5 seconds"
    # Later rather than now, so this script ends, and ssh with it, before the
    # server shuts down.
    run systemd-run --on-active=5 systemctl reboot
    ok "The server is restarting, and is back in a minute or so."
  else
    warn "Not restarted. To restart it later: ${SUDO_USER:+sudo }systemctl reboot"
  fi
}

main() {
  check
  update
  restart
}

main "$@" </dev/null
