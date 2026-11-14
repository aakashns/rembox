# lib.sh: what every rembox script starts with. rembox runs lib.sh, then the
# files with each step's functions, then the script itself, with `bash -c` on
# the server, as one script.
#
# Each script defines main and ends with `main "$@" </dev/null`, so no command
# in it stops to read input; only confirm and ask ask, at the terminal.
#
# rembox passes --color when your terminal shows color.

set -euo pipefail

# Plain messages from every tool, whatever locale ssh passed on from your Mac
# (which the server may not have, so tools like perl warn about it). rembox
# also asks ssh for this locale, but servers can refuse it.
export LC_ALL=C.UTF-8 LANG=C.UTF-8
unset LANGUAGE

# The script's arguments, as rembox gave them, for as_root.
SCRIPT_ARGS=("$@")

if [[ ${1:-} == --color ]]; then
  shift
  BOLD=$'\e[1m' DIM=$'\e[2m' RED=$'\e[31m' CYAN=$'\e[36m' GREEN=$'\e[32m' YELLOW=$'\e[33m' RESET=$'\e[0m'
else
  BOLD='' DIM='' RED='' CYAN='' GREEN='' YELLOW='' RESET=''
fi

# step TITLE: starts a part of the script.
step() {
  printf '\n%s-->%s %s%s%s\n' "$CYAN" "$RESET" "$BOLD" "$1" "$RESET"
}

# quoted WORD...: the words as you'd type them in a shell, each in single
# quotes if it needs them, as rembox shows its own commands.
quoted() {
  local word words=()
  for word; do
    if [[ $word =~ ^[A-Za-z0-9_@%+=:,./-]+$ ]]; then
      words+=("$word")
    else
      words+=("'${word//\'/\'\\\'\'}'")
    fi
  done
  printf '%s' "${words[*]}"
}

# run COMMAND...: shows a command, then runs it with its output indented under
# it. Of a line that a progress message rewrites with carriage returns, only
# the last version is kept. Returns the command's exit status.
run() {
  printf '\n    %s$ %s%s\n' "$CYAN" "$(quoted "$@")" "$RESET"
  local status=0
  "$@" 2>&1 | sed -u "s/\r$//; s/.*\r//; s/^/      ${DIM}/; s/\$/${RESET}/" || status=$?
  return "$status"
}

# at_terminal COMMAND...: shows a command, then runs it at the terminal, for
# commands that ask you things, like passwd. Returns its exit status.
at_terminal() {
  printf '\n    %s$ %s%s\n' "$CYAN" "$(quoted "$@")" "$RESET"
  "$@" </dev/tty >/dev/tty 2>&1
}

# plural N WORD: "1 WORD", or "N WORDs".
plural() {
  if [[ $1 == 1 ]]; then echo "1 $2"; else echo "$1 $2s"; fi
}

# ok MESSAGE: says a step worked.
ok() {
  printf '\n    %s✓%s %s\n' "$GREEN" "$RESET" "$1"
}

# info MESSAGE: says what the script does next, and why.
info() {
  printf '\n    %s\n' "$1"
}

# warn MESSAGE: says something went wrong that the script can carry on after.
warn() {
  printf '\n    %s! %s%s\n' "$YELLOW" "$1" "$RESET"
}

# fail MESSAGE...: says why the script can't carry on, a line each, and stops it.
fail() {
  printf '\n'
  local line
  for line; do
    printf '    %s✗ %s%s\n' "$RED" "$line" "$RESET"
  done
  exit 1
}

# logged_in: says rembox has logged in to the server, unless as_root has run
# the script again.
logged_in() {
  [[ -n ${SUDO_USER:-} ]] || step "Logged in to $HOSTNAME as $(id -un)"
}

# as_root: carries on as root. Unless the script runs as root already, it
# says so, then runs itself again with sudo, which may ask for the password.
as_root() {
  ((EUID != 0)) || return 0
  local me
  me=$(id -un)
  info "$me isn't root, so rembox carries on with sudo, which may ask for $me's password on $HOSTNAME."
  exec sudo bash -c "$BASH_EXECUTION_STRING" bash "${SCRIPT_ARGS[@]}"
}

# confirm QUESTION [yes]: asks a yes/no question at the terminal, which rembox
# runs the script at (ssh -t). Succeeds if the answer is y or yes, or, given
# yes, if there's no answer.
confirm() {
  local answer='' default=${2:-no} choices='[y/N]'
  [[ $default == yes ]] && choices='[Y/n]'
  printf '\n    %s%s%s %s ' "$BOLD" "$1" "$RESET" "$choices"
  read -r answer </dev/tty || true
  answer=${answer,,}
  [[ ${answer:-$default} == y || ${answer:-$default} == yes ]]
}

# ask VAR QUESTION [DEFAULT]: asks QUESTION at the terminal, as rembox asks
# its own, and sets VAR to the answer, or DEFAULT if there's none. Fails at the
# end of input (Ctrl-D).
ask() {
  local answer='' hint=''
  [[ -n ${3:-} ]] && hint=" ${DIM}[default: $3]${RESET}"
  printf '\n    %s%s%s%s: ' "$BOLD" "$2" "$RESET" "$hint"
  if ! read -r answer </dev/tty; then
    printf '\n'
    return 1
  fi
  printf -v "$1" '%s' "${answer:-${3:-}}"
}

# same FILE CONTENT: succeeds if FILE holds just CONTENT (and a newline).
same() {
  [[ -f $1 ]] && cmp -s "$1" <(printf '%s\n' "$2")
}

# put FILE MODE CONTENT [CHECK...]: shows CONTENT, then writes it to FILE, owned
# by the user running the script, with MODE. If given, CHECK runs first with the new file's path at
# the end, and FILE stays as it is unless CHECK succeeds.
put() {
  local file=$1 mode=$2 content=$3
  shift 3
  printf '\n    %s$ cat > %s%s\n' "$CYAN" "$file" "$RESET"
  sed "s/^/      ${DIM}/; s/\$/${RESET}/" <<<"$content"
  local new=$file.rembox-new
  printf '%s\n' "$content" >"$new"
  chown "$(id -un):$(id -gn)" "$new"
  chmod "$mode" "$new"
  if (($# > 0)) && ! run "$@" "$new"; then
    rm -f "$new"
    fail "$1 found a problem with $file, shown above, so it's unchanged."
  fi
  mv "$new" "$file"
}
