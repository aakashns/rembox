# nat64.sh: lets a server with only IPv6 reach sites with only IPv4, like GitHub.

NAT64_CONF=/etc/systemd/resolved.conf.d/99-rembox-nat64.conf
# Public DNS64 resolvers, from two providers, so the server can still look up
# names if one is down: nat64.net's (https://nat64.net) and Trex's
# (https://trex.fi/2011/dns64.html). For a site with only IPv4, they answer
# with an IPv6 address of their NAT64 gateway, which passes its traffic on
# over IPv4.
NAT64_DNS=(2a01:4f8:c2c:123f::1 2a01:4f9:c010:3f02::1 2a00:1098:2b::1 2001:67c:2b0::4 2001:67c:2b0::6)

# ipv6_only: succeeds if the server reaches the internet over IPv6 only: it
# has a default route for IPv6, and none for IPv4.
ipv6_only() {
  command -v ip >/dev/null &&
    [[ -z $(ip -4 route show default) && -n $(ip -6 route show default) ]]
}

# setup_nat64: if the server has only IPv6, has systemd-resolved look up every
# name with public DNS64 resolvers, so it reaches sites with only IPv4 through
# NAT64. If it has IPv4, does nothing, but undo that, if setup did it before.
setup_nat64() {
  if ! ipv6_only; then
    [[ -e $NAT64_CONF ]] || return 0
    step "Stop using public NAT64, as the server has IPv4 now"
    run rm "$NAT64_CONF"
    if systemctl is-active --quiet systemd-resolved; then
      run systemctl restart systemd-resolved
    fi
    ok "The server looks up names with its own DNS servers again."
    return
  fi

  step "Reach sites with only IPv4, like GitHub, through public NAT64, as the server has only IPv6"
  local content changed=false
  content=$(
    echo "# Written by rembox setup. The server has only IPv6, so look up every name with"
    echo "# public DNS64 resolvers, to reach sites with only IPv4 through NAT64."
    echo "[Resolve]"
    echo "DNS=${NAT64_DNS[*]}"
    echo "Domains=~."
  )
  if ! same "$NAT64_CONF" "$content"; then
    [[ -d ${NAT64_CONF%/*} ]] || run mkdir -p "${NAT64_CONF%/*}"
    put "$NAT64_CONF" 644 "$content"
    changed=true
  fi
  # Debian doesn't come with systemd-resolved. Installing it starts it, with
  # the file above, and points /etc/resolv.conf at it.
  install_packages systemd-resolved
  if [[ $(systemctl is-enabled systemd-resolved 2>/dev/null) == masked ]]; then
    warn "systemd-resolved is masked, so it can't look up names with public DNS64 resolvers, and downloads from GitHub will fail."
    checklist warn "Can't reach sites with only IPv4, like GitHub, as systemd-resolved is masked"
    return
  fi
  if ! systemctl is-active --quiet systemd-resolved; then
    run systemctl enable --now systemd-resolved
  elif $changed; then
    run systemctl restart systemd-resolved
  fi

  # getent shows an IPv4 address as ::ffff:IPV4, so look for any other.
  if getent ahostsv6 github.com | grep -qv '^::ffff:'; then
    ok "The server reaches sites with only IPv4, like GitHub, through public NAT64."
    checklist ok "Reaches sites with only IPv4 through public NAT64, as the server has only IPv6"
  else
    warn "github.com still has no IPv6 address, so downloads from GitHub will fail. Check that /etc/resolv.conf points at systemd-resolved, with: resolvectl status"
    checklist warn "Can't reach sites with only IPv4, like GitHub; see resolvectl status"
  fi
}
