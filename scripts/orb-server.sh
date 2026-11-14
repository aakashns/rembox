#!/usr/bin/env bash
# orb-server.sh NAME [IMAGE]: (re)makes an OrbStack machine that looks like a fresh cloud server,
# with cloud-init and root ssh with your key, for trying `rembox setup` on. IMAGE is
# ubuntu:noble by default; debian:bookworm works too. The key is scripts/.keys/id_ed25519,
# made the first time. It deletes the machine NAME first, if there is one.

set -euo pipefail

name=${1:?Usage: scripts/orb-server.sh NAME [IMAGE]}
image=${2:-ubuntu:noble}
key=$(cd "$(dirname "$0")" && pwd)/.keys/id_ed25519
[[ -f $key ]] || { mkdir -p "${key%/*}" && ssh-keygen -q -t ed25519 -N '' -C rembox-dev -f "$key"; }

if orb list -q | grep -qx "$name"; then
  orb delete -f "$name"
fi

userdata=$(mktemp)
trap 'rm -f "$userdata"' EXIT
cat >"$userdata" <<EOF
#cloud-config
disable_root: false
packages: [openssh-server]
runcmd:
  - install -d -m 700 /root/.ssh
  - echo '$(cat "$key.pub")' > /root/.ssh/authorized_keys
  - chmod 600 /root/.ssh/authorized_keys
  - systemctl enable --now ssh
EOF

orb create --isolated --cpus 2 --memory 4G -c "$userdata" "$image" "$name"
orb -m "$name" -u root cloud-init status --wait >/dev/null || true

# A new machine has a new host key, on every port rembox may have moved ssh to.
host=$name.orb.local
grep -o "^\[\?$host\]\?\(:[0-9]*\)\?" ~/.ssh/known_hosts 2>/dev/null | sort -u |
  while read -r h; do ssh-keygen -R "$h" >/dev/null 2>&1; done || true

echo "Ready: rembox setup, with server $host, port 22, user root, and key $key"
