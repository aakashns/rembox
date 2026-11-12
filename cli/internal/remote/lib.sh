# lib.sh: what every rembox script starts with. rembox sends lib.sh followed by
# a script to `bash -s` on the server, as one script.
#
# Each script defines main and ends with `main "$@" </dev/null`, so bash reads
# all of it before running any of it, and no command can read the rest of the
# script from stdin by mistake.
#
# rembox passes --color when your terminal shows color.

set -euo pipefail

# Plain messages from every tool, whatever locale ssh passed on from your Mac
# (which the server may not have, so tools like perl warn about it). rembox
# also asks ssh for this locale, but servers can refuse it.
export LC_ALL=C.UTF-8 LANG=C.UTF-8
unset LANGUAGE

if [[ ${1:-} == --color ]]; then
  BOLD=$'\e[1m' DIM=$'\e[2m' CYAN=$'\e[36m' GREEN=$'\e[32m' YELLOW=$'\e[33m' RESET=$'\e[0m'
else
  BOLD='' DIM='' CYAN='' GREEN='' YELLOW='' RESET=''
fi

# step TITLE ABOUT: starts a part of the script, saying what it does and why.
step() {
  printf '\n%s-->%s %s%s%s\n' "$CYAN" "$RESET" "$BOLD" "$1" "$RESET"
  printf '%s\n' "$2" | fold -s -w 72 | sed 's/ *$//; s/^/    /'
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
