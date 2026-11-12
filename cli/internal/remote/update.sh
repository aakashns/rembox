# update.sh: installs packages with apt.

export DEBIAN_FRONTEND=noninteractive # apt never stops to ask a question
# Wait up to 5 minutes for another apt (like unattended-upgrades) to finish,
# instead of failing because it holds the lock.
APT=(apt-get -y -o DPkg::Lock::Timeout=300)

# update: installs the latest version of every package.
update() {
  # On a new cloud server, cloud-init may still be setting it up, and using apt.
  if command -v cloud-init >/dev/null; then
    step "Wait for cloud-init to finish starting up the server"
    run cloud-init status --wait ||
      warn "cloud-init reported a problem, shown above. Carrying on, since it doesn't stop the updates."
  fi

  step "Refresh the package lists"
  run "${APT[@]}" update

  local plan upgrades
  plan=$(apt-get -s full-upgrade) # -s: only says what it would do
  upgrades=$(grep -c '^Inst ' <<<"$plan" || true)
  if [[ $upgrades == 0 ]]; then
    ok "Every package is already up to date."
  else
    # Where a package comes with a new default config file, keep the server's.
    step "Upgrade $(plural "$upgrades" package), keeping the server's config files"
    run "${APT[@]}" -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold full-upgrade
    ok "Upgraded $(plural "$upgrades" package)."
  fi

  step "Remove packages that are no longer needed"
  run "${APT[@]}" autoremove
}

# install_packages PACKAGE...: installs the packages that aren't installed yet.
install_packages() {
  local missing=() package
  for package; do
    dpkg-query -W -f '${Status}' "$package" 2>/dev/null | grep -q ' installed$' || missing+=("$package")
  done
  if ((${#missing[@]} > 0)); then
    run "${APT[@]}" install "${missing[@]}"
  fi
}
