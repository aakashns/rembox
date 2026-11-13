# locale.sh: adds the locales your Mac uses.

LOCALE_GEN=/etc/locale.gen

# setup_locales LOCALE...: adds the LOCALEs, like en_US.UTF-8, that aren't on
# the server yet. ssh passes on your Mac's locale, so without it on the server,
# programs warn that they can't use it.
setup_locales() {
  (($# > 0)) || return 0
  step "Add the locale your Mac uses: $*"
  install_packages locales
  [[ -f $LOCALE_GEN ]] || fail "There's no $LOCALE_GEN, which says which locales to add."

  local name lang line entry added=0 unknown=0
  for name; do
    [[ $name =~ ^([a-z]{2,3}_[A-Z]{2})\.UTF-8$ ]] || fail "$name isn't a locale rembox can add, like en_US.UTF-8."
    lang=${BASH_REMATCH[1]}
    locale -a | grep -qxF "$lang.utf8" && continue
    # $LOCALE_GEN lists every locale, like "# en_US.UTF-8 UTF-8", or
    # "# en_IN UTF-8", for the ones whose charset is UTF-8 anyway. locale-gen
    # adds the ones that aren't commented out.
    line=$(grep -m 1 -E "^# ?$lang(\.UTF-8)? UTF-8\$" "$LOCALE_GEN" || true)
    if [[ -n $line ]]; then
      entry=${line#\#}
      entry=${entry# }
      run sed -i "s/^$line\$/$entry/" "$LOCALE_GEN"
    elif ! grep -qE "^$lang(\.UTF-8)? UTF-8\$" "$LOCALE_GEN"; then
      warn "The server doesn't know $name, so it's not added."
      unknown=$((unknown + 1))
      continue
    fi
    added=$((added + 1))
  done
  if ((added > 0)); then
    run locale-gen
  fi
  ((unknown > 0)) || ok "The server has the locale your Mac uses: $*"
}
