# lib.sh: what every rembox script starts with. rembox runs lib.sh followed by
# a script with `bash -c` on the server, as one script.
#
# Each script defines main and ends with `main "$@" </dev/null`, so no command
# in it stops to read input; only confirm asks, at the terminal.
#
# rembox passes --color when your terminal shows color.

set -euo pipefail

# Plain messages from every tool, whatever locale ssh passed on from your Mac
# (which the server may not have, so tools like perl warn about it). rembox
# also asks ssh for this locale, but servers can refuse it.
export LC_ALL=C.UTF-8 LANG=C.UTF-8
unset LANGUAGE

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

# run COMMAND...: shows a command, then runs it with its output indented under
# it. Of a line that a progress message rewrites with carriage returns, only
# the last version is kept. Returns the command's exit status.
run() {
  local shown
  shown=$(printf '%q ' "$@")
  printf '\n    %s$ %s%s\n' "$CYAN" "${shown% }" "$RESET"
  local status=0
  "$@" 2>&1 | sed -u "s/\r$//; s/.*\r//; s/^/      ${DIM}/; s/\$/${RESET}/" || status=$?
  return "$status"
}

# plural N WORD: "1 WORD", or "N WORDs".
plural() {
  if [[ $1 == 1 ]]; then echo "1 $2"; else echo "$1 $2s"; fi
}

# ok MESSAGE: says a step worked.
ok() {
  printf '\n    %s✓%s %s\n' "$GREEN" "$RESET" "$1"
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

# confirm QUESTION: asks a yes/no question at the terminal, which rembox runs
# the script at (ssh -t). Succeeds if the answer is y or yes.
confirm() {
  local answer=''
  printf '\n    %s%s%s [y/N] ' "$BOLD" "$1" "$RESET"
  read -r answer </dev/tty || true
  [[ ${answer,,} == y || ${answer,,} == yes ]]
}
