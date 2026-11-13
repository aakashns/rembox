# update.sh: installs packages with apt, and has it install security updates
# every day.

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

AUTO_UPGRADES=/etc/apt/apt.conf.d/99rembox-auto-upgrades

# setup_auto_upgrades: has unattended-upgrades install security updates every
# day. It never restarts the server; restart asks to, the next time setup runs.
setup_auto_upgrades() {
  step "Install security updates every day, with unattended-upgrades"
  install_packages unattended-upgrades
  # 99, so it comes after, and wins over, 20auto-upgrades, which may turn
  # them off. unattended-upgrades' own config says which updates it installs.
  local content
  content=$(
    echo "// Written by rembox setup."
    echo 'APT::Periodic::Update-Package-Lists "1";'
    echo 'APT::Periodic::Unattended-Upgrade "1";'
  )
  same "$AUTO_UPGRADES" "$content" || put "$AUTO_UPGRADES" 644 "$content"
  # apt's timers run unattended-upgrades.
  local timer
  for timer in apt-daily.timer apt-daily-upgrade.timer; do
    systemctl is-enabled --quiet "$timer" || run systemctl enable --now "$timer"
  done

  local lists='' upgrade=''
  eval "$(apt-config shell lists APT::Periodic::Update-Package-Lists upgrade APT::Periodic::Unattended-Upgrade)"
  if [[ $lists == 1 && $upgrade == 1 ]]; then
    ok "unattended-upgrades installs security updates every day."
  else
    warn "A file in /etc/apt/apt.conf.d that comes after $AUTO_UPGRADES turns off daily updates. To have unattended-upgrades install them, remove what it says about APT::Periodic."
  fi
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

# The tools for development every server gets. ufw is here too, though
# setup_firewall installs it first.
BASIC_PACKAGES=(
  build-essential git gh curl wget zip unzip jq ripgrep fd-find fzf
  htop tmux zsh nano man-db dnsutils ufw
)

# install_basics: installs BASIC_PACKAGES, the ones that aren't installed yet.
# Debian names fd's command fdfind, as another package has fd, so it adds fd
# too, unless there's one already.
install_basics() {
  step "Install tools for development: ${BASIC_PACKAGES[*]}"
  install_packages "${BASIC_PACKAGES[@]}"
  if ! command -v fd >/dev/null && [[ -x /usr/bin/fdfind ]]; then
    run ln -s /usr/bin/fdfind /usr/local/bin/fd
  fi
  ok "The tools for development are installed."
}
