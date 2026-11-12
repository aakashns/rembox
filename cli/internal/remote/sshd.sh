# sshd.sh: sets up the SSH server.

SSHD_CONFIG=/etc/ssh/sshd_config.d/00-rembox.conf

# setup_sshd ROOT_LOGIN PORT...: sets sshd to listen on the PORTs and no others, and
# to let in only keys, and root as ROOT_LOGIN says: no, or prohibit-password
# (with a key). Opens the ports in ufw, if it's on.
setup_sshd() {
  local root_login=$1
  shift
  local ports
  mapfile -t ports < <(printf '%s\n' "$@" | sort -nu)

  local where="port ${ports[0]}"
  ((${#ports[@]} == 1)) || where="ports ${ports[0]} and ${ports[1]}"
  local title="Set sshd to $where, with keys only"
  [[ $root_login == no ]] && title+=", and no root login"
  step "$title"

  local content
  content=$(
    echo "# Written by rembox setup. 00- comes first, and sshd takes the first value it reads."
    printf 'Port %s\n' "${ports[@]}"
    echo "PermitRootLogin $root_login"
    echo "PasswordAuthentication no"
    echo "KbdInteractiveAuthentication no"
  )
  local changed=false
  if ! same "$SSHD_CONFIG" "$content"; then
    # sshd -t -f checks the file on its own, as if it were sshd's whole config.
    put "$SSHD_CONFIG" 644 "$content" sshd -t -f
    changed=true
  fi

  # Another config file could still win, or add ports.
  local effective
  effective=$(sshd -T)
  [[ $(awk '$1 == "port" { print $2 }' <<<"$effective" | sort -n | paste -sd ' ') == "${ports[*]}" ]] ||
    fail "Another file in /etc/ssh sets sshd's Port. Remove it, then run rembox setup again."
  grep -qx 'passwordauthentication no' <<<"$effective" ||
    fail "Another file in /etc/ssh turns on PasswordAuthentication. Remove it, then run rembox setup again."
  if [[ $root_login == no ]] && ! grep -qx 'permitrootlogin no' <<<"$effective"; then
    fail "Another file in /etc/ssh sets PermitRootLogin. Remove it, then run rembox setup again."
  fi

  if ufw status 2>/dev/null | grep -qx 'Status: active'; then
    local port
    for port in "${ports[@]}"; do
      run ufw allow "$port/tcp"
    done
  fi

  local port listening=true
  for port in "${ports[@]}"; do
    [[ -n $(ss -Hltn "sport = :$port") ]] || listening=false
  done
  if $changed || ! $listening; then
    # Ubuntu listens with ssh.socket, which it makes from sshd's config.
    if systemctl is-active --quiet ssh.socket; then
      run systemctl daemon-reload
      run systemctl restart ssh.socket
    fi
    # Logged-in sessions, like this one, carry on.
    run systemctl try-restart ssh.service
  fi
  for port in "${ports[@]}"; do
    [[ -n $(ss -Hltn "sport = :$port") ]] || fail "sshd isn't listening on port $port."
  done
  ok "sshd listens on $where."
}
