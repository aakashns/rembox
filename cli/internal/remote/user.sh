# user.sh: adds your user on the server.

# add_user USER: adds USER, if it isn't there yet, lets it use sudo without a
# password, and lets in the SSH keys of the user rembox logged in as.
add_user() {
  local user=$1 login=${SUDO_USER:-root}

  step "Add your user, $user, with sudo"
  if id -u "$user" >/dev/null 2>&1; then
    ok "$user is already there."
  else
    run useradd --create-home --shell /bin/bash "$user"
  fi
  install_packages sudo
  # Its own group (as cloud servers' default users have) shows it's an admin;
  # the sudoers file lets it use sudo without a password, which it has none of.
  if ! id -nG "$user" | tr ' ' '\n' | grep -qx sudo; then
    run usermod --append --groups sudo "$user"
  fi
  local sudoers="/etc/sudoers.d/rembox-$user" rule="$user ALL=(ALL:ALL) NOPASSWD: ALL"
  if ! same "$sudoers" "$rule"; then
    put "$sudoers" 440 "$rule" visudo -c -f
  fi
  ok "$user can use sudo."

  local home group
  home=$(getent passwd "$user" | cut -d: -f6)
  group=$(id -gn "$user")
  local keys=$home/.ssh/authorized_keys
  if [[ $login != "$user" ]]; then
    step "Let $login's SSH keys log in as $user"
    local from
    from=$(getent passwd "$login" | cut -d: -f6)/.ssh/authorized_keys
    [[ -f $keys ]] || run install -D -m 600 -o "$user" -g "$group" /dev/null "$keys"
    local line added=0
    while IFS= read -r line || [[ -n $line ]]; do
      if [[ -n $line && $line != \#* ]] && ! grep -qxF -e "$line" "$keys"; then
        printf '%s\n' "$line" >>"$keys"
        added=$((added + 1))
      fi
    done < <([[ -f $from ]] && cat "$from")
    if ((added > 0)); then
      ok "Added $(plural "$added" key) from $from to $keys."
    else
      ok "$keys has $login's keys already."
    fi
  fi
  grep -q '^[^#]' "$keys" 2>/dev/null ||
    fail "$user has no SSH keys to log in with, in $keys."
  # sshd uses the keys only if no one else can change them.
  if [[ $(stat -c %U:%a "$home/.ssh" "$keys" | paste -sd ' ') != "$user:700 $user:600" ]]; then
    run chown "$user:$group" "$home/.ssh" "$keys"
    run chmod 700 "$home/.ssh"
    run chmod 600 "$keys"
  fi
}
