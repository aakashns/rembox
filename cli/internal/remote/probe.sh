# probe.sh: reports what kind of machine the server is, as "fact NAME=VALUE"
# lines for rembox to read. It changes nothing, so it's safe to run any time.

# fact NAME VALUE: reports one fact, on one line.
fact() {
  local value=${2//$'\n'/ }
  printf 'fact %s=%s\n' "$1" "${value//$'\r'/}"
}

# yes_if COMMAND...: "yes" if the command succeeds, or else "no".
yes_if() {
  if "$@" >/dev/null 2>&1; then echo yes; else echo no; fi
}

main() {
  local ID='' VERSION_ID='' PRETTY_NAME=''
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
  fi
  fact os_id "$ID"
  fact os_version "$VERSION_ID"
  fact os_name "$PRETTY_NAME"

  # Debian's names for processors (arm64, amd64), which downloads use.
  if command -v dpkg >/dev/null; then
    fact arch "$(dpkg --print-architecture)"
  else
    fact arch "$(uname -m)"
  fi
  fact kernel "$(uname -r)"
  fact hostname "$(uname -n)"
  local virt=''
  virt=$(systemd-detect-virt 2>/dev/null) || true # prints "none", and fails, on a physical machine
  fact virt "${virt:-unknown}"

  fact cpus "$(nproc)"
  fact memory_kb "$(awk '/^MemTotal:/ { print $2 }' /proc/meminfo)"
  fact swap_kb "$(awk '/^SwapTotal:/ { print $2 }' /proc/meminfo)"
  local disk
  disk=$(df -Pk / | awk 'NR == 2 { print $2, $4 }')
  fact disk_kb "${disk% *}"
  fact disk_free_kb "${disk#* }"

  fact user "$(id -un)"
  fact uid "$(id -u)"
  fact sudo "$(yes_if command -v sudo)"
  fact apt "$(yes_if command -v apt-get)"
  fact systemd "$(yes_if test -d /run/systemd/system)"
  fact reboot_required "$(yes_if test -e /var/run/reboot-required)"
  fact boot_id "$(cat /proc/sys/kernel/random/boot_id)"
}

main "$@" </dev/null
