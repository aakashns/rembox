# git.sh: installs gh from GitHub's apt repo, logs gh in to GitHub, for git
# too, and sets your name and email for git, from GitHub if it can.

GH_KEYRING=/etc/apt/keyrings/githubcli-archive-keyring.gpg
GH_SOURCES=/etc/apt/sources.list.d/github-cli.list # where GitHub's docs put it

# add_gh_repo: has apt install gh from GitHub's apt repo, as the distros' gh
# lags far behind (2.23 on Debian 12). It goes before update, which then
# upgrades a gh from the distro, and install_basics installs gh from it.
add_gh_repo() {
  step "Install gh from GitHub's apt repo, which has its latest version"
  local dir changed=no content
  dir=$(mktemp -d)
  # The key may change, so it's downloaded again each time.
  if ! run curl -fsSL -o "$dir/key.gpg" https://cli.github.com/packages/githubcli-archive-keyring.gpg; then
    rm -rf "$dir"
    warn "Couldn't download the key for GitHub's apt repo, so gh comes from the distro, or stays as it is. Run rembox setup again later."
    return
  fi
  if ! cmp -s "$dir/key.gpg" "$GH_KEYRING"; then
    [[ -d ${GH_KEYRING%/*} ]] || run install -d -m 755 "${GH_KEYRING%/*}"
    run install -m 644 "$dir/key.gpg" "$GH_KEYRING"
    changed=yes
  fi
  rm -rf "$dir"
  content="deb [arch=$(dpkg --print-architecture) signed-by=$GH_KEYRING] https://cli.github.com/packages stable main"
  if ! same "$GH_SOURCES" "$content"; then
    put "$GH_SOURCES" 644 "$content"
    changed=yes
  fi
  if [[ $changed == yes ]]; then
    ok "apt installs gh from GitHub's apt repo, and update upgrades it."
  else
    ok "apt installs gh from GitHub's apt repo already."
  fi
}

# setup_git MAC_NAME MAC_EMAIL: sets the name and email git puts on your
# commits, unless git has them already. If gh is logged in, they come from your
# GitHub profile: its name, or else your login, and its public email, or else
# GitHub's noreply address for you, which links commits to you without showing
# your email. Otherwise, they're MAC_NAME and MAC_EMAIL, from git on your Mac,
# which may be empty. Runs as your user, after setup_github.
setup_git() {
  step "Set your name and email for git commits"
  ((EUID != 0)) || fail "Run this as your user, not root."
  local name email new_name=$1 new_email=$2 login id profile
  name=$(git config --global --get user.name || true)
  email=$(git config --global --get user.email || true)
  if [[ -z $name || -z $email ]] && gh auth status --hostname github.com &>/dev/null &&
    profile=$(gh api user --jq '.name // "", .email // "", .login, .id'); then
    { IFS= read -r new_name; IFS= read -r new_email; IFS= read -r login; IFS= read -r id; } <<<"$profile"
    new_name=${new_name:-$login}
    new_email=${new_email:-$id+$login@users.noreply.github.com}
  fi
  if [[ -z $name && -n $new_name ]]; then
    name=$new_name
    run git config --global user.name "$name"
  fi
  if [[ -z $email && -n $new_email ]]; then
    email=$new_email
    run git config --global user.email "$email"
  fi
  if [[ -z $name || -z $email ]]; then
    warn "git has no name or email for your commits. To set them: git config --global user.name 'Your Name', and user.email you@example.com"
    checklist warn "; git has no name or email for commits"
  else
    ok "git puts $name <$email> on your commits. To change them: git config --global user.name or user.email"
    checklist ok "; git commits as $name <$email>"
  fi
}

# setup_github: logs gh in to GitHub, with a code you enter on any device, if
# you like: if it's logged in already, it asks whether to log in again. Then
# has git use gh's login for github.com. Runs as your user.
setup_github() {
  step "Log in to GitHub with gh"
  ((EUID != 0)) || fail "Run this as your user, not root."
  local question="Log in to GitHub now?" default=yes login flags help
  if gh auth status --hostname github.com &>/dev/null; then
    login=$(gh api user --jq .login 2>/dev/null || true)
    question="gh is logged in to GitHub${login:+ as $login}. Log in again?" default=no
  else
    info "gh logs in with a code: open the link it shows on any device, and enter the code. Coding agents running as you can use this login too."
  fi
  if confirm "$question" "$default"; then
    # gh shows a code and a link. Without prompts, it doesn't ask whether git
    # should use its login (gh auth setup-git does that below), or wait for
    # Enter to open a browser on the server, which has none. --scopes workflow
    # lets git push GitHub Actions workflows, as gh asks for when git uses its
    # login. Newer gh tries the server's clipboard, which it hasn't, and
    # suggests --expiring tokens, which need git 2.46; their flags stop that.
    flags=(--hostname github.com --git-protocol https --web --scopes workflow)
    help=$(gh auth login --help 2>&1 || true)
    [[ $help == *--clipboard* ]] && flags+=(--clipboard=false)
    [[ $help == *--expiring* ]] && flags+=(--expiring=false)
    if ! GH_PROMPT_DISABLED=1 at_terminal gh auth login "${flags[@]}"; then
      warn "gh couldn't log in, as it says above. To try again: gh auth login, then gh auth setup-git"
      checklist warn "gh isn't logged in to GitHub"
      return
    fi
  elif [[ $default == yes ]]; then
    ok "gh isn't logged in. To log in later: gh auth login, then gh auth setup-git"
    checklist skip "gh isn't logged in to GitHub"
    return
  fi
  if [[ $(git config --global --get-all credential.https://github.com.helper || true) != *'gh auth git-credential'* ]]; then
    run gh auth setup-git --hostname github.com
  fi
  login=$(gh api user --jq .login 2>/dev/null || echo 'you')
  ok "gh is logged in to GitHub as $login, and git uses its login for github.com."
  checklist ok "gh logged in to GitHub as $login"
}
