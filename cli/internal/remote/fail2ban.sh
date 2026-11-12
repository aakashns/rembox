# fail2ban.sh: sets up fail2ban.

FAIL2BAN_JAIL=/etc/fail2ban/jail.d/rembox.local

# setup_fail2ban PORT: sets fail2ban to ban the IP addresses that keep failing to log
# in with ssh on PORT.
setup_fail2ban() {
  local port=$1
  step "Ban IP addresses that keep failing to log in, with fail2ban"
  # python3-systemd, to read sshd's logs from the journal.
  install_packages fail2ban python3-systemd
  local content
  content=$(
    echo "# Written by rembox setup."
    echo "[sshd]"
    echo "enabled = true"
    echo "port = $port"
    echo "backend = systemd"
  )
  if ! same "$FAIL2BAN_JAIL" "$content"; then
    put "$FAIL2BAN_JAIL" 644 "$content"
    run fail2ban-client --test
    run systemctl enable fail2ban
    run systemctl restart fail2ban
  elif ! systemctl is-active --quiet fail2ban; then
    run systemctl enable --now fail2ban
  fi
  ok "fail2ban watches for failed logins on port $port."
}
