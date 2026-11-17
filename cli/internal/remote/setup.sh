# setup.sh: gets the server ready for development. rembox setup runs it
# twice, in two ssh sessions, as the user it logs in as: start, then home,
# which carries on as root with finish. start and finish run themselves
# again with sudo, unless that's root:
#
#   setup.sh start DEFAULT_USER DEFAULT_PORT LOGIN_PORT NAME
#     As root, as the user you log in as, on LOGIN_PORT: checks that rembox
#     can set up the server, makes NAME its hostname, adds swap, has apt
#     install gh from GitHub's apt repo, and PostgreSQL from PostgreSQL's,
#     installs the latest version of every
#     package, has unattended-upgrades install security updates every day,
#     installs tools for development and the latest Go, asks for your user's
#     name (DEFAULT_USER, if you don't give one; DEFAULT_USER may be empty) and
#     adds it with your keys and zsh as its login shell, keeps its services
#     running when it isn't logged in, for T3 Code, asks for the port to move
#     ssh to (DEFAULT_PORT, if you don't give one), and sets sshd to listen on
#     it as well, and ufw, if it's on, to let it in. It prints lines telling
#     rembox the name and the port; see add_user and ask_port.
#
#   setup.sh home GIT_NAME GIT_EMAIL PORT [LOCALE...]
#     As USER, on PORT, which shows that both work: makes ~/work, for USER's
#     projects, sets up zsh and Zellij for USER, logs gh in to GitHub, if USER
#     likes, and sets the name and email for git commits, from GitHub, or if
#     gh isn't logged in, GIT_NAME and GIT_EMAIL, from git on your Mac (either
#     may be empty), installs the latest bun, Node, Python and Rust, or asks to
#     upgrade them, then installs the coding agents USER picks, and logs them
#     in, installs the latest Playwright, with Chromium, for them, and the
#     latest opentunnel, connected, for dev server links, writes instructions
#     for them about the server, and installs T3 Code, connected to USER's
#     devices with T3 Connect. Then carries on with finish, in the same
#     session, so ssh logs in only once.
#
#   setup.sh finish PORT [LOCALE...]
#     As root, after home: lets your user run psql as a PostgreSQL superuser,
#     sets sshd to listen only on PORT, with no root login,
#     turns on ufw to block every incoming connection but ssh on PORT, sets
#     up fail2ban, adds the LOCALEs your Mac uses, then, if an update needs
#     it, asks to restart the server.
#
# Each session sends rembox lines for the checklist it shows once setup is
# done; see checklist in lib.sh.
#
# Running it again is harmless: each step changes only what needs changing.

main() {
  logged_in
  case ${1:-} in
  start)
    (($# == 5)) || fail "Usage: setup.sh start DEFAULT_USER DEFAULT_PORT LOGIN_PORT NAME"
    as_root "$@"
    local default_user=$2 default_port=$3 login_port=$4 name=$5 port
    # rembox logs in as root until it can log in as USER.
    local root_login=no
    [[ -n ${SUDO_USER:-} ]] || root_login=prohibit-password
    check
    set_hostname "$name"
    setup_swap
    checklist_send
    add_gh_repo
    add_postgres_repo
    update
    setup_auto_upgrades
    checklist_send
    install_basics
    install_go
    checklist_send
    add_user "$default_user"
    checklist_send
    ask_port port "$default_port"
    setup_sshd "$root_login" "$login_port" "$port"
    open_ssh_port "$port"
    ;;
  home)
    (($# >= 4)) || fail "Usage: setup.sh home GIT_NAME GIT_EMAIL PORT [LOCALE...]"
    make_projects_dir
    setup_zsh
    setup_zellij
    setup_github
    setup_git "$2" "$3"
    checklist_send
    setup_toolchains
    checklist_send
    setup_agents
    setup_playwright
    checklist_send
    setup_opentunnel
    checklist_send
    setup_agent_instructions
    checklist_send
    setup_t3
    checklist_send
    shift 3
    step "Finish as root"
    as_root finish "$@"
    finish "$@"
    ;;
  finish)
    (($# >= 2)) || fail "Usage: setup.sh finish PORT [LOCALE...]"
    shift
    as_root finish "$@"
    finish "$@"
    ;;
  *)
    fail "Usage: setup.sh start DEFAULT_USER DEFAULT_PORT LOGIN_PORT NAME, setup.sh home GIT_NAME GIT_EMAIL PORT [LOCALE...], or setup.sh finish PORT [LOCALE...]"
    ;;
  esac
}

# finish PORT [LOCALE...]: the steps of setup.sh finish, as root.
finish() {
  local port=$1
  shift
  # rembox carried on as root with sudo, from your user.
  add_postgres_role "$SUDO_USER"
  checklist_send
  setup_sshd no "$port"
  setup_firewall "$port"
  setup_fail2ban "$port"
  checklist ok "ssh on port $port only, with keys and no root login; ufw and fail2ban on"
  checklist_send
  setup_locales "$@"
  restart
  checklist_send
}

main "$@" </dev/null
