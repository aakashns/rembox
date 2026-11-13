# swap.sh: adds swap.

SWAP_FILE=/swapfile
SWAPPINESS=/etc/sysctl.d/99-rembox-swap.conf
# The most swap to add, in GiB: as much as the server has memory, up to this.
SWAP_MAX=4

# setup_swap: adds a swap file, unless the server has swap already, so running
# low on memory, as a big build or a few coding agents can, slows the server
# down rather than having programs killed. The server swaps only once memory
# runs low.
setup_swap() {
  step "Add swap, for when memory runs low"
  if systemd-detect-virt --quiet --container; then
    ok "The server is a container, which uses its host's swap, so it gets none of its own."
    return
  fi

  if [[ -z $(swapon --show=NAME --noheadings) ]]; then
    if [[ ! -e $SWAP_FILE ]]; then
      local fs memory size free
      fs=$(findmnt --noheadings --output FSTYPE --target "${SWAP_FILE%/*}")
      if [[ $fs != ext4 && $fs != xfs ]]; then
        warn "Not adding swap: / is $fs, and rembox adds swap files on ext4 and xfs only."
        return
      fi
      memory=$(awk '/^MemTotal:/ { print $2 }' /proc/meminfo) # KiB
      size=$(((memory + 1048575) / 1048576))                  # GiB, rounded up
      ((size <= SWAP_MAX)) || size=$SWAP_MAX
      free=$(df -Pk / | awk 'NR == 2 { print $4 }') # KiB
      if ((free < (size + 5) * 1048576)); then
        warn "Not adding swap: / has $((free / 1048576)) GiB free, too little for $size GiB of swap and 5 GiB to spare."
        return
      fi
      run fallocate --length "${size}G" "$SWAP_FILE"
      run chmod 600 "$SWAP_FILE"
      run mkswap "$SWAP_FILE"
    fi
    run swapon "$SWAP_FILE"
  fi

  # Turn rembox's swap file on whenever the server starts.
  if swapon --show=NAME --noheadings | grep -qxF "$SWAP_FILE" &&
    ! awk -v f="$SWAP_FILE" '$1 == f && $3 == "swap" { found = 1 } END { exit !found }' /etc/fstab; then
    put /etc/fstab 644 "$(cat /etc/fstab)"$'\n'"$SWAP_FILE none swap sw 0 0"
  fi

  local content
  content=$(
    echo "# Written by rembox setup. Swap only once memory runs low."
    echo "vm.swappiness = 10"
  )
  if ! same "$SWAPPINESS" "$content"; then
    put "$SWAPPINESS" 644 "$content"
  fi
  [[ $(sysctl -n vm.swappiness) == 10 ]] || run sysctl -p "$SWAPPINESS"

  local total
  total=$(free -m | awk '/^Swap:/ { printf "%.1f", $2 / 1024 }')
  ok "The server has $total GiB of swap, used only once memory runs low."
}
