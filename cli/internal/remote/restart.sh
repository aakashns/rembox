# restart.sh: restarts the server, if an update needs it.

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
    warn "Not restarted. To restart it later: sudo systemctl reboot"
  fi
}
