# zsh.sh: makes zsh your shell, with plugins, fzf, zoxide and a git prompt.

# rembox's zsh config is in files of its own, which ~/.zshenv, ~/.zprofile and
# ~/.zshrc each source from a line at their top. Running setup again rewrites
# only these, and your own settings, after that line, win over them.
ZSH_CONFIG_DIR=.config/rembox # in your home folder

ZINIT_DIR=.local/share/zinit # in your home folder
# The plugins rembox's zshrc loads with zinit, as zinit names their folders.
ZINIT_PLUGINS=(
  plugins/zsh-users---zsh-syntax-highlighting
  plugins/zsh-users---zsh-completions
  plugins/zsh-users---zsh-autosuggestions
  snippets/OMZL::git.zsh
  snippets/OMZP::git
)

# setup_zsh: writes rembox's zsh config, adds the lines that source it, and
# downloads zinit and its plugins, so your first login doesn't wait for them.
# Runs as your user.
setup_zsh() {
  step "Set up zsh, with plugins, fzf, zoxide and a git prompt"
  ((EUID != 0)) || fail "Run this as your user, not root."
  local dir=$HOME/$ZSH_CONFIG_DIR name content
  [[ -d $dir ]] || run mkdir -p "$dir"
  for name in zshenv zprofile zshrc; do
    content=$("rembox_$name")
    same "$dir/$name" "$content" || put "$dir/$name" 644 "$content"
    # shellcheck disable=SC2088 # ~ is for zsh, in the line it writes
    source_from "$HOME/.$name" "~/$ZSH_CONFIG_DIR/$name"
  done

  local plugin download=no
  [[ -f $HOME/$ZINIT_DIR/zinit.git/zinit.zsh ]] || download=yes
  for plugin in "${ZINIT_PLUGINS[@]}"; do
    [[ -d $HOME/$ZINIT_DIR/$plugin ]] || download=yes
  done
  if [[ $download == yes ]]; then
    step "Download zinit and its plugins"
    # zsh starts as you log in, which has zinit download what it hasn't yet.
    run zsh -i -c exit
  fi
  ok "zsh is set up. Its config is in ~/$ZSH_CONFIG_DIR; yours goes in ~/.zshrc."
}

# source_from FILE CONFIG: puts a line that sources CONFIG at the top of FILE,
# unless FILE sources it already, so FILE's own lines come after, and win.
source_from() {
  local file=$1 config=$2
  local line="[[ -r $config ]] && source $config # rembox's config; yours goes below."
  if [[ -L $file ]]; then
    file=$(readlink -f "$file") # write to the file, not over the link
  fi
  if [[ ! -e $file ]]; then
    put "$file" 644 "$line"
  elif ! grep -qF "source $config" "$file"; then
    put "$file" "$(stat -c %a "$file")" "$line"$'\n'"$(cat "$file")"
  fi
}

