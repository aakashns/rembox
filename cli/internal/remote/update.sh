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

# The tools for development every server gets, from apt. fzf and zoxide come
# from their own releases instead (see install_release), as the distros' are
# too old: Debian 12 has fzf 0.38 and zoxide 0.4.
BASIC_PACKAGES=(
  build-essential git gh curl wget zip unzip jq ripgrep fd-find htop tmux zsh
  nano man-db dnsutils ufw
)

# install_basics: installs BASIC_PACKAGES, the ones that aren't installed yet,
# then fzf and zoxide. Debian names fd's command fdfind, as another package has
# fd, so it adds fd too, unless there's one already.
install_basics() {
  step "Install tools for development: ${BASIC_PACKAGES[*]} fzf zoxide"
  install_packages "${BASIC_PACKAGES[@]}"
  if ! command -v fd >/dev/null && [[ -x /usr/bin/fdfind ]]; then
    run ln -s /usr/bin/fdfind /usr/local/bin/fd
  fi
  local arch
  arch=$(dpkg --print-architecture)
  install_release fzf "$FZF_VERSION" "${FZF_SHA256[$arch]}" \
    "https://github.com/junegunn/fzf/releases/download/v$FZF_VERSION/fzf-$FZF_VERSION-linux_$arch.tar.gz"
  local target=x86_64
  [[ $arch == arm64 ]] && target=aarch64
  install_release zoxide "$ZOXIDE_VERSION" "${ZOXIDE_SHA256[$arch]}" \
    "https://github.com/ajeetdsouza/zoxide/releases/download/v$ZOXIDE_VERSION/zoxide-$ZOXIDE_VERSION-$target-unknown-linux-musl.tar.gz"
  ok "The tools for development are installed."
}

# The releases of fzf and zoxide rembox installs, and the SHA-256 of their
# builds for each processor, as their releases list them.
FZF_VERSION=0.75.0
declare -A FZF_SHA256=(
  [amd64]=0d6640f83582d991cf429754baad64b07e3a4e439a6019670f4b91da1314141c
  [arm64]=bd0717badb6a5620387e014bb94209aac4d9eab6ab77b5348eabb0833aeb4d59
)
ZOXIDE_VERSION=0.10.0
declare -A ZOXIDE_SHA256=(
  [amd64]=2d93385b99f3e82cf2701609a1bffcad863fbeb75aa3fe7eb6be4d29be68b1ae
  [arm64]=f1f16c5d6298d63dee467eedea1cdcd8490e43e493bea43acd416dc9033ef641
)

# install_release NAME VERSION SHA256 URL: installs the program NAME from the
# .tar.gz at URL, if its SHA-256 is SHA256, as /usr/local/bin/NAME, which
# comes before apt's on PATH, unless that's VERSION or newer already.
install_release() {
  local name=$1 version=$2 sha256=$3 url=$4 file=/usr/local/bin/$1 have
  have=$("$file" --version 2>/dev/null | grep -oE '[0-9]+(\.[0-9]+)+' | head -n 1 || true)
  if [[ -n $have && $(printf '%s\n' "$version" "$have" | sort -V | head -n 1) == "$version" ]]; then
    return
  fi
  local dir
  dir=$(mktemp -d)
  run curl -fsSL -o "$dir/$name.tar.gz" "$url"
  if ! run sha256sum --check --strict <<<"$sha256  $dir/$name.tar.gz"; then
    rm -rf "$dir"
    fail "The download of $name isn't the file rembox expects, so it's not installed."
  fi
  run tar -xzf "$dir/$name.tar.gz" -C "$dir" "$name"
  run install -m 755 "$dir/$name" "$file"
  rm -rf "$dir"
}
