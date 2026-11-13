# setup.sh: gets the server ready for development. rembox setup runs it three
# times, in three ssh sessions:
#
#   setup.sh start USER PORT LOGIN_PORT
#     As root, as the user you log in as, on LOGIN_PORT: checks that rembox
#     can set up the server, installs the latest version of every package,
#     has unattended-upgrades install security updates every day, installs
#     tools for development, adds USER with your keys, and sets sshd to
#     listen on PORT as well, and ufw, if it's on, to let it in.
#
#   setup.sh home
#     As USER, on PORT, which shows that both work: sets up zsh for USER.
#
#   setup.sh finish USER PORT [LOCALE...]
#     As root, as USER: sets sshd to listen only on PORT, with no root login,
#     turns on ufw to block every incoming connection but ssh on PORT, sets
#     up fail2ban, adds swap and the LOCALEs your Mac uses, makes zsh USER's
#     login shell, then, if an update needs it, asks to restart the server.
#
# Running it again is harmless: each step changes only what needs changing.

main() {
  case ${1:-} in
  start)
    (($# == 4)) || fail "Usage: setup.sh start USER PORT LOGIN_PORT"
    local user=$2 port=$3 login_port=$4
    # rembox logs in as root until it can log in as USER.
    local root_login=no
    [[ -n ${SUDO_USER:-} ]] || root_login=prohibit-password
    check
    update
    setup_auto_upgrades
    install_basics
    add_user "$user"
    setup_sshd "$root_login" "$login_port" "$port"
    open_ssh_port "$port"
    ;;
  home)
    (($# == 1)) || fail "Usage: setup.sh home"
    setup_zsh
    ;;
  finish)
    (($# >= 3)) || fail "Usage: setup.sh finish USER PORT [LOCALE...]"
    local user=$2 port=$3
    shift 3
    setup_sshd no "$port"
    setup_firewall "$port"
    setup_fail2ban "$port"
    setup_swap
    setup_locales "$@"
    set_login_shell "$user"
    restart
    ;;
  *)
    fail "Usage: setup.sh start USER PORT LOGIN_PORT, setup.sh home, or setup.sh finish USER PORT [LOCALE...]"
    ;;
  esac
}

main "$@" </dev/null
