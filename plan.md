# rembox plan

What `rembox setup` does so far, and what comes next. Keep this current in every PR. To pick up
where the last agent left off, start at the first unchecked item under **Next**.

The original hand-run version of the whole setup is in `~/workspace/scratch/devbox-setup/`:
`setup.sh` (one script run as root on the server), `SETUP.md` (what it does, plus design notes
and gotchas), and `SETUP-MANUAL.md` (the step-by-step log, dead ends included). Port from there,
but follow this repo's conventions (AGENTS.md, and the style of `cli/internal/remote/*.sh`).

## Done

- [x] Upgrade every package, and turn on daily security updates (unattended-upgrades)
- [x] Add your user, with sudo that asks for its password and root's SSH keys. The server asks
      for its name just before adding it; `userWriter` in `setup.go` reads it back.
- [x] Each session says it has logged in, and why it needs sudo before sudo asks (`as_root`).
- [x] The CLI says when it connects and reconnects, before ssh asks for a passphrase.
- [x] `rembox setup NAME` asks for the name too, and renames the server if it changes.
- [x] The server's name is its hostname (`hostname.sh`), and the prompt shows it in full: `user@devbox.example.com`.
- [x] Move ssh to a random port, with keys only and no root login; fail2ban
- [x] ufw: block every incoming connection but ssh
- [x] Restart the server, if an update needs it
- [x] Base dev packages (`BASIC_PACKAGES` in `update.sh`): build-essential, git, gh, curl, wget,
      zip, unzip, jq, ripgrep, fd-find (plus an `fd` link to `fdfind`), htop, tmux, zsh, nano,
      man-db, dnsutils, ufw. fzf and zoxide come from their latest GitHub releases instead
      (`install_release`), as apt's are old. These install in `start`, as `home` needs zsh and git.
- [x] Swap file as big as memory, up to 4 GiB, unless there's swap already; swappiness 10
      (`swap.sh`). Skipped in containers.
- [x] Generate the locales your Mac sends over ssh (LANG, LC_*), so programs don't warn
      (`locale.sh`). The CLI works them out from its environment and passes them to
      `setup.sh finish`. Not on the site, since it's a small thing.
- [x] zsh as your login shell (`zsh.sh`), based on section 6 of
      https://swiftace.org/posts/remote-development-environment-setup: zinit with
      zsh-syntax-highlighting, zsh-completions, zsh-autosuggestions and OMZ's git aliases
      (`OMZL::git.zsh` + `OMZP::git`; zinit's own aliases are off, so `zi` is zoxide's); fzf's
      `fzf --zsh`; zoxide's `z`; the article's history and completion settings. No
      Powerlevel10k: a hand-rolled prompt (host, folder, git branch, with `*` and `⇡N⇣N` from a
      `git status` run in the background with `zle -F`), so no wizard or Nerd Font. TERM falls back
      to xterm-256color only if the server has no terminfo for yours (like Ghostty's).
  - **Shell config:** rembox owns `~/.config/rembox/{zshenv,zprofile,zshrc}`, which `~/.zshenv`,
    `~/.zprofile` and `~/.zshrc` source from a line at their top, added once; your own settings go
    below it. Re-runs rewrite only rembox's files. zshenv sets `skip_global_compinit` (Debian and
    Ubuntu's /etc/zsh/zshrc runs compinit too early); zprofile sources `~/.profile` (for
    `~/.local/bin`). Later steps that need PATH (bun, uv, cargo) add it to rembox's zprofile and
    run their installers without touching shell files.
  - **Running as your user:** a third ssh session, `setup.sh home`, as your user on the new port,
    without sudo, between `start` and `finish` (finish may restart the server, so it stays last).
    It's a real login (PAM, systemd user bus) at a terminal (`ssh -t`), for device-code logins.
    Later user steps (git/gh, toolchains, agents) go in it. `finish` makes zsh the login shell, so
    that happens only once `home` has set it up.
  - zinit and its plugins download during `home` (`zsh -i -c exit`), unless they're there.
    Re-runs don't update them; `zinit update` does.
  - Later, maybe: Zellij auto-attach (section 7) and Neovim + LazyVim (section 8).

## Next

1. [ ] **Git and GitHub.** `gh` comes from the distro now, which lags (2.23 on Debian 12, 2.45
   on Ubuntu 24.04); consider GitHub's apt repo instead. Ask for your git name and email
   (offer `git config --global user.name/email` from this Mac as defaults), set them for your
   user, run `gh auth login` (device code) and `gh auth setup-git`, in the `home` session.
2. [ ] **Toolchains, as your user, each optional:** bun; Node via Vite+ (`vp`, with
   `VP_BUN_MANAGER=no`); Python via uv; Rust via rustup (minimal + clippy + rustfmt).
3. [ ] **Coding agents, each optional:** Codex, Claude Code, Pi, with their logins. A shared
   `~/.config/rembox/AGENTS.md` (or similar), symlinked to `~/.codex/AGENTS.md`,
   `~/.claude/CLAUDE.md` and `~/.pi/agent/AGENTS.md`; never overwrite real files.
4. [ ] **HTTPS and T3 Code.** Ask for a domain, check its A record points at the server, install
   Caddy, reverse-proxy to T3 on 127.0.0.1:3773, enable linger so T3's systemd user service runs
   without a login, add its PATH drop-in, and pair a device (QR code). See SETUP.md's gotchas.
   - ufw: open 80/tcp, 443/tcp and 443/udp. `setup_firewall` in `firewall.sh` asks "Keep the ufw
     rule to …?" for every allow rule but ssh's, so make it take the ports rembox opens and skip
     the question for them, or every re-run asks about Caddy's ports.

## Open questions

- Should `rembox setup` stay about hardening the server, with toolchains, agents and T3 as
  separate commands (like `rembox install claude`)? That keeps setup's questions and the site's
  list short. Decide before step 2.

## Testing on real machines

See **Development** in README.md: `scripts/e2e.sh` runs `rembox setup` twice on a fresh OrbStack
machine. Run it on `debian:bookworm` too for changes to the server scripts.
To try zsh on a machine without a terminal, `tmux new-session -d 'zsh -l'` on it, then
`tmux send-keys` and `tmux capture-pane -p`.