# set_login_shell USER: makes zsh USER's login shell, unless it is already.
set_login_shell() {
  local user=$1 zsh shell
  step "Make zsh $user's login shell"
  zsh=$(command -v zsh) || fail "zsh isn't installed."
  shell=$(getent passwd "$user" | cut -d: -f7)
  if [[ ${shell##*/} != zsh ]]; then
    run usermod --shell "$zsh" "$user"
  fi
  ok "$user logs in to zsh."
}

rembox_zshenv() {
  cat <<'EOF'
# rembox's zshenv, which ~/.zshenv sources. rembox setup rewrites this file, so
# put your own settings in ~/.zshenv, after the line that sources it.

# Debian and Ubuntu's /etc/zsh/zshrc runs compinit, which rembox's zshrc runs
# once its plugins have added their completions; once is enough.
skip_global_compinit=1
EOF
}

rembox_zprofile() {
  cat <<'EOF'
# rembox's zprofile, which ~/.zprofile sources. rembox setup rewrites this
# file, so put your own settings in ~/.zprofile, after the line that sources it.

# zsh doesn't read ~/.profile, which adds ~/.local/bin and ~/bin to PATH.
[[ -r ~/.profile ]] && emulate sh -c '. ~/.profile'
EOF
}

rembox_zshrc() {
  cat <<'EOF'
# rembox's zshrc, which ~/.zshrc sources. rembox setup rewrites this file, so
# put your own settings in ~/.zshrc, after the line that sources it. Based on
# https://swiftace.org/posts/remote-development-environment-setup (section 6).

# Your terminal's name, as ssh passes it on, unless the server doesn't know it.
if [[ -n $TERM ]] && ! infocmp "$TERM" &>/dev/null; then
  export TERM=xterm-256color
fi

# Plugins, with zinit, which downloads what it doesn't have yet. zinit's
# aliases are off, so zi is zoxide's.
typeset -A ZINIT=(NO_ALIASES 1)
ZINIT_HOME=${XDG_DATA_HOME:-$HOME/.local/share}/zinit/zinit.git
if [[ ! -d $ZINIT_HOME ]]; then
  mkdir -p "${ZINIT_HOME:h}"
  git clone --depth 1 https://github.com/zdharma-continuum/zinit.git "$ZINIT_HOME"
fi
source "$ZINIT_HOME/zinit.zsh"
zinit light zsh-users/zsh-syntax-highlighting
zinit light zsh-users/zsh-completions
zinit light zsh-users/zsh-autosuggestions
zinit snippet OMZL::git.zsh # git_current_branch, for OMZP::git's aliases
zinit snippet OMZP::git     # aliases like gst, gco and ggpush
autoload -Uz compinit && compinit
zinit cdreplay -q

# History
HISTSIZE=5000
HISTFILE=~/.zsh_history
SAVEHIST=$HISTSIZE
HISTDUP=erase
setopt appendhistory sharehistory hist_ignore_space hist_ignore_all_dups
setopt hist_save_no_dups hist_ignore_dups hist_find_no_dups

zstyle ':completion:*' matcher-list 'm:{a-z}={A-Za-z}' # any case
alias ls='ls --color=auto'

# fzf: Ctrl-R searches history, Ctrl-T finds files, Alt-C changes folder.
if [[ -o zle ]] && (($+commands[fzf])); then
  source <(fzf --zsh)
fi

# The prompt: you@the server's full name (%M; %m stops at the first dot), the
# folder, and in a git repo, the branch, with * if there are changes to commit,
# and ⇡N and ⇣N if it's N commits ahead of or behind its upstream. git status
# runs in the background, so a big repo doesn't slow the prompt down; * and the
# arrows show up once it's done.
autoload -Uz add-zsh-hook
setopt prompt_subst
typeset -g _rembox_git='' _rembox_branch='' _rembox_marks='' _rembox_dir='' _rembox_fd=''

# _rembox_git_status: prints the * and arrows for the repo here.
_rembox_git_status() {
  local line dirty='' ahead=0 behind=0 arrows=''
  GIT_OPTIONAL_LOCKS=0 command git status --porcelain=v2 --branch 2>/dev/null |
    while IFS= read -r line; do
      case $line in
        '# branch.ab '*) # like "# branch.ab +2 -1"
          line=${line#'# branch.ab +'}
          ahead=${line%% *} behind=${line##*-}
          ;;
        '#'*) ;;
        *) dirty='*'; break ;;
      esac
    done
  ((ahead)) && arrows+="⇡$ahead"
  ((behind)) && arrows+="⇣$behind"
  print -r -- "$dirty${arrows:+ $arrows}"
}

# _rembox_show_git: sets the prompt's git part, from the branch and marks.
_rembox_show_git() {
  _rembox_git=''
  [[ -n $_rembox_branch ]] && _rembox_git=" %F{green}${_rembox_branch//\%/%%}%F{yellow}$_rembox_marks%f"
}

# _rembox_prompt: shows the branch now, and starts git status for the rest.
_rembox_prompt() {
  if [[ -n $_rembox_fd ]]; then # the last prompt's git status, still running
    zle -F $_rembox_fd
    exec {_rembox_fd}<&-
    _rembox_fd=''
  fi
  _rembox_branch=$(command git symbolic-ref --short -q HEAD 2>/dev/null)
  case $? in
    0) ;;
    1) _rembox_branch=$(command git rev-parse --short HEAD 2>/dev/null) ;; # no branch
    *) _rembox_branch='' ;; # no repo
  esac
  # Keep the last * and arrows until git status is done, if still here.
  [[ -n $_rembox_branch && $PWD == "$_rembox_dir" ]] || _rembox_marks=''
  _rembox_dir=$PWD
  _rembox_show_git
  [[ -n $_rembox_branch && -o zle ]] || return 0
  exec {_rembox_fd}< <(_rembox_git_status)
  zle -F $_rembox_fd _rembox_git_done
}

# _rembox_git_done FD: shows what git status found, from FD.
_rembox_git_done() {
  local fd=$1 marks=''
  IFS= read -r marks <&$fd
  zle -F $fd
  exec {fd}<&-
  _rembox_fd=''
  if [[ $marks != "$_rembox_marks" ]]; then
    _rembox_marks=$marks
    _rembox_show_git
    zle reset-prompt
  fi
}

add-zsh-hook precmd _rembox_prompt
PROMPT='%F{8}%n@%M%f %F{blue}%~%f$_rembox_git %(?.%F{green}.%F{red})❯%f '

# zoxide: z FOLDER goes to the folder you use most that matches, and zi picks
# one with fzf. It goes last, after compinit.
if (($+commands[zoxide])); then
  eval "$(zoxide init zsh)"
fi
EOF
}
