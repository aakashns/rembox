# update.sh: installs the latest version of every package with apt. rembox runs
# it as root, after probe.sh has checked that the server uses apt. Running it
# again is harmless: when everything is up to date, it changes nothing.

main() {
  export DEBIAN_FRONTEND=noninteractive # apt never stops to ask a question
  # Wait up to 5 minutes for another apt (like unattended-upgrades) to finish,
  # instead of failing because it holds the lock.
  local apt=(apt-get -y -o DPkg::Lock::Timeout=300)

  if command -v cloud-init >/dev/null; then
    step "Wait for the server to finish starting up" \
      "When a cloud server first starts, cloud-init sets it up (and may install packages), which keeps apt busy. On a server that's been running a while, this is instant."
    run cloud-init status --wait ||
      warn "cloud-init reported a problem, shown above. Carrying on, since it doesn't stop the updates."
  fi

  step "Refresh the package lists" \
    "Downloads the latest list of packages from the server's package mirrors, so apt knows which updates there are."
  run "${apt[@]}" update

  local plan upgrades
  plan=$(apt-get -s full-upgrade) # -s: only says what it would do
  upgrades=$(grep -c '^Inst ' <<<"$plan" || true)
  if [[ $upgrades == 0 ]]; then
    ok "Every package is already up to date."
  else
    step "Upgrade $(plural "$upgrades" package)" \
      "Installs the newest version of each. A full upgrade can also add or remove packages when an update needs it, as a new kernel does. Where a package comes with a new default config file, the server's current one is kept."
    run "${apt[@]}" -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold full-upgrade
    ok "Upgraded $(plural "$upgrades" package)."
  fi

  step "Remove packages that are no longer needed" \
    "Like old kernels, and libraries that only removed packages used."
  run "${apt[@]}" autoremove
}

main "$@" </dev/null
