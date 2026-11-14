#!/usr/bin/env bash
# e2e.sh [IMAGE]: runs `rembox setup` on a fresh OrbStack machine, rembox-e2e, twice (the second
# run should change nothing), answering its questions with expect, then logs in as your user. It
# runs in a temp HOME, with its own known_hosts, so your own config is untouched. IMAGE is
# ubuntu:noble by default. HOST is how to reach the machine: rembox-e2e.orb.local by default, or
# 127.0.0.1 if your terminal lacks macOS's Local Network permission. Fails if setup prints
# nothing for 3 minutes. The output is in $TMPDIR/rembox-e2e/log.

set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
name=rembox-e2e
host=${HOST:-$name.orb.local}
key=$root/scripts/.keys/id_ed25519
tmp=${TMPDIR:-/tmp}/rembox-e2e
rm -rf "$tmp" && mkdir -p "$tmp/home" "$tmp/bin"

make -C "$root/cli" build
"$root/scripts/orb-server.sh" "$name" "${1:-ubuntu:noble}"

# ssh reads ~/.ssh from your real home, whatever HOME is, so give it a known_hosts of its own.
cat >"$tmp/bin/ssh" <<EOF
#!/bin/sh
exec /usr/bin/ssh -o UserKnownHostsFile=$tmp/known_hosts -o StrictHostKeyChecking=accept-new "\$@"
EOF
chmod +x "$tmp/bin/ssh"

# setup [NAME]: runs rembox setup, answering every question, as user dev with password e2e-pass.
# A question is a line that output stops at, so each pattern ends at the end of the output ($), and
# each finished line is dropped, so text that's printed but not asked can't match later.
setup() {
  HOME=$tmp/home USER=dev PATH=$tmp/bin:$PATH expect -f - "$root/cli/bin/rembox" "$host" "$key" "$name" "$@" <<'EOF'
set timeout 180
log_file -a $env(HOME)/../log
lassign $argv rembox host key name
spawn $rembox setup {*}[lrange $argv 4 end]
while 1 {
  expect {
    -re {Server IP address or hostname[^\n]*: $} { send "$host\r" }
    -re {SSH private key[^\n]*: $} { send "$key\r" }
    -re {Name for this server[^\n]*: $} { send "$name\r" }
    -re {(SSH port|User to log in as|Username)[^\n]*: $} { send "\r" }
    -re {(New password|Retype new password|\[sudo\] password for [^\n]*): $} { send "e2e-pass\r" }
    -re {Move ssh to port [^\n]*\] $} { send "y\r" }
    -re {Keep the ufw rule [^\n]*\] $} { send "\r" }
    -re {Restart it now\?[^\n]*\] $} { send "n\r" }
    -re {to ~/.ssh/config\?[^\n]*\] $} { send "y\r" }
    -re {Replace it\?[^\n]*\] $} { send "y\r" }
    -re {is set up\. To log in: rembox ssh} { expect eof; exit [lindex [wait] 3] }
    -re {[^\n]*\n} { exp_continue }
    timeout {
      set last ""
      expect -timeout 0 -re {[^\n]+$} { regsub -all {\x1b\[[0-9;]*m} $expect_out(0,string) {} last }
      puts "\ne2e: no output for 3 minutes, at: [string trim $last]"
      exit 1
    }
    eof { puts "\ne2e: rembox setup stopped before it finished"; exit 1 }
  }
}
EOF
}

setup
setup "$name"
port=$(sed -n 's/^ *Port //p' "$tmp/home/.config/rembox/ssh_config")
ssh -F /dev/null -o UserKnownHostsFile="$tmp/known_hosts" -i "$key" -o IdentitiesOnly=yes -p "$port" \
  "dev@$host" 'zsh -lic "echo \$SHELL; hostname"'
echo "e2e: passed. Delete the machine with: orb delete -f $name"
