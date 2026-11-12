# check.sh: checks that rembox can set up the server.

# check: checks that rembox can set up the server, and sums it up.
check() {
  step "Check that rembox can set up this server"
  local ID='' PRETTY_NAME='an unknown system'
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
  fi
  local arch
  if command -v dpkg >/dev/null; then
    arch=$(dpkg --print-architecture) # Debian's names, like arm64 and amd64
  else
    arch=$(uname -m)
  fi

  local problems=()
  [[ $ID == ubuntu || $ID == debian ]] ||
    problems+=("rembox sets up Ubuntu and Debian servers, and this one runs $PRETTY_NAME.")
  command -v apt-get >/dev/null || problems+=("It has no apt-get, which rembox installs packages with.")
  [[ -d /run/systemd/system ]] || problems+=("It doesn't run systemd, which rembox runs services with.")
  [[ $arch == arm64 || $arch == amd64 ]] || problems+=("Its processor is $arch; rembox supports arm64 and amd64.")
  if ((${#problems[@]} > 0)); then
    fail "${problems[@]}"
  fi

  local memory disk
  memory=$(awk '/^MemTotal:/ { printf "%.1f", $2 / 1048576 }' /proc/meminfo)
  disk=$(df -Pk / | awk 'NR == 2 { printf "%.1f", $4 / 1048576 }')
  ok "$PRETTY_NAME, $arch, $(plural "$(nproc)" core), $memory GiB memory, $disk GiB disk free"
}
