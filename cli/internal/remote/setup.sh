# setup.sh: gets the server ready for development. rembox setup runs it as
# root, twice, in two ssh sessions:
#
#   setup.sh start USER PORT LOGIN_PORT
#     As the user you log in as, on LOGIN_PORT: checks that rembox can set up
#     the server, installs the latest version of every package, adds USER
#     with your keys, and sets sshd to listen on PORT as well, and ufw, if
#     it's on, to let it in.
#
#   setup.sh finish PORT
#     As USER, on PORT, which shows that both work: sets sshd to listen only
#     on PORT, with no root login, turns on ufw to block every incoming
#     connection but ssh on PORT, sets up fail2ban, then, if an update needs
#     it, asks to restart the server.
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
    add_user "$user"
    setup_sshd "$root_login" "$login_port" "$port"
    open_ssh_port "$port"
    ;;
  finish)
    (($# == 2)) || fail "Usage: setup.sh finish PORT"
    local port=$2
    setup_sshd no "$port"
    setup_firewall "$port"
    setup_fail2ban "$port"
    restart
    ;;
  *)
    fail "Usage: setup.sh start USER PORT LOGIN_PORT, or setup.sh finish PORT"
    ;;
  esac
}

main "$@" </dev/null
