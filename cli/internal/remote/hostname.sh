# hostname.sh: names the server as rembox does.

# set_hostname NAME: makes NAME, rembox's name for the server, its hostname, so
# its prompt and logs say which server it is, if NAME can be a hostname. Points
# NAME at the server in /etc/hosts, so sudo can look it up, and keeps
# cloud-init from changing either back when the server starts.
set_hostname() {
  local name=$1 label='[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?'
  step "Name the server $name"
  if systemd-detect-virt --quiet --container; then
    ok "The server is a container, whose hostname its host sets, so it keeps $(hostname)."
    return
  fi
  if ((${#name} > 64)) || [[ ! $name =~ ^$label(\.$label)*$ || $name =~ ^[0-9.]+$ ]]; then
    warn "$name can't be a hostname, so the server stays $(hostname). A hostname has lowercase letters, digits, - and ., up to 64."
    return
  fi

  local entry="127.0.1.1 $name"
  [[ $name == *.* ]] && entry+=" ${name%%.*}"
  if ! grep -qxF "$entry" /etc/hosts; then
    if grep -q '^127\.0\.1\.1[[:space:]]' /etc/hosts; then
      run sed -i "s/^127\.0\.1\.1[[:space:]].*/$entry/" /etc/hosts
    else
      run sed -i "\$a $entry" /etc/hosts
    fi
  fi
  if [[ $(hostname) != "$name" || $(cat /etc/hostname 2>/dev/null) != "$name" ]]; then
    run hostnamectl set-hostname "$name"
  fi
  if [[ -d /etc/cloud/cloud.cfg.d ]]; then
    local cfg=/etc/cloud/cloud.cfg.d/99-rembox-hostname.cfg
    local content="# rembox names the server: cloud-init keeps its hostname and /etc/hosts.
preserve_hostname: true
manage_etc_hosts: false"
    same "$cfg" "$content" || put "$cfg" 644 "$content"
  fi
  ok "The server is $name."
}
