# firewall.sh: sets up ufw, the firewall.

UFW_DEFAULTS=/etc/default/ufw

# ufw_active: succeeds if ufw is on.
ufw_active() {
  command -v ufw >/dev/null && ufw status | grep -qx 'Status: active'
}

# allow_ssh PORT: has ufw let in ssh on PORT, over IPv4 and IPv6, unless it does
# already. It does, even while ufw is off, once it's on.
allow_ssh() {
  local port=$1
  if ! grep -qE "^### tuple ### allow tcp $port 0\.0\.0\.0/0 any 0\.0\.0\.0/0 in( |\$)" /etc/ufw/user.rules ||
    ! grep -qE "^### tuple ### allow tcp $port ::/0 any ::/0 in( |\$)" /etc/ufw/user6.rules; then
    run ufw allow "$port/tcp"
  fi
}

# open_ssh_port PORT: if ufw is on, has it let in ssh on PORT, so rembox can log
# in there next.
open_ssh_port() {
  local port=$1
  ufw_active || return 0
  step "Let in ssh on port $port through ufw, which is on"
  allow_ssh "$port"
  ok "ufw lets in ssh on port $port."
}

# setup_firewall PORT: turns on ufw, to block every incoming connection but ssh
# on PORT. Deletes the rules for ssh's old port, 22, and asks whether to keep
# each other rule that lets something in.
setup_firewall() {
  local port=$1
  step "Block every incoming connection but ssh on port $port, with ufw"
  install_packages ufw

  local active=false
  ufw_active && active=true
  if ! grep -qx 'IPV6=yes' "$UFW_DEFAULTS"; then
    put "$UFW_DEFAULTS" 644 "$(sed 's/^IPV6=.*/IPV6=yes/' "$UFW_DEFAULTS")"
    # ufw sets up its IPv6 rules when it starts.
    if $active; then run ufw reload; fi
  fi
  allow_ssh "$port"

  # The rules that let something in, as `ufw show added` shows them, without
  # their comments: like allow 80/tcp, limit OpenSSH, or route allow in on eth0.
  local rules rule kept=0
  mapfile -t rules < <(ufw show added | sed -nE "s/^ufw ((route )?(allow|limit) .*)/\1/p" |
    sed -E "s/ comment '.*'\$//" | grep -vE '^(allow|limit) out ')
  for rule in "${rules[@]}"; do
    case $rule in
    "allow $port/tcp") ;;
    allow\ 22 | allow\ 22/tcp | allow\ OpenSSH | limit\ 22 | limit\ 22/tcp | limit\ OpenSSH)
      delete_ufw_rule "$rule"
      ;;
    *)
      if confirm "Keep the ufw rule to $rule?" yes; then
        kept=$((kept + 1))
      else
        delete_ufw_rule "$rule"
      fi
      ;;
    esac
  done

  if ! grep -qE '^DEFAULT_INPUT_POLICY="(DROP|REJECT)"$' "$UFW_DEFAULTS"; then
    run ufw default deny incoming
  fi
  if ! $active; then
    # --force: without it, ufw asks whether to go on, as it may cut off ssh.
    run ufw --force enable
  fi
  # ufw.service turns ufw on when the server starts.
  systemctl is-enabled --quiet ufw || run systemctl enable ufw
  local others=''
  if ((kept == 1)); then
    others=", and what its other rule lets in"
  elif ((kept > 1)); then
    others=", and what its $kept other rules let in"
  fi
  ok "ufw blocks every incoming connection but ssh on port $port$others."
}

# delete_ufw_rule RULE: deletes RULE, as `ufw show added` shows it, like
# allow 'Nginx Full', or route allow in on eth0.
delete_ufw_rule() {
  local words
  mapfile -t words < <(xargs printf '%s\n' <<<"$1") # splits it as the shell would
  if [[ ${words[0]} == route ]]; then
    run ufw route delete "${words[@]:1}"
  else
    run ufw delete "${words[@]}"
  fi
}
