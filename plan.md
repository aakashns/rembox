# rembox plan

What `rembox setup` does so far, and what comes next. Keep this current in every PR. To pick up
where the last agent left off, start at the first unchecked item under **Next**.

The original hand-run version of the whole setup is in `~/workspace/scratch/devbox-setup/`:
`setup.sh` (one script run as root on the server), `SETUP.md` (what it does, plus design notes
and gotchas), and `SETUP-MANUAL.md` (the step-by-step log, dead ends included). Port from there,
but follow this repo's conventions (AGENTS.md, and the style of `cli/internal/remote/*.sh`).

## Done

- [x] Upgrade every package, and turn on daily security updates (unattended-upgrades)
- [x] Add your user, with sudo that asks for its password and root's SSH keys
- [x] Move ssh to a random port, with keys only and no root login; fail2ban
- [x] ufw: block every incoming connection but ssh
- [x] Restart the server, if an update needs it
- [x] Base dev packages (`BASIC_PACKAGES` in `update.sh`): build-essential, git, gh, curl, wget,
      zip, unzip, jq, ripgrep, fd-find (plus an `fd` link to `fdfind`), fzf, htop, tmux, zsh,
      nano, man-db, dnsutils, ufw
- [x] Swap file as big as memory, up to 4 GiB, unless there's swap already; swappiness 10
      (`swap.sh`). Skipped in containers.
- [x] Generate the locales your Mac sends over ssh (LANG, LC_*), so programs don't warn
      (`locale.sh`). The CLI works them out from its environment and passes them to
      `setup.sh finish`. Not on the site, since it's a small thing.

## Next

1. [ ] **zsh, as your login shell, with the config from
   https://swiftace.org/posts/remote-development-environment-setup (section 6), plus zoxide
   as `z`.** Idempotent, like every step. The `.zshrc` there: `TERM=xterm-256color`; zinit
   (cloned to `~/.local/share/zinit/zinit.git` if absent); Powerlevel10k; zsh-syntax-highlighting,
   zsh-completions, zsh-autosuggestions; `OMZP::git` and `OMZP::gh` snippets; `compinit` +
   `zinit cdreplay -q`; `~/.p10k.zsh` if present; history (5000, shared, no dups,
   ignore-space); case-insensitive completion; `alias ls='ls --color'`; fzf's
   `/usr/share/doc/fzf/examples/{completion,key-bindings}.zsh`. Add zoxide (apt package) with
   `eval "$(zoxide init zsh)"`. Then `chsh -s "$(command -v zsh)"` for your user.
   - Decide: rembox owns `~/.zshrc` (back up an existing one first, rewrite only if it differs),
     or writes `~/.config/rembox/zshrc` and sources it from a line in `~/.zshrc`, so your own
     edits survive re-runs. The second is friendlier to re-runs.
   - Prefetch zinit and its plugins during setup (e.g. `zsh -ic exit` as the user), so the first
     login isn't a wall of git clones. The Powerlevel10k wizard runs at first login; tell the
     user that, and that a Nerd Font on the Mac (MesloLGS Nerd Font) makes its icons render.
   - This is the first step that runs as your user, not root. Decide how: probably a third ssh
     session as your user (no sudo), since rembox can already log in as them after `finish`,
     rather than `runuser` from the root script. Settle it here; later
     steps (git, toolchains, agents) reuse it.
   - Later, maybe: Zellij auto-attach (section 7) and Neovim + LazyVim (section 8).
2. [ ] **Git and GitHub.** `gh` comes from the distro now, which lags (2.23 on Debian 12, 2.45
   on Ubuntu 24.04); consider GitHub's apt repo instead. Ask for your git name and email
   (offer `git config --global user.name/email` from this Mac as defaults), set them for your
   user, run `gh auth login` (device code) and `gh auth setup-git`.
3. [ ] **Toolchains, as your user, each optional:** bun; Node via Vite+ (`vp`, with
   `VP_BUN_MANAGER=no`); Python via uv; Rust via rustup (minimal + clippy + rustfmt).
4. [ ] **Coding agents, each optional:** Codex, Claude Code, Pi, with their logins. A shared
   `~/.config/rembox/AGENTS.md` (or similar), symlinked to `~/.codex/AGENTS.md`,
   `~/.claude/CLAUDE.md` and `~/.pi/agent/AGENTS.md`; never overwrite real files.
5. [ ] **HTTPS and T3 Code.** Ask for a domain, check its A record points at the server, install
   Caddy, reverse-proxy to T3 on 127.0.0.1:3773, enable linger so T3's systemd user service runs
   without a login, add its PATH drop-in, and pair a device (QR code). See SETUP.md's gotchas.
   - ufw: open 80/tcp, 443/tcp and 443/udp. `setup_firewall` in `firewall.sh` asks "Keep the ufw
     rule to …?" for every allow rule but ssh's, so make it take the ports rembox opens and skip
     the question for them, or every re-run asks about Caddy's ports.

## Open questions

- Should `rembox setup` stay about hardening the server, with toolchains, agents and T3 as
  separate commands (like `rembox install claude`)? That keeps setup's questions and the site's
  list short. Decide before step 3.

## Testing on real machines

- `make lint test` in `cli/` covers the CLI, and shellchecks the scripts.
- To run new server steps for real, make throwaway OrbStack machines
  (`orb create ubuntu:noble NAME`, `orb create debian:bookworm NAME`), run lib.sh, the step
  files and a small `main` with `orb -m NAME -u root bash -c "$body" bash ARGS`, twice, to check
  the second run changes nothing, then `orb delete -f NAME`. OrbStack machines are containers,
  so they skip swap.
- `~/workspace/scratch/devbox-setup/test/orb-fresh.sh` makes an OrbStack machine that looks
  like a fresh cloud server (cloud-init, root ssh with a key), for running `rembox setup` itself.
