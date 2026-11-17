# instructions.sh: writes ~/.config/rembox/AGENTS.md, which tells coding agents
# about the server, and has Claude Code, Codex, OpenCode and Pi read it: each
# one's global instructions file links to it, or, if you have one of your own,
# starts with a line that points to it. rembox owns the file and that line, and
# running setup again rewrites them; your own instructions go in your file.

AGENT_INSTRUCTIONS=.config/rembox/AGENTS.md # in your home folder

# Each agent's global instructions file, in your home folder.
AGENT_INSTRUCTION_FILES=(.claude/CLAUDE.md .codex/AGENTS.md .config/opencode/AGENTS.md .pi/agent/AGENTS.md)

# setup_agent_instructions: writes rembox's instructions for coding agents, and
# points each agent's global instructions file to them. Runs as your user.
setup_agent_instructions() {
  step "Write instructions for coding agents"
  ((EUID != 0)) || fail "Run this as your user, not root."
  local file=$HOME/$AGENT_INSTRUCTIONS content name
  content=$(agent_instructions)
  [[ -d ${file%/*} ]] || run mkdir -p "${file%/*}"
  same "$file" "$content" || put "$file" 644 "$content"
  for name in "${AGENT_INSTRUCTION_FILES[@]}"; do
    point_to_instructions "$HOME/$name"
  done
  ok "Coding agents read ~/$AGENT_INSTRUCTIONS. For your own, replace an agent's link to it with a file."
  checklist ok "Coding agents follow ~/$AGENT_INSTRUCTIONS"
}

# point_to_instructions FILE: links FILE, an agent's global instructions file,
# to rembox's, unless it's a file of your own, which instead gets a line at its
# top that points to rembox's, unless it has the line already. Claude Code reads
# what @ points to, and the other agents read it when told to.
point_to_instructions() {
  local file=$1 target=$HOME/$AGENT_INSTRUCTIONS
  local line="Also follow @~/$AGENT_INSTRUCTIONS, rembox's instructions for this server."
  [[ $(readlink -f "$file") != "$target" ]] || return 0
  if [[ ! -e $file && ! -L $file ]]; then
    [[ -d ${file%/*} ]] || run mkdir -p "${file%/*}"
    run ln -s "$target" "$file"
    return
  fi
  if [[ -L $file ]]; then
    file=$(readlink -f "$file") # write to the file, not over the link
  fi
  if [[ ! -e $file ]]; then
    put "$file" 644 "$line"
  elif ! grep -qF "@~/$AGENT_INSTRUCTIONS" "$file"; then
    put "$file" "$(stat -c %a "$file")" "$line"$'\n\n'"$(cat "$file")"
  fi
}

# agent_instructions: prints rembox's instructions for coding agents.
agent_instructions() {
  cat <<END
- $(machine_specs)
- This is a development server controlled remotely using T3 Code desktop/mobile.
- Be frugal, run one heavy build at a time, and stop dev servers when you're done.
- Git repositories are cloned in ~/work, don't pollute the home directory
- Bind dev servers to 127.0.0.1. For the user to test one, e.g. on their phone:
  - run \`opentunnel route add PORT\` and share the https link it prints
  - allow its host in the dev server if needed, like Vite's server.allowedHosts
  - run \`opentunnel route remove NAME\` when done
- ufw blocks every other port. Don't change ufw, sshd, fail2ban or T3 Code service.
- Run \`psql\` to access postgres as a superuser, then create per-project user & database(s)
- Use the installed \`playwright\` with Chromium for browser testing. Clean up when done.
- \`gh\` is logged in to GitHub. Work on branches; never force-push the default branch.
- Never print, log or commit secrets (~/.codex, ~/.claude, ~/.pi, ~/.local, ~/.config, ~/.t3)
END
}

# machine_specs: the server's OS, architecture, CPUs, memory, swap and disk,
# like "Ubuntu 24.04.3 LTS, arm64, 2 vCPUs, 3.8 GiB memory, 4 GiB swap, 79 GiB disk".
machine_specs() {
  local os swap
  # shellcheck disable=SC1091
  os=$(. /etc/os-release && echo "$PRETTY_NAME")
  swap="$(gib "$(awk '/^SwapTotal:/ { print $2 }' /proc/meminfo)") swap"
  [[ $swap != "0 GiB swap" ]] || swap="no swap"
  printf '%s, %s, %s vCPUs, %s memory, %s, %s disk' "$os" "$(dpkg --print-architecture)" "$(nproc)" \
    "$(gib "$(awk '/^MemTotal:/ { print $2 }' /proc/meminfo)")" "$swap" \
    "$(gib "$(df -k --output=size / | tail -n 1)")"
}

# gib KIB: KIB kibibytes in GiB, to a tenth, like "3.8 GiB" or "4 GiB".
gib() {
  awk -v k="$1" 'BEGIN { s = sprintf("%.1f", k / 1048576); sub(/\.0$/, "", s); print s " GiB" }'
}
